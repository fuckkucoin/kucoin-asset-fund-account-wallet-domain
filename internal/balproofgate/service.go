package balproofgate

import (
	"context"
	"strings"
	"time"
)

// Service 引擎门面。
type Service struct {
	secret string
	obs    LiveObserver
	store Store
	gen *GenerationEngine
	fsm *LeaseFSM
	drift   *DriftDetector
	revoke  *RevokeGuard
	metrics *GateMetrics
	audit *AuditLog
	clock func() time.Time
}

// NewService 构造。observer 为 nil 时签发失败关闭。
   func NewService(secret string, observer LiveObserver, store Store) *Service {
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
		  gen:    gen,
		  fsm:    fsm,
		  drift:  newDriftDetector(store, clock),
		   revoke: newRevokeGuard(store, fsm, clock),
		   metrics: newGateMetrics(),
		  audit: NewAuditLog(2048),
		  clock: clock,
	   }
   }

// WithClock 注入时钟。

   func (s *Service) WithClock(clock func() time.Time) *Service {
	  if s == nil || clock == nil {
		  return s
	   }
	   s.clock = clock
	   s.gen.clock = clock
	  s.fsm.clock = clock
	   s.drift.clock = clock
	  s.revoke.clock = clock
	   return s
  }

   // WithObserver 替换活账观察器。
   func (s *Service) WithObserver(obs LiveObserver) *Service {
	   if s != nil {
		   s.obs = obs
	  }
	  return s
   }

  func (s *Service) now() time.Time {
	  if s == nil || s.clock == nil {
		   return time.Now().UTC()
	  }
	   return s.clock()
   }

// Issue 签发证明租约；幂等不双签。
   func (s *Service) Issue(ctx context.Context, req IssueReq) (*ProofLease, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	   }
	   s.metrics.issueTotal.Add(1)
	   if strings.TrimSpace(s.secret) == "" {
		   s.metrics.issueDenied.Add(1)
		   return nil, ErrSecret
	  }
	  norm, err := NormalizeIssue(req)
	   if err != nil {
		   s.metrics.issueDenied.Add(1)
		   return nil, err
	   }
	  req = norm
	   exist, err := s.fsm.FindIdempotent(ctx, req.UserID, req.Book, req.Asset, req.Nonce)
	   if err != nil {
		   s.metrics.issueDenied.Add(1)
		  return nil, err
	  }
	   if exist != nil {
		  s.metrics.issueIdempotent.Add(1)
		  s.audit.Append(AuditEvent{Kind: AuditIdempotent, ProofLeaseID: exist.ProofLeaseID, UserID: exist.UserID, Book: exist.Book, Asset: exist.Asset, Detail: "nonce-hit", At: s.now()})
		   cp := exist.Copy()
		  return &cp, nil
	  }
	  if s.obs == nil {
		   s.metrics.issueDenied.Add(1)
		  return nil, ErrObserveUnavailable
	   }
	  view, err := s.obs.Observe(ctx, req.UserID, req.Book, req.Asset)
	   if err != nil {
		   s.metrics.issueDenied.Add(1)
		   return nil, err
	   }
	   snap, err := BuildSnapshot(view)
	  if err != nil {
		   s.metrics.issueDenied.Add(1)
		   return nil, err
	  }
	  lease, err := s.fsm.IssueNew(ctx, req, snap, s.secret)
	  if err != nil {
		   if err == ErrStaleGen || err == ErrConcurrentIssue {
			   s.metrics.genStale.Add(1)
		  }
		  s.metrics.issueDenied.Add(1)
		  return nil, err
	   }
	   s.metrics.issueOk.Add(1)
	   s.metrics.leaseBound.Add(1)
	  s.metrics.genBumped.Add(1)
	  s.audit.Append(AuditEvent{Kind: AuditIssued, ProofLeaseID: lease.ProofLeaseID, UserID: lease.UserID, Book: lease.Book, Asset: lease.Asset, Detail: lease.Signature, At: s.now()})
	   return lease, nil
   }

// Recheck 闸消费前活额复核，不推进 consumed。
   func (s *Service) Recheck(ctx context.Context, req RecheckReq) (*ProofLease, *DriftProof, error) {
	  if s == nil {
		   return nil, nil, ErrUnavailable
	   }
	   s.metrics.recheckTotal.Add(1)
	   lease,
		   err := s.prepareGate(ctx, req.LeaseID, req.Signature)
	  if err != nil {
		  s.metrics.recheckDenied.Add(1)
		  return nil, nil, err
	  }
	  drift,
		   err := s.recheckLive(ctx, lease)
	   if err != nil {
		   s.metrics.recheckDenied.Add(1)
		   return lease, drift, err
	  }
	  s.metrics.recheckOk.Add(1)
	   return lease, nil, nil
   }

