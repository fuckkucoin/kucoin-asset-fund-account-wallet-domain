// @FileName: engine.go
// @Summary: coolwingate 模块 engine。
// 安全变更冷却标记与出金预检窗口竞态收口 — 核心引擎。
// 所有写操作均通过 Engine 方法进入，内部串行化关键路径。
// cool_gen 代次保证 preflight 与 broadcast 绑定同一视图。
// Mark 发生在 preflight pass 与 broadcast 之间会撤回令牌。
// 冷却水位线以 ledger WithdrawLockUntil 为权威源。
// @Author: Jett
// @Created: 2021.03.25

package coolwingate

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/seclock"
	"account-assets.kucoin.net/funding/internal/validate"
)

const (
	DefaultLeaseTTL = 24 * time.Hour
	DefaultBindTTL = 10 * time.Minute
)

// LedgerQuerier 查询账本冷却水位线接口。

type LedgerQuerier interface {
	WithdrawLockUntil(ctx context.Context,
		userID string) (time.Time, string, error)
}

// SecurityMarker 安全变更标记接口。

type SecurityMarker interface {
	Mark(ctx context.Context,
		userID, reason string) error
}

// Engine 竞态收口引擎。

type Engine struct {
	mu      sync.Mutex
	store   Store
	ledger  LedgerQuerier
	secmark SecurityMarker
	clock   func() time.Time
	leaseTTL time.Duration
	bindTTL  time.Duration
	leaseFSM *leaseFSM
	bindFSM *bindFSM
	caseFSM *caseFSM
}

type Config struct {
	Store  Store
	Ledger LedgerQuerier
	SecMark  SecurityMarker
	Clock    func() time.Time
	LeaseTTL time.Duration
	BindTTL  time.Duration
}

  func New(cfg Config) *Engine {
	   if cfg.Store == nil {
		  cfg.Store = NewMemStore()
	  }
	   if cfg.Clock == nil {
		  cfg.Clock = func() time.Time { return time.Now().UTC() }
	   }
	  if cfg.LeaseTTL <= 0 {
		   cfg.LeaseTTL = DefaultLeaseTTL
	  }
	  if cfg.BindTTL <= 0 {
		   cfg.BindTTL = DefaultBindTTL
	   }
	  return &Engine{
		   store: cfg.Store,
		  ledger: cfg.Ledger,
		  secmark: cfg.SecMark,
		   clock:    cfg.Clock,
		   leaseTTL: cfg.LeaseTTL,
		  bindTTL:  cfg.BindTTL,
		  leaseFSM: newLeaseFSM(),
		  bindFSM:  newBindFSM(),
		  caseFSM: newCaseFSM(),
	  }
  }

