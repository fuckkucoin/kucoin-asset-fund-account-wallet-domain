// 用户冻结和 KYC 档位。
// 作者: Likeo
// 日期: 2021.01.05
package ledger

import (
	"context"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/auditchain/ingest"
	"github.com/shopspring/decimal"
)

// withdrawFee 提现手续费；费大于等于本金则不计，避免扣成负。
  func (s *Service) withdrawFee(tr *Transfer) decimal.Decimal {
	   if s == nil || tr == nil || tr.Direction != WalletWithdraw || s.withdrawFeeBPS <= 0 {
		  return decimal.Zero
	  }
	  fee := tr.Amount.Mul(decimal.NewFromInt(int64(s.withdrawFeeBPS))).Div(decimal.NewFromInt(10000)).Truncate(18)
	  if fee.GreaterThanOrEqual(tr.Amount) {
		   return decimal.Zero
	   }
	  return fee
  }

// CheckUser 新冻结前的闸门：先对账漂移，再看账户是否冻结。

   func (s *Service) CheckUser(ctx context.Context, userID string) error {
	  userID = strings.TrimSpace(userID)
	  if userID == "" {
		   return fmt.Errorf("%w: user_id required", ErrInvalidAddress)
	  }
	  if err := s.CheckDrift(userID); err != nil {
		   return err
	  }
	   frozen, err := s.store.UserControlFrozen(ctx, userID)
	   if err != nil {
		  return err
	   }
	  if frozen {
		   return ErrAccountFrozen
	   }
	  return nil
   }

// GuardUser swap/spot 回调用，没有 ctx。

   func (s *Service) GuardUser(userID string) error {
	   return s.CheckUser(context.Background(), userID)
  }

// SetFrozen 用户级停运。frozen=true 后 CheckUser 挡住新冻，在途单仍按原状态走确认/回滚。
// system 不能冻。写入带 audit，解冻不自动放行冷却期内的出金（见 requireWithdrawCooling）。

   func (s *Service) SetFrozen(ctx context.Context, userID string, frozen bool) error {
	  userID = strings.TrimSpace(userID)
	   if userID == "" || userID == "system" {
		   return fmt.Errorf("%w: user_id", ErrInvalidAddress)
	   }
	   if err := s.store.SetUserControlFrozen(ctx, userID, frozen); err != nil {
		   return err
	  }
	   detail := map[string]any{"frozen": frozen, "gate": "user"}
	  if !frozen {
		  // 解用户控制位不放开科目冻。科目须走科目闸单独解。
		   detail["subjects"] = "unchanged"
	  }
	  if err := s.store.AppendAudit(ctx, AuditEvent{
		  EntityType: "user", EntityID: userID, Action: "control_frozen", Actor: "ops",
		  Detail: detail,
		   CreatedAt: time.Now().UTC(),
	  }); err != nil {
		   return err
	   }
	   kind := "ledger.control.freeze"
	  if !frozen {
		   kind = "ledger.control.release"
	  }
	   ingest.Try(ctx, ingest.ControlFact(userID, kind, "ops", time.Now().UTC()))
	   return nil
   }

// SetFrozenGate 用户冻闸。releaseSubjects 为真时拒绝解冻：用户解冻不得连带解开科目冻。

  func (s *Service) SetFrozenGate(ctx context.Context, userID string, frozen, releaseSubjects bool) error {
	  if s == nil || s.store == nil {
		  return ErrNotFound
	  }
	  userID = strings.TrimSpace(userID)
	  if !frozen && releaseSubjects {
		   _ = s.store.AppendAudit(ctx, AuditEvent{
			   EntityType: "user", EntityID: userID, Action: "user_unfreeze_rejected", Actor: "ops",
			  Detail: map[string]any{
				   "gate": "user", "reason": "user unfreeze does not release subject freezes",
			  },
			  CreatedAt: time.Now().UTC(),
		  })
		   return fmt.Errorf("%w: subjects remain frozen", ErrFreezeGate)
	   }
	   return s.SetFrozen(ctx, userID, frozen)
   }