// Consume 复核通过后消费租约。

  func (s *Service) Consume(ctx context.Context, req ConsumeReq) (*ProofLease, *DriftProof, error) {
	  if s == nil {
		   return nil, nil, ErrUnavailable
	  }
	  s.metrics.consumeTotal.Add(1)
	  lease, err := s.prepareGate(ctx, req.LeaseID, req.Signature)
	   if err != nil {
		   s.metrics.consumeDenied.Add(1)
		  return nil, nil, err
	  }
	   drift, err := s.recheckLive(ctx, lease)
	  if err != nil {
		   s.metrics.consumeDenied.Add(1)
		   return lease, drift, err
	  }
	   out, err := s.fsm.Consume(ctx, lease.ID)
	   if err != nil {
		  s.metrics.consumeDenied.Add(1)
		   return nil, nil, err
	  }
	   s.metrics.consumeOk.Add(1)
	  s.metrics.leaseConsumed.Add(1)
	   s.audit.Append(AuditEvent{Kind: AuditConsumed,
		   ProofLeaseID: out.ProofLeaseID, UserID: out.UserID, Book: out.Book, Asset: out.Asset, Detail: req.GateRef, At: s.now()})
	  return out, nil, nil
  }

  func (s *Service) prepareGate(ctx context.Context, leaseID, sig string) (*ProofLease, error) {
	  leaseID = strings.TrimSpace(leaseID)
	   l,
		  err := s.store.GetLease(ctx, leaseID)
	  if err != nil {
		   return nil, err
	   }
	   if l == nil {
		  return nil, ErrLeaseNotFound
	  }
	   if err := s.revoke.BlockedByRevoke(ctx, l.ID); err != nil {
		  s.metrics.revokeHit.Add(1)
		  return nil, err
	   }
	   now := s.now()
	   if l.Status == LeaseExpired || (!l.ExpiresAt.IsZero() && !now.Before(l.ExpiresAt)) {
		   if l.Status != LeaseExpired {
			  _, _ = s.fsm.Expire(ctx, l.ID)
		  }
		  s.metrics.expireHit.Add(1)
		  return nil, ErrLeaseExpired
	  }
	   if l.Status == LeaseRevoked {
		  return nil, ErrRevoked
	  }
	  if l.Status == LeaseConsumed {
		  return nil, ErrLeaseTerminal
	   }
	  if l.Status != LeaseBound && l.Status != LeaseIssued {
		   return nil, ErrLeaseNotBound
	  }
	   if l.Status == LeaseIssued {
		  if _, err := s.fsm.BindIssued(ctx, l.ID); err != nil {
			  return nil, err
		   }
		  l, err = s.store.GetLease(ctx, l.ID)
		  if err != nil || l == nil {
			  return nil, ErrLeaseNotFound
		  }
	  }
	   if err := VerifyHMAC(s.secret, l.Canonical, sig); err != nil {
		   return nil, err
	   }
	   cur,
		   err := s.gen.Current(ctx, l.UserID, l.Book, l.Asset)
	   if err != nil {
		  return nil, err
	   }
	  if l.Generation != cur {
		   s.metrics.staleLease.Add(1)
		  return nil, ErrStaleGen
	   }
	  blocked, err := s.drift.HasBlocking(ctx, l.UserID, l.Book, l.Asset)
	   if err != nil {
		  return nil, err
	   }
	   if blocked {
		   s.metrics.driftBlocked.Add(1)
		   return nil, ErrDriftBlocked
	   }
	   cp := l.Copy()
	   return &cp, nil
  }

   func (s *Service) recheckLive(ctx context.Context, lease *ProofLease) (*DriftProof, error) {
	   if s.obs == nil {
		   return nil, ErrObserveUnavailable
	  }
	  view, err := s.obs.Observe(ctx, lease.UserID, lease.Book, lease.Asset)
	  if err != nil {
		   return nil, err
	  }
	   kind := ClassifyDrift(lease.Snapshot,
		   view)
	  if kind == "" {
		  return nil, nil
	   }
	   p, err := s.drift.OpenFromRecheck(ctx, *lease, view, kind)
	  if err != nil {
		  return nil, err
	  }
	  s.metrics.driftOpened.Add(1)
	  s.metrics.driftProven.Add(1)
	   s.audit.Append(AuditEvent{Kind: AuditDriftOpen, ProofLeaseID: lease.ID, UserID: lease.UserID, Book: lease.Book, Asset: lease.Asset, Detail: string(p.Kind), At: s.now()})
	  return p, ErrLiveDrift
  }

