package liqgengate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/account"
)

// Config 引擎配置。
type Config struct {
	LeaseTTL time.Duration
	// AllowReleaseWithoutGen 必须 false。
	AllowReleaseWithoutGen bool
	// AllowCloseWithoutGen 必须 false。
	AllowCloseWithoutGen bool
	RevokeBatchSize int
	ProofBatchSize  int
	HalfStaleAfter  time.Duration
}

   func (c Config) withDefaults() Config {
	   if c.LeaseTTL <= 0 {
		  c.LeaseTTL = 2 * time.Minute
	   }
	  if c.RevokeBatchSize <= 0 {
		   c.RevokeBatchSize = 200
	   }
	  if c.ProofBatchSize <= 0 {
		   c.ProofBatchSize = 200
	   }
	   if c.HalfStaleAfter <= 0 {
		  c.HalfStaleAfter = 90 * time.Second
	   }
	   return c
   }

// Engine 强平世代引擎。

type Engine struct {
	store Store
	view  MarginView
	freeze FreezeHook
	cfg    Config
	clock func() time.Time
}

// NewEngine 初始化。view / freeze 可为空（单测用 NopView）。
  func NewEngine(store Store, view MarginView, freeze FreezeHook, cfg Config) *Engine {
	  if view == nil {
		   view = NopView{}
	  }
	   return &Engine{
		   store: store, view: view, freeze: freeze,
		   cfg: cfg.withDefaults(), clock: time.Now,
	  }
  }

// WithClock 时钟。

   func (e *Engine) WithClock(fn func() time.Time) *Engine {
	   e.clock = fn
	   return e
  }

  // WithFreeze 绑定 freeze hook。

   func (e *Engine) WithFreeze(fn FreezeHook) *Engine {
	  e.freeze = fn
	  return e
  }

// WithView 绑定 margin 视图。

   func (e *Engine) WithView(v MarginView) *Engine {
	  if v != nil {
		  e.view = v
	   }
	  return e
   }

  func (e *Engine) now() time.Time {
	   if e.clock != nil {
		  return e.clock().UTC()
	   }
	   return time.Now().UTC()
  }

   func (e *Engine) audit(ctx context.Context, borrowID, leaseID, fromP, toP, fromL, toL string, gen uint64, actor, note string) {
	  _ = e.store.InsertAudit(ctx, &LiqAudit{
		  ID: uuid.NewString(), BorrowID: borrowID, LeaseID: leaseID,
		   FromPhase: fromP, ToPhase: toP, FromLease: fromL, ToLease: toL,
		  Gen: gen, Actor: actor, Note: note, CreatedAt: e.now(),
	  })
  }

  func (e *Engine) ensureGen(ctx context.Context, b LiveBorrow, col LiveHold) (*LiqGeneration, error) {
	  g, err := e.store.GetGeneration(ctx, b.ID)
	   if err == nil {
		   return g, nil
	  }
	  if !errors.Is(err, ErrNotFound) {
		   return nil, err
	   }
	   now := e.now()
	  g = &LiqGeneration{
		  ID: uuid.NewString(), BorrowID: b.ID, UserID: b.UserID, CollateralID: b.Collateral,
		  Gen: 0, LTVBPS: b.LTVBPS, BorrowAmt: b.Amount, CollateralAmt: col.Amount,
		   CapBPS: col.CapBPS, Asset: b.Asset, Phase: PhaseOpen, CreatedAt: now, UpdatedAt: now,
	   }
	   if err := e.store.InsertGeneration(ctx, g); err != nil {
		   if errors.Is(err, ErrDuplicate) {
			   return e.store.GetGeneration(ctx, b.ID)
		  }
		   return nil, err
	  }
	  e.audit(ctx, b.ID, "", "", string(PhaseOpen), "", "", 0, "engine", "seed_gen")
	  return g, nil
  }

// ─── FreezeWithGen ────────────────────────────────────────────────────────
// FreezeRequest 冻结请求。

type FreezeRequest struct {
	UserID   string
	BorrowID string
	IdemKey  string
}

// FreezeResult 冻结结果。

