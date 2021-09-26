package lendoccupygate

import (
	"context"
	"errors"
	"strings"
	"time"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// LendingService lending 包能力接口（只读契约，Engine 不直接操作账本）。
// 由 internal/lending.Service 适配实现。
type LendingService interface {
	// Place 放贷挂单：P2PDebit 入 bridge 并写 holdcat lending_order 标签。
	// 返回 holdRef（holdcat row ID 或 ref）与幂等键；出错时 holdErr 非 nil。
	Place(ctx context.Context,
		userID, asset, amount, orderID string) (holdRef string, idemKey string, holdErr error)
	// Settle 成交释放：P2PCredit 回 Funding，并按 order id ReleaseByRef。
	// 由 Engine 在 ConsumeGen 成功后调用。
	Settle(ctx context.Context, userID, asset, amount, orderID string) error
	// Cancel 取消退回：P2PCredit 回 Funding，并按 order id ReleaseByRef。
	// 由 Engine 在 ConsumeGen 成功后调用。
	Cancel(ctx context.Context, userID, asset, amount, orderID string) error
	// CompensateDebit 补偿冲销借记（P2PCredit 以抵消 Place 的 P2PDebit）。
	// 由 Engine 在半截路径（借记成功 hold 失败）调用，幂等键为 "lend-residue-comp:" + orderID。
	CompensateDebit(ctx context.Context, userID, asset, amount, orderID string) error
}

// EngineOption Engine 配置函数。
type EngineOption func(*Engine)

// WithEngineClock 注入时钟。
  func WithEngineClock(fn func() time.Time) EngineOption {
	  return func(e *Engine) {
		   if fn != nil {
			   e.clock = fn
		   }
	  }
}

// WithStaleTTL 设置 placed 租约的最大容忍时间（默认 5min）。
// 超过此时间仍 placed 的租约将被 Scanner 视为半截候选。
   func WithStaleTTL(d time.Duration) EngineOption {
	  return func(e *Engine) {
		  if d > 0 {
			   e.staleTTL = d
		  }
	  }
   }

const defaultStaleTTL = 5 * time.Minute

// Engine lendoccupygate 核心引擎。
type Engine struct {
	store Store
	lending  LendingService
	clock    func() time.Time
	staleTTL time.Duration
}

// NewEngine 构造 Engine。lending 为 nil 时 PlaceBound/Settle/Cancel 将直接报错（不允许 nil 注入）。
   func NewEngine(store Store, lending LendingService, opts ...EngineOption) *Engine {
	   if store == nil {
		  store = NewMemStore()
	  }
	  e := &Engine{
		  store:   store,
		  lending: lending,
		  clock:    func() time.Time { return time.Now().UTC() },
		   staleTTL: defaultStaleTTL,
	   }
	  for _, opt := range opts {
		  opt(e)
	  }
	   return e
  }

// ── PlaceBound ────────────────────────────────────────────────────────────────
// PlaceBound 放贷挂单占用主路径：
// 1. 校验请求参数；
// 2. 检查同 order 是否已有 active gen（幂等防重）；
// 3. 调用 LendingService.Place（P2PDebit + holdcat）；
// 4. 成功：写 LendGen + LendOccupyLease；
// 5. hold 失败：落 LendResidueProof + 补偿冲销借记；
// 6. 返回 PlaceBoundResp（含 Lease + Gen）。
  func (e *Engine) PlaceBound(ctx context.Context, req PlaceBoundReq) (*PlaceBoundResp, error) {
	  if err := e.validateReq(req.UserID, req.Asset, req.Amount, req.OrderID); err != nil {
		   return nil, err
	  }
	   // 幂等检查：同 order 已有 active gen
	   existing, err := e.store.GetGenByOrder(ctx, req.OrderID)
	   if err != nil && !errors.Is(err, ErrGenNotFound) {
		  return nil, err
	   }
	  if existing != nil && !existing.Consumed {
		   return nil, ErrGenAlreadyExists
	  }
	  idemKey := "lend-ord:" + req.OrderID
	  now := e.clock()
	  // 调用 lending.Place（P2PDebit + holdcat.PlaceLendingOrder）
	  _, _, placeErr := e.lending.Place(ctx, req.UserID, req.Asset, req.Amount, req.OrderID)
	   if placeErr != nil {
		   // Place 彻底失败（借记也未成功）——直接返回错误，不需要补偿
		  return nil, placeErr
	  }
	   // 签发 LendGen
	   genID := uuid.NewString()
	   gen := &LendGen{
		  GenID:   genID,
		   OrderID: req.OrderID,
		  UserID:  req.UserID,
		  Asset:   req.Asset,
		   Amount:        req.Amount,
		  LedgerIdemKey: idemKey,
		   Consumed:      false,
		  IssuedAt:      now,
		  CreatedAt: now,
	  }
	  if saveErr := e.store.SaveGen(ctx, gen); saveErr != nil {
		   // gen 写入失败：属于半截情况（借记已成功），需触发补偿
		   return nil, e.handleHoldFailure(ctx, req, genID, idemKey, saveErr, now)
	   }
	   // 写 LendOccupyLease
	  leaseID := uuid.NewString()
	  lease := &LendOccupyLease{
		  LeaseID: leaseID,
		   GenID: genID,
		   OrderID: req.OrderID,
		   UserID:  req.UserID,
		   Asset:   req.Asset,
		  Amount:        req.Amount,
		   State:         LeasePlaced,
		   LedgerIdemKey: idemKey,
		   HoldRef:       req.OrderID,
		   PlacedAt:      now,
		   CreatedAt:     now,
		  UpdatedAt: now,
	   }
	   if saveErr := e.store.SaveLease(ctx, lease); saveErr != nil {
		   if errors.Is(saveErr, ErrGenAlreadyExists) {
			   // 幂等：同 order 的 lease 已存在，gen 已签发，返回现有
			  existLease, _ := e.store.GetLeaseByOrder(ctx, req.OrderID)
			   existGen, _ := e.store.GetGenByOrder(ctx, req.OrderID)
			  if existLease != nil && existGen != nil {
				  return &PlaceBoundResp{Lease: *existLease,
					  Gen: *existGen}, nil
			  }
		  }
		  return nil,
			   e.handleHoldFailure(ctx, req, genID, idemKey, saveErr, now)
	   }
	  return &PlaceBoundResp{Lease: *lease, Gen: *gen}, nil
  }

// handleHoldFailure 半截补偿入口：借记成功但 hold/gen 写入失败。
// 补偿贷记幂等 + 落 LendResidueProof。返回包装后的 ErrHoldFailed。
   func (e *Engine) handleHoldFailure(ctx context.Context, req PlaceBoundReq, genID, idemKey string, cause error, now time.Time) error {
	  compIdem := "lend-residue-comp:" + req.OrderID
	   _ = e.lending.CompensateDebit(ctx,
		  req.UserID, req.Asset, req.Amount, req.OrderID)
	  failReason := cause.Error()
	  if len(failReason) > 256 {
		   failReason = failReason[:256]
	   }
	  proof := &LendResidueProof{
		  ProofID: uuid.NewString(),
		   GenID:   genID,
		   OrderID: req.OrderID,
		  UserID:  req.UserID,
		   Asset: req.Asset,
		  Amount:            req.Amount,
		   Kind:              ResidueHoldFailed,
		   DebitIdemKey:      idemKey,
		  CompensateIdemKey: compIdem,
		   FailReason:        failReason,
		  CreatedAt: now,
	   }
	  // 落地 residue proof（忽略二次写错误，幂等）
	   _ = e.store.SaveResidueProof(ctx, proof)
	  return ErrHoldFailed
   }

// ── Settle ────────────────────────────────────────────────────────────────────
// Settle 成交释放：互斥消费 LendGen，成功后驱动租约状态→settled，调用 LendingService.Settle。
// 重复 Settle 或与 Cancel 并发失败方：落 LendDupReleaseProof，返回 ErrGenConsumed（包含 DupProof）。
// 注意：不在 ConsumeGen 前检查 lease.State——避免并发场景下另一方已更新 lease 后此方误判。
// ConsumeGen CAS 是唯一的互斥门禁；lease.State 更新只在 CAS 胜出后执行。

   func (e *Engine) Settle(ctx context.Context, req SettleReq) (*SettleResp, error) {
	  if err := e.validateReq(req.UserID, req.Asset, req.Amount, req.OrderID); err != nil {
		   return nil, err
	  }
	  gen,
		  lease, err := e.loadGenAndLeaseNoStateCheck(ctx, req.OrderID)
	   if err != nil {
		  return nil, err
	   }
	  if err := e.checkOwnerAndAmount(gen, req.UserID, req.Amount); err != nil {
		  return nil, err
	  }
	   now := e.clock()
	   // 互斥 CAS：原子消费 gen（唯一门禁）
	   consumeErr := e.store.ConsumeGen(ctx, gen.GenID, "settle", now)
	   if consumeErr != nil {
		  if errors.Is(consumeErr, ErrGenConsumed) {
			  // 重复 Settle 或 Settle 竞争失败：落 dup proof
			   dupProof, proofErr := e.saveDupProof(ctx, gen, lease, "settle", now)
			   if proofErr != nil && !errors.Is(proofErr, ErrAlreadyProven) {
				  return nil, proofErr
			  }
			   if proofErr != nil {
				  dupProof, _ = e.store.GetDupProofByGen(ctx, gen.GenID)
			   }
			  resp := &SettleResp{Lease: *lease,
				  DupProof: dupProof}
			  return resp, ErrGenConsumed
		   }
		   return nil, consumeErr
	   }
	  // CAS 成功：驱动租约状态
	  t := now
	   patch := &LendOccupyLease{SettledAt: &t}
	   _ = e.store.UpdateLeaseState(ctx, lease.LeaseID, LeasePlaced, LeaseSettled, patch)
	   // 调用 lending.Settle（P2PCredit + ReleaseByRef）
	  if settleErr := e.lending.Settle(ctx, req.UserID, req.Asset, req.Amount, req.OrderID); settleErr != nil {
		  return nil, settleErr
	   }
	   finalLease,
		   _ := e.store.GetLeaseByOrder(ctx, req.OrderID)
	  if finalLease == nil {
		  finalLease = lease
		  finalLease.State = LeaseSettled
		  finalLease.SettledAt = &t
	   }
	   return &SettleResp{Lease: *finalLease},
		  nil
   }

// ── Cancel ────────────────────────────────────────────────────────────────────
// Cancel 取消退回：互斥消费 LendGen，成功后驱动租约状态→cancelled，调用 LendingService.Cancel。
// Settle 已消费 gen 或重复 Cancel：落 LendDupReleaseProof，返回 ErrGenConsumed（含 DupProof）。

  func (e *Engine) Cancel(ctx context.Context, req CancelReq) (*CancelResp, error) {
	  if err := e.validateReq(req.UserID, req.Asset, req.Amount, req.OrderID); err != nil {
		   return nil, err
	  }
	  gen, lease, err := e.loadGenAndLeaseNoStateCheck(ctx, req.OrderID)
	   if err != nil {
		   return nil, err
	  }
	   if err := e.checkOwnerAndAmount(gen, req.UserID, req.Amount); err != nil {
		   return nil, err
	  }
	   now := e.clock()
	   // 互斥 CAS：原子消费 gen（唯一门禁）
	   consumeErr := e.store.ConsumeGen(ctx, gen.GenID, "cancel", now)
	   if consumeErr != nil {
		  if errors.Is(consumeErr, ErrGenConsumed) {
			  dupProof, proofErr := e.saveDupProof(ctx, gen, lease, "cancel", now)
			   if proofErr != nil && !errors.Is(proofErr, ErrAlreadyProven) {
				   return nil, proofErr
			   }
			   if proofErr != nil {
				   dupProof, _ = e.store.GetDupProofByGen(ctx, gen.GenID)
			  }
			  resp := &CancelResp{Lease: *lease, DupProof: dupProof}
			  return resp, ErrGenConsumed
		   }
		   return nil, consumeErr
	   }
	  t := now
	  patch := &LendOccupyLease{CancelledAt: &t}
	   _ = e.store.UpdateLeaseState(ctx, lease.LeaseID, LeasePlaced, LeaseCancelled, patch)
	  if cancelErr := e.lending.Cancel(ctx, req.UserID, req.Asset, req.Amount, req.OrderID); cancelErr != nil {
		   return nil, cancelErr
	  }
	  finalLease, _ := e.store.GetLeaseByOrder(ctx, req.OrderID)
	  if finalLease == nil {
		  finalLease = lease
		   finalLease.State = LeaseCancelled
		  finalLease.CancelledAt = &t
	  }
	  return &CancelResp{Lease: *finalLease}, nil
  }

// ── CompensateResidue ─────────────────────────────────────────────────────────
// CompensateResidue 半截补偿：对 placed 租约执行补偿冲销并关闭。
// 由 Scanner/Worker 调用；幂等：同 GenID 已有 residue proof 则跳过。

   func (e *Engine) CompensateResidue(ctx context.Context, lease *LendOccupyLease) (*LendResidueProof, error) {
	   if lease == nil {
		  return nil, ErrLeaseNotFound
	   }
	   if lease.State != LeasePlaced {
		  return nil, ErrLeaseNotPlaced
	   }
	  // 幂等：检查是否已有证明
	   existing,
		  err := e.store.GetResidueProofByGen(ctx, lease.GenID)
	   if err == nil && existing != nil {
		   return existing, nil
	   }
	  now := e.clock()
	   compIdem := "lend-residue-comp:" + lease.OrderID
	   // 补偿贷记
	   compensateErr := e.lending.CompensateDebit(ctx, lease.UserID, lease.Asset, lease.Amount, lease.OrderID)
	   failReason := ""
	  kind := ResidueDebitRolledBack
	   if compensateErr != nil {
		   failReason = compensateErr.Error()
		   if len(failReason) > 256 {
			   failReason = failReason[:256]
		   }
	  }
	   proof := &LendResidueProof{
		   ProofID: uuid.NewString(),
		   GenID:   lease.GenID,
		  OrderID: lease.OrderID,
		   UserID: lease.UserID,
		   Asset:  lease.Asset,
		  Amount: lease.Amount,
		  Kind:              kind,
		  DebitIdemKey:      lease.LedgerIdemKey,
		   CompensateIdemKey: compIdem,
		  FailReason: failReason,
		   CreatedAt: now,
	   }
	   if saveErr := e.store.SaveResidueProof(ctx, proof); saveErr != nil {
		  if errors.Is(saveErr, ErrAlreadyProven) {
			   existing, _ = e.store.GetResidueProofByGen(ctx, lease.GenID)
			  return existing, nil
		  }
		   return nil, saveErr
	   }
	   // 标记 gen 为已消费（补偿路径）
	  _ = e.store.ConsumeGen(ctx, lease.GenID, "residue", now)
	   // 关闭租约到 residue_closed
	  t := now
	  patch := &LendOccupyLease{
		  ResidueClosedAt: &t,
		  ResidueProofID:  proof.ProofID,
	   }
	  _ = e.store.UpdateLeaseState(ctx,
		   lease.LeaseID, LeasePlaced, LeaseResidueClosed, patch)
	  return proof, nil
   }

// ── ExportProofs ──────────────────────────────────────────────────────────────
// ExportProofs 导出指定 order 的所有证明（dup + residue）。
   func (e *Engine) ExportProofs(ctx context.Context, req ExportProofReq) (*ExportProofResp, error) {
	  req.OrderID = strings.TrimSpace(req.OrderID)
	  if req.OrderID == "" {
		   return nil, ErrOrderRequired
	   }
	  resp := &ExportProofResp{}
	   if req.Kind == "" || req.Kind == "dup" {
		   dups, _ := e.store.ListDupProofsByOrder(ctx, req.OrderID)
		  for _, p := range dups {
			  resp.DupProofs = append(resp.DupProofs, *p)
		  }
	  }
	  if req.Kind == "" || req.Kind == "residue" {
		  residues, _ := e.store.ListResidueProofsByOrder(ctx, req.OrderID)
		  for _, p := range residues {
			   resp.ResidueProofs = append(resp.ResidueProofs, *p)
		  }
	  }
	   return resp, nil
  }

// ── 辅助 ─────────────────────────────────────────────────────────────────────

   func (e *Engine) validateReq(userID, asset, amount, orderID string) error {
	  userID = strings.TrimSpace(userID)
	  asset = strings.TrimSpace(asset)
	   amount = strings.TrimSpace(amount)
	   orderID = strings.TrimSpace(orderID)
	  if userID == "" {
		  return ErrUserRequired
	   }
	  if asset == "" {
		  return ErrAssetRequired
	   }
	   if orderID == "" {
		   return ErrOrderRequired
	  }
	   if amount == "" {
		  return ErrAmountInvalid
	  }
	   amt, err := decimal.NewFromString(amount)
	   if err != nil || !amt.IsPositive() {
		   return ErrAmountInvalid
	  }
	   return nil
   }

// loadGenAndLease 加载 gen 和 lease；同时检查 lease.State == placed（用于 CompensateResidue）。

   func (e *Engine) loadGenAndLease(ctx context.Context, orderID string) (*LendGen, *LendOccupyLease, error) {
	  gen,
		   err := e.store.GetGenByOrder(ctx, orderID)
	  if err != nil {
		  return nil, nil, err
	  }
	  lease, err := e.store.GetLeaseByOrder(ctx, orderID)
	   if err != nil {
		   return nil, nil, err
	   }
	  if lease.State != LeasePlaced {
		   return nil, nil, ErrLeaseNotPlaced
	   }
	  return gen, lease, nil
  }

// loadGenAndLeaseNoStateCheck 加载 gen 和 lease 但不检查 lease.State。
// 用于 Settle/Cancel：ConsumeGen CAS 是唯一互斥门禁，不应在 CAS 前因 lease.State 拒绝竞争失败方。
   func (e *Engine) loadGenAndLeaseNoStateCheck(ctx context.Context, orderID string) (*LendGen, *LendOccupyLease, error) {
	  gen, err := e.store.GetGenByOrder(ctx, orderID)
	  if err != nil {
		  return nil, nil, err
	  }
	  lease, err := e.store.GetLeaseByOrder(ctx, orderID)
	   if err != nil {
		   return nil, nil, err
	  }
	  return gen, lease, nil
   }

  func (e *Engine) checkOwnerAndAmount(gen *LendGen, userID, amount string) error {
	   if gen.UserID != strings.TrimSpace(userID) {
		   return ErrOwnerMismatch
	   }
	  genAmt, _ := decimal.NewFromString(gen.Amount)
	   reqAmt, err := decimal.NewFromString(strings.TrimSpace(amount))
	   if err != nil || !reqAmt.Equal(genAmt) {
		   return ErrAmountMismatch
	   }
	  return nil
   }

   func (e *Engine) saveDupProof(
	  ctx context.Context,
	  gen *LendGen,
	  lease *LendOccupyLease,
	   attemptedBy string,
	  now time.Time,
   ) (*LendDupReleaseProof, error) {
	   // 重新加载 gen 以获取最新 ConsumedBy
	   fresh, freshErr := e.store.GetGen(ctx, gen.GenID)
	   firstConsumedBy := gen.ConsumedBy
	  firstConsumedAt := now
	   if freshErr == nil && fresh.ConsumedAt != nil {
		   firstConsumedBy = fresh.ConsumedBy
		  firstConsumedAt = *fresh.ConsumedAt
	  }
	   kind := dupKind(firstConsumedBy, attemptedBy)
	   proof := &LendDupReleaseProof{
		   ProofID: uuid.NewString(),
		  GenID:   gen.GenID,
		   OrderID: gen.OrderID,
		   UserID:  gen.UserID,
		   Asset: gen.Asset,
		  Amount:          gen.Amount,
		  Kind:            kind,
		   FirstConsumedBy: firstConsumedBy,
		  FirstConsumedAt: firstConsumedAt,
		   AttemptedBy:     attemptedBy,
		   AttemptedAt: now,
		   CreatedAt: now,
	  }
	   if err := e.store.SaveDupProof(ctx, proof); err != nil {
		   return nil, err
	  }
	   return proof, nil
   }

  func dupKind(first, second string) DupReleaseKind {
	  key := first + "+" + second
	  switch key {
	  case "settle+settle":
		  return DupSettleSettle
	   case "cancel+cancel":
		  return DupCancelCancel
	  default:
		   return DupSettleCancel
	   }
  }
