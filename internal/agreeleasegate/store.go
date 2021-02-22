package agreeleasegate

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Store 覆盖租约、证明、审计、世代、半截计划。
type Store interface {
	InsertLease(ctx context.Context, l *AgreeLease) error
	GetLeaseByID(ctx context.Context,
		id string) (*AgreeLease, error)
	GetLeaseByIdem(ctx context.Context, idemKey string) (*AgreeLease, error)
	UpdateLease(ctx context.Context, l *AgreeLease) error
	ListExpiredLeases(ctx context.Context, before time.Time, limit int) ([]*AgreeLease, error)
	ListActiveLeases(ctx context.Context, limit int) ([]*AgreeLease, error)
	ListActiveByAsset(ctx context.Context,
		asset string, limit int) ([]*AgreeLease, error)
	ListPlannedStale(ctx context.Context, before time.Time, limit int) ([]*AgreeLease, error)
	InsertProof(ctx context.Context, p *AgreeDriftProof) error
	GetProofByID(ctx context.Context, id string) (*AgreeDriftProof, error)
	GetProofByIdem(ctx context.Context, idemKey string) (*AgreeDriftProof, error)
	GetOpenProofByLease(ctx context.Context, leaseID string) (*AgreeDriftProof, error)
	UpdateProof(ctx context.Context, p *AgreeDriftProof) error
	ListProofsByState(ctx context.Context,
		state ProofState, limit int) ([]*AgreeDriftProof, error)
	InsertAudit(ctx context.Context,
		a *LeaseAudit) error
	ListAuditByLease(ctx context.Context, leaseID string, limit int) ([]*LeaseAudit, error)
	InsertGeneration(ctx context.Context, g *AgreeGeneration) error
	ListGenerations(ctx context.Context, limit int) ([]*AgreeGeneration, error)
	InsertHalfPlan(ctx context.Context, h *HalfPlanBlock) error
	GetHalfPlanByLease(ctx context.Context,
		leaseID string) (*HalfPlanBlock, error)
	ListHalfPlans(ctx context.Context, limit int) ([]*HalfPlanBlock, error)
}

// MemStore 线程安全内存实现。读写深拷贝。
type MemStore struct {
	mu sync.RWMutex
	leases    map[string]*AgreeLease
	leaseIdem map[string]string
	proofs    map[string]*AgreeDriftProof
	proofIdem map[string]string
	proofByLease map[string]string
	audits []LeaseAudit
	gens   []AgreeGeneration
	halfs  map[string]*HalfPlanBlock
}

