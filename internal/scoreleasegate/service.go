// @FileName: service.go
// @Summary: scoreleasegate 顶层服务——EvalWithLease、Submit 复验、Halt/Pack/Mode/Window 吊销。
// 核心调用流（EvalWithLease / Issue）：
// 1. 规范化用户/资产/金额/目的地/链/nonce
// 2. 只读调用 riskscore.Eval；非 Enforce Allow 失败即关，不签发
// 3. Shadow 命中落 shadow 证明，不签发 Enforce 租约
// 4. 冻结 Halt/Pack/Mode/Window 世代与窗摘要，落 issued 租约
// 核心调用流（Submit / AssertLease）：
// 1. 无 score_lease_id → 落 missing 证明并拒
// 2. 复验用户/金额/目的地/资产/链/世代/窗摘要
// 3. Halt / Pack / Mode / Window 漂移 → 拒并落 ScoreDriftProof
// 4. consume=true 推进 consumed
// 产出：score_lease_id。不写 Screen 租约、不写图谱限额、不写限额 Check→Commit。
// @Author: KianFord
// @Created: 2021.11.03

package scoreleasegate

import (
	"context"
	"strings"
	"time"
)

type Service struct {
	secret string
	eval   Evaluator
	store  Store
	gen     *GenerationEngine
	fsm     *LeaseFSM
	proof   *ProofEngine
	hold    *LeaseMutex
	metrics *GateMetrics
	audit *AuditLog
	clock func() time.Time
}

  func NewService(secret string, eval Evaluator, store Store) *Service {
	   if store == nil {
		  store = NewMemStore()
	   }
	  if secret == "" {
		   secret = "scoreleasegate-local-hmac"
	  }
	  clock := func() time.Time { return time.Now().UTC() }
	  return &Service{
		   secret: strings.TrimSpace(secret),
		   eval:  eval,
		  store: store,
		  gen:   newGenerationEngine(store, secret, clock),
		   fsm:   newLeaseFSM(store, secret, clock),
		  proof: newProofEngine(store, secret, clock),
		   hold:    NewLeaseMutex(),
		   metrics: newGateMetrics(),
		   audit: NewAuditLog(4096),
		   clock: clock,
	   }
   }

  func (s *Service) WithClock(clock func() time.Time) *Service {
	   if s == nil || clock == nil {
		   return s
	   }
	   s.clock = clock
	  s.gen.clock = clock
	   s.fsm.clock = clock
	   s.proof.clock = clock
	   return s
  }

  func (s *Service) WithEvaluator(ev Evaluator) *Service {
	   if s != nil {
		   s.eval = ev
	   }
	  return s
   }

   func (s *Service) now() time.Time {
	  if s == nil || s.clock == nil {
		  return time.Now().UTC()
	  }
	  return s.clock()
   }

   func (s *Service) Store() Store { return s.store }

   func (s *Service) Stats() GateStats {
	  if s == nil || s.metrics == nil {
		   return GateStats{}
	  }
	   return s.metrics.Snapshot()
  }

