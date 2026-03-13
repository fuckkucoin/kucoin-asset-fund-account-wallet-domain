package occupbucketfence

import (
	"context"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/occupbook"
	"github.com/google/uuid"
)

const defaultFenceTTL = int64(120)

// Engine 围栏引擎。
type Engine struct {
	store Store
	book *occupbook.Book
	clock func() time.Time
	auditFailClosed bool
}

// EngineOption 配置。
type EngineOption func(*Engine)

// WithEngineClock 时钟。

   func WithEngineClock(fn func() time.Time) EngineOption {
	   return func(e *Engine) {
		   if fn != nil {
			   e.clock = fn
		   }
	  }
}

// WithAuditFailClosed 审计写入失败则整笔失败。
  func WithAuditFailClosed(on bool) EngineOption {
	  return func(e *Engine) {
		   e.auditFailClosed = on
	   }
  }

// NewEngine 构造引擎。

  func NewEngine(store Store, book *occupbook.Book, opts ...EngineOption) *Engine {
	   if store == nil {
		   store = NewMemStore()
	   }
	   if book == nil {
		  book = occupbook.NewBook(nil, occupbook.WithRequireFence(true))
	  }
	  e := &Engine{
		  store: store,
		   book: book,
		   clock:           func() time.Time { return time.Now().UTC() },
		   auditFailClosed: true,
	   }
	   for _, opt := range opts {
		  opt(e)
	   }
	   return e
  }

  func (e *Engine) now() time.Time {
	   if e == nil || e.clock == nil {
		  return time.Now().UTC()
	  }
	  return e.clock().UTC()
   }

  func (e *Engine) Book() *occupbook.Book {
	  if e == nil {
		   return nil
	  }
	  return e.book
  }

// IssueFence 签发 issued 围栏。同 lot 已有非终态围栏则复用。
  func (e *Engine) IssueFence(ctx context.Context, req IssueFenceReq) (*BucketFence, error) {
	   if e == nil || e.store == nil || e.book == nil {
		   return nil, ErrUnavailable
	  }
	  lotID := strings.TrimSpace(req.LotID)
	   if lotID == "" {
		   return nil, ErrLotRequired
	   }
	   if exist, err := e.store.GetActiveFenceByLot(ctx, lotID); err == nil && exist != nil {
		   if e.now().After(exist.ExpiresAt) {
			   _ = e.expireFence(ctx, exist)
		   } else {
			   return exist, nil
		  }
	   }
	   lot, err := e.book.GetLot(ctx, lotID)
	  if err != nil && !errors.Is(err, occupbook.ErrLotNotFound) {
		  return nil, err
	  }
	   ttl := req.TTLSeconds
	   if ttl <= 0 {
		  ttl = defaultFenceTTL
	  }
	   if ttl > 3600 {
		  return nil, ErrTTLInvalid
	   }
	  now := e.now()
	   fp := ""
	  gen := int64(0)
	   user, asset := "", ""
	  if lot != nil {
		  fp = FingerprintLot(lot)
		  gen = lot.Generation
		  user, asset = lot.UserID, lot.Asset
	  }
	   f := &BucketFence{
		   FenceID: uuid.NewString(),
		  LotID:   lotID,
		   UserID: user,
		   Asset:             asset,
		   BucketFingerprint: fp,
		   Generation:        gen,
		   TTLSeconds: ttl,
		   State:      FenceIssued,
		   IssuedAt:  now,
		  ExpiresAt: now.Add(time.Duration(ttl) * time.Second),
		  CreatedAt: now,
	   }
	  if err := e.store.SaveFence(ctx, f); err != nil {
		   return nil, err
	   }
	   return f, nil
  }

   func (e *Engine) expireFence(ctx context.Context, f *BucketFence) error {
	   now := e.now()
	  f.State = FenceExpired
	  f.ConsumedBy = "ttl"
	   f.ConsumedAt = &now
	  return e.store.UpdateFence(ctx, f)
  }

   func (e *Engine) loadFence(ctx context.Context, fenceID, lotID string) (*BucketFence, error) {
	  if strings.TrimSpace(fenceID) == "" {
		  return nil, ErrFenceRequired
	   }
	   f, err := e.store.GetFence(ctx, strings.TrimSpace(fenceID))
	   if err != nil {
		  return nil, err
	  }
	   if lotID != "" && f.LotID != lotID {
		  return nil, ErrFenceMismatch
	   }
	  if f.State.Terminal() {
		  if f.State == FenceExpired {
			   return nil, ErrFenceExpired
		   }
		  return nil, ErrFenceConsumed
	  }
	   if e.now().After(f.ExpiresAt) {
		  _ = e.expireFence(ctx, f)
		  return nil, ErrFenceExpired
	  }
	   return f, nil
  }