// NewMemStore 创建空存储。
   func NewMemStore() *MemStore {
	  return &MemStore{
		   leases: map[string]*AgreeLease{},
		   leaseIdem: map[string]string{},
		   proofs: map[string]*AgreeDriftProof{},
		  proofIdem:    map[string]string{},
		   proofByLease: map[string]string{},
		  halfs:        map[string]*HalfPlanBlock{},
	  }
   }

   func cloneLease(l *AgreeLease) *AgreeLease {
	  if l == nil {
		  return nil
	  }
	   c := *l
	  if l.PlannedAt != nil {
		  t := *l.PlannedAt
		   c.PlannedAt = &t
	   }
	  if l.ExecutingAt != nil {
		   t := *l.ExecutingAt
		   c.ExecutingAt = &t
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

   func cloneProof(p *AgreeDriftProof) *AgreeDriftProof {
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

  func cloneHalf(h *HalfPlanBlock) *HalfPlanBlock {
	   if h == nil {
		   return nil
	  }
	  c := *h
	  return &c
   }

   func (m *MemStore) InsertLease(_ context.Context, l *AgreeLease) error {
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

  func (m *MemStore) GetLeaseByID(_ context.Context, id string) (*AgreeLease, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  l, ok := m.leases[id]
	  if !ok {
		   return nil, ErrNotFound
	   }
	   return cloneLease(l), nil
  }

   func (m *MemStore) GetLeaseByIdem(_ context.Context, idemKey string) (*AgreeLease, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   id, ok := m.leaseIdem[idemKey]
	  if !ok {
		   return nil, ErrNotFound
	   }
	   l, ok := m.leases[id]
	   if !ok {
		  return nil, ErrNotFound
	   }
	   return cloneLease(l), nil
   }

  func (m *MemStore) UpdateLease(_ context.Context, l *AgreeLease) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   if _, ok := m.leases[l.ID]; !ok {
		  return ErrNotFound
	   }
	  m.leases[l.ID] = cloneLease(l)
	   return nil
   }

   func (m *MemStore) ListExpiredLeases(_ context.Context, before time.Time, limit int) ([]*AgreeLease, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	  var out []*AgreeLease
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
	  sort.Slice(out, func(i, j int) bool { return out[i].ExpiresAt.Before(out[j].ExpiresAt) })
	  return out, nil
   }

   func (m *MemStore) ListActiveLeases(_ context.Context, limit int) ([]*AgreeLease, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	   var out []*AgreeLease
	  for _, l := range m.leases {
		  if !l.State.Active() {
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

  func (m *MemStore) ListActiveByAsset(_ context.Context, asset string, limit int) ([]*AgreeLease, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	   asset = upperTrim(asset)
	  var out []*AgreeLease
	  for _, l := range m.leases {
		   if !l.State.Active() {
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
	  sort.Slice(out, func(i, j int) bool { return out[i].IssuedAt.Before(out[j].IssuedAt) })
	  return out, nil
  }

  func (m *MemStore) ListPlannedStale(_ context.Context, before time.Time, limit int) ([]*AgreeLease, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	   var out []*AgreeLease
	  for _, l := range m.leases {
		   if l.State != LeaseStatePlanned {
			  continue
		   }
		  if l.PlannedAt != nil && (l.PlannedAt.Before(before) || l.PlannedAt.Equal(before)) {
			   out = append(out, cloneLease(l))
		   }
		   if limit > 0 && len(out) >= limit {
			  break
		   }
	  }
	   return out, nil
   }

   func (m *MemStore) InsertProof(_ context.Context, p *AgreeDriftProof) error {
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

   func (m *MemStore) GetProofByID(_ context.Context, id string) (*AgreeDriftProof, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  p, ok := m.proofs[id]
	   if !ok {
		   return nil, ErrNotFound
	  }
	  return cloneProof(p), nil
   }

  func (m *MemStore) GetProofByIdem(_ context.Context, idemKey string) (*AgreeDriftProof, error) {
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

   func (m *MemStore) GetOpenProofByLease(_ context.Context, leaseID string) (*AgreeDriftProof, error) {
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

   func (m *MemStore) UpdateProof(_ context.Context, p *AgreeDriftProof) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if _, ok := m.proofs[p.ID]; !ok {
		  return ErrNotFound
	  }
	  m.proofs[p.ID] = cloneProof(p)
	   if p.State != ProofStateOpen && p.LeaseID != "" {
		  if cur, ok := m.proofByLease[p.LeaseID]; ok && cur == p.ID {
			  delete(m.proofByLease, p.LeaseID)
		   }
	  }
	   return nil
  }

  func (m *MemStore) ListProofsByState(_ context.Context, state ProofState, limit int) ([]*AgreeDriftProof, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   var out []*AgreeDriftProof
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

   func (m *MemStore) InsertGeneration(_ context.Context, g *AgreeGeneration) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  cp := *g
	  m.gens = append(m.gens, cp)
	  return nil
   }

   func (m *MemStore) ListGenerations(_ context.Context, limit int) ([]*AgreeGeneration, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	   n := len(m.gens)
	  if limit <= 0 || limit > n {
		   limit = n
	  }
	   out := make([]*AgreeGeneration,
		   0, limit)
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

   func (m *MemStore) InsertHalfPlan(_ context.Context, h *HalfPlanBlock) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  if _, ok := m.halfs[h.LeaseID]; ok {
		  return ErrDuplicate
	  }
	  m.halfs[h.LeaseID] = cloneHalf(h)
	   return nil
   }

   func (m *MemStore) GetHalfPlanByLease(_ context.Context, leaseID string) (*HalfPlanBlock, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	  h, ok := m.halfs[leaseID]
	   if !ok {
		  return nil, ErrNotFound
	   }
	   return cloneHalf(h), nil
  }

  func (m *MemStore) ListHalfPlans(_ context.Context, limit int) ([]*HalfPlanBlock, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  var out []*HalfPlanBlock
	   for _, h := range m.halfs {
		   out = append(out, cloneHalf(h))
		  if limit > 0 && len(out) >= limit {
			   break
		   }
	  }
	  return out, nil
  }
