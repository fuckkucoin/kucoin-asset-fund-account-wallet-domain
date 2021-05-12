package feeresgate

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/feereserve"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

// DefaultLeaseTTL 租约默认有效期。
const DefaultLeaseTTL = 10 * time.Minute

// Engine 竞态收口引擎。所有写操作均通过 Engine 方法进入，内部串行化关键路径。
type Engine struct {
	mu sync.Mutex
	store feereserve.Store
	clock func() time.Time
	ttl time.Duration
	leaseFSM *leaseFSMHelper
	caseFSM  *caseFSMHelper
}

type Config struct {
	Store feereserve.Store
	Clock    func() time.Time
	LeaseTTL time.Duration
}

   func New(cfg Config) *Engine {
	  if cfg.Store == nil {
		   cfg.Store = feereserve.NewMemStore()
	  }
	   if cfg.Clock == nil {
		  cfg.Clock = func() time.Time { return time.Now().UTC() }
	  }
	   if cfg.LeaseTTL <= 0 {
		   cfg.LeaseTTL = DefaultLeaseTTL
	   }
	  return &Engine{
		  store: cfg.Store,
		  clock: cfg.Clock,
		  ttl: cfg.LeaseTTL,
		  leaseFSM: newLeaseFSMHelper(),
		   caseFSM:  newCaseFSMHelper(),
	   }
   }

// ──────────────────────────────────────────
// Reserve 预留手续费
// ──────────────────────────────────────────

   func (e *Engine) Reserve(ctx context.Context, req feereserve.ReserveReq) (feereserve.ReserveLease, error) {
	  if e == nil {
		  return feereserve.ReserveLease{},
			  ErrEngineNil
	  }
	   wid := strings.TrimSpace(req.WithdrawID)
	   if wid == "" {
		   return feereserve.ReserveLease{}, feereserve.ErrWithdrawID
	   }
	  uid, err := validate.UserID(req.UserID)
	   if err != nil {
		  return feereserve.ReserveLease{},
			   feereserve.ErrWithdrawID
	  }
	  asset, err := validate.Asset(req.Asset)
	   if err != nil {
		  return feereserve.ReserveLease{}, feereserve.ErrAsset
	  }
	   fee, err := money.Parse(req.Fee)
	  if err != nil || !fee.IsPositive() {
		  return feereserve.ReserveLease{}, feereserve.ErrAmount
	  }
	  e.mu.Lock()
	  defer e.mu.Unlock()
	  existing, err := e.store.GetLeaseByWithdraw(ctx, wid)
	   if err == nil {
		   if existing.State == feereserve.LeaseReserved {
			   tok := feereserve.NewLeaseToken()
			  now := e.clock()
			  existing.Generation++
			  existing.Token = tok
			  existing.ReservedFee = fee.String()
			  existing.LeaseExpiry = now.Add(e.ttl)
			  existing.UpdatedAt = now
			   saved, err := e.store.SaveLease(ctx, existing)
			  if err != nil {
				  return feereserve.ReserveLease{}, err
			  }
			  return saved, nil
		  }
		  return feereserve.ReserveLease{}, feereserve.ErrBlocked
	  }
	  if !errors.Is(err, feereserve.ErrLeaseNotFound) {
		  return feereserve.ReserveLease{}, err
	   }
	   now := e.clock()
	  lease := feereserve.ReserveLease{
		   ID: uuid.NewString(),
		   WithdrawID:  wid,
		   UserID:      uid,
		   Asset:       asset,
		   ReservedFee: fee.String(),
		   Generation: 1,
		  Token: feereserve.NewLeaseToken(),
		   State: feereserve.LeaseReserved,
		  LeaseExpiry: now.Add(e.ttl),
		  CreatedAt: now,
		   UpdatedAt: now,
	   }
	  saved, err := e.store.SaveLease(ctx, lease)
	   if err != nil {
		  return feereserve.ReserveLease{}, err
	  }
	  return saved, nil
  }