// HoldWithFence 观察（若需要）+ 签发/绑定围栏 + Hold。半截落 HalfBucketCase。
  func (e *Engine) HoldWithFence(ctx context.Context, req HoldWithFenceReq) (*HoldWithFenceResp, error) {
	  if e == nil || e.store == nil || e.book == nil {
		  return nil, ErrUnavailable
	   }
	   lotID := strings.TrimSpace(req.LotID)
	  if lotID == "" {
		   return nil, ErrLotRequired
	  }
	   if half, err := e.store.GetOpenHalfByLot(ctx, lotID); err == nil && half != nil {
		   return &HoldWithFenceResp{Half: half, LotID: lotID}, ErrHalfOpen
	   }
	   lot,
		  err := e.book.GetLot(ctx, lotID)
	  if errors.Is(err, occupbook.ErrLotNotFound) {
		  if strings.TrimSpace(req.UserID) == "" {
			  return nil, ErrUserRequired
		   }
		   if strings.TrimSpace(req.Asset) == "" {
			  return nil, ErrAssetRequired
		   }
		   lot, err = e.book.Observe(ctx, occupbook.ObserveReq{
			  LotID:  lotID,
			   UserID: req.UserID,
			   Asset: req.Asset,
			  Amount:     req.Amount,
			  RiskClass:  req.RiskClass,
			  ObserveRef: req.ObserveRef,
		   })
		   if err != nil {
			  return nil, err
		   }
	  } else if err != nil {
		   return nil, err
	  }
	   if lot.RiskClass.RequiresFullOccupancy() && strings.TrimSpace(req.FenceID) == "" && strings.TrimSpace(req.HoldID) == "" {
		  // T4/mixer 必须走围栏：允许本入口签发，但禁止无本函数的旁路（由 Book.RequireFence 保证）。
	   }
	  var fence *BucketFence
	  if strings.TrimSpace(req.FenceID) != "" {
		  fence, err = e.loadFence(ctx, req.FenceID, lotID)
		  if err != nil {
			  return nil, err
		   }
	  } else {
		   fence, err = e.IssueFence(ctx, IssueFenceReq{LotID: lotID, TTLSeconds: req.TTLSeconds})
		  if err != nil {
			   return nil, err
		  }
	  }
	  hold, lot2, holdErr := e.book.Hold(ctx, occupbook.HoldReq{
		   LotID: lotID,
		   HoldID: req.HoldID,
		   Amount: req.Amount,
		   IdemKey:   req.IdemKey,
		   RiskClass: req.RiskClass,
	   }, true)
	  if holdErr != nil && !errors.Is(holdErr, occupbook.ErrHalfPersist) {
		  if errors.Is(holdErr, occupbook.ErrConserveBroken) {
			  _, _ = e.openBreak(ctx, lot2, fence, "hold_conserve", "", "", holdErr.Error())
			   return nil, ErrConserveBroken
		  }
		  if errors.Is(holdErr, occupbook.ErrMixerBypass) {
			  return nil, ErrMixerBypass
		  }
		  return nil, holdErr
	  }
	   now := e.now()
	  patch := &BucketFence{
		  BoundHoldID:       hold.HoldID,
		   BucketFingerprint: FingerprintLot(lot2),
		  Generation:        lot2.Generation,
		  BoundAt:           &now,
	  }
	  from := fence.State
	  if from == FenceIssued || from == FenceBound {
		   if casErr := e.store.CASFenceState(ctx, fence.FenceID, from, FenceBound, patch); casErr != nil {
			   return nil, casErr
		  }
	  }
	  fence, _ = e.store.GetFence(ctx, fence.FenceID)
	   resp := &HoldWithFenceResp{
		  Fence: fence,
		   HoldID: hold.HoldID,
		   LotID:  lotID,
		  Inflight: lot2.Buckets.Inflight.String(),
		  Available: lot2.Buckets.Available.String(),
		   Void:      lot2.Buckets.Void.String(),
	  }
	   if errors.Is(holdErr, occupbook.ErrHalfPersist) || (hold != nil && !hold.PersistOK) {
		   half := &HalfBucketCase{
			   CaseID: uuid.NewString(),
			   LotID: lotID,
			  HoldID: hold.HoldID,
			   FenceID: fence.FenceID,
			  State:   HalfOpen,
			  Amount: hold.Amount.String(),
			   Reason:   "bucket_persist_interrupted",
			  OpenedAt: now,
		  }
		  if err := e.store.SaveHalf(ctx, half); err != nil {
			  return resp, err
		  }
		  resp.Half = half
		   return resp, ErrHalfOpen
	   }
	  return resp, nil
  }