// ──────────────────────────────────────────
// MarkCoolDown 安全变更写入冷却标记并 bump cool_gen
// ──────────────────────────────────────────

  func (e *Engine) MarkCoolDown(ctx context.Context, req MarkReq) (CoolLease, error) {
	  if e == nil {
		  return CoolLease{}, ErrEngineNil
	  }
	   uid, err := validate.UserID(req.UserID)
	  if err != nil {
		  return CoolLease{}, ErrUserRequired
	  }
	  reason := strings.TrimSpace(req.Reason)
	   if reason == "" {
		   return CoolLease{}, ErrReasonRequired
	  }
	  reason = seclock.NormalizeReason(reason)
	  e.mu.Lock()
	  defer e.mu.Unlock()
	   now := e.clock()
	  if e.secmark != nil {
		  if err := e.secmark.Mark(ctx, uid, reason); err != nil {
			   return CoolLease{}, fmt.Errorf("secmark: %w", err)
		   }
	   }
	  prevGen, _ := e.store.LatestGenForUser(ctx, uid)
	  newGen := prevGen + 1
	   lease := CoolLease{
		   ID:     uuid.NewString(),
		  UserID: uid,
		  Reason:      reason,
		   CoolGen:     newGen,
		   State:       LeaseOpen,
		  LeaseExpiry: now.Add(e.leaseTTL),
		   CreatedAt: now,
		   UpdatedAt: now,
	   }
	   saved, err := e.store.SaveLease(ctx, lease)
	  if err != nil {
		   return CoolLease{}, err
	  }
	  openBinds, _ := e.store.ListOpenBindsByUser(ctx, uid, 200)
	   for _, b := range openBinds {
		  if b.State != BindOpen {
			   continue
		  }
		  revokedBind := b
		   revokedBind.State = BindRevoked
		  revokedBind.RevokeReason = "mark_after_preflight:" + reason
		   rNow := now
		   revokedBind.RevokedAt = &rNow
		   revokedBind.UpdatedAt = now
		   _, _ = e.store.SaveBind(ctx, revokedBind)
		  wc := CoolWindowCase{
			   ID:         uuid.NewString(),
			   UserID:     uid,
			   WithdrawID: b.WithdrawID,
			  BindID:     b.ID,
			   LeaseID:    saved.ID,
			   Kind: CaseMarkAfterPreflight,
			  State: CaseOpen,
			  Detail: fmt.Sprintf("mark_reason=%s preflight_gen=%d mark_gen=%d", reason, b.CoolGen, newGen),
			  CreatedAt: now,
			   UpdatedAt: now,
		   }
		   _, _ = e.store.SaveCase(ctx, wc)
		   proof := WindowProof{
			  ID: uuid.NewString(),
			   CaseID: wc.ID,
			  UserID: uid,
			   WithdrawID: b.WithdrawID,
			  LeaseID:    saved.ID,
			   BindID:     b.ID,
			   MarkReason: reason,
			   PreflightGen:  b.CoolGen,
			   MarkGen:       newGen,
			   PreflightTime: b.CheckedAt,
			  MarkTime:      now,
			  CreatedAt:     now,
		   }
		   if e.ledger != nil {
			   lockUntil, _, _ := e.ledger.WithdrawLockUntil(ctx, uid)
			   proof.LedgerLockUntil = lockUntil
		  }
		  _, _ = e.store.SaveProof(ctx, proof)
	   }
	  return saved, nil
  }

// ──────────────────────────────────────────
// PreflightBind 出金预检绑定代次
// ──────────────────────────────────────────

  func (e *Engine) BindPreflight(ctx context.Context, req PreflightBindReq) (PreflightBind, error) {
	   if e == nil {
		  return PreflightBind{}, ErrEngineNil
	  }
	  uid, err := validate.UserID(req.UserID)
	  if err != nil {
		   return PreflightBind{},
			  ErrUserRequired
	  }
	   wid := strings.TrimSpace(req.WithdrawID)
	   if wid == "" {
		  return PreflightBind{}, ErrWithdrawRequired
	   }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	   now := e.clock()
	   if e.ledger != nil {
		   lockUntil, reason, err := e.ledger.WithdrawLockUntil(ctx, uid)
		  if err != nil {
			  return PreflightBind{}, fmt.Errorf("ledger query: %w", err)
		  }
		   if lockUntil.After(now) {
			  remain := lockUntil.Sub(now)
			   return PreflightBind{}, fmt.Errorf("%w: reason=%s remain=%s", ErrLockWindowActive, reason, remain)
		  }
	   }
	   if existing, err := e.store.GetBindByWithdraw(ctx, wid); err == nil {
		  if existing.State == BindOpen {
			   return PreflightBind{}, fmt.Errorf("%w: withdraw_id=%s", ErrDuplicateBind, wid)
		  }
	  }
	   currentGen,
		  _ := e.store.LatestGenForUser(ctx, uid)
	   var leaseID string
	  if lease, err := e.store.GetLeaseByUser(ctx, uid); err == nil {
		  leaseID = lease.ID
		  if lease.State == LeaseOpen && lease.LeaseExpiry.Before(now) {
			  expLease := lease
			   expLease.State = LeaseExpired
			   expNow := now
			  expLease.ExpiredAt = &expNow
			  expLease.UpdatedAt = now
			  _,
				  _ = e.store.SaveLease(ctx, expLease)
		   }
	  }
	  token := uuid.NewString()
	   bind := PreflightBind{
		   ID: uuid.NewString(),
		   UserID:     uid,
		   WithdrawID: wid,
		  CoolGen:    currentGen,
		  LeaseID: leaseID,
		   State:     BindOpen,
		  Token:     token,
		  CheckedAt: now,
		   BindExpiry: now.Add(e.bindTTL),
		  CreatedAt:  now,
		   UpdatedAt: now,
	  }
	  saved, err := e.store.SaveBind(ctx, bind)
	   if err != nil {
		  return PreflightBind{}, err
	   }
	  return saved, nil
  }