// ──────────────────────────────────────────
// Settle 结算——出金 confirm 时扣费
// ──────────────────────────────────────────

   func (e *Engine) Settle(ctx context.Context, req feereserve.SettleReq) (feereserve.ReserveLease, error) {
	  if e == nil {
		   return feereserve.ReserveLease{}, ErrEngineNil
	   }
	   if strings.TrimSpace(req.WithdrawID) == "" || strings.TrimSpace(req.LeaseID) == "" {
		   return feereserve.ReserveLease{}, feereserve.ErrWithdrawID
	   }
	  if strings.TrimSpace(req.VoucherID) == "" {
		  return feereserve.ReserveLease{}, ErrReqInvalid
	   }
	  actualFee, err := money.Parse(req.ActualFee)
	   if err != nil || !actualFee.IsPositive() {
		   return feereserve.ReserveLease{}, feereserve.ErrAmount
	   }
	   e.mu.Lock()
	   defer e.mu.Unlock()
	  lease, err := e.store.GetLease(ctx, req.LeaseID)
	  if err != nil {
		  return feereserve.ReserveLease{},
			  err
	   }
	  if lease.WithdrawID != req.WithdrawID {
		   return feereserve.ReserveLease{}, feereserve.ErrWithdrawID
	  }
	   if lease.IsTerminal() {
		  return e.handleOperateAfterTerminal(ctx, lease, feereserve.LegSettle, req.VoucherID)
	   }
	  if err := e.validateGenToken(lease, req.Generation, req.Token); err != nil {
		  return feereserve.ReserveLease{}, err
	   }
	  tb,
		   bound, err := e.store.GetTerminalBind(ctx, req.WithdrawID)
	   if err != nil {
		   return feereserve.ReserveLease{}, err
	  }
	  if bound {
		   if tb.Leg == feereserve.LegSettle && tb.VoucherID == req.VoucherID {
			  return lease, nil
		   }
		   return e.handleTerminalConflict(ctx,
			   lease, tb, feereserve.LegSettle, req.VoucherID)
	  }
	  if err := e.store.SaveTerminalBind(ctx, feereserve.TerminalBind{
		  WithdrawID: req.WithdrawID,
		  Leg:        feereserve.LegSettle,
		   LeaseID:    lease.ID,
		   Generation: req.Generation,
		  Token:      req.Token,
		   VoucherID: req.VoucherID,
		   BoundAt:   e.clock(),
	  }); err != nil {
		   return feereserve.ReserveLease{}, err
	   }
	   now := e.clock()
	   next, err := e.leaseFSM.apply(lease.State, feereserve.LeaseEvSettle)
	   if err != nil {
		  return feereserve.ReserveLease{}, feereserve.ErrState
	  }
	   lease.State = next
	  lease.TerminalLeg = string(feereserve.LegSettle)
	   lease.ActualFee = actualFee.String()
	   lease.VoucherID = req.VoucherID
	   lease.UpdatedAt = now
	  lease.SettledAt = &now
	   saved, err := e.store.SaveLease(ctx, lease)
	  if err != nil {
		   return feereserve.ReserveLease{}, err
	   }
	  if err := e.detectDelta(ctx, saved); err != nil {
		  return saved, nil
	   }
	   return saved, nil
  }

// ──────────────────────────────────────────
// Release 释放——出金 cancel 时退费
// ──────────────────────────────────────────

   func (e *Engine) Release(ctx context.Context, req feereserve.ReleaseReq) (feereserve.ReserveLease, error) {
	   if e == nil {
		  return feereserve.ReserveLease{}, ErrEngineNil
	   }
	  if strings.TrimSpace(req.WithdrawID) == "" || strings.TrimSpace(req.LeaseID) == "" {
		  return feereserve.ReserveLease{},
			   feereserve.ErrWithdrawID
	  }
	   if strings.TrimSpace(req.VoucherID) == "" {
		  return feereserve.ReserveLease{},
			  ErrReqInvalid
	   }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	   lease, err := e.store.GetLease(ctx, req.LeaseID)
	   if err != nil {
		   return feereserve.ReserveLease{}, err
	  }
	  if lease.WithdrawID != req.WithdrawID {
		  return feereserve.ReserveLease{}, feereserve.ErrWithdrawID
	   }
	  if lease.IsTerminal() {
		   return e.handleOperateAfterTerminal(ctx, lease, feereserve.LegRelease, req.VoucherID)
	  }
	   if err := e.validateGenToken(lease, req.Generation, req.Token); err != nil {
		  return feereserve.ReserveLease{}, err
	   }
	  tb,
		   bound, err := e.store.GetTerminalBind(ctx, req.WithdrawID)
	   if err != nil {
		  return feereserve.ReserveLease{}, err
	  }
	   if bound {
		   if tb.Leg == feereserve.LegRelease && tb.VoucherID == req.VoucherID {
			   return lease, nil
		  }
		   return e.handleTerminalConflict(ctx, lease, tb, feereserve.LegRelease, req.VoucherID)
	   }
	  if err := e.store.SaveTerminalBind(ctx, feereserve.TerminalBind{
		   WithdrawID: req.WithdrawID,
		  Leg:        feereserve.LegRelease,
		  LeaseID: lease.ID,
		  Generation: req.Generation,
		  Token:      req.Token,
		   VoucherID: req.VoucherID,
		   BoundAt: e.clock(),
	  }); err != nil {
		   return feereserve.ReserveLease{}, err
	  }
	   now := e.clock()
	  next, err := e.leaseFSM.apply(lease.State, feereserve.LeaseEvRelease)
	   if err != nil {
		  return feereserve.ReserveLease{}, feereserve.ErrState
	   }
	   lease.State = next
	  lease.TerminalLeg = string(feereserve.LegRelease)
	  lease.ActualFee = "0"
	   lease.VoucherID = req.VoucherID
	  lease.UpdatedAt = now
	   lease.ReleasedAt = &now
	   saved, err := e.store.SaveLease(ctx, lease)
	  if err != nil {
		   return feereserve.ReserveLease{},
			  err
	   }
	   return saved, nil
   }

