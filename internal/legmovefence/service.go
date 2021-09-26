// @FileName: service.go
// @Summary: 领域服务：业务用例编排与校验。
// @Author: Cason
// @Created: 2021.08.17

package legmovefence

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/georebal"
	"github.com/google/uuid"
)

const (
	DefaultTTLSeconds = 300
	MaxTTLSeconds    = 1800
	ExpireBatchSize  = 200
	PatrolBatchSize  = 100
	ProofExportBatch = 100
)

// Service 腿围栏核心服务。

type Service struct {
	store Store
	book *georebal.Book
	exec *georebal.BookExecutor
	sweep *georebal.SweepObserve
	clock   func() time.Time
	metrics *Metrics
}

  func New(book *georebal.Book, exec *georebal.BookExecutor) *Service {
	   if book == nil {
		  book = georebal.NewBook()
	  }
	  if exec == nil {
		  exec = georebal.NewBookExecutor(book)
	   }
	   return &Service{
		   store: NewMemStore(),
		   book: book,
		  exec: exec,
		   sweep: &georebal.SweepObserve{},
		   clock: time.Now,
		   metrics: NewMetrics(nil),
	  }
  }

  func NewWithStore(store Store, book *georebal.Book, exec *georebal.BookExecutor) *Service {
	   if store == nil {
		  store = NewMemStore()
	  }
	  if book == nil {
		   book = georebal.NewBook()
	   }
	   if exec == nil {
		  exec = georebal.NewBookExecutor(book)
	   }
	   return &Service{store: store, book: book, exec: exec, sweep: &georebal.SweepObserve{}, clock: time.Now, metrics: NewMetrics(nil)}
   }

  func (s *Service) WithClock(fn func() time.Time) *Service {
	  if s != nil && fn != nil {
		   s.clock = fn
	  }
	  return s
  }

   func (s *Service) WithMetrics(m *Metrics) *Service {
	  if s != nil && m != nil {
		  s.metrics = m
	   }
	  return s
  }

   func (s *Service) now() time.Time {
	   if s == nil || s.clock == nil {
		  return time.Now().UTC()
	  }
	   return s.clock().UTC()
   }

   func (s *Service) Store() Store {
	   if s == nil {
		   return nil
	  }
	  return s.store
   }

  func (s *Service) Book() *georebal.Book          { return s.book }
   func (s *Service) Exec() *georebal.BookExecutor  { return s.exec }
   func (s *Service) Sweep() *georebal.SweepObserve { return s.sweep }

// ExecLegReq 带围栏执行腿。

type ExecLegReq struct {
	PlanID string
	LegID  string
	IdempotencyKey string
	TTLSeconds     int64
	Actor          string
	ForceTransfer  bool
}

// ExecResult 执行结果。
type ExecResult struct {
	Fence LegMoveFence `json:"fence"`
	IntentRef  string `json:"intent_ref,omitempty"`
	TransferOK bool   `json:"transfer_ok"`
	MoveRef    string `json:"move_ref,omitempty"`
	Idempotent bool   `json:"idempotent"`
	HalfCase *HalfMoveCase `json:"half_case,omitempty"`
	Proof *GeoDoubleMoveProof `json:"proof,omitempty"`
}

// RetryReq 失败腿复验围栏后重试。

type RetryReq struct {
	FenceID        string
	IdempotencyKey string
	Actor string
	ForceTransfer bool
}

// ResumeReq 运维续提半截案。

type ResumeReq struct {
	CaseID string
	Actor  string
	MarkOnly bool
}

