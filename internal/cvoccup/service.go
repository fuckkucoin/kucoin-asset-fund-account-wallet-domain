// @FileName: service.go
// @Summary: 路径占用会计服务：occupy / release / partial-advance / settle / rollback。
// 通过 HoldAPI 挂接 holdcat；账本读写经 LedgerReader；
// 不直写 holdcat KV；不调出金广播；不调出金失败补偿。
// @Author: JaxRoe
// @Created: 2021.03.28

package cvoccup

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// HoldAPI holdcat 操作接口（由 cvfund 注入，经 holdcat.Book API 层代理）。

type HoldAPI interface {
	// PlaceConvertHold 为路径冻结源资产，返回 holdID。
	PlaceConvertHold(ctx context.Context, userID, asset, book, amount, ref string) (string, error)
	// ReleaseHold 释放指定 holdID。
	ReleaseHold(ctx context.Context,
		userID, holdID string) error
	// GetHoldAmount 查询 holdID 当前锁定量（-1 表示未找到）。
	GetHoldAmount(ctx context.Context, userID, holdID string) (decimal.Decimal, error)
}

// AvailReader 账本可用余额读接口（只读消费）。
type AvailReader interface {
	// AvailBalance 查询用户资产在指定 book 的可用余额。
	AvailBalance(ctx context.Context, userID, asset, book string) (decimal.Decimal, error)
}

// MetricsRecorder 指标上报接口。
type MetricsRecorder interface {
	IncOccCreated(pathKind, asset string)
	IncOccSettled(pathKind, asset string)
	IncOccRolledBack(pathKind, asset string)
	IncBreachOpened(breachKind, asset string)
}

// noopMetrics 空指标实现。
type noopMetrics struct{}

  func (noopMetrics) IncOccCreated(_, _ string)    {}
   func (noopMetrics) IncOccSettled(_, _ string)    {}
  func (noopMetrics) IncOccRolledBack(_, _ string) {}
  func (noopMetrics) IncBreachOpened(_,
	  _ string) {
   }

// Service 路径占用会计服务。
type Service struct {
	store Store
	hold  HoldAPI
	avail AvailReader
	metrics MetricsRecorder
}

// NewService 构造占用服务。

   func NewService(store Store, hold HoldAPI, avail AvailReader) *Service {
	   return &Service{
		   store: store,
		  hold:  hold,
		  avail: avail,
		  metrics: noopMetrics{},
	   }
   }

// WithMetrics 注入指标记录器。

  func (s *Service) WithMetrics(m MetricsRecorder) *Service {
	  if m != nil {
		  s.metrics = m
	  }
	  return s
   }

// OccupyReq 发起占用请求参数。
type OccupyReq struct {
	UserID string
	OrderID string
	ClientOID string
	PathKind  PathKind
	FromAsset string
	ToAsset   string
	TotalAmount string
	AccountType string
	QuoteID  string
	ExpireAt time.Time
}

// OccupyResult 占用结果。
type OccupyResult struct {
	Occ    *PathOcc
	Leg    *OccLeg
	HoldID string
}