// ──────────────────────────────────────────
// CheckBroadcast 广播前校验绑定令牌
// ──────────────────────────────────────────

   func (e *Engine) CheckBroadcast(ctx context.Context, req BroadcastCheckReq) error {
	  if e == nil {
		  return ErrEngineNil
	   }
	   uid,
		  err := validate.UserID(req.UserID)
	   if err != nil {
		  return ErrUserRequired
	  }
	   wid := strings.TrimSpace(req.WithdrawID)
	   if wid == "" {
		   return ErrWithdrawRequired
	  }
	  token := strings.TrimSpace(req.BindToken)
	   if token == "" {
		   return ErrBindTokenRequired
	  }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	   now := e.clock()
	  bind, err := e.store.GetBindByWithdraw(ctx, wid)
	  if err != nil {
		  return fmt.Errorf("%w: withdraw_id=%s", ErrBindNotFound, wid)
	   }
	  if bind.UserID != uid {
		  return ErrUserRequired
	   }
	   if bind.Token != token {
		   return ErrBindTokenRequired
	   }
	   switch bind.State {
	   case BindRevoked:
		  return fmt.Errorf("%w: reason=%s", ErrBroadcastBlocked, bind.RevokeReason)
	   case BindConsumed:
		   return ErrBindConsumed
	  case BindOpen:
	  default:
		  return ErrBindNotFound
	  }
	  if bind.BindExpiry.Before(now) {
		  expBind := bind
		  expBind.State = BindRevoked
		  expBind.RevokeReason = "bind_expired"
		  rNow := now
		   expBind.RevokedAt = &rNow
		   expBind.UpdatedAt = now
		   _, _ = e.store.SaveBind(ctx, expBind)
		  return ErrBindExpired
	   }
	  currentGen, _ := e.store.LatestGenForUser(ctx, uid)
	   if currentGen != bind.CoolGen {
		  revBind := bind
		  revBind.State = BindRevoked
		   revBind.RevokeReason = fmt.Sprintf("gen_mismatch: bind=%d current=%d", bind.CoolGen, currentGen)
		  rNow := now
		   revBind.RevokedAt = &rNow
		  revBind.UpdatedAt = now
		   _, _ = e.store.SaveBind(ctx, revBind)
		   wc := CoolWindowCase{
			   ID: uuid.NewString(),
			  UserID:     uid,
			  WithdrawID: wid,
			   BindID: bind.ID,
			   LeaseID: bind.LeaseID,
			   Kind:      CaseGenMismatch,
			  State:     CaseOpen,
			  Detail:    fmt.Sprintf("preflight_gen=%d current_gen=%d", bind.CoolGen, currentGen),
			   CreatedAt: now,
			  UpdatedAt: now,
		   }
		  _, _ = e.store.SaveCase(ctx, wc)
		  return fmt.Errorf("%w: bind_gen=%d current_gen=%d", ErrGenMismatch, bind.CoolGen, currentGen)
	   }
	   if e.ledger != nil {
		   lockUntil, reason, err := e.ledger.WithdrawLockUntil(ctx, uid)
		  if err != nil {
			   return fmt.Errorf("ledger query: %w", err)
		  }
		   if lockUntil.After(now) {
			   revBind := bind
			   revBind.State = BindRevoked
			   revBind.RevokeReason = "ledger_lock_active:" + reason
			   rNow := now
			   revBind.RevokedAt = &rNow
			   revBind.UpdatedAt = now
			  _, _ = e.store.SaveBind(ctx, revBind)
			   return fmt.Errorf("%w: reason=%s", ErrCacheBypass, reason)
		   }
	   }
	  consumed := bind
	   consumed.State = BindConsumed
	   cNow := now
	   consumed.ConsumedAt = &cNow
	  consumed.UpdatedAt = now
	  _, _ = e.store.SaveBind(ctx, consumed)
	   return nil
   }

