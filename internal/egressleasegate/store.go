package egressleasegate

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Store 覆盖租约、证明、审计、世代。
type Store interface {
	InsertLease(ctx context.Context, l *EgressLease) error
	GetLeaseByID(ctx context.Context,
		id string) (*EgressLease, error)
	GetLeaseByIdem(ctx context.Context,
		idemKey string) (*EgressLease, error)
	UpdateLease(ctx context.Context, l *EgressLease) error
	ListExpiredLeases(ctx context.Context, before time.Time, limit int) ([]*EgressLease, error)
	ListActiveLeases(ctx context.Context, limit int) ([]*EgressLease, error)
	ListActiveByScope(ctx context.Context, chain, asset string, limit int) ([]*EgressLease, error)
	InsertProof(ctx context.Context, p *EgressDriftProof) error
	GetProofByID(ctx context.Context, id string) (*EgressDriftProof, error)
	GetProofByIdem(ctx context.Context,
		idemKey string) (*EgressDriftProof, error)
	GetOpenProofByLease(ctx context.Context, leaseID string) (*EgressDriftProof, error)
	UpdateProof(ctx context.Context, p *EgressDriftProof) error
	ListProofsByState(ctx context.Context,
		state ProofState, limit int) ([]*EgressDriftProof, error)
	InsertAudit(ctx context.Context, a *LeaseAudit) error
	ListAuditByLease(ctx context.Context, leaseID string, limit int) ([]*LeaseAudit, error)
	InsertGeneration(ctx context.Context, g *EgressGeneration) error
	ListGenerations(ctx context.Context, limit int) ([]*EgressGeneration, error)
	GetLease(ctx context.Context, id string) (*EgressLease, error)
	ListOpen(ctx context.Context, chain, asset string, limit int) ([]*EgressLease, error)
	ListExpired(ctx context.Context, before time.Time, limit int) ([]*EgressLease, error)
	UpdateLeaseCAS(ctx context.Context, l *EgressLease, from LeaseState) error
	GetProof(ctx context.Context, id string) (*EgressDriftProof, error)
	ListAudit(ctx context.Context, leaseID string, limit int) ([]*LeaseAudit, error)
	GetGeneration(ctx context.Context,
		scopeKey string) (*EgressGeneration, error)
	PutGeneration(ctx context.Context, g *EgressGeneration) error
}

// MemStore 线程安全内存实现。读写深拷贝。

type MemStore struct {
	mu sync.RWMutex
	leases    map[string]*EgressLease
	leaseIdem map[string]string
	proofs map[string]*EgressDriftProof
	proofIdem map[string]string
	proofByLease map[string]string
	audits []LeaseAudit
	gens []EgressGeneration
}