// ResumeHalf 半截续提，拒绝另开全额占用。
   func (e *Engine) ResumeHalf(ctx context.Context, lotID, caseID string) (*HalfBucketCase, error) {
	  if e == nil {
		  return nil, ErrUnavailable
	   }
	  var half *HalfBucketCase
	  var err error
	   if strings.TrimSpace(caseID) != "" {
		   half, err = e.store.GetHalf(ctx, caseID)
	  } else {
		   half, err = e.store.GetOpenHalfByLot(ctx, lotID)
	   }
	   if err != nil {
		   return nil, err
	   }
	  if half.State != HalfOpen {
		   return half, nil
	  }
	  _, _, err = e.book.ResumeHalfHold(ctx, half.LotID, half.HoldID)
	   if err != nil {
		   if errors.Is(err, occupbook.ErrConserveBroken) {
			   lot, _ := e.book.GetLot(ctx, half.LotID)
			  f, _ := e.store.GetFence(ctx, half.FenceID)
			   _, _ = e.openBreak(ctx, lot, f, "resume_conserve", "", "", err.Error())
			  return half, ErrConserveBroken
		  }
		  return half, err
	   }
	  now := e.now()
	   half.State = HalfResumed
	   half.ResumedAt = &now
	  if err := e.store.UpdateHalf(ctx, half); err != nil {
		   return half, err
	  }
	  return half, nil
  }

// ReleaseAssert 带围栏确认释放。

  func (e *Engine) ReleaseAssert(ctx context.Context, req ReleaseAssertReq) (*MutateResp, error) {
	  return e.mutate(ctx, "release", req.LotID, req.HoldID, req.FenceID, req.Fingerprint, req.Generation, func() (*occupbook.Lot, error) {
		   _, lot, err := e.book.Release(ctx, occupbook.ReleaseReq{
			  LotID: req.LotID,
			   HoldID: req.HoldID,
			   ReleaseID: req.ReleaseID,
			   Amount:    req.Amount,
			  IdemKey: req.IdemKey,
		  })
		   return lot, err
	  })
  }

