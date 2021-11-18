// @FileName: service.go
// @Summary: 领域服务：业务用例编排与校验。
// @Author: Bronson
// @Created: 2021.11.08

package shardholdgen

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/wdshard"
	"github.com/google/uuid"
)

// Service 腿占世代核心服务。

type Service struct {
	store Store
	book  *wdshard.Book
	holds   wdshard.HoldAPI
	align   wdshard.AlignHook
	clock   func() time.Time
	metrics *Metrics
}

  func New(book *wdshard.Book) *Service {
	   if book == nil {
		  book = wdshard.NewBook()
	   }
	  return &Service{
		   store: NewMemStore(),
		   book:  book,
		   holds: wdshard.BookHoldFunc{Book: book},
		  align: wdshard.BookAlign{Book: book},
		  clock: time.Now,
		   metrics: NewMetrics(nil),
	   }
   }

  func NewWithStore(store Store, book *wdshard.Book) *Service {
	  if store == nil {
		   store = NewMemStore()
	   }
	   if book == nil {
		  book = wdshard.NewBook()
	  }
	   return &Service{
		  store: store, book: book,
		   holds: wdshard.BookHoldFunc{Book: book},
		  align: wdshard.BookAlign{Book: book},
		   clock: time.Now, metrics: NewMetrics(nil),
	   }
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

  func (s *Service) Book() *wdshard.Book { return s.book }

// AdvanceReq AdvanceWithGen 请求。

type AdvanceReq struct {
	PlanID string
	LegID          string
	IdempotencyKey string
	TTLSeconds     int64
	Actor         string
	SkipBroadcast bool
	ForceFail     bool
}

// AdvanceResult Advance 结果。

type AdvanceResult struct {
	Gen       LegHoldGen `json:"gen"`
	HoldID    string     `json:"hold_id,omitempty"`
	Broadcast string     `json:"broadcast_id,omitempty"`
	Idempotent bool         `json:"idempotent"`
	HalfCase   *HalfLegCase `json:"half_case,omitempty"`
	Proof *ShardHoldProof `json:"proof,omitempty"`
}

// ReleaseReq ReleaseAssert 请求。
type ReleaseReq struct {
	PlanID string
	LegID  string
	GenID  string
	Amount int64
	IdempotencyKey string
	Actor string
}

// ReleaseResult 释放结果。
type ReleaseResult struct {
	Gen        LegHoldGen      `json:"gen"`
	Released   int64           `json:"released"`
	Idempotent bool            `json:"idempotent"`
	Proof      *ShardHoldProof `json:"proof,omitempty"`
}

// ResumeReq 半截续提。
type ResumeReq struct {
	CaseID string
	Actor string
	MarkOnly bool
	Abandon  bool
}

// AdvanceWithGen 签发/复用世代，冻结并提交广播。无世代 fail-closed。
   func (s *Service) AdvanceWithGen(ctx context.Context, req AdvanceReq) (AdvanceResult, error) {
	  if s == nil || s.store == nil || s.book == nil {
		  return AdvanceResult{}, ErrUnavailable
	   }
	  planID := strings.TrimSpace(req.PlanID)
	   legID := strings.TrimSpace(req.LegID)
	  idem := strings.TrimSpace(req.IdempotencyKey)
	   if planID == "" || legID == "" {
		  return AdvanceResult{}, ErrInvalidReq
	  }
	  if idem == "" {
		   return AdvanceResult{}, ErrIdem
	  }
	  if prev, ok, err := s.store.GetGenByIdem(ctx, idem); err != nil {
		   return AdvanceResult{}, err
	  } else if ok {
		  return s.replayAdvance(ctx, prev, req)
	   }
	  snap, err := s.book.Snapshot(planID, legID)
	   if err != nil {
		  return AdvanceResult{}, err
	  }
	   now := s.now()
	   if existing, ok, err := s.store.GetGenByLeg(ctx, planID, legID); err != nil {
		   return AdvanceResult{}, err
	  } else if ok {
		   return s.advanceExisting(ctx, existing, snap, req, now)
	   }
	   g := LegHoldGen{
		   GenID: newID("lhg-", planID, legID, idem, now.String()),
		  PlanID: planID,
		  LegID:          legID,
		   IdempotencyKey: idem,
		   UserID: snap.Plan.UserID,
		  Asset:  snap.Plan.Asset,
		  Amount: snap.Leg.Amount,
		   Generation: 1,
		  State: GenIssued,
		  TTLSeconds: NormalizeTTL(req.TTLSeconds),
		   IssuedAt:   now,
		   CreatedAt:   now,
		  UpdatedAt:   now,
		   Fingerprint: Fingerprint(planID, legID, "", snap.Leg.Amount, 1),
	  }
	  if err := RequireGeneration(&g); err != nil {
		   return AdvanceResult{}, err
	   }
	  if err := s.store.SaveGen(ctx, g); err != nil {
		  return AdvanceResult{}, err
	   }
	  _ = s.audit(ctx, g.GenID, AuditIssue, req.Actor, "", GenIssued, "issue")
	  s.metrics.IncIssued()
	   return s.advanceExisting(ctx, g, snap, req, now)
  }

  func (s *Service) replayAdvance(ctx context.Context, prev LegHoldGen, req AdvanceReq) (AdvanceResult, error) {
	  p := s.openProof(ctx, ProofReplay, prev, "idempotent_replay", req.Actor)
	  return AdvanceResult{Gen: prev, HoldID: prev.HoldID, Broadcast: prev.BroadcastID, Idempotent: true, Proof: p}, nil
  }

  func (s *Service) advanceExisting(ctx context.Context, g LegHoldGen, snap wdshard.Snapshot, req AdvanceReq, now time.Time) (AdvanceResult, error) {
	   if err := MayAdvance(snap.Leg, &g, now); err != nil {
		  p := s.openProof(ctx,
			   proofKindOf(err), g, err.Error(), req.Actor)
		   return AdvanceResult{Gen: g,
			   Proof: p}, err
	   }
	  ok, err := s.store.TryLockLeg(ctx, g.PlanID, g.LegID, MutexAdvance)
	   if err != nil {
		   return AdvanceResult{}, err
	   }
	  if !ok {
		   p := s.openProof(ctx, ProofConflict, g, "advance_vs_release", req.Actor)
		   s.metrics.IncConflict()
		   return AdvanceResult{Gen: g, Proof: p}, ErrConflict
	  }
	   defer func() { _ = s.store.UnlockLeg(ctx, g.PlanID, g.LegID, MutexAdvance) }()
	   var hold wdshard.Hold
	   reuseHold := g.State == GenHeld && g.HoldID != "" && snap.Hold != nil && snap.Hold.Active && snap.Hold.Residual() > 0
	  if reuseHold {
		  hold = *snap.Hold
		   if snap.Leg.Status != wdshard.HoldLegFailed && snap.Leg.Status != wdshard.HoldLegPending && snap.Leg.Status != wdshard.HoldLegAdvancing {
			  p := s.openProof(ctx, ProofDoubleHold, g, "second_freeze", req.Actor)
			   s.metrics.IncDoubleHold()
			  return AdvanceResult{Gen: g,
				  Proof: p}, ErrDoubleHold
		  }
	  } else if snap.Hold != nil && snap.Hold.Active && snap.Hold.Residual() > 0 {
		   hold = *snap.Hold
	   } else {
		   hold, err = s.holds.Freeze(g.PlanID, g.LegID, g.UserID, g.Asset, g.Amount)
		  if err != nil {
			   if errors.Is(err, wdshard.ErrAlreadyHeld) {
				  p := s.openProof(ctx, ProofDoubleHold, g, err.Error(), req.Actor)
				   return AdvanceResult{Gen: g,
					  Proof: p}, ErrDoubleHold
			  }
			  return AdvanceResult{Gen: g}, err
		   }
	  }
	   g.HoldID = hold.HoldID
	   g.HoldAmt = hold.Amount
	   g.Fingerprint = Fingerprint(g.PlanID, g.LegID, g.HoldID, g.Amount, g.Generation)
	  if g.State == GenIssued {
		  if err := ApplyGenTransition(&g, GenHeld, now); err != nil {
			   return AdvanceResult{Gen: g}, err
		   }
	  }
	   g.AdvanceCount++
	   g.MutexOwner = ""
	  g.UpdatedAt = now
	  var half *HalfLegCase
	   bcID := ""
	  if req.SkipBroadcast {
		  half = s.openHalf(ctx, g, HalfFreezeNoBroadcast, req.Actor, "skip_broadcast")
		   if err := s.store.UpdateGen(ctx, g); err != nil {
			   return AdvanceResult{}, err
		   }
		  _ = s.audit(ctx, g.GenID, AuditAdvance, req.Actor, GenIssued, GenHeld, "held_no_broadcast")
		   s.metrics.IncHeld()
		  s.metrics.IncHalf(string(HalfFreezeNoBroadcast))
		  return AdvanceResult{Gen: g, HoldID: g.HoldID, HalfCase: half}, nil
	  }
	   leg, err := s.book.SubmitBroadcast(g.PlanID, g.LegID, "bc-"+g.GenID[:min(12, len(g.GenID))])
	  if err != nil || req.ForceFail {
		   reason := "broadcast_fail"
		  if err != nil {
			   reason = err.Error()
		   }
		   _,
			  _ = s.book.FailBroadcast(g.PlanID, g.LegID, reason)
		   half = s.openHalf(ctx,
			   g, HalfFreezeNoBroadcast, req.Actor, reason)
		  if err := s.store.UpdateGen(ctx, g); err != nil {
			   return AdvanceResult{Gen: g, HoldID: g.HoldID, HalfCase: half}, err
		   }
		  _ = s.audit(ctx, g.GenID, AuditAdvance, req.Actor, GenIssued, GenHeld, reason)
		  s.metrics.IncHeld()
		  return AdvanceResult{Gen: g,
			  HoldID: g.HoldID, HalfCase: half}, nil
	   }
	   bcID = leg.BroadcastID
	  g.BroadcastID = bcID
	  g.LiveHash = LiveHashFence(g.PlanID, g.LegID, g.HoldID, bcID, g.Generation)
	  if err := s.store.UpdateGen(ctx, g); err != nil {
		   return AdvanceResult{}, err
	   }
	   _ = s.audit(ctx, g.GenID, AuditAdvance, req.Actor, GenIssued, GenHeld, "broadcast")
	  s.metrics.IncHeld()
	   s.metrics.IncBroadcast()
	   return AdvanceResult{Gen: g, HoldID: g.HoldID, Broadcast: bcID}, nil
  }

// ReleaseAssert 对 failed 腿释放冻结。在播拒释；超额拒释；无世代 fail-closed。

  func (s *Service) ReleaseAssert(ctx context.Context, req ReleaseReq) (ReleaseResult, error) {
	  if s == nil || s.store == nil || s.book == nil {
		   return ReleaseResult{}, ErrUnavailable
	   }
	   planID := strings.TrimSpace(req.PlanID)
	  legID := strings.TrimSpace(req.LegID)
	   if planID == "" || legID == "" {
		   return ReleaseResult{},
			  ErrInvalidReq
	  }
	   now := s.now()
	  var g LegHoldGen
	  var err error
	   if strings.TrimSpace(req.GenID) != "" {
		   g, err = s.store.GetGen(ctx, req.GenID)
		   if err != nil {
			  p := s.openProof(ctx, ProofNoGeneration, LegHoldGen{PlanID: planID, LegID: legID}, "gen_not_found", req.Actor)
			   return ReleaseResult{Proof: p}, err
		   }
	   } else {
		   var ok bool
		  g, ok, err = s.store.GetGenByLeg(ctx, planID, legID)
		   if err != nil {
			  return ReleaseResult{}, err
		   }
		  if !ok {
			   p := s.openProof(ctx, ProofNoGeneration, LegHoldGen{PlanID: planID, LegID: legID}, "no_generation", req.Actor)
			  s.metrics.IncFailClosed()
			  return ReleaseResult{Proof: p}, ErrNoGeneration
		  }
	   }
	   if err := RequireGeneration(&g); err != nil {
		  p := s.openProof(ctx, ProofNoGeneration, g, "no_generation", req.Actor)
		  return ReleaseResult{Gen: g, Proof: p}, err
	   }
	   if g.State == GenReleased {
		   return ReleaseResult{Gen: g, Released: g.ReleaseAmt, Idempotent: true}, nil
	   }
	  snap, err := s.book.Snapshot(planID, legID)
	   if err != nil {
		   return ReleaseResult{Gen: g}, err
	   }
	   amount := req.Amount
	   if amount <= 0 {
		   amount = g.ResidualHold()
		   if amount <= 0 {
			   amount = snap.Leg.HoldAmt
		   }
		   if amount <= 0 {
			   amount = snap.Leg.Amount
		   }
	   }
	   if err := MayRelease(snap.Leg, &g, amount, now); err != nil {
		  kind := proofKindOf(err)
		  p := s.openProof(ctx,
			  kind, g, err.Error(), req.Actor)
		  if errors.Is(err, ErrBroadcastLive) {
			  s.metrics.IncLiveHash()
		   }
		   return ReleaseResult{Gen: g, Proof: p}, err
	  }
	   ok, err := s.store.TryLockLeg(ctx, g.PlanID, g.LegID, MutexRelease)
	  if err != nil {
		  return ReleaseResult{}, err
	  }
	  if !ok {
		   p := s.openProof(ctx, ProofConflict, g, "release_vs_advance", req.Actor)
		   s.metrics.IncConflict()
		   return ReleaseResult{Gen: g, Proof: p}, ErrConflict
	  }
	  defer func() { _ = s.store.UnlockLeg(ctx, g.PlanID, g.LegID, MutexRelease) }()
	  holdID := g.HoldID
	   if holdID == "" && snap.Hold != nil {
		  holdID = snap.Hold.HoldID
		  g.HoldID = holdID
		   g.HoldAmt = snap.Hold.Amount
	   }
	   if holdID == "" {
		   p := s.openProof(ctx, ProofMissRelease, g, "no_hold_to_release", req.Actor)
		  s.metrics.IncMissRelease()
		  return ReleaseResult{Gen: g, Proof: p}, ErrMissRelease
	   }
	  h,
		   err := s.holds.Release(holdID, amount)
	  if err != nil {
		   if errors.Is(err, wdshard.ErrHoldExceed) {
			   p := s.openProof(ctx, ProofOverRelease, g, err.Error(), req.Actor)
			  return ReleaseResult{Gen: g,
				  Proof: p}, ErrOverRelease
		   }
		   return ReleaseResult{Gen: g}, err
	   }
	   g.ReleaseAmt += amount
	  g.HoldAmt = h.Residual()
	   if err := ApplyGenTransition(&g, GenReleased, now); err != nil {
		   return ReleaseResult{Gen: g}, err
	  }
	   g.ReleaseCount++
	   g.UpdatedAt = now
	   if err := s.store.UpdateGen(ctx, g); err != nil {
		  return ReleaseResult{}, err
	  }
	   _ = s.audit(ctx, g.GenID, AuditRelease, req.Actor, GenHeld, GenReleased, fmt.Sprintf("released=%d", amount))
	  s.metrics.IncReleased()
	   return ReleaseResult{Gen: g, Released: amount}, nil
  }

// ResumeHalf 半截 Advance 续提，互斥。

  func (s *Service) ResumeHalf(ctx context.Context, req ResumeReq) (HalfLegCase, error) {
	   if s == nil || s.store == nil {
		  return HalfLegCase{}, ErrUnavailable
	   }
	   c, err := s.store.GetHalf(ctx, strings.TrimSpace(req.CaseID))
	  if err != nil {
		   return HalfLegCase{}, err
	   }
	   if c.State != HalfOpen {
		   return c, nil
	   }
	  if c.ResumeMu {
		   p := s.openProof(ctx, ProofHalfMutex, LegHoldGen{GenID: c.GenID, PlanID: c.PlanID, LegID: c.LegID, HoldID: c.HoldID}, "resume_mutex", req.Actor)
		  _ = p
		  return c, ErrHalfLocked
	   }
	   ok, err := s.store.TryLockLeg(ctx, c.PlanID, c.LegID, MutexResume)
	   if err != nil {
		   return c, err
	  }
	  if !ok {
		  c.ResumeMu = true
		  _ = s.store.UpdateHalf(ctx, c)
		   return c, ErrHalfLocked
	   }
	   defer func() { _ = s.store.UnlockLeg(ctx, c.PlanID, c.LegID, MutexResume) }()
	  now := s.now()
	  if req.Abandon {
		  if err := ApplyHalfTransition(&c, HalfAbandoned, actorOr(req.Actor), now); err != nil {
			   return c, err
		  }
		  if err := s.store.UpdateHalf(ctx, c); err != nil {
			  return c, err
		  }
		  _ = s.audit(ctx, c.GenID, AuditHalfAbandon, req.Actor, "", "", c.CaseID)
		  return c, nil
	   }
	  if !req.MarkOnly {
		   g,
			   err := s.store.GetGen(ctx, c.GenID)
		  if err != nil {
			  return c, err
		  }
		   if g.State != GenHeld {
			  return c, ErrResumeDenied
		  }
		  leg, err := s.book.SubmitBroadcast(c.PlanID, c.LegID, "bc-resume-"+c.CaseID[:min(8, len(c.CaseID))])
		  if err != nil {
			   return c, err
		   }
		   g.BroadcastID = leg.BroadcastID
		  g.LiveHash = LiveHashFence(g.PlanID,
			   g.LegID, g.HoldID, g.BroadcastID, g.Generation)
		  g.UpdatedAt = now
		  if err := s.store.UpdateGen(ctx, g); err != nil {
			   return c, err
		  }
	  }
	   if err := ApplyHalfTransition(&c, HalfResumed, actorOr(req.Actor), now); err != nil {
		   return c, err
	   }
	   if err := s.store.UpdateHalf(ctx, c); err != nil {
		  return c, err
	  }
	  _ = s.audit(ctx, c.GenID, AuditHalfResume, req.Actor, "", "", c.CaseID)
	  s.metrics.IncResume()
	   return c, nil
  }

// AlignWithGen AlignWorker 挂钩：在播腿推进成交，写世代挡写证明字段。

  func (s *Service) AlignWithGen(ctx context.Context, hint wdshard.AlignHint) (wdshard.Leg, error) {
	  if s == nil || s.book == nil {
		   return wdshard.Leg{}, ErrUnavailable
	  }
	   snap,
		   err := s.book.Snapshot(hint.PlanID, hint.LegID)
	   if err != nil {
		   return wdshard.Leg{}, err
	   }
	  if snap.Leg.Status != wdshard.HoldLegBroadcasting {
		   return snap.Leg, ErrAdvanceDenied
	  }
	  g, ok, err := s.store.GetGenByLeg(ctx, hint.PlanID, hint.LegID)
	  if err != nil {
		  return snap.Leg, err
	  }
	   if !ok || g.Generation <= 0 {
		  p := s.openProof(ctx, ProofNoGeneration, LegHoldGen{PlanID: hint.PlanID, LegID: hint.LegID}, "align_no_gen", "align")
		  _ = p
		   return snap.Leg, ErrNoGeneration
	  }
	  want := LiveHashFence(g.PlanID, g.LegID, g.HoldID, g.BroadcastID, g.Generation)
	   if g.LiveHash != "" && g.LiveHash != want {
		  p := s.openProof(ctx, ProofLiveHash, g, "align_live_hash", "align")
		   _ = p
		  return snap.Leg, ErrLiveHash
	  }
	  leg, err := s.align.Align(hint)
	   if err != nil {
		   return snap.Leg, err
	  }
	  _ = s.audit(ctx, g.GenID, AuditAlign, "align", g.State, g.State, hint.TxHash)
	   return leg, nil
  }

// ExpireGens TTL 巡检。held 过期不自动释金，落 miss-release 证明待运维。

  func (s *Service) ExpireGens(ctx context.Context) (int, error) {
	   if s == nil || s.store == nil {
		   return 0, ErrUnavailable
	  }
	   now := s.now()
	  list, err := s.store.ListExpiredActive(ctx, now, ExpireBatchSize)
	   if err != nil {
		   return 0, err
	   }
	  n := 0
	  for _, g := range list {
		   if g.State == GenHeld && g.ResidualHold() > 0 {
			  _ = s.openProof(ctx, ProofMissRelease, g, "ttl_held_unreleased", "expire")
			  s.metrics.IncMissRelease()
		  }
		   from := g.State
		   if err := ApplyGenTransition(&g, GenExpired, now); err != nil {
			  continue
		   }
		  if err := s.store.UpdateGen(ctx, g); err != nil {
			   continue
		   }
		   _ = s.audit(ctx, g.GenID, AuditExpire, "expire", from, GenExpired, "")
		   s.metrics.IncExpired()
		   n++
	   }
	   return n, nil
   }

// PatrolHalf 半截巡检。

   func (s *Service) PatrolHalf(ctx context.Context) (int, error) {
	  if s == nil || s.store == nil {
		  return 0, ErrUnavailable
	  }
	  list,
		   err := s.store.ListOpenHalf(ctx, PatrolBatchSize)
	   if err != nil {
		   return 0, err
	  }
	  n := 0
	  for _, c := range list {
		  _ = s.audit(ctx, c.GenID, AuditPatrol, "patrol", "", "", c.CaseID)
		  s.metrics.IncPatrol()
		   n++
	  }
	   return n, nil
   }

// ExportProofs 证明导出。
   func (s *Service) ExportProofs(ctx context.Context, filter ExportFilter) ([]ShardHoldProof, error) {
	  if s == nil || s.store == nil {
		  return nil, ErrUnavailable
	  }
	  if filter.Limit <= 0 {
		  filter.Limit = ProofExportBatch
	  }
	  list, err := s.store.ListProofs(ctx, filter)
	   if err != nil {
		  return nil, err
	  }
	  ids := make([]string, 0, len(list))
	   for _, p := range list {
		   ids = append(ids, p.ProofID)
	   }
	  if len(ids) > 0 {
		   _ = s.store.MarkProofsExported(ctx, ids, s.now())
		  s.metrics.AddExport(len(ids))
	   }
	   return list, nil
   }

   func (s *Service) GetGen(ctx context.Context, genID string) (LegHoldGen, error) {
	  return s.store.GetGen(ctx, genID)
   }

   func (s *Service) GetProof(ctx context.Context, proofID string) (ShardHoldProof, error) {
	  return s.store.GetProof(ctx, proofID)
   }

  func (s *Service) ListProofs(ctx context.Context, filter ExportFilter) ([]ShardHoldProof, error) {
	   return s.store.ListProofs(ctx, filter)
  }

   func (s *Service) ListAudit(ctx context.Context, genID string, limit int) ([]GenAudit, error) {
	  return s.store.ListAudit(ctx, genID, limit)
   }

  func (s *Service) openProof(ctx context.Context, kind ProofKind, g LegHoldGen, evidence, actor string) *ShardHoldProof {
	   if kind == "" {
		   kind = ProofConflict
	  }
	   now := s.now()
	   p := ShardHoldProof{
		   ProofID: newID("shp-", string(kind), g.PlanID, g.LegID, evidence, now.String()),
		  Kind:    kind,
		  GenID:   g.GenID,
		  PlanID:  g.PlanID,
		   LegID:   g.LegID,
		  HoldID:  g.HoldID,
		  IdemKey: g.IdempotencyKey,
		   Amount:   g.Amount,
		  BindGen:  g.Generation,
		  LiveGen:  g.Generation,
		  BindHash: g.Fingerprint,
		   LiveHash: g.LiveHash,
		  Evidence: evidence,
		  DetectedAt: now,
		   CreatedAt:  now,
	  }
	  p.Digest = ProofDigest(kind, g.PlanID, g.LegID, g.HoldID, p.BindGen, p.LiveGen, evidence)
	  if err := s.store.SaveProof(ctx, p); err != nil {
		  return nil
	  }
	   _ = s.audit(ctx, g.GenID, AuditProof, actor, g.State, g.State, string(kind))
	  s.metrics.IncProof(string(kind))
	   return &p
  }

  func (s *Service) openHalf(ctx context.Context, g LegHoldGen, kind HalfKind, actor, note string) *HalfLegCase {
	  now := s.now()
	   c := HalfLegCase{
		  CaseID: "hlc-" + uuid.NewString()[:12],
		   GenID:  g.GenID,
		   PlanID: g.PlanID,
		  LegID:  g.LegID,
		  HoldID: g.HoldID,
		   Kind:  kind,
		  State: HalfOpen,
		  Actor: actorOr(actor),
		  Note:  note,
		  CreatedAt: now,
		  UpdatedAt: now,
	   }
	   if err := s.store.SaveHalf(ctx, c); err != nil {
		  return nil
	   }
	   _ = s.audit(ctx, g.GenID, AuditHalfOpen, actor, g.State, g.State, string(kind))
	  return &c
  }

   func (s *Service) audit(ctx context.Context, genID string, act AuditAction, actor string, from, to GenState, note string) error {
	  a := GenAudit{
		   AuditID: "aud-" + uuid.NewString()[:12],
		   GenID:   genID, Action: act, Actor: actorOr(actor),
		  FromState: from, ToState: to, Note: note, At: s.now(),
	   }
	  return s.store.SaveAudit(ctx, a)
  }

   func proofKindOf(err error) ProofKind {
	  switch {
	   case errors.Is(err, ErrBroadcastLive):
		   return ProofBroadcastRel
	  case errors.Is(err, ErrOverRelease):
		  return ProofOverRelease
	   case errors.Is(err, ErrNoGeneration):
		  return ProofNoGeneration
	   case errors.Is(err, ErrGenExpired):
		  return ProofExpired
	  case errors.Is(err, ErrDoubleHold):
		  return ProofDoubleHold
	   case errors.Is(err, ErrMissRelease):
		   return ProofMissRelease
	   case errors.Is(err, ErrConflict):
		   return ProofConflict
	  case errors.Is(err, ErrLiveHash):
		  return ProofLiveHash
	   default:
		   return ProofGeneration
	   }
  }

   func min(a, b int) int {
	   if a < b {
		   return a
	   }
	  return b
   }