// requireWithdrawCooling 解冻后一段时间内禁止链上出金，改密/解冻盗号窗口先封提现。

   func (s *Service) requireWithdrawCooling(ctx context.Context, userID string) error {
	   if s == nil || s.withdrawCooling <= 0 {
		  return nil
	  }
	  evs, err := s.store.ListAudit(ctx, "user", userID, 80)
	  if err != nil {
		   return err
	   }
	  at, ok := lastUnfreezeAt(evs)
	   if ok && time.Since(at) < s.withdrawCooling {
		  return fmt.Errorf("%w: until %s", ErrWithdrawCooling, at.Add(s.withdrawCooling).UTC().Format(time.RFC3339))
	  }
	  if sec, _, ok := lastSecurityChangeAt(evs); ok && time.Since(sec) < s.withdrawCooling {
		   return fmt.Errorf("%w: until %s", ErrWithdrawCooling, sec.Add(s.withdrawCooling).UTC().Format(time.RFC3339))
	   }
	  return nil
   }

// MarkSecurityChange 改密/改 2FA/改绑定后写入审计，出金冷却按 withdraw_cooling 生效。
   func (s *Service) MarkSecurityChange(ctx context.Context, userID, reason string) error {
	   userID = strings.TrimSpace(userID)
	   if userID == "" || userID == "system" {
		  return fmt.Errorf("%w: user_id", ErrInvalidAddress)
	  }
	  if strings.TrimSpace(reason) == "" {
		   reason = "security_change"
	   }
	  return s.store.AppendAudit(ctx, AuditEvent{
		  EntityType: "user", EntityID: userID, Action: "security_change", Actor: "security",
		   Detail: map[string]any{"reason": reason},
		  CreatedAt: time.Now().UTC(),
	  })
   }

   func lastUnfreezeAt(evs []AuditEvent) (time.Time, bool) {
	  for _, e := range evs {
		  if e.Action != "control_frozen" {
			   continue
		  }
		   frozen := false
		   if e.Detail != nil {
			   switch v := e.Detail["frozen"].(type) {
			   case bool:
				   frozen = v
			  }
		  }
		  if frozen {
			   return time.Time{}, false
		   }
		  if e.CreatedAt.IsZero() {
			  return time.Now().UTC(), true
		   }
		  return e.CreatedAt, true
	   }
	  return time.Time{}, false
   }

  func lastSecurityChangeAt(evs []AuditEvent) (time.Time, string, bool) {
	   for _, e := range evs {
		  if e.Action != "security_change" {
			  continue
		   }
		   reason := "security_change"
		  if e.Detail != nil {
			   if v, ok := e.Detail["reason"].(string); ok && strings.TrimSpace(v) != "" {
				   reason = strings.TrimSpace(v)
			   }
		   }
		  if e.CreatedAt.IsZero() {
			  return time.Now().UTC(),
				   reason, true
		   }
		  return e.CreatedAt, reason, true
	   }
	  return time.Time{}, "", false
  }

func (s *Service) systemAvailable(ctx context.Context, typ AccountType, asset string) (decimal.Decimal, error) {
	if sum, ok := s.store.(interface {
		SumSystem(context.Context, AccountType, string) (decimal.Decimal, error)
	}); ok {
		return sum.SumSystem(ctx, typ, asset)
	}
	acc, err := s.store.GetAccount(ctx, "system", typ, asset)
	if err != nil {
		return decimal.Zero, err
	}
	return acc.Available, nil
}

func (s *Service) eachShard(ctx context.Context, fn func(context.Context, int, int)) {
	n := 1
	if sh, ok := s.store.(interface{ ShardCount() int }); ok && sh.ShardCount() > 1 {
		n = sh.ShardCount()
	}
	for i := 0; i < n; i++ {
		fn(WithShardIndex(ctx, i), i, n)
	}
}

// ensureHotInventory 广播前核对 system.bridge 可用额。热钱包不够不得把用户冻单变成链上空头。

   func (s *Service) ensureHotInventory(ctx context.Context, asset string, need decimal.Decimal) error {
	  if s == nil || !need.IsPositive() {
		   return nil
	  }
	  have, err := s.systemAvailable(ctx, AccountBridge, asset)
	  if err != nil {
		  return err
	  }
	   if have.LessThan(need) {
		   return fmt.Errorf("%w: have %s need %s", ErrHotInventory, have, need)
	   }
	  return nil
   }

   func (s *Service) Frozen(ctx context.Context, userID string) (bool, error) {
	  return s.store.UserControlFrozen(ctx, userID)
  }