// ExecLegWithFence 签发/复用围栏，Enqueue 后 TransferInv。

  func (s *Service) ExecLegWithFence(ctx context.Context, req ExecLegReq) (ExecResult, error) {
	  if s == nil || s.store == nil || s.book == nil || s.exec == nil {
		  return ExecResult{}, ErrUnavailable
	  }
	   planID := strings.TrimSpace(req.PlanID)
	  legID := strings.TrimSpace(req.LegID)
	   idem := strings.TrimSpace(req.IdempotencyKey)
	   if planID == "" || legID == "" {
		   return ExecResult{}, ErrInvalidReq
	   }
	  if idem == "" {
		   return ExecResult{}, ErrIdem
	  }
	   if prev, ok, err := s.store.GetFenceByIdem(ctx, idem); err != nil {
		  return ExecResult{}, err
	  } else if ok {
		  return s.replayOrRetry(ctx, prev, req.Actor, req.ForceTransfer)
	   }
	  snap, err := s.book.Snapshot(ctx, planID, legID)
	  if err != nil {
		  return ExecResult{}, err
	  }
	  if snap.Src.Status == georebal.RegionHalted || snap.Dst.Status == georebal.RegionHalted {
		   p := s.openProof(ctx, ProofHaltedOld, LegMoveFence{PlanID: planID, LegID: legID, SrcRegion: snap.Leg.SrcRegion, DstRegion: snap.Leg.DstRegion}, "region already halted before issue")
		  return ExecResult{Proof: &p}, ErrRegionHalted
	   }
	  ttl := req.TTLSeconds
	  if ttl <= 0 {
		  ttl = DefaultTTLSeconds
	  }
	  if ttl > MaxTTLSeconds {
		  ttl = MaxTTLSeconds
	  }
	  now := s.now()
	   fp := LiveFingerprint(snap)
	  gen := CombinedHaltGen(snap.Src.HaltGen, snap.Dst.HaltGen)
	  f := LegMoveFence{
		  FenceID: "lmf-" + uuid.NewString(),
		  PlanID:         planID,
		   LegID:          legID,
		   IdempotencyKey: idem,
		   SrcRegion:      snap.Leg.SrcRegion,
		  DstRegion:      snap.Leg.DstRegion,
		   Asset:          snap.Leg.Asset,
		   Amount:         snap.Leg.Amount,
		   Fingerprint: fp,
		  Generation:  gen,
		   State: FenceIssued,
		   TTLSeconds: ttl,
		   IssuedAt:  now,
		   CreatedAt: now,
		  UpdatedAt: now,
	  }
	  if err := s.store.SaveFence(ctx, f); err != nil {
		   return ExecResult{}, err
	  }
	   _ = s.book.AttachFence(ctx, legID, f.FenceID)
	   _ = s.audit(ctx, f, AuditIssue, firstNonEmpty(req.Actor, "system"), "issued")
	   if s.metrics != nil {
		   s.metrics.IncIssued()
	  }
	   return s.runEnqueueTransfer(ctx, f, req.Actor, false)
   }

  func (s *Service) replayOrRetry(ctx context.Context, prev LegMoveFence, actor string, force bool) (ExecResult, error) {
	   if prev.State == FenceMoved {
		   if force {
			   p := s.openProof(ctx, ProofDoubleTransfer, prev, "force transfer after moved")
			   return ExecResult{Fence: prev, TransferOK: true, MoveRef: prev.TransferRef, Idempotent: true, Proof: &p}, ErrDoubleMove
		  }
		   return ExecResult{Fence: prev, TransferOK: true, MoveRef: prev.TransferRef, Idempotent: true}, nil
	  }
	   return s.RetryAssertFence(ctx, RetryReq{FenceID: prev.FenceID, IdempotencyKey: prev.IdempotencyKey, Actor: actor, ForceTransfer: force})
  }