// EvalWithLease 在 riskscore Eval Allow 后签发 score_lease_id。
   func (s *Service) EvalWithLease(ctx context.Context, req IssueReq) (*ScoreLease, error) {
	  return s.Issue(ctx, req)
  }

  func (s *Service) Issue(ctx context.Context, req IssueReq) (*ScoreLease, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	   if s.eval == nil {
		   return nil, ErrUnavailable
	  }
	  if err := rejectIsolationFields(req.Nonce + req.Kind + req.Note); err != nil {
		   s.metrics.denied.Add(1)
		  return nil, err
	  }
	   snap, err := s.eval.Eval(ctx, req)
	   if err != nil {
		   s.metrics.denied.Add(1)
		   s.audit.Append(AuditEvent{Kind: "issue_deny", UserID: req.UserID, Note: err.Error(), At: s.now()})
		   return nil, mapEvalErr(err)
	   }
	   if snap.Mode == "shadow" {
		  _, _ = s.proof.OpenShadow(ctx, req.UserID, "shadow allow cannot mint enforce lease")
		   s.metrics.denied.Add(1)
		   s.audit.Append(AuditEvent{Kind: "shadow_deny", UserID: req.UserID, At: s.now()})
		   return nil, ErrShadowLease
	   }
	  if !snap.Allow {
		   s.metrics.denied.Add(1)
		  s.audit.Append(AuditEvent{Kind: "issue_deny", UserID: req.UserID, Note: "no allow", At: s.now()})
		  return nil, ErrEvalDenied
	   }
	   gen, err := s.gen.Current(ctx)
	  if err != nil {
		   return nil, err
	   }
	  if gen.PackFP == "" {
		  gen.PackFP = packFingerprint(s.secret, firstNonEmpty(snap.Pack, req.Pack), firstNonEmpty(snap.Mode, gen.Mode), "riskscore-engine")
	  }
	   if snap.Window.Digest == "" {
		   snap.Window.Digest = windowDigest(s.secret, snap.Window)
	   }
	  if gen.WindowDigest == "" {
		   _, _ = s.gen.BumpWindow(ctx, snap.Window.Digest)
		  gen, err = s.gen.Current(ctx)
		   if err != nil {
			  return nil, err
		   }
	  }
	  lease, err := s.fsm.Issue(ctx, req, snap, gen)
	  if err != nil {
		   s.metrics.denied.Add(1)
		   s.audit.Append(AuditEvent{Kind: "issue_deny", UserID: req.UserID, Note: err.Error(), At: s.now()})
		  return nil, err
	  }
	  s.metrics.issued.Add(1)
	  s.audit.Append(AuditEvent{Kind: "issue", LeaseID: lease.ID, UserID: lease.UserID, At: s.now()})
	   _ = s.store.PutAudit(ctx, AuditEvent{Kind: "issue", LeaseID: lease.ID, UserID: lease.UserID, At: s.now()})
	   return &lease, nil
  }