type FreezeResult struct {
	Allowed bool
	Generation   *LiqGeneration
	Lease        *LiqFreezeLease
	HoldID       string
	HalfProof    *LiqProof
	RejectReason string
}

// FreezeWithGen 推进 liq_gen 并签发冻结租约，再调用 freeze hook。
// 不变量：
// - 先占互斥锁，再推进世代，再记 Hold，最后调 hook。
// - hook 失败不得回滚世代；相位落 half_open 并落 LiqHalfProof。
// - 不得把借款标为已完全释放。
  func (e *Engine) FreezeWithGen(ctx context.Context, req FreezeRequest) (*FreezeResult, error) {
	  if e == nil || e.store == nil {
		   return nil, ErrUnavailable
	  }
	   uid := trim(req.UserID)
	  bid := trim(req.BorrowID)
	   if uid == "" || bid == "" {
		  return nil, fmt.Errorf("liqgengate: user_id and borrow_id required")
	   }
	   b, ok := e.view.LookupBorrow(bid)
	  if !ok || b.UserID != uid {
		  return nil, ErrNotFound
	   }
	  col, ok := e.view.LookupHold(b.Collateral)
	  if !ok {
		  return nil, ErrNotFound
	  }
	   g, err := e.ensureGen(ctx, b, col)
	   if err != nil {
		  return nil, err
	  }
	  if req.IdemKey != "" {
		  if existing, err := e.store.GetLeaseByIdem(ctx, req.IdemKey); err == nil {
			  gg, _ := e.store.GetGeneration(ctx, bid)
			  return &FreezeResult{Allowed: existing.State == LeaseFrozen, Generation: gg, Lease: existing, HoldID: existing.HoldID}, nil
		   } else if !errors.Is(err, ErrNotFound) {
			  return nil, err
		  }
	   }
	  if g.Phase == PhaseClosed {
		   return nil, ErrClosed
	  }
	   if g.Phase == PhaseHalfOpen {
		   return e.retryHalfFreeze(ctx, req, g, b, col)
	   }
	   if !g.Phase.Freezable() && g.Phase != PhaseOpen {
		  p, _ := e.recordRace(ctx, bid, uid, g.LeaseID, req.IdemKey, g.Gen, g.Gen, RaceUnexpectedPhase, g.Phase, "freeze")
		   return &FreezeResult{Allowed: false,
			  Generation: g, HalfProof: nil, RejectReason: string(RaceUnexpectedPhase)}, wrapRace(ErrUnexpectedPhase, p)
	   }
	  owner := "freeze:" + uuid.NewString()
	  if err := e.store.TryAcquireMutex(ctx, &MutexRecord{
		  BorrowID: bid, Owner: owner, Kind: "freeze", Gen: g.Gen + 1, AcquiredAt: e.now(),
	  }); err != nil {
		  p, _ := e.recordRace(ctx, bid, uid, g.LeaseID, req.IdemKey, g.Gen, g.Gen, RaceMutexHeld, g.Phase, "freeze")
		  return &FreezeResult{Allowed: false, Generation: g, RejectReason: string(RaceMutexHeld)}, wrapRace(ErrMutexHeld, p)
	  }
	  defer func() {
		   IssuedAt: now, ExpiresAt: now.Add(e.cfg.LeaseTTL), CreatedAt: now, UpdatedAt: now,
	  }
	  if err := e.store.InsertLease(ctx, lease); err != nil {
		   return nil, fmt.Errorf("liqgengate: insert lease: %w", err)
	   }
	   g.LeaseID = lease.ID
	   holdID := uuid.NewString()
	   if err := e.view.RecordLiqHold(LiveHold{
		  ID: holdID, UserID: uid, Kind: "liq_freeze", Asset: col.Asset, Amount: col.Amount,
		   CapBPS: col.CapBPS, Status: "frozen", BorrowID: bid,
	   }); err != nil {
		  return nil, err
	   }
	  lease.HoldID = holdID
	   lease.State = LeaseFreezing
	  lease.UpdatedAt = now
	   g.HoldID = holdID
	   if err := e.store.UpdateLease(ctx, lease); err != nil {
		   return nil, err
	  }
	   if err := e.store.UpdateGeneration(ctx, g); err != nil {
		   return nil, err
	   }
	  _ = e.view.ApplyBorrowPhase(bid, PhaseFreezing, next, "frozen")
	  e.audit(ctx, bid, lease.ID, string(fromPhase), string(PhaseFreezing), string(LeaseIssued), string(LeaseFreezing), next, "engine", "freeze_begin")
	  incFreezeIssued()
	   if e.freeze != nil {
		   if err := e.freeze(uid, account.BookMargin, true); err != nil {
			   return e.failHalf(ctx,
				  g, lease, holdID, err)
		  }
	   }
	   now = e.now()
	   if !CanTransitPhase(g.Phase, PhaseFrozen) {
		   return nil, ErrInvalidTransit
	  }
	  g.Phase = PhaseFrozen
	  g.UpdatedAt = now
	  if !CanTransitLease(lease.State, LeaseFrozen) {
		  return nil, ErrInvalidTransit
	  }
	   lease.State = LeaseFrozen
	   t := now
	  lease.FrozenAt = &t
	   lease.UpdatedAt = now
	  if err := e.store.UpdateLease(ctx, lease); err != nil {
		  return nil, err
	   }
	   if err := e.store.UpdateGeneration(ctx, g); err != nil {
		   return nil, err
	  }
	   _ = e.view.ApplyBorrowPhase(bid, PhaseFrozen, next, "frozen")
	  e.audit(ctx, bid, lease.ID, string(PhaseFreezing), string(PhaseFrozen), string(LeaseFreezing), string(LeaseFrozen), next, "engine", "freeze_ok")
	  incFreezeFrozen()
	   return &FreezeResult{Allowed: true, Generation: g, Lease: lease, HoldID: holdID}, nil
   }

   func (e *Engine) failHalf(ctx context.Context, g *LiqGeneration, lease *LiqFreezeLease, holdID string, hookErr error) (*FreezeResult, error) {
	  now := e.now()
	   g.Phase = PhaseHalfOpen
	  g.UpdatedAt = now
	   lease.State = LeaseHalfOpen
	  lease.UpdatedAt = now
	   p, err := e.recordHalf(ctx, g.BorrowID, g.UserID, lease.ID, holdID, g.Gen, HalfHookFailed, hookErr.Error(), PhaseHalfOpen)
	  if err == nil && p != nil {
		   lease.HalfProofID = p.ID
	   }
	  _ = e.store.UpdateLease(ctx, lease)
	   _ = e.store.UpdateGeneration(ctx, g)
	  _ = e.view.ApplyBorrowPhase(g.BorrowID, PhaseHalfOpen, g.Gen, "frozen")
	  e.audit(ctx, g.BorrowID, lease.ID, string(PhaseFreezing), string(PhaseHalfOpen), string(LeaseFreezing), string(LeaseHalfOpen), g.Gen, "engine", "hook_fail")
	  incHalfProof()
	   return &FreezeResult{
		  Allowed: false, Generation: g, Lease: lease, HoldID: holdID, HalfProof: p,
		   RejectReason: string(HalfHookFailed),
	   }, fmt.Errorf("%w: %v", ErrHookFailed, hookErr)
  }

   func (e *Engine) retryHalfFreeze(ctx context.Context, req FreezeRequest, g *LiqGeneration, b LiveBorrow, col LiveHold) (*FreezeResult, error) {
	   owner := "freeze-retry:" + uuid.NewString()
	  if err := e.store.TryAcquireMutex(ctx, &MutexRecord{
		   BorrowID: b.ID, Owner: owner, Kind: "freeze", Gen: g.Gen, AcquiredAt: e.now(),
	  }); err != nil {
		   p, _ := e.recordRace(ctx, b.ID, b.UserID, g.LeaseID, req.IdemKey, g.Gen, g.Gen, RaceMutexHeld, g.Phase, "freeze")
		   return &FreezeResult{Allowed: false, Generation: g, RejectReason: string(RaceMutexHeld)}, wrapRace(ErrMutexHeld, p)
	   }
	  defer func() {
		   _ = e.store.ReleaseMutex(ctx,
			  b.ID, owner)
	  }()
	  lease,
		   err := e.store.GetLeaseByID(ctx, g.LeaseID)
	  if err != nil {
		   return nil, err
	  }
	  if e.freeze != nil {
		   if herr := e.freeze(req.UserID, account.BookMargin, true); herr != nil {
			   return e.failHalf(ctx, g, lease, g.HoldID, herr)
		   }
	   }
	   now := e.now()
	  g.Phase = PhaseFrozen
	   g.UpdatedAt = now
	   lease.State = LeaseFrozen
	   t := now
	  lease.FrozenAt = &t
	  lease.UpdatedAt = now
	  _ = e.store.UpdateLease(ctx, lease)
	   _ = e.store.UpdateGeneration(ctx, g)
	   _ = e.view.ApplyBorrowPhase(b.ID, PhaseFrozen, g.Gen, "frozen")
	  e.audit(ctx, b.ID, lease.ID, string(PhaseHalfOpen), string(PhaseFrozen), string(LeaseHalfOpen), string(LeaseFrozen), g.Gen, "engine", "half_retry_ok")
	   return &FreezeResult{Allowed: true,
		  Generation: g, Lease: lease, HoldID: g.HoldID}, nil
  }