// ReclassAssert 带围栏升档作废。
   func (e *Engine) ReclassAssert(ctx context.Context, req ReclassAssertReq) (*MutateResp, error) {
	   return e.mutate(ctx, "reclass", req.LotID, req.HoldID, req.FenceID, req.Fingerprint, req.Generation, func() (*occupbook.Lot, error) {
		  _, lot, err := e.book.Reclass(ctx, occupbook.ReclassReq{
			   LotID: req.LotID,
			   HoldID:    req.HoldID,
			   ReclassID: req.ReclassID,
			   Amount: req.Amount,
			   ToClass:    req.ToClass,
			  ReasonCode: req.ReasonCode,
			  IdemKey: req.IdemKey,
		   })
		   return lot, err
	   })
   }

  func (e *Engine) mutate(ctx context.Context, op, lotID, holdID, fenceID, fp string, gen int64, apply func() (*occupbook.Lot, error)) (*MutateResp, error) {
	   if e == nil || e.store == nil || e.book == nil {
		   return nil, ErrUnavailable
	   }
	   lotID = strings.TrimSpace(lotID)
	   if lotID == "" {
		   return nil, ErrLotRequired
	  }
	  if strings.TrimSpace(holdID) == "" {
		  return nil, ErrHoldRequired
	   }
	  if half, err := e.store.GetOpenHalfByLot(ctx, lotID); err == nil && half != nil {
		  return nil, ErrHalfOpen
	   }
	  fence, err := e.loadFence(ctx, fenceID, lotID)
	   if err != nil {
		  return nil, err
	   }
	  if fence.State != FenceBound && fence.State != FenceIssued {
		   return nil, ErrFenceNotBound
	   }
	   lot, err := e.book.GetLot(ctx, lotID)
	   if err != nil {
		  return nil, err
	   }
	  if strings.TrimSpace(fp) != "" {
		   if err := MatchFingerprint(lot, fp, gen); err != nil {
			  p, _ := e.openBreak(ctx, lot, fence, "fingerprint_mismatch", op, "", err.Error())
			   return &MutateResp{Fence: fence, Proof: p, LotID: lotID, Conserve: false}, err
		   }
	   } else if fence.BucketFingerprint != "" && fence.BucketFingerprint != FingerprintLot(lot) && fence.Generation != 0 && fence.Generation != lot.Generation {
		  p, _ := e.openBreak(ctx, lot, fence, "fingerprint_mismatch", op, "", "stored fence fingerprint stale")
		   return &MutateResp{Fence: fence, Proof: p, LotID: lotID, Conserve: false}, ErrFenceMismatch
	  }
	  if err := e.store.TryLockLotOp(ctx, lotID, op); err != nil {
		  p, _ := e.openBreak(ctx, lot, fence, "reclass_release_mutex", op, "", err.Error())
		   return &MutateResp{Fence: fence, Proof: p, LotID: lotID, Winner: "", Conserve: lot.ConserveOK()}, ErrReclassMutex
	  }
	   defer e.store.UnlockLotOp(ctx, lotID)
	   lot2, applyErr := apply()
	  if lot2 == nil {
		  lot2 = lot
	  }
	  resp := &MutateResp{
		  LotID:     lotID,
		  Inflight:  lot2.Buckets.Inflight.String(),
		   Available: lot2.Buckets.Available.String(),
		  Void:      lot2.Buckets.Void.String(),
		  Conserve:  lot2.ConserveOK(),
	  }
	   if applyErr != nil {
		   kind := "mutate_failed"
		  if errors.Is(applyErr, occupbook.ErrConserveBroken) {
			   kind = "conserve_broken"
			   applyErr = ErrConserveBroken
		   }
		  if errors.Is(applyErr, occupbook.ErrHoldAlreadyReleased) || errors.Is(applyErr, occupbook.ErrHoldAlreadyReclassed) {
			   kind = "double_exit"
			  applyErr = ErrReclassMutex
		  }
		   p, _ := e.openBreak(ctx, lot2, fence, kind, op, "", applyErr.Error())
		   resp.Proof = p
		   resp.Fence = fence
		   return resp, applyErr
	  }
	   now := e.now()
	  if casErr := e.store.CASFenceState(ctx, fence.FenceID, fence.State, FenceConsumed, &BucketFence{
		   ConsumedBy: op,
		  ConsumedAt: &now,
		   BucketFingerprint: FingerprintLot(lot2),
		   Generation: lot2.Generation,
	   }); casErr != nil {
		p, _ := e.openBreak(ctx, lot2, fence, "fence_cas_conflict", op, "", casErr.Error())
		resp.Proof = p
		resp.Fence = fence
		return resp, ErrReclassMutex
	}
	fence, _ = e.store.GetFence(ctx, fence.FenceID)
	resp.Fence = fence
	resp.Winner = op
	if !lot2.ConserveOK() {
		p, _ := e.openBreak(ctx, lot2, fence, "post_write_conserve", op, op, "sum!=total")
		resp.Proof = p
		resp.Conserve = false
		return resp, ErrConserveBroken
	}
	return resp, nil
}

   func (e *Engine) openBreak(ctx context.Context, lot *occupbook.Lot, fence *BucketFence, kind, conflict, winner, reason string) (*OccupancyBreakProof, error) {
	   if lot == nil {
		   return nil, ErrLotRequired
	  }
	  if exist, err := e.store.GetOpenProofByLot(ctx, lot.LotID); err == nil && exist != nil {
		   return exist, nil
	  }
	  sum := lot.Buckets.Sum()
	  fid := ""
	   if fence != nil {
		  fid = fence.FenceID
	   }
	  now := e.now()
	   p := &OccupancyBreakProof{
		   ProofID: uuid.NewString(),
		   LotID:   lot.LotID,
		   FenceID: fid,
		   State: BreakOpen,
		  Kind:  kind,
		   Inflight: lot.Buckets.Inflight.String(),
		  Available: lot.Buckets.Available.String(),
		   Void:      lot.Buckets.Void.String(),
		  Sum: sum.String(),
		  Total: lot.OccupiedTotal.String(),
		   Delta:      sum.Sub(lot.OccupiedTotal).String(),
		   ConflictOp: conflict,
		   WinnerOp:   winner,
		   Reason:   reason,
		  OpenedAt: now,
	   }
	   p.Digest = ProofDigest(p)
	  if err := e.store.SaveProof(ctx, p); err != nil {
		   if errors.Is(err, ErrAlreadyProven) {
			   return e.store.GetOpenProofByLot(ctx, lot.LotID)
		   }
		  if e.auditFailClosed {
			   return p, ErrAuditFailed
		   }
		  return p, err
	   }
	  return p, nil
   }

