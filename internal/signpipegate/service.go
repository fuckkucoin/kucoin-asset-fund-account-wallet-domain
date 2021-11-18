package signpipegate

import (
	"context"
	"strings"
	"time"
)

// Service 引擎门面。
type Service struct {
	secret string
	obs JobObserver
	store Store
	gen *GenerationEngine
	fsm *LeaseFSM
	fault *FaultInjector
	close *CloseoutEngine
	art *ArtifactBinder
	hold    *MovementMutex
	metrics *GateMetrics
	audit *AuditLog
	clock func() time.Time
}

  func NewService(secret string, observer JobObserver, store Store) *Service {
	  if store == nil {
		   store = NewMemStore()
	  }
	  clock := func() time.Time { return time.Now().UTC() }
	  gen := newGenerationEngine(store,
		   clock)
	   fsm := newLeaseFSM(store, gen, clock)
	  return &Service{
		   secret: strings.TrimSpace(secret),
		   obs:    observer,
		   store: store,
		   gen:   gen,
		   fsm:   fsm,
		  fault: newFaultInjector(store, observer, clock),
		   close:   newCloseoutEngine(store, secret, clock),
		  art:     newArtifactBinder(store, secret, clock),
		  hold:    NewMovementMutex(),
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
	   s.fault.clock = clock
	  s.close.clock = clock
	   s.art.clock = clock
	  return s
   }

   func (s *Service) WithObserver(obs JobObserver) *Service {
	  if s != nil {
		  s.obs = obs
		   s.fault.obs = obs
	  }
	   return s
  }

  func (s *Service) now() time.Time {
	  if s == nil || s.clock == nil {
		   return time.Now().UTC()
	  }
	  return s.clock()
   }

// ArmRetry 武装在途重试租约。

  func (s *Service) ArmRetry(ctx context.Context, req ArmReq) (*InflightLease, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	  }
	  s.metrics.armTotal.Add(1)
	   if err := distinctActors(req.RequestedBy, req.ApprovedBy, req.Actor); err != nil {
		  s.metrics.armDenied.Add(1)
		   return nil, err
	  }
	   lease, err := s.fsm.Arm(ctx, req)
	   if err != nil {
		  s.metrics.armDenied.Add(1)
		   s.audit.Append(AuditEvent{Kind: "arm_deny", MovementID: req.MovementID, Note: err.Error(), At: s.now()})
		   return nil, err
	   }
	   s.metrics.armOK.Add(1)
	  s.audit.Append(AuditEvent{Kind: "arm", MovementID: lease.MovementID, LeaseID: lease.ID, Generation: lease.Generation, At: s.now()})
	  return lease, nil
  }

// Enter 进入在途窗口。

   func (s *Service) Enter(ctx context.Context, leaseID string) (*InflightLease, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	  }
	  lease, err := s.store.GetLease(ctx, leaseID)
	   if err != nil {
		  return nil, err
	   }
	   if err := s.hold.TryHold(lease.MovementID, lease.ID); err != nil {
		   return nil, err
	  }
	  got, err := s.fsm.EnterInflight(ctx, leaseID)
	   if err != nil {
		   s.hold.Release(lease.MovementID,
			  lease.ID)
		  return nil, err
	  }
	   s.metrics.enterOK.Add(1)
	   return got, nil
   }

// Seal 封口并绑定产物。
   func (s *Service) Seal(ctx context.Context, req SealReq) (*InflightLease, *SignedArtifact, error) {
	   if s == nil {
		  return nil, nil, ErrUnavailable
	  }
	   s.metrics.sealTotal.Add(1)
	  if err := validateActor(req.Actor); err != nil {
		  s.metrics.sealDenied.Add(1)
		   return nil, nil, err
	  }
	  lease,
		  err := s.store.GetLease(ctx, req.LeaseID)
	   if err != nil {
		   s.metrics.sealDenied.Add(1)
		   return nil, nil, err
	   }
	  if lease.ExpiredAt(s.now()) {
		  s.metrics.sealDenied.Add(1)
		  _, _ = s.close.Open(ctx, lease, CloseTTL, "seal after ttl")
		   return nil, nil, s.fsm.expire(ctx, lease)
	  }
	  if req.FenceToken != "" && (req.FenceEpoch != lease.FenceEpoch || req.FenceToken != lease.FenceToken) {
		   s.metrics.sealDenied.Add(1)
		   return nil, nil, ErrFenceStale
	  }
	   sig := strings.TrimSpace(req.SigHex)
	  if sig == "" && s.obs != nil {
		   if job, err := s.obs.Observe(ctx, lease.JobID); err == nil && job != nil {
			  sig = job.SigHex
		   }
	   }
	  if sig == "" {
		   s.metrics.sealDenied.Add(1)
		   return nil, nil, ErrSignerNil
	   }
	   sealed, err := s.fsm.Seal(ctx, lease, sig)
	  if err != nil {
		  s.metrics.sealDenied.Add(1)
		   return nil, nil, err
	   }
	  art, err := s.art.Bind(ctx, sealed, sig)
	  if err != nil {
		  s.metrics.sealDenied.Add(1)
		  if err == ErrDoubleArtifact {
			  _, _ = s.close.Open(ctx, sealed, CloseDoubleGuard, "second artifact refused")
		  }
		  return nil, nil, err
	   }
	   s.hold.Release(sealed.MovementID, sealed.ID)
	   s.metrics.sealOK.Add(1)
	   s.audit.Append(AuditEvent{Kind: "seal", MovementID: sealed.MovementID, LeaseID: sealed.ID, Generation: sealed.Generation, At: s.now()})
	   return sealed, art, nil
  }