// ─── ReleaseWithGen ───────────────────────────────────────────────────────
// ReleaseRequest 释放请求。

type ReleaseRequest struct {
	UserID string
	BorrowID string
	HoldID   string
	LiqGen  uint64
	HasGen  bool
	LeaseID string
	IdemKey string
}

// CloseRequest 关借请求。
type CloseRequest struct {
	UserID string
	BorrowID string
	LiqGen  uint64
	HasGen  bool
	IdemKey string
}

// GateResult 释放/关借结果。
type GateResult struct {
	Allowed    bool
	Generation *LiqGeneration
	Lease      *LiqFreezeLease
	RaceProof  *LiqProof
	RejectReason string
}

// ReleaseWithGen 释放必须携带匹配 liq_gen。
  func (e *Engine) ReleaseWithGen(ctx context.Context, req ReleaseRequest) (*GateResult, error) {
	   if e == nil || e.store == nil {
		   return nil, ErrUnavailable
	   }
	  bid := trim(req.BorrowID)
	   if bid == "" {
		   return nil, fmt.Errorf("liqgengate: borrow_id required")
	   }
	  g,
		  err := e.store.GetGeneration(ctx, bid)
	  if err != nil {
		   if errors.Is(err, ErrNotFound) && !e.cfg.AllowReleaseWithoutGen {
			   p, _ := e.recordRace(ctx, bid, req.UserID, "", req.IdemKey, req.LiqGen, 0, RaceMissingGen, PhaseOpen, "release")
			  return &GateResult{Allowed: false, RaceProof: p, RejectReason: string(RaceMissingGen)}, wrapRace(ErrMissingGen, p)
		   }
		  return nil, err
	  }
	   if !req.HasGen && !e.cfg.AllowReleaseWithoutGen {
		  p, _ := e.recordRace(ctx, bid, req.UserID, g.LeaseID, req.IdemKey, 0, g.Gen, RaceMissingGen, g.Phase, "release")
		   return &GateResult{Allowed: false, Generation: g, RaceProof: p, RejectReason: string(RaceMissingGen)}, wrapRace(ErrMissingGen, p)
	  }
	  if req.HasGen && req.LiqGen != g.Gen {
		  p, _ := e.recordRace(ctx, bid, req.UserID, g.LeaseID, req.IdemKey, req.LiqGen, g.Gen, RaceStaleGen, g.Phase, "release")
		  return &GateResult{Allowed: false,
			   Generation: g, RaceProof: p, RejectReason: string(RaceStaleGen)}, wrapRace(ErrStaleGen, p)
	  }
	   owner := "release:" + uuid.NewString()
	  if err := e.store.TryAcquireMutex(ctx, &MutexRecord{
		  BorrowID: bid, Owner: owner, Kind: "release", Gen: g.Gen, AcquiredAt: e.now(),
	   }); err != nil {
		   p, _ := e.recordRace(ctx, bid, req.UserID, g.LeaseID, req.IdemKey, req.LiqGen, g.Gen, RaceMutexHeld, g.Phase, "release")
		   return &GateResult{Allowed: false,
			   Generation: g, RaceProof: p, RejectReason: string(RaceMutexHeld)}, wrapRace(ErrMutexHeld, p)
	   }
	   defer func() { _ = e.store.ReleaseMutex(ctx, bid, owner) }()
	  if g.Phase == PhaseHalfOpen {
		   p, _ := e.recordRace(ctx, bid, req.UserID, g.LeaseID, req.IdemKey, req.LiqGen, g.Gen, RaceHalfBlocked, g.Phase, "release")
		  return &GateResult{Allowed: false, Generation: g, RaceProof: p, RejectReason: string(RaceHalfBlocked)}, wrapRace(ErrHalfOpen, p)
	   }
	   if !g.Phase.Releasable() {
		  reason := RaceUnexpectedPhase
		   var base error = ErrUnexpectedPhase
		  if g.Phase == PhaseClosed || g.Phase == PhaseReleasing {
			   reason = RaceDoubleRelease
			   base = ErrDoubleRelease
		  }
		   p, _ := e.recordRace(ctx, bid, req.UserID, g.LeaseID, req.IdemKey, req.LiqGen, g.Gen, reason, g.Phase, "release")
		   return &GateResult{Allowed: false, Generation: g, RaceProof: p, RejectReason: string(reason)}, wrapRace(base, p)
	   }
	  // 二次读取，避免锁等待期间世代推进。
	  g,
		   err = e.store.GetGeneration(ctx, bid)
	  if err != nil {
		  return nil, err
	   }
	   if req.HasGen && req.LiqGen != g.Gen {
		  p, _ := e.recordRace(ctx, bid, req.UserID, g.LeaseID, req.IdemKey, req.LiqGen, g.Gen, RaceStaleGen, g.Phase, "release")
		   return &GateResult{Allowed: false,
			  Generation: g, RaceProof: p, RejectReason: string(RaceStaleGen)}, wrapRace(ErrStaleGen, p)
	  }
	  lease, err := e.store.GetActiveLease(ctx, bid)
	   if err != nil {
		   p, _ := e.recordRace(ctx, bid, req.UserID, g.LeaseID, req.IdemKey, req.LiqGen, g.Gen, RaceLeaseMismatch, g.Phase, "release")
		  return &GateResult{Allowed: false, Generation: g, RaceProof: p, RejectReason: string(RaceLeaseMismatch)}, wrapRace(ErrLeaseMismatch, p)
	  }
	   if req.LeaseID != "" && req.LeaseID != lease.ID {
		  p, _ := e.recordRace(ctx, bid, req.UserID, lease.ID, req.IdemKey, req.LiqGen, g.Gen, RaceLeaseMismatch, g.Phase, "release")
		   return &GateResult{Allowed: false, Generation: g, Lease: lease, RaceProof: p, RejectReason: string(RaceLeaseMismatch)}, wrapRace(ErrLeaseMismatch, p)
	  }
	  if lease.Expired(e.now()) {
		   p, _ := e.recordRace(ctx, bid, req.UserID, lease.ID, req.IdemKey, req.LiqGen, g.Gen, RaceExpiredLease, g.Phase, "release")
		  return &GateResult{Allowed: false, Generation: g, Lease: lease, RaceProof: p, RejectReason: string(RaceExpiredLease)}, wrapRace(ErrLeaseExpired, p)
	  }
	  now := e.now()
	   fromP, fromL := g.Phase, lease.State
	  if !CanTransitPhase(g.Phase, PhaseReleasing) || !CanTransitLease(lease.State, LeaseReleasing) {
		  return nil, ErrInvalidTransit
	  }
	   g.Phase = PhaseReleasing
	   g.UpdatedAt = now
	  lease.State = LeaseReleasing
	  t := now
	  lease.ReleasedAt = &t
	  lease.UpdatedAt = now
	  holdID := firstNonEmpty(req.HoldID, g.HoldID, lease.HoldID)
	  if holdID != "" {
		  if h, ok := e.view.LookupHold(holdID); ok {
			   if h.Status == "released" || h.Status == "closed" {
				   p, _ := e.recordRace(ctx, bid, req.UserID, lease.ID, req.IdemKey, req.LiqGen, g.Gen, RaceDoubleRelease, g.Phase, "release")
				   return &GateResult{Allowed: false, Generation: g, Lease: lease, RaceProof: p, RejectReason: string(RaceDoubleRelease)}, wrapRace(ErrDoubleRelease, p)
			   }
			   _ = e.view.ApplyHoldStatus(holdID, "released")
		   }
	  }
	  if err := e.store.UpdateLease(ctx, lease); err != nil {
		  return nil, err
	  }
	  if err := e.store.UpdateGeneration(ctx, g); err != nil {
		  return nil, err
	   }
	  _ = e.view.ApplyBorrowPhase(bid, PhaseReleasing, g.Gen, "frozen")
	  e.audit(ctx, bid, lease.ID, string(fromP), string(PhaseReleasing), string(fromL), string(LeaseReleasing), g.Gen, "engine", "release")
	  incReleaseOK()
	  return &GateResult{Allowed: true,
		  Generation: g, Lease: lease}, nil
  }

