package liqgengate

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Store 覆盖世代、租约、证明、审计、互斥占位。
type Store interface {
	InsertGeneration(ctx context.Context, g *LiqGeneration) error
	GetGeneration(ctx context.Context, borrowID string) (*LiqGeneration, error)
	UpdateGeneration(ctx context.Context, g *LiqGeneration) error
	ListGenerations(ctx context.Context, limit int) ([]*LiqGeneration, error)
	ListByPhase(ctx context.Context, phase LiqPhase, limit int) ([]*LiqGeneration, error)
	InsertLease(ctx context.Context, l *LiqFreezeLease) error
	GetLeaseByID(ctx context.Context, id string) (*LiqFreezeLease, error)
	GetLeaseByIdem(ctx context.Context, idemKey string) (*LiqFreezeLease, error)
	GetActiveLease(ctx context.Context,
		borrowID string) (*LiqFreezeLease, error)
	UpdateLease(ctx context.Context, l *LiqFreezeLease) error
	ListExpiredLeases(ctx context.Context, before time.Time, limit int) ([]*LiqFreezeLease, error)
	ListActiveLeases(ctx context.Context, limit int) ([]*LiqFreezeLease, error)
	ListLeasesByBorrow(ctx context.Context,
		borrowID string, limit int) ([]*LiqFreezeLease, error)
	InsertProof(ctx context.Context, p *LiqProof) error
	GetProofByID(ctx context.Context, id string) (*LiqProof, error)
	GetProofByIdem(ctx context.Context, idemKey string) (*LiqProof, error)
	UpdateProof(ctx context.Context, p *LiqProof) error
	ListProofsByState(ctx context.Context, state ProofState, limit int) ([]*LiqProof, error)
	ListProofsByBorrow(ctx context.Context, borrowID string, limit int) ([]*LiqProof, error)
	ListProofsByKind(ctx context.Context, kind ProofKind, limit int) ([]*LiqProof, error)
	InsertAudit(ctx context.Context, a *LiqAudit) error
	ListAuditByBorrow(ctx context.Context, borrowID string, limit int) ([]*LiqAudit, error)
	TryAcquireMutex(ctx context.Context, rec *MutexRecord) error
	ReleaseMutex(ctx context.Context, borrowID, owner string) error
	GetMutex(ctx context.Context,
		borrowID string) (*MutexRecord, error)
}

// MemStore 线程安全内存实现。读写深拷贝。
type MemStore struct {
	mu sync.RWMutex
	gens   map[string]*LiqGeneration
	leases map[string]*LiqFreezeLease
	leaseIdem map[string]string
	active map[string]string
	proofs map[string]*LiqProof
	proofIdem map[string]string
	audits  []LiqAudit
	mutexes map[string]*MutexRecord
}

