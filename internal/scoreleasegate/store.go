package scoreleasegate

import (
	"context"
	"sync"
	"time"
)

type Store interface {
	GetGeneration(ctx context.Context) (ScoreGeneration, error)
	CASBumpHalt(ctx context.Context, expect int64, on bool, at time.Time) (ScoreGeneration, error)
	CASBumpPack(ctx context.Context, expect int64, pack, fp string, at time.Time) (ScoreGeneration, error)
	CASBumpMode(ctx context.Context, expect int64, mode string, at time.Time) (ScoreGeneration, error)
	CASBumpWindow(ctx context.Context, expect int64, digest string, at time.Time) (ScoreGeneration, error)
	PutLease(ctx context.Context, lease ScoreLease) error
	GetLease(ctx context.Context, id string) (ScoreLease, error)
	FindLeaseByNonce(ctx context.Context, user, nonce string) (ScoreLease, bool, error)
	ListOpenLeases(ctx context.Context) ([]ScoreLease, error)
	ListUserOpen(ctx context.Context, user string) ([]ScoreLease, error)
	UpdateLease(ctx context.Context, lease ScoreLease) error
	PutProof(ctx context.Context, p ScoreDriftProof) error
	GetProof(ctx context.Context,
		id string) (ScoreDriftProof, error)
	FindProof(ctx context.Context, leaseID string, kind ProofKind, halt, packGen int64) (ScoreDriftProof, bool, error)
	ListOpenProofs(ctx context.Context) ([]ScoreDriftProof, error)
	UpdateProof(ctx context.Context, p ScoreDriftProof) error
	ListProofsByLease(ctx context.Context,
		leaseID string) ([]ScoreDriftProof, error)
	ListProofsByUser(ctx context.Context, user string) ([]ScoreDriftProof, error)
	PutAudit(ctx context.Context,
		ev AuditEvent) error
	ListAudit(ctx context.Context, leaseID string) ([]AuditEvent, error)
}