// RetryAssertFence 失败腿重试验证围栏后至多一次成功划转。
  func (s *Service) RetryAssertFence(ctx context.Context, req RetryReq) (ExecResult, error) {
	   if s == nil || s.store == nil {
		   return ExecResult{}, ErrUnavailable
	   }
	  f,
		   err := s.store.GetFence(ctx, strings.TrimSpace(req.FenceID))
	  if err != nil {
		   return ExecResult{}, err
	  }
	  if req.IdempotencyKey != "" && f.IdempotencyKey != req.IdempotencyKey {
		  return ExecResult{}, ErrRetryDenied
	   }
	   if f.State == FenceMoved {
		   if req.ForceTransfer {
			  p := s.openProof(ctx, ProofReplay, f, "retry after moved")
			   return ExecResult{Fence: f,
				  TransferOK: true, MoveRef: f.TransferRef, Idempotent: true, Proof: &p}, ErrDoubleMove
		  }
		  return ExecResult{Fence: f, TransferOK: true, MoveRef: f.TransferRef, Idempotent: true}, nil
	   }
	  if err := s.reverify(ctx, &f); err != nil {
		   return ExecResult{Fence: f}, err
	   }
	  _ = s.audit(ctx, f, AuditRetry, firstNonEmpty(req.Actor, "system"), "retry_assert")
	   return s.runEnqueueTransfer(ctx, f, req.Actor, true)
  }

  func (s *Service) runEnqueueTransfer(ctx context.Context, f LegMoveFence, actor string, retry bool) (ExecResult, error) {
	  now := s.now()
	  if f.State == FenceIssued {
		   if err := ApplyFenceTransition(&f, FenceEnqueued, "", now); err != nil {
			  return ExecResult{Fence: f}, err
		   }
		   in, err := s.exec.Enqueue(ctx, georebal.EnqueueReq{PlanID: f.PlanID, LegID: f.LegID, FenceID: f.FenceID, Actor: actor})
		  if err != nil {
			   _ = s.book.UpdateLegState(ctx, f.LegID, georebal.LegFailed, err.Error())
			  return ExecResult{Fence: f}, err
		   }
		   f.IntentRef = in.IntentID
		  if err := s.store.UpdateFence(ctx, f); err != nil {
			  return ExecResult{Fence: f}, err
		  }
		   if s.sweep != nil {
			   s.sweep.NoteEnqueue(f.PlanID, f.LegID, f.FenceID)
		   }
		  _ = s.audit(ctx, f, AuditEnqueue, firstNonEmpty(actor, "system"), in.IntentID)
		   if s.metrics != nil {
			  s.metrics.IncEnqueued()
		  }
	  }
	   if err := s.reverify(ctx, &f); err != nil {
		   return ExecResult{Fence: f}, err
	  }
	   if s.book.SuccessfulMoves(f.LegID) > 0 && f.State != FenceMoved {
		   // 库存已迁但围栏未标 moved：半截续标，禁止再划转。
		   return s.markMovedNoReplay(ctx, f, actor, "inventory already moved")
	   }
	   mv, err := s.exec.TransferInv(ctx, georebal.TransferReq{PlanID: f.PlanID, LegID: f.LegID, FenceID: f.FenceID, Actor: actor})
	  if err != nil {
		   _ = s.book.UpdateLegState(ctx,
			   f.LegID, georebal.LegFailed, err.Error())
		   half := s.openHalf(ctx, f, HalfEnqueueNoTransfer, err.Error(), actor)
		  if s.metrics != nil {
			   s.metrics.IncHalf(string(HalfEnqueueNoTransfer))
		   }
		  return ExecResult{Fence: f, IntentRef: f.IntentRef, HalfCase: &half}, err
	  }
	  now = s.now()
	   if err := ApplyFenceTransition(&f, FenceMoved, "", now); err != nil {
		   return ExecResult{Fence: f}, err
	   }
	   f.TransferRef = mv.MoveID
	   if err := s.store.UpdateFence(ctx, f); err != nil {
		  return ExecResult{Fence: f}, err
	   }
	   _ = s.audit(ctx, f, AuditMove, firstNonEmpty(actor, "system"), mv.MoveID)
	   if s.metrics != nil {
		   s.metrics.IncMoved()
	  }
	   if err := s.exec.MarkDone(ctx, f.LegID); err != nil {
		  half := s.openHalf(ctx,
			  f, HalfTransferNoDone, err.Error(), actor)
		   if s.metrics != nil {
			   s.metrics.IncHalf(string(HalfTransferNoDone))
		   }
		   return ExecResult{Fence: f, IntentRef: f.IntentRef, TransferOK: true, MoveRef: mv.MoveID, HalfCase: &half}, err
	   }
	  if retry {
		  if c, ok, _ := s.store.GetOpenHalfByFence(ctx, f.FenceID); ok {
			   _ = ApplyHalfTransition(&c,
				   HalfResumed, actor, s.now())
			   _ = s.store.UpdateHalf(ctx, c)
		   }
	  }
	   return ExecResult{Fence: f,
		   IntentRef: f.IntentRef, TransferOK: true, MoveRef: mv.MoveID}, nil
   }

   func (s *Service) markMovedNoReplay(ctx context.Context, f LegMoveFence, actor, reason string) (ExecResult, error) {
	   now := s.now()
	   if f.State != FenceMoved {
		   if err := ApplyFenceTransition(&f, FenceMoved, reason, now); err != nil && !errors.Is(err, ErrInvalidTransition) {
			  return ExecResult{Fence: f}, err
		  }
		  if last := s.book.LastMove(f.LegID); last != nil {
			   f.TransferRef = last.MoveID
		   }
		   _ = s.store.UpdateFence(ctx, f)
	   }
	   _ = s.exec.MarkDone(ctx, f.LegID)
	   if c, ok, _ := s.store.GetOpenHalfByFence(ctx, f.FenceID); ok {
		  _ = ApplyHalfTransition(&c, HalfResumed, actor, now)
		  _ = s.store.UpdateHalf(ctx, c)
		  _ = s.audit(ctx, f, AuditHalfResume, actor, reason)
	  }
	   p := s.openProof(ctx, ProofDoubleTransfer, f, reason)
	  return ExecResult{Fence: f, TransferOK: true, MoveRef: f.TransferRef, Idempotent: true, Proof: &p}, nil
   }

   func (s *Service) reverify(ctx context.Context, f *LegMoveFence) error {
	   if err := AssertTransferable(f, s.now()); err != nil {
		   kind := ProofConsumed
		  switch {
		   case errors.Is(err, ErrFenceExpired):
			   kind = ProofExpired
		  case errors.Is(err, ErrFenceRevoked):
			   kind = ProofRevoked
		   case errors.Is(err, ErrNoFence):
			  kind = ProofNoFence
		  }
		  s.openProof(ctx,
			  kind, *f, err.Error())
		  return err
	   }
	   snap, err := s.book.Snapshot(ctx, f.PlanID, f.LegID)
	  if err != nil {
		  return err
	   }
	  live := LiveFingerprint(snap)
	   if !MatchFingerprint(f.Fingerprint, live) {
		  s.openProof(ctx, ProofFingerprint, *f, "live="+live)
		  return ErrFingerprint
	  }
	  if snap.Src.Status == georebal.RegionHalted || snap.Dst.Status == georebal.RegionHalted {
		   _ = s.revokeFence(ctx, f, "region_halted")
		   s.openProof(ctx, ProofHaltedOld, *f, "src_or_dst_halted")
		   return ErrRegionHalted
	   }
	   liveGen := CombinedHaltGen(snap.Src.HaltGen, snap.Dst.HaltGen)
	   if liveGen != f.Generation {
		   _ = s.revokeFence(ctx, f, "generation_stale")
		   s.openProof(ctx, ProofGeneration, *f, fmt.Sprintf("bind=%d live=%d", f.Generation, liveGen))
		   return ErrGeneration
	  }
	  return nil
   }

  func (s *Service) revokeFence(ctx context.Context, f *LegMoveFence, reason string) error {
	   if f.State == FenceMoved || f.State == FenceRevoked || f.State == FenceExpired {
		  return nil
	   }
	  if err := ApplyFenceTransition(f, FenceRevoked, reason, s.now()); err != nil {
		  return err
	   }
	  if err := s.store.UpdateFence(ctx,
		  *f); err != nil {
		  return err
	  }
	  _ = s.audit(ctx, *f, AuditRevoke, "system", reason)
	  if s.metrics != nil {
		  s.metrics.IncRevoked(reason)
	  }
	   return nil
   }

  func (s *Service) openHalf(ctx context.Context, f LegMoveFence, kind HalfKind, reason, actor string) HalfMoveCase {
	  if c, ok, _ := s.store.GetOpenHalfByFence(ctx, f.FenceID); ok {
		   return c
	  }
	  now := s.now()
	  c := HalfMoveCase{
		   CaseID: "hmc-" + uuid.NewString(),
		   Kind: kind,
		  FenceID: f.FenceID,
		  PlanID: f.PlanID,
		  LegID: f.LegID,
		  State: HalfOpen,
		   Reason:   reason,
		   OpenedAt: now,
		   Actor: actor,
		  CreatedAt: now,
		   UpdatedAt: now,
	   }
	   _ = s.store.SaveHalf(ctx, c)
	   _ = s.audit(ctx, f, AuditHalfOpen, firstNonEmpty(actor, "system"), string(kind)+" "+reason)
	   return c
  }

   func (s *Service) openProof(ctx context.Context, kind ProofKind, f LegMoveFence, extra string) GeoDoubleMoveProof {
	  now := s.now()
	   p := GeoDoubleMoveProof{
		   ProofID: "gdp-" + uuid.NewString(),
		   Kind:    kind,
		  FenceID: f.FenceID,
		  PlanID: f.PlanID,
		   LegID:  f.LegID,
		   IdemKey: f.IdempotencyKey,
		  SrcRegion: f.SrcRegion,
		  DstRegion: f.DstRegion,
		   Amount:    f.Amount,
		  BindFP:     f.Fingerprint,
		  BindGen:    f.Generation,
		  Evidence:   EvidenceLine(kind, f.FenceID, extra),
		   DetectedAt: now,
		   CreatedAt: now,
	  }
	   if snap, err := s.book.Snapshot(ctx, f.PlanID, f.LegID); err == nil {
		   p.LiveFP = LiveFingerprint(snap)
		   p.LiveGen = CombinedHaltGen(snap.Src.HaltGen, snap.Dst.HaltGen)
	  }
	  _ = s.store.SaveProof(ctx, p)
	   _ = s.audit(ctx, f, AuditProof, "system", string(kind))
	   if s.metrics != nil {
		  s.metrics.IncProof(string(kind))
	   }
	  return p
   }

   func (s *Service) audit(ctx context.Context, f LegMoveFence, act AuditAction, actor, detail string) error {
	   return s.store.SaveAudit(ctx, FenceAudit{
		  AuditID: "aud-" + uuid.NewString(), FenceID: f.FenceID, PlanID: f.PlanID, LegID: f.LegID,
		   Action: act, Actor: actor, Detail: detail, CreatedAt: s.now(),
	   })
  }

   func firstNonEmpty(vs ...string) string {
	   for _, v := range vs {
		  if strings.TrimSpace(v) != "" {
			   return v
		  }
	  }
	   return ""
   }

