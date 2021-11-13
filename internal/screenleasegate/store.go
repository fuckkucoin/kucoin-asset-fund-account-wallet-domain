package screenleasegate

import (
	"context"
	"sync"
	"time"
)

type Store interface {
	GetGeneration(ctx context.Context) (ScreenGeneration, error)
	CASBumpHalt(ctx context.Context,
		expect int64, on bool, at time.Time) (ScreenGeneration, error)
	CASBumpPack(ctx context.Context, expect int64, pack, mode, fp string, at time.Time) (ScreenGeneration, error)
	PutLease(ctx context.Context, lease ScreenLease) error
	GetLease(ctx context.Context, id string) (ScreenLease, error)
	FindLeaseByNonce(ctx context.Context, user, nonce string) (ScreenLease, bool, error)
	ListOpenLeases(ctx context.Context) ([]ScreenLease, error)
	ListUserOpen(ctx context.Context, user string) ([]ScreenLease, error)
	UpdateLease(ctx context.Context,
		lease ScreenLease) error
	PutProof(ctx context.Context, p ScreenDriftProof) error
	GetProof(ctx context.Context,
		id string) (ScreenDriftProof, error)
	FindProof(ctx context.Context, leaseID string, kind ProofKind, halt, packGen int64) (ScreenDriftProof, bool, error)
	ListOpenProofs(ctx context.Context) ([]ScreenDriftProof, error)
	UpdateProof(ctx context.Context,
		p ScreenDriftProof) error
	ListProofsByLease(ctx context.Context, leaseID string) ([]ScreenDriftProof, error)
}

type MemStore struct {
	mu    sync.Mutex
	gen   ScreenGeneration
	lease map[string]ScreenLease
	proof map[string]ScreenDriftProof
}

   func NewMemStore() *MemStore {
	   return &MemStore{
		  gen: ScreenGeneration{
			   ID: "singleton",
			   HaltGen: 1,
			  PackGen: 1,
			  Pack:    "kyc0",
			  Mode:    "enforce",
			  PackFP:  "",
			  UpdatedAt: time.Unix(0, 0).UTC(),
		  },
		   lease: map[string]ScreenLease{},
		  proof: map[string]ScreenDriftProof{},
	  }
  }

  func (m *MemStore) GetGeneration(ctx context.Context) (ScreenGeneration, error) {
	  _ = ctx
	  if m == nil {
		  return ScreenGeneration{}, ErrUnavailable
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   return m.gen.Copy(), nil
   }

   func (m *MemStore) CASBumpHalt(ctx context.Context, expect int64, on bool, at time.Time) (ScreenGeneration, error) {
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

   func (m *MemStore) CASBumpPack(ctx context.Context, expect int64, pack, mode, fp string, at time.Time) (ScreenGeneration, error) {
	  _ = ctx
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   if m.gen.PackGen != expect {
		  return m.gen.Copy(), ErrStaleGen
	  }
	  m.gen.PackGen++
	   m.gen.Pack = pack
	   m.gen.Mode = mode
	   m.gen.PackFP = fp
	   m.gen.UpdatedAt = at
	   return m.gen.Copy(), nil
   }

  func (m *MemStore) PutLease(ctx context.Context, lease ScreenLease) error {
	   _ = ctx
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  if _, ok := m.lease[lease.ID]; ok {
		  return ErrIdempotentNonce
	   }
	  m.lease[lease.ID] = lease.Copy()
	   return nil
   }

   func (m *MemStore) GetLease(ctx context.Context, id string) (ScreenLease, error) {
	   _ = ctx
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   got, ok := m.lease[id]
	  if !ok {
		   return ScreenLease{}, ErrLeaseNotFound
	   }
	   return got.Copy(), nil
  }

  func (m *MemStore) FindLeaseByNonce(ctx context.Context, user, nonce string) (ScreenLease, bool, error) {
	   _ = ctx
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   for _, l := range m.lease {
		   if l.UserID == user && l.Nonce == nonce {
			   return l.Copy(), true, nil
		   }
	   }
	  return ScreenLease{}, false, nil
   }

  func (m *MemStore) ListOpenLeases(ctx context.Context) ([]ScreenLease, error) {
	  _ = ctx
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   out := make([]ScreenLease, 0, len(m.lease))
	   for _, l := range m.lease {
		   if !l.Status.Terminal() {
			  out = append(out, l.Copy())
		   }
	  }
	   return out, nil
   }

  func (m *MemStore) ListUserOpen(ctx context.Context, user string) ([]ScreenLease, error) {
	   _ = ctx
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   out := []ScreenLease{}
	   for _, l := range m.lease {
		  if l.UserID == user && !l.Status.Terminal() {
			   out = append(out, l.Copy())
		   }
	   }
	  return out, nil
  }

   func (m *MemStore) UpdateLease(ctx context.Context, lease ScreenLease) error {
	  _ = ctx
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  if _, ok := m.lease[lease.ID]; !ok {
		  return ErrLeaseNotFound
	   }
	  m.lease[lease.ID] = lease.Copy()
	  return nil
  }

   func (m *MemStore) PutProof(ctx context.Context, p ScreenDriftProof) error {
	   _ = ctx
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  m.proof[p.ID] = p.Copy()
	   return nil
  }

   func (m *MemStore) GetProof(ctx context.Context, id string) (ScreenDriftProof, error) {
	  _ = ctx
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   got, ok := m.proof[id]
	   if !ok {
		  return ScreenDriftProof{}, ErrProofNotFound
	   }
	   return got.Copy(), nil
  }

  func (m *MemStore) FindProof(ctx context.Context,
	  leaseID string, kind ProofKind, halt, packGen int64) (ScreenDriftProof, bool, error) {
	  _ = ctx
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  for _, p := range m.proof {
		  if p.LeaseID == leaseID && p.Kind == kind && p.ExpectedHalt == halt && p.ObservedHalt == packGen {
			  return p.Copy(), true, nil
		  }
	   }
	   // 次级匹配：同 lease+kind 且未关闭。
	   for _, p := range m.proof {
		  if p.LeaseID == leaseID && p.Kind == kind && p.Status != ProofClosed {
			   return p.Copy(), true, nil
		  }
	  }
	  return ScreenDriftProof{},
		  false, nil
  }

  func (m *MemStore) ListOpenProofs(ctx context.Context) ([]ScreenDriftProof, error) {
	  _ = ctx
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  out := []ScreenDriftProof{}
	   for _, p := range m.proof {
		   if p.Status != ProofClosed {
			   out = append(out, p.Copy())
		  }
	  }
	  return out, nil
   }

   func (m *MemStore) UpdateProof(ctx context.Context, p ScreenDriftProof) error {
	   _ = ctx
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   if _, ok := m.proof[p.ID]; !ok {
		  return ErrProofNotFound
	  }
	   m.proof[p.ID] = p.Copy()
	   return nil
   }

  func (m *MemStore) ListProofsByLease(ctx context.Context, leaseID string) ([]ScreenDriftProof, error) {
	   _ = ctx
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  out := []ScreenDriftProof{}
	   for _, p := range m.proof {
		   if p.LeaseID == leaseID {
			   out = append(out, p.Copy())
		  }
	   }
	   return out, nil
  }