// ──────────────────────────────────────────
// Block 封锁——不允许该笔出金再做 fee-reserve 写操作
// ──────────────────────────────────────────

  func (e *Engine) Block(ctx context.Context, req feereserve.BlockReq) (feereserve.ReserveLease, error) {
	  if e == nil {
		   return feereserve.ReserveLease{}, ErrEngineNil
	   }
	  if strings.TrimSpace(req.WithdrawID) == "" || strings.TrimSpace(req.LeaseID) == "" {
		  return feereserve.ReserveLease{}, feereserve.ErrWithdrawID
	  }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	  return e.blockLocked(ctx, req)
   }

  func (e *Engine) blockLocked(ctx context.Context, req feereserve.BlockReq) (feereserve.ReserveLease, error) {
	  lease,
		   err := e.store.GetLease(ctx, req.LeaseID)
	   if err != nil {
		  return feereserve.ReserveLease{}, err
	  }
	  if lease.WithdrawID != req.WithdrawID {
		   return feereserve.ReserveLease{}, feereserve.ErrWithdrawID
	  }
	   if lease.State == feereserve.LeaseBlocked {
		  return lease, nil
	  }
	  next, err := e.leaseFSM.apply(lease.State, feereserve.LeaseEvBlock)
	   if err != nil {
		   return feereserve.ReserveLease{}, feereserve.ErrState
	   }
	  now := e.clock()
	   lease.State = next
	  lease.BlockReason = req.Reason
	   lease.UpdatedAt = now
	   return e.store.SaveLease(ctx, lease)
   }

// ──────────────────────────────────────────
// 查询
// ──────────────────────────────────────────

   func (e *Engine) GetLease(ctx context.Context, id string) (feereserve.ReserveLease, error) {
	  if e == nil {
		   return feereserve.ReserveLease{}, ErrEngineNil
	  }
	   return e.store.GetLease(ctx, id)
   }

  func (e *Engine) GetLeaseByWithdraw(ctx context.Context, withdrawID string) (feereserve.ReserveLease, error) {
	  if e == nil {
		   return feereserve.ReserveLease{}, ErrEngineNil
	   }
	  return e.store.GetLeaseByWithdraw(ctx, withdrawID)
  }

  func (e *Engine) ListLeasesByState(ctx context.Context, state feereserve.LeaseState, limit int) ([]feereserve.ReserveLease, error) {
	  if e == nil {
		   return nil, ErrEngineNil
	  }
	  return e.store.ListLeasesByState(ctx, state, limit)
  }