// ProveBreak open→proven。
  func (e *Engine) ProveBreak(ctx context.Context, proofID string) (*OccupancyBreakProof, error) {
	   p,
		   err := e.store.GetProof(ctx, proofID)
	  if err != nil {
		  return nil, err
	  }
	  if p.State != BreakOpen {
		   return p, ErrInvalidTransition
	  }
	   now := e.now()
	   p.State = BreakProven
	   p.ProvenAt = &now
	   p.Digest = ProofDigest(p)
	  if err := e.store.UpdateProof(ctx, p); err != nil {
		  return nil, err
	  }
	   return p, nil
   }

// CloseBreak proven→closed。

   func (e *Engine) CloseBreak(ctx context.Context, proofID string) (*OccupancyBreakProof, error) {
	  p, err := e.store.GetProof(ctx, proofID)
	   if err != nil {
		  return nil, err
	   }
	   if p.State != BreakProven {
		  return p, ErrInvalidTransition
	   }
	  now := e.now()
	  p.State = BreakClosed
	   p.ClosedAt = &now
	  if err := e.store.UpdateProof(ctx, p); err != nil {
		  return nil, err
	   }
	  return p, nil
   }

// ExportProofs 导出批次证明。
  func (e *Engine) ExportProofs(ctx context.Context, lotID string) ([]*OccupancyBreakProof, error) {
	  if e == nil {
		  return nil, ErrUnavailable
	  }
	  return e.store.ListProofsByLot(ctx, strings.TrimSpace(lotID))
  }

// GetFence 只读围栏。
   func (e *Engine) GetFence(ctx context.Context, fenceID string) (*BucketFence, error) {
	   if e == nil {
		  return nil, ErrUnavailable
	  }
	   return e.store.GetFence(ctx, strings.TrimSpace(fenceID))
  }

// ExpireStale 过期 issued/bound 围栏。

  func (e *Engine) ExpireStale(ctx context.Context, limit int) (int, error) {
	   list, err := e.store.ListExpiredBound(ctx, e.now(), limit)
	   if err != nil {
		   return 0, err
	   }
	  n := 0
	   for _, f := range list {
		  if err := e.expireFence(ctx, f); err == nil {
			  n++
		  }
	  }
	  return n, nil
  }

// PatrolConserve 巡检打开批次并落证明。
   func (e *Engine) PatrolConserve(ctx context.Context, limit int) ([]*OccupancyBreakProof, error) {
	   sums, err := e.book.Patrol(ctx, limit)
	  if err != nil {
		  return nil, err
	  }
	  out := make([]*OccupancyBreakProof,
		   0, len(sums))
	  for _, s := range sums {
		  lot, err := e.book.GetLot(ctx, s.LotID)
		   if err != nil {
			   continue
		   }
		  f, _ := e.store.GetActiveFenceByLot(ctx, s.LotID)
		  p, _ := e.openBreak(ctx, lot, f, "patrol_conserve", "", "", s.Reason)
		  if p != nil {
			   if p.State == BreakOpen {
				  p, _ = e.ProveBreak(ctx, p.ProofID)
			  }
			  out = append(out, p)
		   }
	  }
	  return out, nil
  }