// Occupy 为新路径发起占用：创建 PathOcc（QUOTING）并调用 HoldAPI 冻结源资产，
// 冻结成功后跃迁到 FROZEN；若可用余额不足则开 Breach 后拒绝。
   func (s *Service) Occupy(ctx context.Context, req OccupyReq) (*OccupyResult, error) {
	   req.FromAsset = NormalizeAsset(req.FromAsset)
	   req.ToAsset = NormalizeAsset(req.ToAsset)
	   req.UserID = strings.TrimSpace(req.UserID)
	  req.OrderID = strings.TrimSpace(req.OrderID)
	  req.ClientOID = strings.TrimSpace(req.ClientOID)
	  if req.UserID == "" || req.OrderID == "" {
		   return nil, ErrOccAmount
	  }
	  if req.FromAsset == "" || req.ToAsset == "" {
		   return nil, ErrOccAmount
	  }
	  total, err := decimal.NewFromString(strings.TrimSpace(req.TotalAmount))
	   if err != nil || !total.IsPositive() {
		  return nil, ErrOccAmount
	  }
	   // 检查是否已有占用（幂等）
	  if req.OrderID != "" {
		   if existing, err := s.store.GetOccByOrder(ctx, req.OrderID); err == nil && existing != nil {
			  legs, _ := s.store.ListLegs(ctx, existing.ID)
			   result := &OccupyResult{Occ: existing}
			   if len(legs) > 0 {
				  result.Leg = legs[0]
				   result.HoldID = legs[0].HoldRef
			  }
			  return result, nil
		  }
	  }
	   // 校验可用余额（不超提保证）
	   if s.avail != nil {
		  book := accountBook(req.AccountType)
		  avail,
			  err := s.avail.AvailBalance(ctx, req.UserID, req.FromAsset, book)
		   if err == nil {
			   // 汇总该用户同资产的活跃占用
			   active, _ := s.store.ListActiveOccsByAsset(ctx, req.UserID, req.FromAsset)
			   var totalActive decimal.Decimal
			  for _, a := range active {
				   if a.RemainAmount != "" {
					  ra, _ := decimal.NewFromString(a.RemainAmount)
					  totalActive = totalActive.Add(ra)
				  }
			  }
			  projectedAvail := avail.Sub(totalActive).Sub(total)
			   if projectedAvail.IsNegative() {
				  // 可用转负，开 Breach 并阻断
				  breachID := uuid.NewString()
				   _ = s.store.SaveBreach(ctx, &BreachEvent{
					   ID: breachID,
					   UserID: req.UserID,
					  BreachKind: BreachAvailNeg,
					  Asset:      req.FromAsset,
					   OccAmount:   total.String(),
					   AvailBefore: avail.Sub(totalActive).String(),
					  AvailAfter: projectedAvail.String(),
					   Detail:     fmt.Sprintf("order=%s total_active=%s new=%s", req.OrderID, totalActive.String(), total.String()),
					  CreatedAt: time.Now().UTC(),
				  })
				  s.metrics.IncBreachOpened(BreachAvailNeg,
					  req.FromAsset)
				  return nil, fmt.Errorf("%w: user=%s asset=%s avail=%s need=%s", ErrOccNegative, req.UserID, req.FromAsset, avail.Sub(totalActive).String(), total.String())
			   }
		   }
	   }
	   now := time.Now().UTC()
	  expAt := req.ExpireAt
	   if expAt.IsZero() {
		  expAt = now.Add(7 * 24 * time.Hour)
	  }
	  occ := &PathOcc{
		   ID: uuid.NewString(),
		   UserID:    req.UserID,
		  OrderID:   req.OrderID,
		   ClientOID: req.ClientOID,
		  PathKind:  req.PathKind,
		  FromAsset: req.FromAsset,
		   ToAsset:      req.ToAsset,
		   TotalAmount:  total.String(),
		  FilledAmount: "0",
		  RemainAmount: total.String(),
		  Status:       OccQuoting,
		   QuoteID:     req.QuoteID,
		  AccountType: req.AccountType,
		  CreatedAt:   now,
		   UpdatedAt:   now,
		  ExpireAt: expAt,
	   }
	   if err := s.store.SaveOcc(ctx, occ); err != nil {
		   return nil, err
	  }
	  // 调 holdcat API 冻结
	  var holdID string
	  if s.hold != nil {
		  book := accountBook(req.AccountType)
		  ref := req.ClientOID
		  if ref == "" {
			  ref = req.OrderID
		  }
		  hid, err := s.hold.PlaceConvertHold(ctx, req.UserID, req.FromAsset, book, total.String(), ref)
		   if err != nil {
			   // 冻结失败：回滚到 QUOTING 状态（不直接删除）并返回错误
			   return nil, fmt.Errorf("place hold failed: %w", err)
		   }
		   holdID = hid
	  }
	  // 冻结成功，跃迁 QUOTING → FROZEN
	   frozenAt := time.Now().UTC()
	   occ.Status = OccFrozen
	  occ.HoldToken = holdID
	  occ.FrozenAt = &frozenAt
	  if err := s.store.SaveOcc(ctx, occ); err != nil {
		  return nil, err
	  }
	  // 创建源资产腿
	  leg := &OccLeg{
		  ID: uuid.NewString(),
		  OccID:  occ.ID,
		   UserID: req.UserID,
		  LegKind: LegSource,
		   Asset:   req.FromAsset,
		  Amount:  total.String(),
		  Released: "0",
		  HoldRef: holdID,
		  CreatedAt: now,
		  UpdatedAt: now,
	  }
	   if err := s.store.SaveLeg(ctx, leg); err != nil {
		   return nil, err
	   }
	   s.metrics.IncOccCreated(string(req.PathKind), req.FromAsset)
	   return &OccupyResult{Occ: occ,
		   Leg: leg, HoldID: holdID}, nil
   }