// CloseWithGen 关借必须携带匹配 liq_gen。
  func (e *Engine) CloseWithGen(ctx context.Context, req CloseRequest) (*GateResult, error) {
	   if e == nil || e.store == nil {
		  return nil, ErrUnavailable
	   }
	  bid := trim(req.BorrowID)
	  if bid == "" {
		   return nil, fmt.Errorf("liqgengate: borrow_id required")
	  }
	  g, err := e.store.GetGeneration(ctx, bid)
	   if err != nil {
		  if errors.Is(err, ErrNotFound) && !e.cfg.AllowCloseWithoutGen {
			  p, _ := e.recordRace(ctx, bid, req.UserID, "", req.IdemKey, req.LiqGen, 0, RaceMissingGen, PhaseOpen, "close")
			  return &GateResult{Allowed: false, RaceProof: p, RejectReason: string(RaceMissingGen)}, wrapRace(ErrMissingGen, p)
		   }
		   return nil, err
	  }
	   if !req.HasGen && !e.cfg.AllowCloseWithoutGen {
		   p, _ := e.recordRace(ctx, bid, req.UserID, g.LeaseID, req.IdemKey, 0, g.Gen, RaceMissingGen, g.Phase, "close")
		  return &GateResult{Allowed: false, Generation: g, RaceProof: p, RejectReason: string(RaceMissingGen)}, wrapRace(ErrMissingGen, p)
	  }
	  if req.HasGen && req.LiqGen != g.Gen {
		  p, _ := e.recordRace(ctx, bid, req.UserID, g.LeaseID, req.IdemKey, req.LiqGen, g.Gen, RaceStaleGen, g.Phase, "close")
		   return &GateResult{Allowed: false, Generation: g, RaceProof: p, RejectReason: string(RaceStaleGen)}, wrapRace(ErrStaleGen, p)
	   }
	   if g.Phase == PhaseHalfOpen {
		  p, _ := e.recordRace(ctx, bid, req.UserID, g.LeaseID, req.IdemKey, req.LiqGen, g.Gen, RaceHalfBlocked, g.Phase, "close")
		  return &GateResult{Allowed: false, Generation: g, RaceProof: p, RejectReason: string(RaceHalfBlocked)}, wrapRace(ErrHalfOpen, p)
	   }
	   if g.Phase != PhaseReleasing && g.Phase != PhaseFrozen {
		  if g.Phase == PhaseClosed {
			  p, _ := e.recordRace(ctx, bid, req.UserID, g.LeaseID, req.IdemKey, req.LiqGen, g.Gen, RaceDoubleRelease, g.Phase, "close")
			  return &GateResult{Allowed: false,
				  Generation: g, RaceProof: p, RejectReason: string(RaceDoubleRelease)}, wrapRace(ErrDoubleRelease, p)
		  }
		   p, _ := e.recordRace(ctx, bid, req.UserID, g.LeaseID, req.IdemKey, req.LiqGen, g.Gen, RaceUnexpectedPhase, g.Phase, "close")
		   return &GateResult{Allowed: false, Generation: g, RaceProof: p, RejectReason: string(RaceUnexpectedPhase)}, wrapRace(ErrUnexpectedPhase, p)
	  }
	  owner := "close:" + uuid.NewString()
	  if err := e.store.TryAcquireMutex(ctx, &MutexRecord{
		   BorrowID: bid, Owner: owner, Kind: "close", Gen: g.Gen, AcquiredAt: e.now(),
	  }); err != nil {
		  p, _ := e.recordRace(ctx, bid, req.UserID, g.LeaseID, req.IdemKey, req.LiqGen, g.Gen, RaceMutexHeld, g.Phase, "close")
		  return &GateResult{Allowed: false, Generation: g, RaceProof: p, RejectReason: string(RaceMutexHeld)}, wrapRace(ErrMutexHeld, p)
	  }
	   defer func() { _ = e.store.ReleaseMutex(ctx, bid, owner) }()
	   lease,
		   lerr := e.store.GetActiveLease(ctx, bid)
	  now := e.now()
	  fromP := g.Phase
	  g.Phase = PhaseClosed
	   g.UpdatedAt = now
	   if lerr == nil {
		   if !CanTransitLease(lease.State, LeaseConsumed) && lease.State != LeaseReleasing && lease.State != LeaseFrozen {
			  return nil, ErrInvalidTransit
		  }
		  lease.State = LeaseConsumed
		   t := now
		   lease.ConsumedAt = &t
		  lease.UpdatedAt = now
		  _ = e.store.UpdateLease(ctx, lease)
	   }
	   if err := e.store.UpdateGeneration(ctx, g); err != nil {
		   return nil, err
	   }
	   _ = e.view.ApplyBorrowPhase(bid, PhaseClosed, g.Gen, "closed")
	  fromL := ""
	   if lease != nil {
		   fromL = string(LeaseReleasing)
	   }
	  e.audit(ctx, bid, g.LeaseID, string(fromP), string(PhaseClosed), fromL, string(LeaseConsumed), g.Gen, "engine", "close")
	   incCloseOK()
	   return &GateResult{Allowed: true, Generation: g, Lease: lease}, nil
  }