// ──────────────────────────────────────────
// RequireWithdraw 出金锁窗口查询（账本权威）
// ──────────────────────────────────────────

  func (e *Engine) RequireWithdraw(ctx context.Context, req RequireWithdrawReq) (RequireWithdrawResp, error) {
	   if e == nil {
		   return RequireWithdrawResp{}, ErrEngineNil
	  }
	   uid,
		   err := validate.UserID(req.UserID)
	  if err != nil {
		   return RequireWithdrawResp{},
			  ErrUserRequired
	  }
	  now := e.clock()
	  if e.ledger == nil {
		  return RequireWithdrawResp{Allowed: true}, nil
	  }
	  lockUntil, reason, err := e.ledger.WithdrawLockUntil(ctx, uid)
	  if err != nil {
		  return RequireWithdrawResp{}, fmt.Errorf("ledger query: %w", err)
	   }
	  if lockUntil.After(now) {
		  remain := lockUntil.Sub(now)
		   return RequireWithdrawResp{
			   Allowed: false,
			  Reason: reason,
			  LockUntil: lockUntil,
			  Remain: remain.String(),
		   }, nil
	   }
	   return RequireWithdrawResp{Allowed: true}, nil
  }

// ──────────────────────────────────────────
// CoolGenQuery 代次查询
// ──────────────────────────────────────────

   func (e *Engine) CoolGenQuery(ctx context.Context, req CoolGenQueryReq) (CoolGenResp, error) {
	  if e == nil {
		   return CoolGenResp{},
			  ErrEngineNil
	  }
	   uid, err := validate.UserID(req.UserID)
	  if err != nil {
		   return CoolGenResp{}, ErrUserRequired
	   }
	  gen, _ := e.store.LatestGenForUser(ctx, uid)
	   resp := CoolGenResp{
		  UserID: uid,
		   CurrentGen: gen,
	  }
	   if lease, err := e.store.GetLeaseByUser(ctx, uid); err == nil {
		   resp.ActiveLeaseID = lease.ID
		  resp.Reason = lease.Reason
	  }
	   if e.ledger != nil {
		   lockUntil, _, _ := e.ledger.WithdrawLockUntil(ctx, uid)
		   resp.LockUntil = lockUntil
	  }
	   return resp, nil
  }