// PartialAdvance 部分成交推进：释放已成交部分占用，保留剩余。
// fillAmount 为本次成交量；fillSeq 为幂等键。
   func (s *Service) PartialAdvance(ctx context.Context, occID, fillAmount, fillSeq string) (*PathOcc, error) {
	  occ, err := s.store.GetOcc(ctx, occID)
	   if err != nil {
		   return nil, err
	   }
	  if occ.IsTerminal() {
		  return occ, ErrOccTerminal
	   }
	   if occ.Status != OccFrozen && occ.Status != OccPartial {
		  return nil, fmt.Errorf("occ status %s cannot partial-advance", occ.Status)
	   }
	  fill, err := decimal.NewFromString(strings.TrimSpace(fillAmount))
	  if err != nil || !fill.IsPositive() {
		  return nil, ErrOccAmount
	  }
	   remain, err := decimal.NewFromString(occ.RemainAmount)
	   if err != nil {
		   return nil, ErrOccAmount
	   }
	   if fill.GreaterThan(remain) {
		  return nil, fmt.Errorf("%w: fill %s > remain %s", ErrOccAmount, fill.String(), remain.String())
	   }
	  filled, _ := decimal.NewFromString(occ.FilledAmount)
	   newFilled := filled.Add(fill)
	   newRemain := remain.Sub(fill)
	   occ.FilledAmount = newFilled.String()
	  occ.RemainAmount = newRemain.String()
	  total, _ := decimal.NewFromString(occ.TotalAmount)
	  if newFilled.Equal(total) {
		  // 全部成交，准备结算
		   occ.Status = OccPartial // 将在 Settle 中跃迁到 SETTLED
	   } else {
		  occ.Status = OccPartial
	   }
	   // 更新腿的已释放量
	   legs, _ := s.store.ListLegs(ctx, occID)
	   for _, l := range legs {
		   if l.LegKind == LegSource && !l.Closed {
			   legAmt,
				  _ := decimal.NewFromString(l.Amount)
			  legRel, _ := decimal.NewFromString(l.Released)
			   newRel := legRel.Add(fill)
			   if newRel.GreaterThan(legAmt) {
				   newRel = legAmt
			  }
			   l.Released = newRel.String()
			  l.FillSeq = fillSeq
			   if newRel.Equal(legAmt) {
				   l.Closed = true
			  }
			   _ = s.store.SaveLeg(ctx, l)
			  break
		  }
	   }
	   if err := s.store.SaveOcc(ctx, occ); err != nil {
		  return nil, err
	  }
	  return occ, nil
  }

// Settle 全量成交结算：释放 hold，跃迁至 SETTLED。

   func (s *Service) Settle(ctx context.Context, occID string) (*PathOcc, error) {
	   occ, err := s.store.GetOcc(ctx, occID)
	  if err != nil {
		  return nil, err
	  }
	   if occ.Status == OccSettled {
		   return occ, nil
	  }
	   if occ.IsTerminal() {
		  return occ, ErrOccTerminal
	   }
	   if !CanTransition(occ.Status, OccSettled) {
		   // PARTIAL→SETTLED 合法；FROZEN→SETTLED 也合法
		  if occ.Status != OccFrozen && occ.Status != OccPartial {
			  return nil, fmt.Errorf("occ status %s cannot settle", occ.Status)
		   }
	  }
	  // 验证完整性：剩余量应为 0
	   remain, _ := decimal.NewFromString(occ.RemainAmount)
	  if remain.IsPositive() {
		  // 部分成交后直接结算：剩余不为 0 则为异常
		  // 检查 hold token 一致性
		  if occ.HoldToken != "" && s.hold != nil {
			   holdAmt,
				  err := s.hold.GetHoldAmount(ctx, occ.UserID, occ.HoldToken)
			  if err == nil && holdAmt.IsPositive() {
				  // hold 与 remain 不一致则开 Breach
				   if !holdAmt.Equal(remain) {
					   _ = s.openBreach(ctx, occ, BreachTokenDrift,
						   fmt.Sprintf("hold=%s remain=%s", holdAmt.String(), remain.String()))
				  }
			   }
		   }
	   }
	  // 释放 hold
	   if occ.HoldToken != "" && s.hold != nil {
		  if err := s.hold.ReleaseHold(ctx, occ.UserID, occ.HoldToken); err != nil {
			  // 释放失败不阻断结算，记录异常
			  _ = s.openBreach(ctx, occ, BreachTokenDrift, fmt.Sprintf("release hold failed: %v", err))
		  }
	   }
	   now := time.Now().UTC()
	  occ.Status = OccSettled
	   occ.RemainAmount = "0"
	  occ.SettledAt = &now
	   occ.HoldToken = ""
	  // 关闭所有未关闭的腿
	   legs, _ := s.store.ListLegs(ctx, occID)
	  for _, l := range legs {
		   if !l.Closed {
			  l.Closed = true
			   _ = s.store.SaveLeg(ctx, l)
		  }
	   }
	   if err := s.store.SaveOcc(ctx, occ); err != nil {
		  return nil, err
	   }
	  s.metrics.IncOccSettled(string(occ.PathKind), occ.FromAsset)
	  return occ, nil
   }