// TransferWithoutFence fail-closed：无围栏不得划转。
   func (s *Service) TransferWithoutFence(ctx context.Context, planID, legID string) error {
	   p := s.openProof(ctx, ProofNoFence, LegMoveFence{PlanID: planID, LegID: legID}, "missing fence")
	  _ = p
	   return ErrNoFence
   }

// ExpireFences issued/enqueued 超 TTL → expired。
   func (s *Service) ExpireFences(ctx context.Context) (int, error) {
	   if s == nil || s.store == nil {
		   return 0, ErrUnavailable
	  }
	   list, err := s.store.ListExpiredActive(ctx, s.now(), ExpireBatchSize)
	   if err != nil {
		  return 0, err
	  }
	   n := 0
	  for _, f := range list {
		  if err := ApplyFenceTransition(&f, FenceExpired, "ttl", s.now()); err != nil {
			  continue
		   }
		  if err := s.store.UpdateFence(ctx, f); err != nil {
			   continue
		   }
		  _ = s.audit(ctx, f, AuditExpire, "expire-worker", "ttl")
		  if s.metrics != nil {
			   s.metrics.IncExpired()
		   }
		  n++
	  }
	  return n, nil
  }

// RevokeHalted 源或目标 Halted 后吊销未 moved 的旧围栏。
  func (s *Service) RevokeHalted(ctx context.Context, regionID string) (int, error) {
	   if s == nil || s.store == nil {
		   return 0, ErrUnavailable
	  }
	  list, err := s.store.ListByRegion(ctx, regionID, ExpireBatchSize)
	  if err != nil {
		  return 0, err
	   }
	   n := 0
	   for i := range list {
		   f := list[i]
		  halted, _, err := georebal.BookSurface{Book: s.book}.RegionHalted(ctx, f.SrcRegion)
		  if err != nil {
			  continue
		   }
		   dh, _, _ := georebal.BookSurface{Book: s.book}.RegionHalted(ctx, f.DstRegion)
		  if !halted && !dh {
			   continue
		  }
		   if err := s.revokeFence(ctx, &f, "region_halted"); err != nil {
			  continue
		   }
		  s.openProof(ctx, ProofHaltedOld, f, "halt_revoke_pump")
		   n++
	  }
	  return n, nil
   }