// Revoke 吊销旧签名。

  func (s *Service) Revoke(ctx context.Context, req RevokeReq) (*Revocation, *ProofLease, error) {
	  if s == nil {
		   return nil, nil, ErrUnavailable
	  }
	   s.metrics.revokeTotal.Add(1)
	  rev, lease, err := s.revoke.Revoke(ctx, req)
	  if err != nil {
		   return nil, nil, err
	  }
	   s.metrics.revokeOk.Add(1)
	  s.metrics.leaseRevoked.Add(1)
	  s.audit.Append(AuditEvent{Kind: AuditRevoked,
		   ProofLeaseID: lease.ProofLeaseID, UserID: lease.UserID, Book: lease.Book, Asset: lease.Asset, Detail: req.Reason, At: s.now()})
	   return rev, lease, nil
  }

// GetLease 查询。

  func (s *Service) GetLease(ctx context.Context, id string) (*ProofLease, error) {
	   if s == nil || s.store == nil {
		  return nil, ErrUnavailable
	   }
	  l, err := s.store.GetLease(ctx, id)
	  if err != nil {
		   return nil, err
	  }
	   if l == nil {
		  return nil, ErrLeaseNotFound
	  }
	  cp := l.Copy()
	   return &cp, nil
  }

// GetGeneration 查询世代。
   func (s *Service) GetGeneration(ctx context.Context, userID, book, asset string) (*ProofGeneration, error) {
	  return s.gen.Get(ctx, userID, book, asset)
  }

// GetDrift 查询漂移证明。

   func (s *Service) GetDrift(ctx context.Context, id string) (*DriftProof, error) {
	   p, err := s.store.GetDrift(ctx, id)
	  if err != nil {
		   return nil, err
	   }
	   if p == nil {
		   return nil, ErrDriftNotFound
	   }
	   cp := p.Copy()
	   return &cp, nil
   }

// CloseDrift 收口。
   func (s *Service) CloseDrift(ctx context.Context, id, operator string) (*DriftProof, error) {
	   p,
		   err := s.drift.Close(ctx, id, operator)
	  if err != nil {
		   return nil, err
	   }
	  s.metrics.driftClosed.Add(1)
	   return p, nil
  }

// Export 导出证明包。

  func (s *Service) Export(ctx context.Context,
	  leaseID string) (*ExportBundle, error) {
	   l, err := s.GetLease(ctx, leaseID)
	   if err != nil {
		  return nil, err
	   }
	  drifts := make([]DriftProof, 0)
	   if open, _ := s.store.FindOpenDrift(ctx, leaseID); open != nil {
		  drifts = append(drifts, open.Copy())
	  }
	  var rev *Revocation
	  if r, _ := s.store.GetRevocationByLease(ctx, leaseID); r != nil {
		  cp := r.Copy()
		  rev = &cp
	  }
	   return &ExportBundle{
		  ProofLeaseID: l.ProofLeaseID,
		  Lease:        l.Copy(),
		   Drifts:       drifts,
		   Revocation: rev,
		   IsolationMark: IsolationMark,
		  ExportedAt:    s.now(),
	  }, nil
   }

// Stats 指标快照。
  func (s *Service) Stats() map[string]int64 {
	  if s == nil || s.metrics == nil {
		  return map[string]int64{}
	  }
	  return s.metrics.Snapshot()
   }

// Store 暴露给 worker。

   func (s *Service) Store() Store { return s.store }

// FSM 暴露给 worker。
  func (s *Service) FSM() *LeaseFSM { return s.fsm }

// Revoker 暴露给 worker。

   func (s *Service) Revoker() *RevokeGuard { return s.revoke }

// Drift 暴露给 worker。

   func (s *Service) Drift() *DriftDetector { return s.drift }

// Observer 暴露给 worker。
  func (s *Service) Observer() LiveObserver { return s.obs }

// Metrics 暴露给 worker。
  func (s *Service) Metrics() *GateMetrics { return s.metrics }

// AuditRecent 最近审计。

   func (s *Service) AuditRecent(n int) []AuditEvent {
	  if s == nil || s.audit == nil {
		  return nil
	  }
	   return s.audit.Recent(n)
   }