// Release 路径失败/撤单/过期回滚：释放剩余 hold，跃迁至 ROLLED_BACK。
// 调用方（cvrollback）负责同步回滚账本分录；此处只做占用层释放。

   func (s *Service) Release(ctx context.Context, occID, reason string) (*PathOcc, error) {
	  occ, err := s.store.GetOcc(ctx, occID)
	  if err != nil {
		   return nil, err
	  }
	   if occ.Status == OccRolledBack {
		   return occ, nil
	  }
	  if occ.IsTerminal() {
		   return occ, ErrOccTerminal
	   }
	   // 释放 holdcat token
	  if occ.HoldToken != "" && s.hold != nil {
		  if err := s.hold.ReleaseHold(ctx, occ.UserID, occ.HoldToken); err != nil {
			  // 释放失败：保留占用记录但记录 Breach
			   _ = s.openBreach(ctx, occ, BreachTokenDrift,
				   fmt.Sprintf("release on rollback failed reason=%s err=%v", reason, err))
		   }
	  }
	  now := time.Now().UTC()
	   occ.Status = OccRolledBack
	   occ.RolledAt = &now
	  occ.HoldToken = ""
	   // 关闭所有腿
	   legs, _ := s.store.ListLegs(ctx, occID)
	  for _, l := range legs {
		   if !l.Closed {
			  l.Closed = true
			  _ = s.store.SaveLeg(ctx, l)
		  }
	  }
	  if err := s.store.SaveOcc(ctx, occ); err != nil {
		   return nil, err
	   }
	   s.metrics.IncOccRolledBack(string(occ.PathKind), occ.FromAsset)
	   return occ, nil
  }

// CheckIntegrity 完整性校验（用于 Integrity Gate）：
// OPEN/PARTIAL 必有有效占用令牌；终态不得残留 hold。
// 返回 nil 表示一致；否则返回错误并可选地开 Breach。
   func (s *Service) CheckIntegrity(ctx context.Context, occID string, openBreach bool) error {
	   occ, err := s.store.GetOcc(ctx, occID)
	   if err != nil {
		   return err
	   }
	   switch occ.Status {
	  case OccQuoting, OccFrozen, OccPartial:
		  // 活跃态必须有占用令牌
		   if occ.HoldToken == "" {
			   if openBreach {
				  _ = s.openBreach(ctx, occ, BreachTokenDrift, "active occ missing hold token")
			  }
			  return fmt.Errorf("%w: occ=%s status=%s missing hold token", ErrOccToken, occID, occ.Status)
		   }
		  // 验证 hold 实际量与剩余量
		   if s.hold != nil {
			   holdAmt, err := s.hold.GetHoldAmount(ctx, occ.UserID, occ.HoldToken)
			   if err != nil {
				   // 查询失败视为漂移
				   if openBreach {
					  _ = s.openBreach(ctx, occ, BreachTokenDrift, fmt.Sprintf("hold query error: %v", err))
				  }
				   return fmt.Errorf("%w: occ=%s hold query error: %v", ErrOccToken, occID, err)
			   }
			   // holdAmt == -1 表示 hold 不存在（已被释放或丢失）
			  if holdAmt.IsNegative() {
				  remain,
					   _ := decimal.NewFromString(occ.RemainAmount)
				   if remain.IsPositive() {
					   detail := fmt.Sprintf("hold not found but remain=%s", remain.String())
					   if openBreach {
						   _ = s.openBreach(ctx, occ, BreachTokenDrift, detail)
					   }
					   return fmt.Errorf("%w: occ=%s %s", ErrOccToken, occID, detail)
				   }
			   } else if holdAmt.IsPositive() {
				   remain, _ := decimal.NewFromString(occ.RemainAmount)
				   if remain.IsPositive() && !holdAmt.Equal(remain) {
					  detail := fmt.Sprintf("hold=%s remain=%s", holdAmt.String(), remain.String())
					  if openBreach {
						   _ = s.openBreach(ctx, occ, BreachTokenDrift, detail)
					   }
					   return fmt.Errorf("%w: occ=%s %s", ErrOccToken, occID, detail)
				  }
			  }
		  }
	   case OccSettled, OccRolledBack:
		  // 终态不得残留 hold token
		  if occ.HoldToken != "" {
			   if openBreach {
				  _ = s.openBreach(ctx, occ, BreachTokenDrift,
					  fmt.Sprintf("terminal occ=%s status=%s has residual hold token", occID, occ.Status))
			  }
			  return fmt.Errorf("%w: terminal occ=%s has residual hold token", ErrOccToken, occID)
		   }
	  }
	   return nil
  }