// ──────────────────────────────────────────
// CaseConfirm 窗口案例确认
// ──────────────────────────────────────────

  func (e *Engine) CaseConfirm(ctx context.Context, req CaseConfirmReq) (CoolWindowCase, error) {
	   if e == nil {
		   return CoolWindowCase{}, ErrEngineNil
	   }
	  caseID := strings.TrimSpace(req.CaseID)
	   if caseID == "" {
		  return CoolWindowCase{}, ErrCaseNotFound
	  }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	   now := e.clock()
	  wc, err := e.store.GetCase(ctx, caseID)
	  if err != nil {
		   return CoolWindowCase{}, err
	  }
	  if wc.State == CaseClosed {
		   return CoolWindowCase{}, ErrCaseAlreadyClosed
	  }
	   if len(req.ProofIDs) == 0 && wc.State == CaseOpen {
		   return CoolWindowCase{}, ErrCaseNotProven
	  }
	   if wc.State == CaseOpen && len(req.ProofIDs) > 0 {
		  for _, pid := range req.ProofIDs {
			   if _, err := e.store.GetProof(ctx, pid); err != nil {
				   return CoolWindowCase{}, fmt.Errorf("%w: %s", ErrProofNotFound, pid)
			  }
		  }
		  nextState, err := e.caseFSM.Transition(wc.State, CaseEvProve)
		   if err != nil {
			   return CoolWindowCase{}, err
		  }
		  wc.State = nextState
		  pNow := now
		   wc.ProvenAt = &pNow
		   wc.ProofIDs = req.ProofIDs
		   wc.UpdatedAt = now
		   wc, _ = e.store.SaveCase(ctx, wc)
	   }
	  if wc.State == CaseProven {
		  nextState, err := e.caseFSM.Transition(wc.State, CaseEvClose)
		  if err != nil {
			  return CoolWindowCase{}, err
		  }
		  wc.State = nextState
		  cNow := now
		  wc.ClosedAt = &cNow
		  wc.Resolution = req.Resolution
		   if wc.Resolution == "" {
			   wc.Resolution = "confirmed"
		   }
		   wc.UpdatedAt = now
		   wc, _ = e.store.SaveCase(ctx, wc)
	  }
	   return wc, nil
  }

// ──────────────────────────────────────────
// CaseList / CaseListByWithdraw / CaseListByUser
// ──────────────────────────────────────────

  func (e *Engine) CaseList(ctx context.Context, state CaseState, limit int) ([]CoolWindowCase, error) {
	   if e == nil {
		   return nil, ErrEngineNil
	  }
	   if limit <= 0 {
		  limit = 50
	   }
	   return e.store.ListCasesByState(ctx,
		   state, limit)
   }

  func (e *Engine) CaseListByWithdraw(ctx context.Context, withdrawID string, limit int) ([]CoolWindowCase, error) {
	   if e == nil {
		  return nil, ErrEngineNil
	  }
	  if limit <= 0 {
		   limit = 50
	   }
	   return e.store.ListCasesByWithdraw(ctx, withdrawID, limit)
  }

  func (e *Engine) CaseListByUser(ctx context.Context, userID string, limit int) ([]CoolWindowCase, error) {
	  if e == nil {
		   return nil, ErrEngineNil
	   }
	  if limit <= 0 {
		  limit = 50
	   }
	  return e.store.ListCasesByUser(ctx, userID, limit)
   }

// ──────────────────────────────────────────
// ProofExport 证明导出
// ──────────────────────────────────────────

   func (e *Engine) ProofExport(ctx context.Context, req ProofExportReq) ([]WindowProof, error) {
	   if e == nil {
		  return nil, ErrEngineNil
	   }
	  limit := req.Limit
	   if limit <= 0 {
		  limit = 100
	   }
	  if req.CaseID != "" {
		  return e.store.ListProofsByCase(ctx, req.CaseID, limit)
	   }
	   if req.UserID != "" {
		   return e.store.ListProofsByUser(ctx, req.UserID, limit)
	   }
	   return nil, fmt.Errorf("coolwingate: user_id or case_id required for proof export")
   }

// ──────────────────────────────────────────
// SealProof 封口证明
// ──────────────────────────────────────────

   func (e *Engine) SealProof(ctx context.Context, proofID string) (WindowProof, error) {
	   if e == nil {
		  return WindowProof{}, ErrEngineNil
	  }
	  e.mu.Lock()
	  defer e.mu.Unlock()
	   p, err := e.store.GetProof(ctx, proofID)
	  if err != nil {
		   return WindowProof{}, err
	   }
	   if p.Sealed {
		   return p, nil
	   }
	   now := e.clock()
	   p.Sealed = true
	  p.SealedAt = &now
	   return e.store.SaveProof(ctx, p)
   }