// PatrolHalf 半截巡检：库存已迁则续标 done，并禁止再划转。

  func (s *Service) PatrolHalf(ctx context.Context) (int, error) {
	  if s == nil || s.store == nil {
		  return 0, ErrUnavailable
	  }
	   list, err := s.store.ListOpenHalf(ctx, PatrolBatchSize)
	   if err != nil {
		  return 0, err
	  }
	  n := 0
	   for _, c := range list {
		   f, err := s.store.GetFence(ctx, c.FenceID)
		  if err != nil {
			   continue
		  }
		  if s.book.SuccessfulMoves(f.LegID) > 0 {
			  if f.State != FenceMoved {
				   _ = ApplyFenceTransition(&f, FenceMoved, "patrol", s.now())
				   if last := s.book.LastMove(f.LegID); last != nil {
					   f.TransferRef = last.MoveID
				   }
				   _ = s.store.UpdateFence(ctx, f)
			  }
			  _ = s.exec.MarkDone(ctx, f.LegID)
			  _ = ApplyHalfTransition(&c, HalfResumed, "patrol", s.now())
			  _ = s.store.UpdateHalf(ctx, c)
			   _ = s.audit(ctx, f, AuditPatrol, "patrol", "resume-mark")
			  n++
			   if s.metrics != nil {
				  s.metrics.IncPatrol()
			  }
			  continue
		   }
		  if f.State == FenceRevoked || f.State == FenceExpired {
			  _ = ApplyHalfTransition(&c, HalfAbandoned, "patrol", s.now())
			   _ = s.store.UpdateHalf(ctx, c)
			  n++
		  }
	  }
	  return n, nil
  }