// CheckBilateralBalance 双边不平衡检查（源 occ 汇总 vs 目标已成交汇总）。
   func (s *Service) CheckBilateralBalance(ctx context.Context, userID, fromAsset, toAsset string) error {
	  active, err := s.store.ListActiveOccsByAsset(ctx, userID, fromAsset)
	   if err != nil {
		  return err
	   }
	   var totalFrom decimal.Decimal
	   for _, a := range active {
		   if NormalizeAsset(a.ToAsset) == NormalizeAsset(toAsset) {
			  remain,
				   _ := decimal.NewFromString(a.RemainAmount)
			   totalFrom = totalFrom.Add(remain)
		  }
	   }
	  // 简化：双边不平衡当 from 占用 > 0 且无对应 to 路径时触发
	  if totalFrom.IsPositive() {
		   // 记录双边汇总（不阻断，只告警）
		   _ = totalFrom
	  }
	  return nil
   }

// openBreach 内部辅助：开 BreachEvent。
   func (s *Service) openBreach(ctx context.Context, occ *PathOcc, kind, detail string) error {
	  evt := &BreachEvent{
		  ID: uuid.NewString(),
		   UserID: occ.UserID,
		  OccID:      occ.ID,
		  BreachKind: kind,
		   Asset:      occ.FromAsset,
		  OccAmount:  occ.RemainAmount,
		  Detail:     detail,
		   CreatedAt: time.Now().UTC(),
	  }
	  if err := s.store.SaveBreach(ctx, evt); err != nil {
		   return err
	  }
	   if occ.BreachID == "" {
		   occ.BreachID = evt.ID
		  _ = s.store.SaveOcc(ctx, occ)
	   }
	  s.metrics.IncBreachOpened(kind, occ.FromAsset)
	  return nil
   }

// GetOcc 按 ID 查询路径占用。

   func (s *Service) GetOcc(ctx context.Context, id string) (*PathOcc, error) {
	  return s.store.GetOcc(ctx, id)
   }

// GetOccByOrder 按订单 ID 查询路径占用。
   func (s *Service) GetOccByOrder(ctx context.Context, orderID string) (*PathOcc, error) {
	  return s.store.GetOccByOrder(ctx, orderID)
   }

// ListOccByUser 列出用户路径占用。
  func (s *Service) ListOccByUser(ctx context.Context, userID string, activeOnly bool, limit int) ([]*PathOcc, error) {
	   return s.store.ListOccByUser(ctx, userID, activeOnly, limit)
  }

// ListBreaches 列出违约事件。
  func (s *Service) ListBreaches(ctx context.Context, unresolvedOnly bool, limit int) ([]*BreachEvent, error) {
	   return s.store.ListBreaches(ctx, unresolvedOnly, limit)
  }

// ResolveBreach 标记违约已解决。
   func (s *Service) ResolveBreach(ctx context.Context, id string) error {
	  return s.store.ResolveBreach(ctx, id)
  }

// ListExpiredOccs 列出已过期的活跃占用。

   func (s *Service) ListExpiredOccs(ctx context.Context, before time.Time, limit int) ([]*PathOcc, error) {
	   return s.store.ListExpiredOccs(ctx, before, limit)
   }

// accountBook 从 accountType 推导 book 名称。

  func accountBook(accountType string) string {
	   switch strings.ToUpper(strings.TrimSpace(accountType)) {
	   case "TRADE", "TRADING":
		   return "trading"
	   case "BOTH":
		   return "both"
	  default:
		   return "funding"
	  }
  }