// ──────────────────────────────────────────
// GetLease / GetBind
// ──────────────────────────────────────────

  func (e *Engine) GetLease(ctx context.Context, id string) (CoolLease, error) {
	  if e == nil {
		   return CoolLease{}, ErrEngineNil
	  }
	   return e.store.GetLease(ctx, id)
  }

  func (e *Engine) GetLeaseByUser(ctx context.Context, userID string) (CoolLease, error) {
	   if e == nil {
		   return CoolLease{}, ErrEngineNil
	   }
	   return e.store.GetLeaseByUser(ctx, userID)
  }

  func (e *Engine) GetBind(ctx context.Context, id string) (PreflightBind, error) {
	   if e == nil {
		  return PreflightBind{}, ErrEngineNil
	   }
	   return e.store.GetBind(ctx, id)
   }

  func (e *Engine) GetBindByWithdraw(ctx context.Context, withdrawID string) (PreflightBind, error) {
	   if e == nil {
		   return PreflightBind{}, ErrEngineNil
	   }
	  return e.store.GetBindByWithdraw(ctx, withdrawID)
  }

// ──────────────────────────────────────────
// RevokeBind 手动撤回预检绑定
// ──────────────────────────────────────────

   func (e *Engine) RevokeBind(ctx context.Context, bindID, reason string) (PreflightBind, error) {
	  if e == nil {
		  return PreflightBind{}, ErrEngineNil
	  }
	  e.mu.Lock()
	  defer e.mu.Unlock()
	  bind, err := e.store.GetBind(ctx, bindID)
	   if err != nil {
		  return PreflightBind{}, err
	  }
	  if bind.State != BindOpen {
		   return bind, nil
	   }
	   now := e.clock()
	   bind.State = BindRevoked
	  bind.RevokeReason = reason
	   bind.RevokedAt = &now
	  bind.UpdatedAt = now
	  return e.store.SaveBind(ctx, bind)
   }

// ──────────────────────────────────────────
// ExpireLease 手动过期租约
// ──────────────────────────────────────────

  func (e *Engine) ExpireLease(ctx context.Context, leaseID string) (CoolLease, error) {
	   if e == nil {
		   return CoolLease{}, ErrEngineNil
	  }
	   e.mu.Lock()
	   defer e.mu.Unlock()
	  lease, err := e.store.GetLease(ctx, leaseID)
	  if err != nil {
		  return CoolLease{}, err
	   }
	   nextState, err := e.leaseFSM.Transition(lease.State, LeaseEvExpire)
	  if err != nil {
		  return lease, nil
	  }
	   now := e.clock()
	  lease.State = nextState
	  lease.ExpiredAt = &now
	   lease.UpdatedAt = now
	  return e.store.SaveLease(ctx, lease)
   }

// ──────────────────────────────────────────
// BindLease 将租约绑定到出金单
// ──────────────────────────────────────────

  func (e *Engine) BindLease(ctx context.Context, leaseID, withdrawID string) (CoolLease, error) {
	  if e == nil {
		  return CoolLease{}, ErrEngineNil
	   }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	  lease, err := e.store.GetLease(ctx, leaseID)
	   if err != nil {
		  return CoolLease{}, err
	  }
	   now := e.clock()
	  if lease.LeaseExpiry.Before(now) {
		  lease.State = LeaseExpired
		  lease.ExpiredAt = &now
		  lease.UpdatedAt = now
		  _,
			   _ = e.store.SaveLease(ctx, lease)
		  return lease, ErrLeaseExpired
	  }
	  nextState, err := e.leaseFSM.Transition(lease.State, LeaseEvBind)
	   if err != nil {
		   return lease, err
	   }
	   lease.State = nextState
	  lease.BoundTo = withdrawID
	  bNow := now
	  lease.BoundAt = &bNow
	   lease.UpdatedAt = now
	   return e.store.SaveLease(ctx, lease)
  }