// DiscoverDoubleMove 扫描同腿多次成功划转并落证明。
  func (s *Service) DiscoverDoubleMove(ctx context.Context) (int, error) {
	   if s == nil || s.book == nil {
		  return 0, ErrUnavailable
	   }
	  moved, err := s.store.ListFencesByState(ctx, FenceMoved, PatrolBatchSize)
	   if err != nil {
		  return 0, err
	   }
	   n := 0
	   for _, f := range moved {
		   if s.book.SuccessfulMoves(f.LegID) > 1 {
			   s.openProof(ctx, ProofDoubleTransfer, f, fmt.Sprintf("moves=%d", s.book.SuccessfulMoves(f.LegID)))
			   n++
		   }
	  }
	  return n, nil
   }

// OpsResume 运维续提：持互斥后仅允许标 done 或放弃，不得二次 TransferInv。

   func (s *Service) OpsResume(ctx context.Context, req ResumeReq) (HalfMoveCase, error) {
	  c, err := s.store.GetHalf(ctx, strings.TrimSpace(req.CaseID))
	   if err != nil {
		  return HalfMoveCase{}, err
	  }
	  if c.State != HalfOpen {
		  return c, ErrResumeDenied
	  }
	  if c.ResumeLock != "" && c.ResumeLock != req.Actor {
		  return c, ErrHalfLocked
	   }
	  now := s.now()
	  c.ResumeLock = firstNonEmpty(req.Actor, "ops")
	  t := now
	   c.LockedAt = &t
	   c.UpdatedAt = now
	   if err := s.store.UpdateHalf(ctx, c); err != nil {
		   return c, err
	  }
	   f, err := s.store.GetFence(ctx, c.FenceID)
	   if err != nil {
		  return c, err
	   }
	   if req.MarkOnly || s.book.SuccessfulMoves(f.LegID) > 0 {
		   if f.State != FenceMoved && CanTransitFence(f.State, FenceMoved) {
			   _ = ApplyFenceTransition(&f, FenceMoved, "ops_resume", now)
			  _ = s.store.UpdateFence(ctx, f)
		  }
		  _ = s.exec.MarkDone(ctx, f.LegID)
		  _ = ApplyHalfTransition(&c, HalfResumed, req.Actor, now)
		  _ = s.store.UpdateHalf(ctx, c)
		   _ = s.audit(ctx, f, AuditHalfResume, req.Actor, "ops")
		   return c, nil
	   }
	   _ = ApplyHalfTransition(&c, HalfAbandoned, req.Actor, now)
	  _ = s.store.UpdateHalf(ctx, c)
	  _ = s.audit(ctx, f, AuditHalfAbandon, req.Actor, "ops abandon without transfer")
	   return c, nil
   }