// ──────────────────────────────────────────
// CaseConfirm 确认案例 → proven / closed
// ──────────────────────────────────────────

  func (e *Engine) CaseConfirm(ctx context.Context, req feereserve.CaseConfirmReq) (feereserve.ReserveCase, error) {
	   if e == nil {
		  return feereserve.ReserveCase{}, ErrEngineNil
	  }
	  if strings.TrimSpace(req.CaseID) == "" {
		   return feereserve.ReserveCase{},
			  feereserve.ErrCaseNotFound
	   }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	  rc, err := e.store.GetCase(ctx, req.CaseID)
	   if err != nil {
		   return feereserve.ReserveCase{}, err
	   }
	   now := e.clock()
	   switch rc.State {
	  case feereserve.CaseOpen:
		  next,
			  err := e.caseFSM.apply(rc.State, feereserve.CaseEvProve)
		   if err != nil {
			   return feereserve.ReserveCase{}, feereserve.ErrState
		   }
		  rc.State = next
		  rc.Resolution = req.Resolution
		  rc.ProvenAt = &now
		   rc.UpdatedAt = now
	   case feereserve.CaseProven:
		   next, err := e.caseFSM.apply(rc.State, feereserve.CaseEvClose)
		   if err != nil {
			  return feereserve.ReserveCase{}, feereserve.ErrState
		  }
		   rc.State = next
		   if req.Resolution != "" {
			   rc.Resolution = req.Resolution
		  }
		  rc.ClosedAt = &now
		  rc.UpdatedAt = now
	   case feereserve.CaseClosed:
		  return feereserve.ReserveCase{},
			  feereserve.ErrCaseClosed
	  }
	  return e.store.SaveCase(ctx, rc)
  }

// ──────────────────────────────────────────
// ProofExport 导出差额证明
// ──────────────────────────────────────────

  func (e *Engine) ProofExport(ctx context.Context,
	   req feereserve.ProofExportReq) ([]feereserve.DeltaProof, error) {
	   if e == nil {
		   return nil, ErrEngineNil
	   }
	  limit := req.Limit
	   if limit <= 0 {
		   limit = 100
	   }
	  if req.WithdrawID != "" {
		   return e.store.ListProofsByWithdraw(ctx, req.WithdrawID, limit)
	   }
	   if req.LeaseID != "" {
		  return e.store.ListProofsByLease(ctx, req.LeaseID, limit)
	   }
	   return nil, ErrProofExport
   }

// ──────────────────────────────────────────
// CaseList 按状态查询案例
// ──────────────────────────────────────────

  func (e *Engine) CaseList(ctx context.Context, state feereserve.CaseState, limit int) ([]feereserve.ReserveCase, error) {
	   if e == nil {
		   return nil, ErrEngineNil
	   }
	   return e.store.ListCasesByState(ctx, state, limit)
  }

  func (e *Engine) CaseListByWithdraw(ctx context.Context, withdrawID string, limit int) ([]feereserve.ReserveCase, error) {
	  if e == nil {
		   return nil, ErrEngineNil
	  }
	  return e.store.ListCasesByWithdraw(ctx, withdrawID, limit)
  }

