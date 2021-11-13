package screenleasegate

import (
	"context"
	"strings"
	"time"
)

type Service struct {
	secret string
	screen Screener
	store  Store
	gen    *GenerationEngine
	fsm *LeaseFSM
	proof   *ProofEngine
	hold    *LeaseMutex
	metrics *GateMetrics
	audit   *AuditLog
	clock   func() time.Time
}

  func NewService(secret string, screen Screener, store Store) *Service {
	  if store == nil {
		  store = NewMemStore()
	   }
	   if secret == "" {
		   secret = "screenleasegate-local-hmac"
	   }
	   clock := func() time.Time { return time.Now().UTC() }
	   return &Service{
		   secret: strings.TrimSpace(secret),
		  screen: screen,
		   store:  store,
		   gen: newGenerationEngine(store, secret, clock),
		  fsm: newLeaseFSM(store, secret, clock),
		   proof: newProofEngine(store, secret, clock),
		  hold:    NewLeaseMutex(),
		  metrics: newGateMetrics(),
		  audit:   NewAuditLog(2048),
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

  func (s *Service) WithScreener(sc Screener) *Service {
	   if s != nil {
		  s.screen = sc
	   }
	  return s
  }

  func (s *Service) now() time.Time {
	  if s == nil || s.clock == nil {
		  return time.Now().UTC()
	   }
	   return s.clock()
  }

// Issue 在 Screen Allow 后签发 screen_lease_id。

  func (s *Service) Issue(ctx context.Context, req IssueReq) (*ScreenLease, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	  }
	  if s.screen == nil {
		   return nil, ErrUnavailable
	  }
	   if err := rejectIsolationFields(req.Nonce + req.Kind); err != nil {
		   s.metrics.denied.Add(1)
		  return nil, err
	  }
	  v, err := s.screen.Screen(toRiskReq(req))
	  if mapErr := mapScreenErr(err, v); mapErr != nil {
		  s.metrics.denied.Add(1)
		  s.audit.Append(AuditEvent{Kind: "issue_deny", UserID: req.UserID, Note: mapErr.Error(), At: s.now()})
		   return nil, mapErr
	   }
	  gen, err := s.gen.Current(ctx)
	  if err != nil {
		   return nil, err
	   }
	  if gen.PackFP == "" {
		  gen.PackFP = packFingerprint(s.secret, firstNonEmpty(v.Pack, req.Pack), firstNonEmpty(v.Mode, gen.Mode), "riskgate-engine")
	   }
	   win := snapshotWindows(s.screen.Windows(), req.UserID, req.Dest, 0)
	   lease, err := s.fsm.Issue(ctx, req, toScreenSnap(v), win, gen)
	  if err != nil {
		  s.metrics.denied.Add(1)
		  s.audit.Append(AuditEvent{Kind: "issue_deny", UserID: req.UserID, Note: err.Error(), At: s.now()})
		  return nil, err
	   }
	  s.metrics.issued.Add(1)
	   s.audit.Append(AuditEvent{Kind: "issue", LeaseID: lease.ID, UserID: lease.UserID, At: s.now()})
	  return &lease, nil
  }

// Submit 绑定并复验租约。失败即关并落漂移证明。
  func (s *Service) Submit(ctx context.Context, req SubmitReq) (*ScreenLease, *ScreenDriftProof, error) {
	   if s == nil {
		  return nil, nil, ErrUnavailable
	   }
	   if strings.TrimSpace(req.LeaseID) == "" {
		   p, _ := s.proof.OpenMissing(ctx, req.UserID, "submit without screen_lease_id")
		   s.metrics.submitDeny.Add(1)
		   s.metrics.proofs.Add(1)
		   s.audit.Append(AuditEvent{Kind: "submit_missing", UserID: req.UserID, ProofID: p.ID, At: s.now()})
		  return nil, &p, ErrLeaseMissing
	   }
	  if err := s.proof.UserBlocked(ctx, req.UserID); err != nil {
		  s.metrics.submitDeny.Add(1)
		  var attached *ScreenDriftProof
		   if req.LeaseID != "" {
			   if list, e := s.store.ListProofsByLease(ctx, req.LeaseID); e == nil && len(list) > 0 {
				  p := list[0]
				   attached = &p
			   }
		   }
		  return nil, attached, err
	  }
	  ticket := req.Ticket
	  if ticket == "" {
		  ticket = req.LeaseID
	  }
	  if err := s.hold.TryHold(req.LeaseID, ticket); err != nil {
		  s.metrics.submitDeny.Add(1)
		   return nil, nil, err
	  }
	   defer s.hold.Release(req.LeaseID, ticket)
	   gen, err := s.gen.Current(ctx)
	   if err != nil {
		   return nil, nil, err
	   }
	  leaseHint,
		   _ := s.store.GetLease(ctx, req.LeaseID)
	  win := WindowSnap{}
	  if s.screen != nil {
		   win = snapshotWindows(s.screen.Windows(), firstNonEmpty(req.UserID, leaseHint.UserID), firstNonEmpty(req.Dest, leaseHint.Dest), 0)
		   // 复验时必须使用租约冻结的窗摘要对应快照；若 Screen 后无新窗，摘要应一致。
		  if leaseHint.ID != "" {
			   win = WindowSnap{
				   Velocity1h: leaseHint.Velocity1h,
				   Velocity24h: leaseHint.Velocity24h,
				  DestCount:    leaseHint.DestCount,
				   DestCount24h: leaseHint.DestCount24h,
			   }
		   }
	   }
	  lease, err := s.fsm.Bind(ctx, req, gen, win)
	  if err != nil {
		  kind := kindFromErr(err)
		  p, _ := s.proof.Open(ctx, lease, kind, gen, err.Error())
		   s.metrics.submitDeny.Add(1)
		   s.metrics.proofs.Add(1)
		   s.audit.Append(AuditEvent{Kind: "submit_deny", LeaseID: req.LeaseID, UserID: req.UserID, ProofID: p.ID, Note: err.Error(), At: s.now()})
		   return &lease, &p, err
	   }
	   s.metrics.bound.Add(1)
	  s.metrics.submitOK.Add(1)
	  if lease.Status == LeaseConsumed {
		  s.metrics.consumed.Add(1)
	   }
	   s.audit.Append(AuditEvent{Kind: "submit_ok", LeaseID: lease.ID, UserID: lease.UserID, At: s.now()})
	  return &lease, nil, nil
  }

// SetHalt 抬 Halt 世代并吊销未终态租约。
  func (s *Service) SetHalt(ctx context.Context, req SetHaltReq) (ScreenGeneration, int, error) {
	   if s == nil {
		   return ScreenGeneration{}, 0, ErrUnavailable
	  }
	  if _, err := normalizeActor(req.Operator); err != nil {
		  return ScreenGeneration{},
			   0, ErrOperatorInvalid
	  }
	   gen, err := s.gen.BumpHalt(ctx, req.On)
	   if err != nil {
		  return ScreenGeneration{},
			   0, err
	  }
	   s.metrics.haltBumps.Add(1)
	   n, leases, err := s.fsm.RevokeOpen(ctx, "halt_flip")
	  if err != nil {
		   return gen, n, err
	  }
	   s.metrics.revoked.Add(int64(n))
	   for _, l := range leases {
		  _, _ = s.proof.Open(ctx, l, ProofHaltFlip, gen, "set_halt revoked lease")
		   s.metrics.proofs.Add(1)
	   }
	   s.audit.Append(AuditEvent{Kind: "halt_bump", Note: req.Operator, At: s.now()})
	   return gen, n, nil
  }

// FlipPack 抬 Pack/Engine 世代并吊销未终态租约。

   func (s *Service) FlipPack(ctx context.Context, req FlipPackReq) (ScreenGeneration, int, error) {
	   if s == nil {
		   return ScreenGeneration{}, 0, ErrUnavailable
	  }
	   if _, err := normalizeActor(req.Operator); err != nil {
		  return ScreenGeneration{}, 0, ErrOperatorInvalid
	   }
	  if strings.TrimSpace(req.Pack) == "" {
		   return ScreenGeneration{}, 0, ErrPackInvalid
	   }
	   gen, err := s.gen.BumpPack(ctx, req.Pack, req.Mode)
	  if err != nil {
		  return ScreenGeneration{}, 0, err
	   }
	  s.metrics.packBumps.Add(1)
	   n, leases, err := s.fsm.RevokeOpen(ctx, "pack_flip")
	   if err != nil {
		   return gen, n, err
	   }
	   s.metrics.revoked.Add(int64(n))
	   for _, l := range leases {
		  _, _ = s.proof.Open(ctx, l, ProofPackFlip, gen, "flip_pack revoked lease")
		  s.metrics.proofs.Add(1)
	  }
	  s.audit.Append(AuditEvent{Kind: "pack_bump", Note: req.Pack, At: s.now()})
	   return gen, n, nil
   }

  func (s *Service) GetLease(ctx context.Context, id string) (ScreenLease, error) {
	  if s == nil {
		  return ScreenLease{}, ErrUnavailable
	  }
	  return s.store.GetLease(ctx, id)
   }

   func (s *Service) GetProof(ctx context.Context, id string) (ScreenDriftProof, error) {
	  if s == nil {
		  return ScreenDriftProof{}, ErrUnavailable
	  }
	  return s.store.GetProof(ctx, id)
   }

  func (s *Service) Generation(ctx context.Context) (ScreenGeneration, error) {
	   if s == nil {
		   return ScreenGeneration{}, ErrUnavailable
	   }
	  return s.gen.Current(ctx)
  }

   func (s *Service) FourEye(ctx context.Context, req FourEyeReq) (ScreenDriftProof, error) {
	   if s == nil {
		  return ScreenDriftProof{},
			   ErrUnavailable
	  }
	  p, err := s.proof.FourEyeClose(ctx, req)
	  if err != nil {
		  return ScreenDriftProof{}, err
	  }
	   s.audit.Append(AuditEvent{Kind: "four_eye", ProofID: p.ID, Note: req.Reason, At: s.now()})
	   return p, nil
   }

   func (s *Service) ExpireSweep(ctx context.Context) (int, error) {
	   if s == nil {
		   return 0, ErrUnavailable
	  }
	   n, err := s.fsm.ExpireDue(ctx)
	   if err != nil {
		  return n, err
	   }
	   s.metrics.expired.Add(int64(n))
	  return n, nil
   }

   func (s *Service) Stats() GateStats {
	   if s == nil {
		  return GateStats{}
	   }
	  return s.metrics.Snapshot()
   }

  func (s *Service) Audit() []AuditEvent {
	   if s == nil {
		  return nil
	   }
	  return s.audit.List()
   }

  func (s *Service) ListOpenProofs(ctx context.Context) ([]ScreenDriftProof, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	   }
	   return s.store.ListOpenProofs(ctx)
  }

   func (s *Service) ListProofsByLease(ctx context.Context, leaseID string) ([]ScreenDriftProof, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	  }
	   return s.store.ListProofsByLease(ctx, leaseID)
   }

  func (s *Service) ListOpenLeases(ctx context.Context) ([]ScreenLease, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	  return s.store.ListOpenLeases(ctx)
  }