type MemStore struct {
	mu sync.Mutex
	gen   ScoreGeneration
	lease map[string]ScoreLease
	proof map[string]ScoreDriftProof
	audit []AuditEvent
}

   func NewMemStore() *MemStore {
	  return &MemStore{
		   gen: ScoreGeneration{
			   ID: "singleton",
			  HaltGen: 1,
			   PackGen: 1,
			   ModeGen: 1,
			  WindowGen: 1,
			  Pack: "kyc0",
			   Mode:      "enforce",
			  UpdatedAt: time.Unix(0, 0).UTC(),
		  },
		  lease: map[string]ScoreLease{},
		  proof: map[string]ScoreDriftProof{},
		  audit: []AuditEvent{},
	   }
  }

   func (m *MemStore) GetGeneration(ctx context.Context) (ScoreGeneration, error) {
	   _ = ctx
	   if m == nil {
		  return ScoreGeneration{}, ErrUnavailable
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   return m.gen.Copy(), nil
  }

   func (m *MemStore) CASBumpHalt(ctx context.Context, expect int64, on bool, at time.Time) (ScoreGeneration, error) {
	  _ = ctx
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  if m.gen.HaltGen != expect {
		   return m.gen.Copy(), ErrStaleGen
	  }
	   m.gen.HaltGen++
	  m.gen.HaltOn = on
	   m.gen.UpdatedAt = at
	   return m.gen.Copy(), nil
  }

   func (m *MemStore) CASBumpPack(ctx context.Context, expect int64, pack, fp string, at time.Time) (ScoreGeneration, error) {
	   _ = ctx
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if m.gen.PackGen != expect {
		  return m.gen.Copy(), ErrStaleGen
	   }
	   m.gen.PackGen++
	   m.gen.Pack = pack
	   m.gen.PackFP = fp
	   m.gen.UpdatedAt = at
	  return m.gen.Copy(), nil
  }

   func (m *MemStore) CASBumpMode(ctx context.Context, expect int64, mode string, at time.Time) (ScoreGeneration, error) {
	   _ = ctx
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   if m.gen.ModeGen != expect {
		   return m.gen.Copy(), ErrStaleGen
	   }
	  m.gen.ModeGen++
	   m.gen.Mode = mode
	   m.gen.UpdatedAt = at
	   return m.gen.Copy(), nil
   }

   func (m *MemStore) CASBumpWindow(ctx context.Context, expect int64, digest string, at time.Time) (ScoreGeneration, error) {
	  _ = ctx
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   if m.gen.WindowGen != expect {
		   return m.gen.Copy(),
			   ErrStaleGen
	  }
	   m.gen.WindowGen++
	   m.gen.WindowDigest = digest
	  m.gen.UpdatedAt = at
	   return m.gen.Copy(), nil
  }

  func (m *MemStore) PutLease(ctx context.Context, lease ScoreLease) error {
	   _ = ctx
	   if m == nil {
		   return ErrUnavailable
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   if _, ok := m.lease[lease.ID]; ok {
		   return ErrIdempotentNonce
	   }
	  for _, existing := range m.lease {
		   if existing.UserID == lease.UserID && existing.Nonce == lease.Nonce {
			   return ErrIdempotentNonce
		   }
	  }
	  m.lease[lease.ID] = lease.Copy()
	  return nil
   }

  func (m *MemStore) GetLease(ctx context.Context, id string) (ScoreLease, error) {
	  _ = ctx
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  got, ok := m.lease[id]
	  if !ok {
		  return ScoreLease{},
			  ErrLeaseNotFound
	  }
	  return got.Copy(), nil
  }

  func (m *MemStore) FindLeaseByNonce(ctx context.Context, user, nonce string) (ScoreLease, bool, error) {
	  _ = ctx
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  for _, l := range m.lease {
		   if l.UserID == user && l.Nonce == nonce {
			  return l.Copy(), true, nil
		   }
	   }
	  return ScoreLease{}, false, nil
  }

  func (m *MemStore) ListOpenLeases(ctx context.Context) ([]ScoreLease, error) {
	   _ = ctx
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   out := make([]ScoreLease, 0)
	   for _, l := range m.lease {
		  if l.Status.OpenLike() {
			  out = append(out, l.Copy())
		   }
	   }
	  return out, nil
   }

  func (m *MemStore) ListUserOpen(ctx context.Context, user string) ([]ScoreLease, error) {
	  _ = ctx
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  out := make([]ScoreLease, 0)
	   for _, l := range m.lease {
		  if l.UserID == user && l.Status.OpenLike() {
			   out = append(out, l.Copy())
		  }
	  }
	   return out, nil
   }

   func (m *MemStore) UpdateLease(ctx context.Context, lease ScoreLease) error {
	   _ = ctx
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  if _, ok := m.lease[lease.ID]; !ok {
		   return ErrLeaseNotFound
	  }
	  m.lease[lease.ID] = lease.Copy()
	   return nil
  }

   func (m *MemStore) PutProof(ctx context.Context, p ScoreDriftProof) error {
	   _ = ctx
	   if m == nil {
		  return ErrUnavailable
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  m.proof[p.ID] = p.Copy()
	   return nil
  }

  func (m *MemStore) GetProof(ctx context.Context, id string) (ScoreDriftProof, error) {
	  _ = ctx
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   got, ok := m.proof[id]
	  if !ok {
		   return ScoreDriftProof{}, ErrProofNotFound
	  }
	   return got.Copy(), nil
  }

  func (m *MemStore) FindProof(ctx context.Context, leaseID string, kind ProofKind, halt, packGen int64) (ScoreDriftProof, bool, error) {
	  _ = ctx
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   for _, p := range m.proof {
		  if p.LeaseID == leaseID && p.Kind == kind && p.ExpectedHalt == halt && p.ObservedHalt == packGen {
			  return p.Copy(), true, nil
		   }
	  }
	  return ScoreDriftProof{},
		  false, nil
  }

  func (m *MemStore) ListOpenProofs(ctx context.Context) ([]ScoreDriftProof, error) {
	   _ = ctx
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   out := make([]ScoreDriftProof, 0)
	  for _, p := range m.proof {
		   if p.BlocksSubmit() {
			  out = append(out, p.Copy())
		   }
	   }
	   return out, nil
   }

   func (m *MemStore) UpdateProof(ctx context.Context, p ScoreDriftProof) error {
	  _ = ctx
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   if _, ok := m.proof[p.ID]; !ok {
		   return ErrProofNotFound
	  }
	  m.proof[p.ID] = p.Copy()
	  return nil
  }

  func (m *MemStore) ListProofsByLease(ctx context.Context, leaseID string) ([]ScoreDriftProof, error) {
	  _ = ctx
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   out := make([]ScoreDriftProof, 0)
	  for _, p := range m.proof {
		  if p.LeaseID == leaseID {
			  out = append(out, p.Copy())
		  }
	   }
	  return out, nil
   }

  func (m *MemStore) ListProofsByUser(ctx context.Context, user string) ([]ScoreDriftProof, error) {
	   _ = ctx
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   out := make([]ScoreDriftProof, 0)
	   for _, p := range m.proof {
		  if p.UserID == user {
			   out = append(out, p.Copy())
		  }
	  }
	   return out, nil
   }

   func (m *MemStore) PutAudit(ctx context.Context, ev AuditEvent) error {
	  _ = ctx
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   m.audit = append(m.audit, ev)
	  return nil
  }

  func (m *MemStore) ListAudit(ctx context.Context, leaseID string) ([]AuditEvent, error) {
	   _ = ctx
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  out := make([]AuditEvent, 0)
	  for _, ev := range m.audit {
		  if leaseID == "" || ev.LeaseID == leaseID {
			   out = append(out, ev)
		  }
	  }
	  return out, nil
  }