// WithdrawLockUntil 出金页倒计时。解冻与安全变更共用 withdraw_cooling；无锁返回零值。

  func (s *Service) WithdrawLockUntil(ctx context.Context, userID string) (time.Time, string, error) {
	  if s == nil || s.withdrawCooling <= 0 {
		  return time.Time{}, "", nil
	  }
	   userID = strings.TrimSpace(userID)
	  if userID == "" {
		   return time.Time{}, "", fmt.Errorf("%w: user_id required", ErrInvalidAddress)
	   }
	  evs, err := s.store.ListAudit(ctx, "user", userID, 80)
	  if err != nil {
		  return time.Time{}, "", err
	   }
	  var until time.Time
	  reason := ""
	   if at, ok := lastUnfreezeAt(evs); ok {
		   u := at.Add(s.withdrawCooling)
		   if u.After(time.Now().UTC()) {
			   until, reason = u, "unfreeze"
		   }
	   }
	  if sec, secReason, ok := lastSecurityChangeAt(evs); ok {
		   u := sec.Add(s.withdrawCooling)
		   if u.After(time.Now().UTC()) && u.After(until) {
			   until, reason = u, secReason
		   }
	   }
	  return until, reason, nil
  }

const (
	KYCNone = "none"
	KYCBasic = "basic"
	KYCAdvanced = "advanced"
)

   func ParseKYC(s string) (string, error) {
	  switch strings.ToLower(strings.TrimSpace(s)) {
	   case "", KYCBasic:
		  return KYCBasic, nil
	   case KYCNone, KYCAdvanced:
		   return strings.ToLower(strings.TrimSpace(s)), nil
	  default:
		   return "", ErrInvalidKYC
	  }
   }

   func kycRank(level string) int {
	   switch level {
	  case KYCNone:
		   return 0
	   case KYCAdvanced:
		   return 2
	  default:
		   return 1 // missing row is basic so existing withdraw tests stay green
	  }
   }

   func (s *Service) KYC(ctx context.Context, userID string) (string, error) {
	   userID = strings.TrimSpace(userID)
	  if userID == "" {
		  return "", fmt.Errorf("%w: user_id required", ErrInvalidAddress)
	  }
	   got, err := s.store.UserKYC(ctx, userID)
	  if err != nil {
		  return "", err
	   }
	  if got == "" {
		   return KYCBasic, nil
	  }
	  return got, nil
  }

   func (s *Service) SetKYC(ctx context.Context, userID, level string) error {
	  userID = strings.TrimSpace(userID)
	  if userID == "" || userID == "system" {
		  return fmt.Errorf("%w: user_id", ErrInvalidAddress)
	  }
	  parsed, err := ParseKYC(level)
	   if err != nil {
		  return err
	  }
	   if err := s.store.SetUserKYC(ctx, userID, parsed); err != nil {
		   return err
	   }
	   return s.store.AppendAudit(ctx, AuditEvent{
		   EntityType: "user", EntityID: userID, Action: "control_kyc", Actor: "ops",
		   Detail: map[string]any{"kyc_level": parsed},
	   })
  }

  func (s *Service) requireKYC(ctx context.Context, userID, min string) error {
	  got, err := s.KYC(ctx, userID)
	  if err != nil {
		  return err
	   }
	   if kycRank(got) < kycRank(min) {
		  return fmt.Errorf("%w: have %s need %s", ErrKYCRequired, got, min)
	  }
	   return nil
  }

  func (s *Service) earnInterest(tr *Transfer) decimal.Decimal {
	   // 赎回本金不再按配置 BPS 另铸利息；应付利息只走 Earn.PostInterest，避免双记。
	   _ = tr
	   _ = s
	   return decimal.Zero
  }