// InjectFault 落故障案并观测。

  func (s *Service) InjectFault(ctx context.Context, req InjectReq) (*FaultCase, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	  s.metrics.faultTotal.Add(1)
	  lease, err := s.store.GetLease(ctx, req.LeaseID)
	   if err != nil {
		  return nil, err
	  }
	  fc, err := s.fault.Inject(ctx, lease, req.Kind, req.Operator)
	   if fc != nil && faultBlocksRetry(req.Kind) {
		   kind := CloseFault
		  if req.Kind == FaultExpire {
			   kind = CloseTTL
		  }
		   if req.Kind == FaultRetryExhausted {
			   kind = CloseExhausted
		   }
		  if req.Kind == FaultDualBroadcast {
			  kind = CloseDoubleGuard
		   }
		   _, _ = s.close.Open(ctx, lease, kind, describeFault(req.Kind))
		   if !lease.Terminal() && (req.Kind == FaultExpire || req.Kind == FaultRetryExhausted) {
			  _, _ = s.fsm.Abort(ctx, lease.ID, describeFault(req.Kind))
		   }
	  }
	  if err != nil {
		   s.metrics.faultObserved.Add(1)
		  s.audit.Append(AuditEvent{Kind: "fault", MovementID: lease.MovementID, LeaseID: lease.ID, Note: string(req.Kind), At: s.now()})
		  return fc, err
	  }
	  s.metrics.faultObserved.Add(1)
	   return fc, nil
   }

// Abort 运维中止。

   func (s *Service) Abort(ctx context.Context, req AbortReq) (*InflightLease, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	  }
	   if err := validateOperator(req.Operator); err != nil {
		   return nil, err
	  }
	   if err := validateReason(req.Reason); err != nil {
		   return nil, err
	   }
	   lease, err := s.fsm.Abort(ctx, req.LeaseID, req.Reason)
	   if err != nil {
		  return nil, err
	   }
	  _, _ = s.close.Open(ctx, lease, CloseAbort, req.Reason)
	   s.hold.Release(lease.MovementID, lease.ID)
	  s.audit.Append(AuditEvent{Kind: "abort", MovementID: lease.MovementID, LeaseID: lease.ID, Note: req.Reason, At: s.now()})
	  return lease, nil
  }

  func (s *Service) CloseFault(ctx context.Context, req FaultCloseReq) (*FaultCase, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	   }
	  return s.fault.Close(ctx, req.FaultID, req.Operator, req.Reason)
   }

   func (s *Service) CloseProof(ctx context.Context, req CloseoutCloseReq) (*CloseoutProof, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	   return s.close.Close(ctx,
		  req.ProofID, req.Operator, req.Reason)
   }

  func (s *Service) GetLease(ctx context.Context, id string) (*InflightLease, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	  return s.store.GetLease(ctx, id)
  }

   func (s *Service) GetGeneration(ctx context.Context, movementID, jobID string) (int64, error) {
	  if s == nil {
		   return 0, ErrUnavailable
	  }
	  return s.gen.Current(ctx, movementID, jobID)
   }

   func (s *Service) GetArtifact(ctx context.Context, movementID string) (*SignedArtifact, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	  }
	   return s.art.Get(ctx, movementID)
  }

  func (s *Service) GetFault(ctx context.Context, id string) (*FaultCase, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	   }
	  return s.store.GetFault(ctx, id)
   }

   func (s *Service) GetCloseout(ctx context.Context, id string) (*CloseoutProof, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	   }
	  return s.store.GetCloseout(ctx, id)
   }

   func (s *Service) Stats() GateStats {
	   if s == nil || s.metrics == nil {
		   return GateStats{}
	  }
	  return s.metrics.Snapshot()
   }

  func (s *Service) AuditTail(n int) []AuditEvent {
	   if s == nil || s.audit == nil {
		  return nil
	   }
	  return s.audit.Tail(n)
  }

  func (s *Service) Store() Store {
	   if s == nil {
		  return nil
	   }
	   return s.store
   }