// Submit / AssertLease 出金冻结或广播提交前复验租约。

  func (s *Service) AssertLease(ctx context.Context, req SubmitReq) (*ScoreLease, *ScoreDriftProof, error) {
	   return s.Submit(ctx, req)
  }

   func (s *Service) Submit(ctx context.Context, req SubmitReq) (*ScoreLease, *ScoreDriftProof, error) {
	   if s == nil {
		  return nil, nil, ErrUnavailable
	  }
	   if strings.TrimSpace(req.ScoreLeaseID) == "" {
		   p, _ := s.proof.OpenMissing(ctx, req.UserID, "score_lease_id missing")
		  s.metrics.denied.Add(1)
		   s.audit.Append(AuditEvent{Kind: "missing", UserID: req.UserID, ProofID: p.ID, At: s.now()})
		   return nil, &p, ErrLeaseMissing
	  }
	   if !s.hold.TryLock(req.ScoreLeaseID) {
		   s.metrics.denied.Add(1)
		  return nil, nil, ErrConcurrent
	  }
	   defer s.hold.Unlock(req.ScoreLeaseID)
	  lease, err := s.store.GetLease(ctx, req.ScoreLeaseID)
	   if err != nil {
		   p, _ := s.proof.OpenMissing(ctx, req.UserID, "lease not found")
		  s.metrics.denied.Add(1)
		  return nil, &p, err
	   }
	  now := s.now()
	   if lease.ExpiredAt(now) {
		  lease, _ = s.fsm.Expire(ctx, lease, now)
		   p, _ := s.proof.OpenExpired(ctx, lease, "ttl elapsed before submit")
		  s.metrics.denied.Add(1)
		   s.metrics.expired.Add(1)
		  return &lease, &p, ErrLeaseExpired
	  }
	  if err := matchSubmitFields(lease, req); err != nil {
		   gen, _ := s.gen.Current(ctx)
		   kind := ProofAmountDrift
		   if err == ErrLeaseDest {
			  kind = ProofDestDrift
		  }
		   if err == ErrLeaseChain || err == ErrLeaseAsset {
			   kind = ProofFingerprint
		  }
		   p, _ := s.proof.Open(ctx, lease, kind, gen, err.Error())
		  s.metrics.denied.Add(1)
		  return &lease, &p, err
	   }
	  ticket, err := normalizeTicket(req.Ticket)
	   if err != nil {
		   return &lease, nil, err
	   }
	   if _, err := normalizeActor(req.Actor); err != nil {
		  return &lease, nil, err
	  }
	  gen, err := s.gen.Current(ctx)
	   if err != nil {
		   return &lease, nil, err
	   }
	  if merr := MatchLease(lease, gen); merr != nil {
		   p,
			  _ := s.proof.Open(ctx, lease, kindFromMatchErr(merr), gen, merr.Error())
		  lease, _ = s.fsm.Revoke(ctx, lease, now, merr.Error())
		   s.metrics.denied.Add(1)
		  s.metrics.revoked.Add(1)
		  s.metrics.proofs.Add(1)
		   s.audit.Append(AuditEvent{Kind: "drift", LeaseID: lease.ID, ProofID: p.ID, Note: merr.Error(), At: now})
		  return &lease, &p, merr
	  }
	   bound, err := s.fsm.Bind(ctx, lease, ticket, now)
	  if err != nil {
		   s.metrics.denied.Add(1)
		   return &bound, nil, err
	   }
	  s.metrics.bound.Add(1)
	  if req.Consume {
		   bound,
			   err = s.fsm.Consume(ctx, bound, now)
		  if err != nil {
			   s.metrics.denied.Add(1)
			   return &bound, nil, err
		  }
		  s.metrics.consumed.Add(1)
	   }
	  s.audit.Append(AuditEvent{Kind: "submit", LeaseID: bound.ID, UserID: bound.UserID, Actor: req.Actor, At: now})
	   _ = s.store.PutAudit(ctx, AuditEvent{Kind: "submit", LeaseID: bound.ID, UserID: bound.UserID, Actor: req.Actor, At: now})
	   return &bound, nil, nil
   }

  func (s *Service) SetHalt(ctx context.Context, req SetHaltReq) (ScoreGeneration, int, error) {
	  if _, err := normalizeActor(req.Actor); err != nil {
		   return ScoreGeneration{}, 0, err
	  }
	   gen, err := s.gen.BumpHalt(ctx, req.On)
	  if err != nil {
		   return ScoreGeneration{}, 0, err
	  }
	   n, err := s.revokeOpen(ctx, gen, ProofHaltFlip, "halt flip")
	   if err != nil {
		  return gen, n, err
	   }
	   s.metrics.haltBumps.Add(1)
	   s.audit.Append(AuditEvent{Kind: "halt", Actor: req.Actor, Note: req.Reason, At: s.now()})
	  return gen, n, nil
   }

  func (s *Service) SetPack(ctx context.Context, req SetPackReq) (ScoreGeneration, int, error) {
	   if _, err := normalizeActor(req.Actor); err != nil {
		  return ScoreGeneration{}, 0, err
	  }
	   gen, err := s.gen.Current(ctx)
	  if err != nil {
		   return ScoreGeneration{}, 0, err
	   }
	  gen,
		  err = s.gen.BumpPack(ctx, req.Pack, gen.Mode)
	   if err != nil {
		   return ScoreGeneration{}, 0, err
	   }
	  n, err := s.revokeOpen(ctx, gen, ProofPackFlip, "pack flip")
	  if err != nil {
		   return gen, n, err
	   }
	   s.metrics.packBumps.Add(1)
	   return gen, n, nil
   }

   func (s *Service) SetMode(ctx context.Context, req SetModeReq) (ScoreGeneration, int, error) {
	   if _, err := normalizeActor(req.Actor); err != nil {
		   return ScoreGeneration{}, 0, err
	   }
	  mode := normalizeMode(req.Mode)
	   if mode != "enforce" && mode != "shadow" {
		   return ScoreGeneration{}, 0, ErrModeInvalid
	  }
	  gen, err := s.gen.BumpMode(ctx, mode)
	   if err != nil {
		   return ScoreGeneration{}, 0, err
	  }
	  n, err := s.revokeOpen(ctx, gen, ProofModeFlip, "mode flip")
	  if err != nil {
		  return gen, n, err
	  }
	   s.metrics.modeBumps.Add(1)
	  return gen, n, nil
   }

   func (s *Service) BumpWindow(ctx context.Context, req BumpWindowReq) (ScoreGeneration, int, error) {
	   if _, err := normalizeActor(req.Actor); err != nil {
		  return ScoreGeneration{}, 0, err
	  }
	   digest := strings.TrimSpace(req.Digest)
	   if digest == "" {
		   digest = windowDigest(s.secret, WindowSnap{Velocity1h: int(s.now().Unix() % 97)})
	   }
	  gen, err := s.gen.BumpWindow(ctx, digest)
	  if err != nil {
		  return ScoreGeneration{}, 0, err
	  }
	   n, err := s.revokeOpen(ctx, gen, ProofWindowDrift, "window jump")
	   if err != nil {
		  return gen, n, err
	   }
	   s.metrics.windowBump.Add(1)
	   return gen, n, nil
  }

  func (s *Service) Revoke(ctx context.Context, req RevokeReq) (*ScoreLease, error) {
	  if _, err := normalizeActor(req.Actor); err != nil {
		  return nil, err
	  }
	   if strings.TrimSpace(req.Reason) == "" {
		  return nil, ErrReasonRequired
	   }
	  lease,
		   err := s.store.GetLease(ctx, req.ScoreLeaseID)
	  if err != nil {
		  return nil, err
	   }
	  got, err := s.fsm.Revoke(ctx, lease, s.now(), req.Reason)
	  if err != nil {
		  return nil, err
	   }
	   s.metrics.revoked.Add(1)
	   return &got, nil
   }

  func (s *Service) ExpireSweep(ctx context.Context) (int, error) {
	   open, err := s.store.ListOpenLeases(ctx)
	  if err != nil {
		  return 0, err
	  }
	   now := s.now()
	  n := 0
	   for _, l := range open {
		   if !l.ExpiredAt(now) {
			   continue
		  }
		  if _, err := s.fsm.Expire(ctx, l, now); err != nil {
			  return n, err
		   }
		  _, _ = s.proof.OpenExpired(ctx, l, "expire sweep")
		  n++
		  s.metrics.expired.Add(1)
	  }
	   return n, nil
  }

  func (s *Service) FourEye(ctx context.Context, req FourEyeReq) (*ScoreDriftProof, error) {
	   p, err := s.proof.CloseFourEye(ctx, req.ProofID, req.ActorA, req.ActorB, req.Reason)
	   if err != nil {
		   return nil, err
	  }
	   s.audit.Append(AuditEvent{Kind: "four_eye", ProofID: p.ID, Actor: req.ActorA, Note: req.Reason, At: s.now()})
	  return &p, nil
  }

   func (s *Service) GetLease(ctx context.Context, id string) (ScoreLease, error) {
	   return s.store.GetLease(ctx, id)
  }

   func (s *Service) GetProof(ctx context.Context,
	   id string) (ScoreDriftProof, error) {
	  return s.store.GetProof(ctx, id)
   }

  func (s *Service) Generation(ctx context.Context) (ScoreGeneration, error) {
	   return s.gen.Current(ctx)
  }

  func (s *Service) Export(ctx context.Context, leaseID string) (ExportBundle, error) {
	   lease, err := s.store.GetLease(ctx, leaseID)
	  if err != nil {
		  return ExportBundle{}, err
	   }
	  proofs,
		  err := s.store.ListProofsByLease(ctx, leaseID)
	  if err != nil {
		   return ExportBundle{}, err
	  }
	   gen, err := s.gen.Current(ctx)
	  if err != nil {
		  return ExportBundle{}, err
	   }
	  return ExportBundle{
		   Lease:  lease,
		  Proofs: proofs,
		   Gen:    gen,
		  Mark: IsolationMark,
		  Host: "kyt.kucoin.net",
	   }, nil
   }

   func (s *Service) ListUserOpen(ctx context.Context, user string) ([]ScoreLease, error) {
	   return s.store.ListUserOpen(ctx, user)
   }

  func (s *Service) ListOpenProofs(ctx context.Context) ([]ScoreDriftProof, error) {
	  return s.store.ListOpenProofs(ctx)
   }

  func (s *Service) revokeOpen(ctx context.Context, gen ScoreGeneration, kind ProofKind, note string) (int, error) {
	   open, err := s.store.ListOpenLeases(ctx)
	  if err != nil {
		   return 0, err
	   }
	   now := s.now()
	   n := 0
	   for _, l := range open {
		  if _, err := s.fsm.Revoke(ctx, l, now, note); err != nil {
			  return n, err
		  }
		  if _, err := s.proof.Open(ctx, l, kind, gen, note); err != nil {
			  return n, err
		   }
		   n++
		   s.metrics.revoked.Add(1)
		   s.metrics.proofs.Add(1)
	   }
	  return n, nil
  }

   func mapEvalErr(err error) error {
	  if err == nil {
		  return nil
	  }
	  msg := strings.ToLower(err.Error())
	  if strings.Contains(msg, "halt") {
		   return ErrEvalHalt
	  }
	  return ErrEvalFailed
  }