// ──────────────────────────────────────────
// 内部辅助
// ──────────────────────────────────────────

   func (e *Engine) validateGenToken(lease feereserve.ReserveLease, gen int64, token string) error {
	  if lease.Generation != gen {
		  return feereserve.ErrLeaseGeneration
	  }
	   if lease.Token != token {
		  return feereserve.ErrLeaseGeneration
	  }
	  if lease.Expired(e.clock()) {
		  return feereserve.ErrLeaseExpired
	   }
	   return nil
  }

   func (e *Engine) handleTerminalConflict(
	  ctx context.Context,
	  lease feereserve.ReserveLease,
	  existing feereserve.TerminalBind,
	   incomingLeg feereserve.TerminalLeg,
	   incomingVoucher string,
   ) (feereserve.ReserveLease, error) {
	  now := e.clock()
	   rc := feereserve.ReserveCase{
		  ID:         uuid.NewString(),
		   WithdrawID: lease.WithdrawID,
		  LeaseID: lease.ID,
		  Kind:  feereserve.CaseDualVoucher,
		  State: feereserve.CaseOpen,
		  Detail: "terminal_conflict: bound=" + string(existing.Leg) +
			  " voucher=" + existing.VoucherID +
			   " incoming=" + string(incomingLeg) +
			  " incoming_voucher=" + incomingVoucher,
		  CreatedAt: now,
		   UpdatedAt: now,
	   }
	  _, _ = e.store.SaveCase(ctx, rc)
	  return feereserve.ReserveLease{}, feereserve.ErrTerminalConflict
   }

  func (e *Engine) handleOperateAfterTerminal(
	  ctx context.Context,
	  lease feereserve.ReserveLease,
	  incomingLeg feereserve.TerminalLeg,
	  incomingVoucher string,
  ) (feereserve.ReserveLease, error) {
	   if lease.TerminalLeg == string(incomingLeg) && lease.VoucherID == incomingVoucher {
		  return lease, nil
	  }
	  now := e.clock()
	  kind := feereserve.CaseAfterTerminal
	  if incomingVoucher != "" && lease.VoucherID != "" && incomingVoucher != lease.VoucherID {
		   kind = feereserve.CaseDualVoucher
	  }
	  rc := feereserve.ReserveCase{
		  ID: uuid.NewString(),
		  WithdrawID: lease.WithdrawID,
		  LeaseID: lease.ID,
		  Kind:  kind,
		   State: feereserve.CaseOpen,
		  Detail: "operate_after_terminal: current_state=" + string(lease.State) +
			  " current_leg=" + lease.TerminalLeg +
			  " incoming_leg=" + string(incomingLeg) +
			   " incoming_voucher=" + incomingVoucher,
		  CreatedAt: now,
		   UpdatedAt: now,
	   }
	   _, _ = e.store.SaveCase(ctx, rc)
	   if kind == feereserve.CaseDualVoucher {
		   return feereserve.ReserveLease{}, feereserve.ErrDualVoucher
	   }
	  return feereserve.ReserveLease{}, feereserve.ErrOperateAfterTerm
   }

  func (e *Engine) detectDelta(ctx context.Context, lease feereserve.ReserveLease) error {
	  reserved, err := money.Parse(lease.ReservedFee)
	  if err != nil {
		  return err
	   }
	   actual, err := money.Parse(lease.ActualFee)
	   if err != nil {
		  return err
	  }
	  cmp := actual.Cmp(reserved)
	  if cmp == 0 {
		   return nil
	  }
	  var deltaAmt money.Amount
	  var deltaSign string
	   var caseKind feereserve.CaseKind
	  if cmp < 0 {
		   deltaAmt, err = reserved.Sub(actual)
		  if err != nil {
			   return err
		  }
		  deltaSign = "undercharge"
		  caseKind = feereserve.CaseUndercharge
	  } else {
		  deltaAmt, err = actual.Sub(reserved)
		   if err != nil {
			  return err
		   }
		   deltaSign = "over_refund"
		  caseKind = feereserve.CaseOverRefund
	   }
	   now := e.clock()
	  proof := feereserve.DeltaProof{
		   ID:         uuid.NewString(),
		  WithdrawID: lease.WithdrawID,
		   LeaseID: lease.ID,
		   ReservedFee: lease.ReservedFee,
		  ActualFee: lease.ActualFee,
		   DeltaAmt: deltaAmt.String(),
		  DeltaSign: deltaSign,
		  CreatedAt: now,
	  }
	   saved, err := e.store.SaveProof(ctx, proof)
	  if err != nil {
		   return err
	   }
	  rc := feereserve.ReserveCase{
		   ID: uuid.NewString(),
		  WithdrawID: lease.WithdrawID,
		  LeaseID:  lease.ID,
		   Kind:     caseKind,
		   State:    feereserve.CaseOpen,
		  Detail:   "delta=" + deltaAmt.String() + " sign=" + deltaSign,
		  ProofIDs: []string{saved.ID},
		  CreatedAt: now,
		  UpdatedAt: now,
	  }
	  _, _ = e.store.SaveCase(ctx, rc)
	   return nil
  }

// ──────────────────────────────────────────
// SealProof 封口差额证明——封口后阻止该笔出金再写
// ──────────────────────────────────────────

   func (e *Engine) SealProof(ctx context.Context, proofID string) (feereserve.DeltaProof, error) {
	   if e == nil {
		   return feereserve.DeltaProof{},
			   ErrEngineNil
	  }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	  p, err := e.store.GetProof(ctx, proofID)
	   if err != nil {
		   return feereserve.DeltaProof{}, err
	  }
	   if p.Sealed {
		   return p, nil
	   }
	   now := e.clock()
	  p.Sealed = true
	   p.SealedAt = &now
	   saved, err := e.store.SaveProof(ctx, p)
	   if err != nil {
		   return feereserve.DeltaProof{}, err
	   }
	  _, _ = e.blockLocked(ctx, feereserve.BlockReq{
		   WithdrawID: p.WithdrawID,
		  LeaseID: p.LeaseID,
		  Reason:  "proof_sealed:" + proofID,
	  })
	  return saved, nil
   }
