package invleasegate

import (
	"context"
	"sort"
	"sync"
	"time"
)

// MemStore 基于内存的 Store 实现（专用）。
type MemStore struct {
	mu     sync.RWMutex
	leases map[string]*InvPrecheckLease // key=LeaseID
	proofs       map[string]*InvGateDriftProof // key=ProofID
	proofByLease map[string]*InvGateDriftProof // key=LeaseID
	clock func() time.Time
}

// NewMemStore 构造内存 Store。

   func NewMemStore() *MemStore {
	  return &MemStore{
		   leases:       make(map[string]*InvPrecheckLease),
		   proofs:       make(map[string]*InvGateDriftProof),
		   proofByLease: make(map[string]*InvGateDriftProof),
		  clock: func() time.Time { return time.Now().UTC() },
	  }
   }

// WithClock 注入时钟（用）。
  func (m *MemStore) WithClock(fn func() time.Time) *MemStore {
	  if fn != nil {
		  m.clock = fn
	  }
	   return m
   }

   // ── InvPrecheckLease ──────────────────────────────────────────────────────────

   func (m *MemStore) SaveLease(_ context.Context, l *InvPrecheckLease) error {
	   if m == nil || l == nil {
		   return ErrUnavailable
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   cp := l.Copy()
	   m.leases[l.LeaseID] = &cp
	   return nil
  }

  func (m *MemStore) GetLease(_ context.Context, id string) (*InvPrecheckLease, error) {
	  if m == nil {
		   return nil, ErrUnavailable
	  }
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  l, ok := m.leases[id]
	   if !ok {
		  return nil, ErrLeaseNotFound
	  }
	  cp := l.Copy()
	   return &cp, nil
   }

  func (m *MemStore) UpdateLeaseState(_ context.Context, id string, from, to LeaseState, patch *InvPrecheckLease) error {
	  if m == nil {
		  return ErrUnavailable
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  l, ok := m.leases[id]
	   if !ok {
		  return ErrLeaseNotFound
	   }
	  if l.State != from {
		   return ErrInvalidTransition
	   }
	  now := m.clock()
	  if patch != nil {
		  cp := patch.Copy()
		  cp.State = to
		   cp.UpdatedAt = now
		   m.leases[id] = &cp
	  } else {
		  l.State = to
		   l.UpdatedAt = now
	  }
	   return nil
  }

   func (m *MemStore) ListActiveLeasesForAsset(_ context.Context, asset string, limit int) ([]*InvPrecheckLease, error) {
	   if m == nil {
		  return nil, ErrUnavailable
	   }
	   if limit <= 0 {
		  limit = 50
	  }
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  var out []*InvPrecheckLease
	  for _, l := range m.leases {
		  if l.Asset == asset && (l.State == LeaseIssued || l.State == LeaseBound) {
			   cp := l.Copy()
			   out = append(out, &cp)
		   }
	   }
	  sort.Slice(out, func(i, j int) bool {
		   return out[i].IssuedAt.After(out[j].IssuedAt)
	  })
	  if len(out) > limit {
		   out = out[:limit]
	  }
	  return out, nil
  }

   func (m *MemStore) ListExpiredLeases(_ context.Context, cutoff time.Time, limit int) ([]*InvPrecheckLease, error) {
	  if m == nil {
		  return nil, ErrUnavailable
	   }
	   if limit <= 0 {
		   limit = 100
	   }
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  var out []*InvPrecheckLease
	  for _, l := range m.leases {
		   if !l.State.Terminal() && !l.ExpiresAt.After(cutoff) {
			  cp := l.Copy()
			   out = append(out, &cp)
		   }
	   }
	   sort.Slice(out, func(i, j int) bool {
		   return out[i].ExpiresAt.Before(out[j].ExpiresAt)
	   })
	  if len(out) > limit {
		   out = out[:limit]
	   }
	   return out, nil
  }

   func (m *MemStore) ListLeasesByAsset(_ context.Context, asset string, limit int) ([]*InvPrecheckLease, error) {
	   if m == nil {
		   return nil, ErrUnavailable
	   }
	  if limit <= 0 {
		  limit = 50
	  }
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  var out []*InvPrecheckLease
	   for _, l := range m.leases {
		   if l.Asset == asset {
			   cp := l.Copy()
			   out = append(out, &cp)
		   }
	   }
	  sort.Slice(out, func(i, j int) bool {
		  return out[i].IssuedAt.After(out[j].IssuedAt)
	  })
	   if len(out) > limit {
		  out = out[:limit]
	   }
	  return out, nil
   }

// ── InvGateDriftProof ────────────────────────────────────────────────────────

  func (m *MemStore) SaveProof(_ context.Context, p *InvGateDriftProof) error {
	   if m == nil || p == nil {
		  return ErrUnavailable
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  // 同 lease_id 二次写拒绝（lease 为空字符串时不限制）
	   if p.LeaseID != "" {
		   if _, dup := m.proofByLease[p.LeaseID]; dup {
			  return ErrAlreadyProven
		  }
	  }
	  cp := p.Copy()
	  m.proofs[p.ProofID] = &cp
	  if p.LeaseID != "" {
		  m.proofByLease[p.LeaseID] = &cp
	  }
	   return nil
   }

  func (m *MemStore) GetProof(_ context.Context, id string) (*InvGateDriftProof, error) {
	   if m == nil {
		  return nil, ErrUnavailable
	  }
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  p, ok := m.proofs[id]
	   if !ok {
		  return nil, ErrProofNotFound
	  }
	   cp := p.Copy()
	  return &cp, nil
  }

   func (m *MemStore) GetProofByLease(_ context.Context, leaseID string) (*InvGateDriftProof, error) {
	   if m == nil {
		  return nil, ErrUnavailable
	  }
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  p, ok := m.proofByLease[leaseID]
	   if !ok {
		  return nil, ErrProofNotFound
	  }
	  cp := p.Copy()
	   return &cp, nil
  }

   func (m *MemStore) UpdateProofState(_ context.Context, id string, from, to ProofState, patch *InvGateDriftProof) error {
	  if m == nil {
		   return ErrUnavailable
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  p, ok := m.proofs[id]
	  if !ok {
		  return ErrProofNotFound
	  }
	  if p.State != from {
		   return ErrInvalidTransition
	   }
	   now := m.clock()
	  if patch != nil {
		   cp := patch.Copy()
		   cp.State = to
		  cp.UpdatedAt = now
		   m.proofs[id] = &cp
		  if p.LeaseID != "" {
			  m.proofByLease[p.LeaseID] = &cp
		   }
	   } else {
		   p.State = to
		  p.UpdatedAt = now
	   }
	   return nil
   }

   func (m *MemStore) ListOpenProofs(_ context.Context, limit int) ([]*InvGateDriftProof, error) {
	   if m == nil {
		  return nil, ErrUnavailable
	  }
	  if limit <= 0 {
		  limit = 100
	  }
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   var out []*InvGateDriftProof
	   for _, p := range m.proofs {
		  if p.State == ProofOpen {
			   cp := p.Copy()
			   out = append(out, &cp)
		  }
	   }
	  sort.Slice(out, func(i, j int) bool {
		  return out[i].CreatedAt.Before(out[j].CreatedAt)
	   })
	  if len(out) > limit {
		  out = out[:limit]
	   }
	   return out, nil
   }