// ForceClose 运维强制收口半截冻结；须显式确认，不得静默当已强平释放。

  func (e *Engine) ForceClose(ctx context.Context, borrowID, note string) (*LiqGeneration, error) {
	  if strings.TrimSpace(note) == "" {
		  return nil, ErrForceDenied
	  }
	   g, err := e.store.GetGeneration(ctx, borrowID)
	  if err != nil {
		  return nil, err
	   }
	   if g.Phase != PhaseHalfOpen && g.Phase != PhaseFrozen && g.Phase != PhaseReleasing {
		  return nil, ErrForceDenied
	  }
	  now := e.now()
	   from := g.Phase
	   g.Phase = PhaseClosed
	   g.UpdatedAt = now
	   if err := e.store.UpdateGeneration(ctx, g); err != nil {
		   return nil, err
	  }
	   if lease, err := e.store.GetActiveLease(ctx, borrowID); err == nil {
		  lease.State = LeaseRevoked
		   t := now
		  lease.RevokedAt = &t
		   lease.RevokeReason = "ops_force"
		  lease.UpdatedAt = now
		   _ = e.store.UpdateLease(ctx, lease)
	  }
	  _ = e.view.ApplyBorrowPhase(borrowID, PhaseClosed, g.Gen, "closed")
	  e.audit(ctx, borrowID, g.LeaseID, string(from), string(PhaseClosed), "", string(LeaseRevoked), g.Gen, "ops", note)
	   return g, nil
   }

  func firstNonEmpty(ss ...string) string {
	   for _, s := range ss {
		   if strings.TrimSpace(s) != "" {
			  return s
		  }
	   }
	   return ""
  }

   func wrapRace(err error, _ *LiqProof) error {
	   return err
   }