// ExportProofs 导出未导出证明。
  func (s *Service) ExportProofs(ctx context.Context) (int, error) {
	  list,
		   err := s.store.ListProofs(ctx, ExportFilter{UnexportedOnly: true, Limit: ProofExportBatch})
	  if err != nil {
		  return 0, err
	   }
	   ids := make([]string, 0, len(list))
	   for _, p := range list {
		  ids = append(ids, p.ProofID)
	  }
	  if err := s.store.MarkProofsExported(ctx, ids, s.now()); err != nil {
		  return 0, err
	  }
	   if s.metrics != nil && len(ids) > 0 {
		  s.metrics.IncExport(len(ids))
	   }
	   return len(ids), nil
   }

// GetFence 查询。
  func (s *Service) GetFence(ctx context.Context, id string) (LegMoveFence, error) {
	   return s.store.GetFence(ctx, id)
  }

   func (s *Service) GetProof(ctx context.Context, id string) (GeoDoubleMoveProof, error) {
	   return s.store.GetProof(ctx, id)
   }

  func (s *Service) ListProofs(ctx context.Context, f ExportFilter) ([]GeoDoubleMoveProof, error) {
	  return s.store.ListProofs(ctx, f)
  }

   func (s *Service) ListAudit(ctx context.Context, fenceID string, limit int) ([]FenceAudit, error) {
	   return s.store.ListAudit(ctx, fenceID, limit)
  }

  func (s *Service) GetHalf(ctx context.Context, id string) (HalfMoveCase, error) {
	   return s.store.GetHalf(ctx, id)
   }