// NewMemStore 创建空存储。
   func NewMemStore() *MemStore {
	   return &MemStore{
		   leases:    map[string]*EgressLease{},
		  leaseIdem: map[string]string{},
		   proofs: map[string]*EgressDriftProof{},
		   proofIdem:    map[string]string{},
		   proofByLease: map[string]string{},
	   }
   }

   func cloneLease(l *EgressLease) *EgressLease {
	  if l == nil {
		  return nil
	   }
	  c := *l
	   if l.BoundAt != nil {
		   t := *l.BoundAt
		   c.BoundAt = &t
	   }
	   if l.ConsumedAt != nil {
		  t := *l.ConsumedAt
		  c.ConsumedAt = &t
	  }
	  if l.RevokedAt != nil {
		  t := *l.RevokedAt
		   c.RevokedAt = &t
	  }
	   return &c
   }

   func cloneProof(p *EgressDriftProof) *EgressDriftProof {
	   if p == nil {
		   return nil
	   }
	   c := *p
	   if p.ProvenAt != nil {
		  t := *p.ProvenAt
		  c.ProvenAt = &t
	   }
	   if p.ClosedAt != nil {
		   t := *p.ClosedAt
		  c.ClosedAt = &t
	   }
	  return &c
  }

   func (m *MemStore) InsertLease(_ context.Context, l *EgressLease) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  if _, ok := m.leases[l.ID]; ok {
		   return ErrDuplicate
	  }
	  if l.IdemKey != "" {
		  if _, ok := m.leaseIdem[l.IdemKey]; ok {
			  return ErrDuplicate
		  }
		  m.leaseIdem[l.IdemKey] = l.ID
	   }
	   m.leases[l.ID] = cloneLease(l)
	  return nil
  }

  func (m *MemStore) GetLeaseByID(_ context.Context, id string) (*EgressLease, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	   l, ok := m.leases[id]
	   if !ok {
		  return nil, ErrNotFound
	  }
	   return cloneLease(l), nil
   }

  func (m *MemStore) GetLeaseByIdem(_ context.Context, idemKey string) (*EgressLease, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  id, ok := m.leaseIdem[idemKey]
	  if !ok {
		  return nil, ErrNotFound
	   }
	  l, ok := m.leases[id]
	  defer m.mu.RUnlock()
	  var out []*EgressLease
	  for _, l := range m.leases {
		   if l.State.Closed() {
			  continue
		  }
		  if l.ExpiresAt.Before(before) || l.ExpiresAt.Equal(before) {
			  out = append(out, cloneLease(l))
		   }
		   if limit > 0 && len(out) >= limit {
			  break
		  }
	  }
	  sort.Slice(out,
		  func(i, j int) bool { return out[i].ExpiresAt.Before(out[j].ExpiresAt) })
	   return out, nil
  }

  func (m *MemStore) ListActiveLeases(_ context.Context, limit int) ([]*EgressLease, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  var out []*EgressLease
	  for _, l := range m.leases {
		  if l.State.Closed() {
			   continue
		  }
		  out = append(out, cloneLease(l))
		  if limit > 0 && len(out) >= limit {
			  break
		  }
	   }
	   sort.Slice(out, func(i, j int) bool { return out[i].IssuedAt.Before(out[j].IssuedAt) })
	   return out, nil
   }

  func (m *MemStore) ListActiveByScope(_ context.Context, chain, asset string, limit int) ([]*EgressLease, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   var out []*EgressLease
	   for _, l := range m.leases {
		  if l.State.Closed() {
			   continue
		  }
		   if chain != "" && l.Chain != chain {
			   continue
		  }
		   if asset != "" && l.Asset != asset {
			   continue
		   }
		  out = append(out, cloneLease(l))
		   if limit > 0 && len(out) >= limit {
			  break
		  }
	   }
	  return out, nil
  }

  func (m *MemStore) InsertProof(_ context.Context, p *EgressDriftProof) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  if _, ok := m.proofs[p.ID]; ok {
		  return ErrDuplicate
	   }
	  if p.IdemKey != "" {
		   if _, ok := m.proofIdem[p.IdemKey]; ok {
			   return ErrDuplicate
		   }
		  m.proofIdem[p.IdemKey] = p.ID
	  }
	   if p.LeaseID != "" && p.State == ProofStateOpen {
		   m.proofByLease[p.LeaseID] = p.ID
	   }
	   m.proofs[p.ID] = cloneProof(p)
	   return nil
   }

  func (m *MemStore) GetProofByID(_ context.Context, id string) (*EgressDriftProof, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  p, ok := m.proofs[id]
	  if !ok {
		   return nil, ErrNotFound
	  }
	   return cloneProof(p), nil
  }

  func (m *MemStore) GetProofByIdem(_ context.Context, idemKey string) (*EgressDriftProof, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   id, ok := m.proofIdem[idemKey]
	   if !ok {
		  return nil, ErrNotFound
	  }
	   p, ok := m.proofs[id]
	   if !ok {
		  return nil, ErrNotFound
	   }
	  return cloneProof(p), nil
  }

   func (m *MemStore) GetOpenProofByLease(_ context.Context, leaseID string) (*EgressDriftProof, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   id, ok := m.proofByLease[leaseID]
	  if !ok {
		  return nil, ErrNotFound
	   }
	  p, ok := m.proofs[id]
	   if !ok || p.State != ProofStateOpen {
		   return nil, ErrNotFound
	  }
	  return cloneProof(p), nil
   }

  func (m *MemStore) UpdateProof(_ context.Context, p *EgressDriftProof) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   if _, ok := m.proofs[p.ID]; !ok {
		   return ErrNotFound
	  }
	  if p.State != ProofStateOpen && p.LeaseID != "" {
		   if m.proofByLease[p.LeaseID] == p.ID {
			   delete(m.proofByLease, p.LeaseID)
		  }
	  }
	   m.proofs[p.ID] = cloneProof(p)
	  return nil
  }

   func (m *MemStore) ListProofsByState(_ context.Context, state ProofState, limit int) ([]*EgressDriftProof, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  var out []*EgressDriftProof
	   for _, p := range m.proofs {
		   if p.State != state {
			  continue
		   }
		   out = append(out, cloneProof(p))
		  if limit > 0 && len(out) >= limit {
			   break
		   }
	  }
	  sort.Slice(out,
		  func(i, j int) bool { return out[i].OpenedAt.Before(out[j].OpenedAt) })
	  return out, nil
  }

  func (m *MemStore) InsertAudit(_ context.Context, a *LeaseAudit) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  cp := *a
	   m.audits = append(m.audits, cp)
	   return nil
   }

   func (m *MemStore) ListAuditByLease(_ context.Context, leaseID string, limit int) ([]*LeaseAudit, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	  var out []*LeaseAudit
	   for i := range m.audits {
		  if m.audits[i].LeaseID != leaseID {
			  continue
		  }
		   cp := m.audits[i]
		  out = append(out, &cp)
		   if limit > 0 && len(out) >= limit {
			   break
		   }
	  }
	   return out, nil
   }

  func (m *MemStore) InsertGeneration(_ context.Context, g *EgressGeneration) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  cp := *g
	  m.gens = append(m.gens, cp)
	   return nil
  }

   func (m *MemStore) ListGenerations(_ context.Context, limit int) ([]*EgressGeneration, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  n := len(m.gens)
	  if limit <= 0 || limit > n {
		   limit = n
	   }
	   out := make([]*EgressGeneration, 0, limit)
	   start := n - limit
	   if start < 0 {
		  start = 0
	   }
	  for i := start; i < n; i++ {
		   cp := m.gens[i]
		   out = append(out, &cp)
	  }
	   return out, nil
   }

   func (m *MemStore) GetLease(ctx context.Context, id string) (*EgressLease, error) {
	   return m.GetLeaseByID(ctx, id)
   }

   func (m *MemStore) ListOpen(ctx context.Context, chain, asset string, limit int) ([]*EgressLease, error) {
	  return m.ListActiveByScope(ctx, chain, asset, limit)
  }

   func (m *MemStore) ListExpired(ctx context.Context, before time.Time, limit int) ([]*EgressLease, error) {
	  return m.ListExpiredLeases(ctx, before, limit)
   }

  func (m *MemStore) UpdateLeaseCAS(_ context.Context, l *EgressLease, from LeaseState) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  cur, ok := m.leases[l.ID]
	   if !ok {
		   return ErrNotFound
	   }
	  if cur.State != from {
		   return ErrInvalidTransit
	  }
	  m.leases[l.ID] = cloneLease(l)
	   return nil
  }

  func (m *MemStore) GetProof(ctx context.Context, id string) (*EgressDriftProof, error) {
	  return m.GetProofByID(ctx, id)
  }

   func (m *MemStore) ListAudit(ctx context.Context, leaseID string, limit int) ([]*LeaseAudit, error) {
	  return m.ListAuditByLease(ctx, leaseID, limit)
   }

  func (m *MemStore) GetGeneration(_ context.Context, scopeKey string) (*EgressGeneration, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	  for i := len(m.gens) - 1; i >= 0; i-- {
		  g := m.gens[i]
		   if g.ScopeKey == scopeKey || ScopeKey(g.Chain, g.Asset) == scopeKey {
			   cp := g
			   return &cp, nil
		  }
	  }
	   return nil, ErrNotFound
  }

  func (m *MemStore) PutGeneration(_ context.Context, g *EgressGeneration) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   if g == nil {
		   return ErrUnavailable
	   }
	   cp := *g
	   m.gens = append(m.gens, cp)
	   return nil
  }