// NewMemStore 创建空存储。
  func NewMemStore() *MemStore {
	  return &MemStore{
		   gens: map[string]*LiqGeneration{},
		  leases: map[string]*LiqFreezeLease{},
		  leaseIdem: map[string]string{},
		  active:    map[string]string{},
		   proofs:    map[string]*LiqProof{},
		   proofIdem: map[string]string{},
		  mutexes: map[string]*MutexRecord{},
	  }
  }

   func cloneGen(g *LiqGeneration) *LiqGeneration {
	   if g == nil {
		  return nil
	  }
	   c := *g
	  return &c
   }

   func cloneLease(l *LiqFreezeLease) *LiqFreezeLease {
	   if l == nil {
		   return nil
	  }
	   c := *l
	  if l.FrozenAt != nil {
		  t := *l.FrozenAt
		   c.FrozenAt = &t
	  }
	  if l.ReleasedAt != nil {
		   t := *l.ReleasedAt
		   c.ReleasedAt = &t
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

   func cloneProof(p *LiqProof) *LiqProof {
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

  func cloneMutex(m *MutexRecord) *MutexRecord {
	  if m == nil {
		  return nil
	  }
	  c := *m
	  return &c
   }

  func (m *MemStore) InsertGeneration(_ context.Context, g *LiqGeneration) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  if _, ok := m.gens[g.BorrowID]; ok {
		  return ErrDuplicate
	   }
	  m.gens[g.BorrowID] = cloneGen(g)
	   return nil
  }

  func (m *MemStore) GetGeneration(_ context.Context, borrowID string) (*LiqGeneration, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   g, ok := m.gens[borrowID]
	  if !ok {
		  return nil, ErrNotFound
	  }
	   return cloneGen(g), nil
   }

   func (m *MemStore) UpdateGeneration(_ context.Context, g *LiqGeneration) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   if _, ok := m.gens[g.BorrowID]; !ok {
		  return ErrNotFound
	  }
	  m.gens[g.BorrowID] = cloneGen(g)
	  return nil
  }

   func (m *MemStore) ListGenerations(_ context.Context, limit int) ([]*LiqGeneration, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   out := make([]*LiqGeneration, 0, len(m.gens))
	   for _, g := range m.gens {
		  out = append(out, cloneGen(g))
	   }
	   sort.Slice(out, func(i, j int) bool {
		  if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			   return out[i].BorrowID < out[j].BorrowID
		   }
		   return out[i].UpdatedAt.After(out[j].UpdatedAt)
	   })
	   if limit > 0 && len(out) > limit {
		  out = out[:limit]
	   }
	   return out, nil
  }

   func (m *MemStore) ListByPhase(_ context.Context, phase LiqPhase, limit int) ([]*LiqGeneration, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  var out []*LiqGeneration
	   for _, g := range m.gens {
		  if g.Phase == phase {
			   out = append(out, cloneGen(g))
		   }
		  if limit > 0 && len(out) >= limit {
			  break
		   }
	  }
	  sort.Slice(out, func(i, j int) bool { return out[i].BorrowID < out[j].BorrowID })
	   return out, nil
  }

  func (m *MemStore) InsertLease(_ context.Context, l *LiqFreezeLease) error {
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
	  if l.State.Active() {
		   m.active[l.BorrowID] = l.ID
	  }
	  return nil
  }

  func (m *MemStore) GetLeaseByID(_ context.Context, id string) (*LiqFreezeLease, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  l, ok := m.leases[id]
	   if !ok {
		  return nil, ErrNotFound
	  }
	   return cloneLease(l), nil
   }

  func (m *MemStore) GetLeaseByIdem(_ context.Context, idemKey string) (*LiqFreezeLease, error) {
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

   func (m *MemStore) GetActiveLease(_ context.Context, borrowID string) (*LiqFreezeLease, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   id, ok := m.active[borrowID]
	   if !ok {
		   return nil, ErrNotFound
	  }
	   l, ok := m.leases[id]
	   if !ok || !l.State.Active() {
		   return nil, ErrNotFound
	   }
	   return cloneLease(l), nil
   }

  func (m *MemStore) UpdateLease(_ context.Context, l *LiqFreezeLease) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  if _, ok := m.leases[l.ID]; !ok {
		   return ErrNotFound
	  }
	  m.leases[l.ID] = cloneLease(l)
	   if l.State.Active() {
		  m.active[l.BorrowID] = l.ID
	   } else if m.active[l.BorrowID] == l.ID {
		  delete(m.active, l.BorrowID)
	   }
	   return nil
   }

  func (m *MemStore) ListExpiredLeases(_ context.Context, before time.Time, limit int) ([]*LiqFreezeLease, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   var out []*LiqFreezeLease
	   for _, l := range m.leases {
		   if l.State.Closed() {
			   continue
		   }
		  if !l.ExpiresAt.IsZero() && (l.ExpiresAt.Before(before) || l.ExpiresAt.Equal(before)) {
			  out = append(out, cloneLease(l))
		  }
		   if limit > 0 && len(out) >= limit {
			   break
		  }
	  }
	   return out, nil
   }

  func (m *MemStore) ListActiveLeases(_ context.Context, limit int) ([]*LiqFreezeLease, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   var out []*LiqFreezeLease
	   for _, l := range m.leases {
		   if l.State.Active() {
			  out = append(out, cloneLease(l))
		   }
		  if limit > 0 && len(out) >= limit {
			   break
		   }
	  }
	   sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	   return out, nil
  }

   func (m *MemStore) ListLeasesByBorrow(_ context.Context, borrowID string, limit int) ([]*LiqFreezeLease, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  var out []*LiqFreezeLease
	   for _, l := range m.leases {
		  if l.BorrowID == borrowID {
			  out = append(out,
				   cloneLease(l))
		   }
	   }
	  sort.Slice(out, func(i, j int) bool { return out[i].IssuedAt.After(out[j].IssuedAt) })
	   if limit > 0 && len(out) > limit {
		   out = out[:limit]
	   }
	   return out, nil
   }

   func (m *MemStore) InsertProof(_ context.Context, p *LiqProof) error {
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
	  m.proofs[p.ID] = cloneProof(p)
	  return nil
  }

  func (m *MemStore) GetProofByID(_ context.Context, id string) (*LiqProof, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   p, ok := m.proofs[id]
	   if !ok {
		   return nil, ErrNotFound
	   }
	   return cloneProof(p), nil
   }

   func (m *MemStore) GetProofByIdem(_ context.Context, idemKey string) (*LiqProof, error) {
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

  func (m *MemStore) UpdateProof(_ context.Context, p *LiqProof) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   if _, ok := m.proofs[p.ID]; !ok {
		   return ErrNotFound
	  }
	  m.proofs[p.ID] = cloneProof(p)
	  return nil
   }

   func (m *MemStore) ListProofsByState(_ context.Context, state ProofState, limit int) ([]*LiqProof, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  var out []*LiqProof
	  for _, p := range m.proofs {
		   if p.State == state {
			  out = append(out, cloneProof(p))
		  }
		  if limit > 0 && len(out) >= limit {
			  break
		   }
	   }
	  sort.Slice(out,
		  func(i, j int) bool { return out[i].ID < out[j].ID })
	  return out, nil
  }

   func (m *MemStore) ListProofsByBorrow(_ context.Context, borrowID string, limit int) ([]*LiqProof, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	  var out []*LiqProof
	  for _, p := range m.proofs {
		   if p.BorrowID == borrowID {
			  out = append(out, cloneProof(p))
		   }
	   }
	   sort.Slice(out,
		   func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	  if limit > 0 && len(out) > limit {
		  out = out[:limit]
	   }
	  return out, nil
  }

  func (m *MemStore) ListProofsByKind(_ context.Context, kind ProofKind, limit int) ([]*LiqProof, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	   var out []*LiqProof
	   for _, p := range m.proofs {
		  if p.Kind == kind {
			   out = append(out, cloneProof(p))
		   }
		   if limit > 0 && len(out) >= limit {
			   break
		  }
	   }
	  sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	   return out, nil
   }

   func (m *MemStore) InsertAudit(_ context.Context, a *LiqAudit) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  cp := *a
	  m.audits = append(m.audits, cp)
	   return nil
   }

   func (m *MemStore) ListAuditByBorrow(_ context.Context, borrowID string, limit int) ([]*LiqAudit, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  var out []*LiqAudit
	  for i := range m.audits {
		   if m.audits[i].BorrowID == borrowID {
			  cp := m.audits[i]
			   out = append(out, &cp)
		  }
	   }
	  if limit > 0 && len(out) > limit {
		   out = out[:limit]
	  }
	  return out, nil
  }

   func (m *MemStore) TryAcquireMutex(_ context.Context, rec *MutexRecord) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if cur, ok := m.mutexes[rec.BorrowID]; ok {
		  if cur.Owner == rec.Owner {
			  return nil
		  }
		   return ErrMutexHeld
	   }
	  m.mutexes[rec.BorrowID] = cloneMutex(rec)
	   return nil
   }

  func (m *MemStore) ReleaseMutex(_ context.Context, borrowID, owner string) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   cur, ok := m.mutexes[borrowID]
	  if !ok {
		   return nil
	   }
	   if cur.Owner != owner {
		   return ErrMutexHeld
	   }
	  delete(m.mutexes, borrowID)
	  return nil
   }

  func (m *MemStore) GetMutex(_ context.Context, borrowID string) (*MutexRecord, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   cur, ok := m.mutexes[borrowID]
	   if !ok {
		  return nil, ErrNotFound
	   }
	  return cloneMutex(cur), nil
   }