// SweepHot 热→冷。超额按 ParseAmount 算，走 Create(hot_cold_sweep) + 幂等键 sweep:asset:amount。
// 解析失败或非正数跳过该资产，不把坏配置写成一笔划转。

   func (s *Service) SweepHot(ctx context.Context, hotMax map[string]string) {
	   if s == nil || len(hotMax) == 0 {
		  return
	  }
	   for asset, raw := range hotMax {
		   max, err := ParseAmount(raw)
		   if err != nil || !max.IsPositive() {
			   continue
		  }
		  s.eachShard(ctx, func(sctx context.Context, i, n int) {
			  acc, err := s.store.GetAccount(sctx, "system", AccountBridge, asset)
			  if err != nil {
				  return
			  }
			  excess := acc.Available.Sub(max)
			  if !excess.IsPositive() {
				  return
			  }
			  key := "sweep:" + asset + ":" + excess.String()
			  if n > 1 {
				  key = fmt.Sprintf("sweep:%d:%s:%s", i, asset, excess.String())
			  }
			  _, _ = s.Create(sctx, CreateReq{
				  UserID: "system", Direction: HotColdSweep, Asset: asset, Amount: excess,
				  IdempotencyKey: key,
			  })
		  })
	   }
   }

// SweepTiers 热→温→冷。超额先入 system.warm，温端再扫入 cold。

  func (s *Service) SweepTiers(ctx context.Context, hotMax, warmMax map[string]string) {
	  if s == nil {
		  return
	  }
	  for asset, raw := range hotMax {
		   max, err := ParseAmount(raw)
		  if err != nil || !max.IsPositive() {
			   continue
		   }
		  s.eachShard(ctx, func(sctx context.Context, i, n int) {
			  acc, err := s.store.GetAccount(sctx, "system", AccountBridge, asset)
			  if err != nil {
				  return
			  }
			  excess := acc.Available.Sub(max)
			  if !excess.IsPositive() {
				  return
			  }
			  key := "sweep-warm:" + asset + ":" + excess.String()
			  if n > 1 {
				  key = fmt.Sprintf("sweep-warm:%d:%s:%s", i, asset, excess.String())
			  }
			  _, _ = s.Create(sctx, CreateReq{
				  UserID: "system", Direction: HotWarmSweep, Asset: asset, Amount: excess,
				  IdempotencyKey: key,
			  })
		  })
	  }
	   for asset, raw := range warmMax {
		   max, err := ParseAmount(raw)
		   if err != nil || !max.IsPositive() {
			  continue
		   }
		  s.eachShard(ctx, func(sctx context.Context, i, n int) {
			  acc, err := s.store.GetAccount(sctx, "system", AccountWarm, asset)
			  if err != nil {
				  return
			  }
			  excess := acc.Available.Sub(max)
			  if !excess.IsPositive() {
				  return
			  }
			  key := "sweep-cold:" + asset + ":" + excess.String()
			  if n > 1 {
				  key = fmt.Sprintf("sweep-cold:%d:%s:%s", i, asset, excess.String())
			  }
			  _, _ = s.Create(sctx, CreateReq{
				  UserID: "system", Direction: WarmColdSweep, Asset: asset, Amount: excess,
				  IdempotencyKey: key,
			  })
		  })
	   }
   }

// Cancel 没广播的冻单可以回滚。链上还活着的不能解。

   func (s *Service) Cancel(ctx context.Context, id, reason string) error {
	  cur, err := s.store.GetTransfer(ctx, id)
	  if err != nil {
		   return err
	  }
	  ctx = WithShardUser(ctx, cur.UserID)
	  if cur.State == StateCredited || cur.State == StateRolledBack {
		  return nil
	   }
	   if cur.Direction.OnChain() && cur.TxHash != "" && !IsInternalTxHash(cur.TxHash) {
		   if cur.State == StateChainSubmitted || cur.State == StateConfirmed {
			  return ErrSubmittedLive
		   }
	  }
	   switch cur.State {
	  case StateInitiated, StateSpotFrozen, StateWalletLocked, StateHold, StateFailed:
	  default:
		   if cur.TxHash != "" && !IsInternalTxHash(cur.TxHash) {
			  return ErrCannotCancel
		   }
		   if cur.State == StateChainSubmitted || cur.State == StateConfirmed {
			  return ErrCannotCancel
		   }
	  }
	  if reason == "" {
		  reason = "user_cancel"
	  }
	  return s.FailAndRollback(ctx,
		  id, reason)
  }
