package lendoccupygate

import (
	"context"
	"sort"
	"sync"
	"time"
)

// MemStore 基于内存的 Store 实现（专用）。
type MemStore struct {
	mu sync.Mutex
	// gens 按 GenID 索引
	gens map[string]*LendGen
	// gensByOrder 按 OrderID 索引（每 order 至多一个 gen）
	gensByOrder map[string]*LendGen
	// leases 按 LeaseID 索引
	leases map[string]*LendOccupyLease
	// leasesByOrder 按 OrderID 索引（每 order 至多一个 lease）
	leasesByOrder map[string]*LendOccupyLease
	// dupProofs 按 ProofID 索引
	dupProofs map[string]*LendDupReleaseProof
	// dupProofsByGen 按 GenID 索引（每 gen 至多一条 dup 证明）
	dupProofsByGen map[string]*LendDupReleaseProof
	// residueProofs 按 ProofID 索引
	residueProofs map[string]*LendResidueProof
	// residueProofsByGen 按 GenID 索引（每 gen 至多一条 residue 证明）
	residueProofsByGen map[string]*LendResidueProof
	clock func() time.Time
}

// NewMemStore 构造内存 Store。
   func NewMemStore() *MemStore {
	   return &MemStore{
		  gens: make(map[string]*LendGen),
		  gensByOrder: make(map[string]*LendGen),
		  leases: make(map[string]*LendOccupyLease),
		   leasesByOrder: make(map[string]*LendOccupyLease),
		   dupProofs:      make(map[string]*LendDupReleaseProof),
		   dupProofsByGen: make(map[string]*LendDupReleaseProof),
		   residueProofs:  make(map[string]*LendResidueProof),
		  residueProofsByGen: make(map[string]*LendResidueProof),
		   clock:              func() time.Time { return time.Now().UTC() },
	   }
  }

// WithClock 注入时钟。

   func (m *MemStore) WithClock(fn func() time.Time) *MemStore {
	   if fn != nil {
		  m.clock = fn
	  }
	   return m
   }

   // ── LendGen ──────────────────────────────────────────────────────────────────

  func (m *MemStore) SaveGen(_ context.Context, g *LendGen) error {
	   if m == nil || g == nil {
		  return ErrUnavailable
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   // 幂等：同 GenID 已存在则忽略
	   if _, ok := m.gens[g.GenID]; ok {
		   return nil
	  }
	   cp := cloneGen(g)
	  m.gens[g.GenID] = cp
	   m.gensByOrder[g.OrderID] = cp
	  return nil
  }

   func (m *MemStore) GetGenByOrder(_ context.Context, orderID string) (*LendGen, error) {
	  if m == nil {
		  return nil, ErrUnavailable
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   g, ok := m.gensByOrder[orderID]
	  if !ok {
		   return nil, ErrGenNotFound
	   }
	  cp := cloneGen(g)
	   return cp, nil
  }

  func (m *MemStore) GetGen(_ context.Context, genID string) (*LendGen, error) {
	  if m == nil {
		  return nil, ErrUnavailable
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  g, ok := m.gens[genID]
	  if !ok {
		  return nil, ErrGenNotFound
	  }
	   cp := cloneGen(g)
	  return cp, nil
   }

  func (m *MemStore) ConsumeGen(_ context.Context, genID string, consumedBy string, at time.Time) error {
	  if m == nil {
		  return ErrUnavailable
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  g, ok := m.gens[genID]
	   if !ok {
		   return ErrGenNotFound
	   }
	  if g.Consumed {
		   return ErrGenConsumed
	   }
	   g.Consumed = true
	   g.ConsumedBy = consumedBy
	   t := at
	   g.ConsumedAt = &t
	   // 同步更新 byOrder 索引
	  m.gensByOrder[g.OrderID] = g
	  return nil
  }

// ── LendOccupyLease ──────────────────────────────────────────────────────────

  func (m *MemStore) SaveLease(_ context.Context, l *LendOccupyLease) error {
	   if m == nil || l == nil {
		   return ErrUnavailable
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   if _, ok := m.leasesByOrder[l.OrderID]; ok {
		  return ErrGenAlreadyExists
	  }
	  cp := cloneLease(l)
	   m.leases[l.LeaseID] = cp
	   m.leasesByOrder[l.OrderID] = cp
	  return nil
  }

  func (m *MemStore) GetLeaseByOrder(_ context.Context, orderID string) (*LendOccupyLease, error) {
	   if m == nil {
		   return nil, ErrUnavailable
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   l, ok := m.leasesByOrder[orderID]
	  if !ok {
		  return nil, ErrLeaseNotFound
	  }
	   return cloneLease(l), nil
   }

  func (m *MemStore) GetLease(_ context.Context, leaseID string) (*LendOccupyLease, error) {
	   if m == nil {
		   return nil, ErrUnavailable
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  l, ok := m.leases[leaseID]
	   if !ok {
		  return nil, ErrLeaseNotFound
	   }
	  return cloneLease(l), nil
   }

  func (m *MemStore) UpdateLeaseState(_ context.Context, leaseID string, from, to LeaseState, patch *LendOccupyLease) error {
	   if m == nil {
		   return ErrUnavailable
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   l, ok := m.leases[leaseID]
	   if !ok {
		  return ErrLeaseNotFound
	   }
	   if l.State != from {
		   return ErrInvalidTransition
	  }
	   l.State = to
	  l.UpdatedAt = m.clock()
	   if patch != nil {
		   applyLeasePatch(l, patch)
	   }
	   m.leasesByOrder[l.OrderID] = l
	   return nil
   }

  func (m *MemStore) ListStalePlacedLeases(_ context.Context, staleThreshold time.Time, limit int) ([]*LendOccupyLease, error) {
	  if m == nil {
		  return nil, ErrUnavailable
	  }
	  if limit <= 0 {
		  limit = 200
	  }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  var out []*LendOccupyLease
	   for _, l := range m.leases {
		  if l.State == LeasePlaced && l.PlacedAt.Before(staleThreshold) {
			   cp := cloneLease(l)
			   out = append(out, cp)
		   }
	   }
	  sort.Slice(out, func(i, j int) bool {
		   return out[i].PlacedAt.Before(out[j].PlacedAt)
	  })
	  if len(out) > limit {
		  out = out[:limit]
	   }
	   return out, nil
   }

  func (m *MemStore) ListLeasesByUser(_ context.Context, userID string, limit int) ([]*LendOccupyLease, error) {
	   if m == nil {
		   return nil, ErrUnavailable
	  }
	   if limit <= 0 {
		   limit = 50
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   var out []*LendOccupyLease
	   for _, l := range m.leases {
		  if l.UserID == userID {
			  out = append(out, cloneLease(l))
		   }
	  }
	  sort.Slice(out, func(i, j int) bool {
		   return out[i].PlacedAt.After(out[j].PlacedAt)
	  })
	   if len(out) > limit {
		   out = out[:limit]
	  }
	   return out, nil
   }

// ── LendDupReleaseProof ───────────────────────────────────────────────────────

   func (m *MemStore) SaveDupProof(_ context.Context, p *LendDupReleaseProof) error {
	   if m == nil || p == nil {
		   return ErrUnavailable
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  if _, ok := m.dupProofsByGen[p.GenID]; ok {
		   return ErrAlreadyProven
	   }
	   cp := cloneDupProof(p)
	  m.dupProofs[p.ProofID] = cp
	  m.dupProofsByGen[p.GenID] = cp
	  return nil
   }

  func (m *MemStore) GetDupProofByGen(_ context.Context, genID string) (*LendDupReleaseProof, error) {
	  if m == nil {
		  return nil, ErrUnavailable
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   p, ok := m.dupProofsByGen[genID]
	  if !ok {
		   return nil, ErrDupProofNotFound
	  }
	  return cloneDupProof(p), nil
  }

  func (m *MemStore) GetDupProof(_ context.Context, proofID string) (*LendDupReleaseProof, error) {
	  if m == nil {
		   return nil, ErrUnavailable
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  p, ok := m.dupProofs[proofID]
	  if !ok {
		  return nil, ErrDupProofNotFound
	  }
	   return cloneDupProof(p), nil
  }

  func (m *MemStore) ListDupProofsByOrder(_ context.Context, orderID string) ([]*LendDupReleaseProof, error) {
	   if m == nil {
		  return nil, ErrUnavailable
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   var out []*LendDupReleaseProof
	  for _, p := range m.dupProofs {
		  if p.OrderID == orderID {
			   out = append(out, cloneDupProof(p))
		  }
	  }
	  sort.Slice(out, func(i, j int) bool {
		   return out[i].AttemptedAt.Before(out[j].AttemptedAt)
	   })
	  return out, nil
   }

// ── LendResidueProof ──────────────────────────────────────────────────────────

  func (m *MemStore) SaveResidueProof(_ context.Context, p *LendResidueProof) error {
	  if m == nil || p == nil {
		   return ErrUnavailable
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   if _, ok := m.residueProofsByGen[p.GenID]; ok {
		   return ErrAlreadyProven
	  }
	  cp := cloneResidueProof(p)
	   m.residueProofs[p.ProofID] = cp
	  m.residueProofsByGen[p.GenID] = cp
	  return nil
  }

   func (m *MemStore) GetResidueProofByGen(_ context.Context, genID string) (*LendResidueProof, error) {
	   if m == nil {
		   return nil, ErrUnavailable
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   p, ok := m.residueProofsByGen[genID]
	  if !ok {
		   return nil,
			   ErrResidueProofNotFound
	  }
	  return cloneResidueProof(p), nil
  }

   func (m *MemStore) GetResidueProof(_ context.Context, proofID string) (*LendResidueProof, error) {
	  if m == nil {
		   return nil, ErrUnavailable
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   p, ok := m.residueProofs[proofID]
	   if !ok {
		   return nil, ErrResidueProofNotFound
	  }
	   return cloneResidueProof(p), nil
  }

  func (m *MemStore) ListResidueProofsByOrder(_ context.Context, orderID string) ([]*LendResidueProof, error) {
	   if m == nil {
		   return nil, ErrUnavailable
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  var out []*LendResidueProof
	   for _, p := range m.residueProofs {
		  if p.OrderID == orderID {
			   out = append(out, cloneResidueProof(p))
		  }
	  }
	   sort.Slice(out, func(i, j int) bool {
		   return out[i].CreatedAt.Before(out[j].CreatedAt)
	   })
	   return out, nil
  }

// ── 克隆辅助 ─────────────────────────────────────────────────────────────────

   func cloneGen(g *LendGen) *LendGen {
	  cp := *g
	   if g.ConsumedAt != nil {
		  t := *g.ConsumedAt
		   cp.ConsumedAt = &t
	  }
	  return &cp
  }

   func cloneLease(l *LendOccupyLease) *LendOccupyLease {
	  cp := *l
	   if l.SettledAt != nil {
		   t := *l.SettledAt
		   cp.SettledAt = &t
	   }
	  if l.CancelledAt != nil {
		  t := *l.CancelledAt
		  cp.CancelledAt = &t
	  }
	   if l.ResidueClosedAt != nil {
		   t := *l.ResidueClosedAt
		   cp.ResidueClosedAt = &t
	  }
	   return &cp
   }

   func cloneDupProof(p *LendDupReleaseProof) *LendDupReleaseProof {
	  cp := *p
	  return &cp
  }

  func cloneResidueProof(p *LendResidueProof) *LendResidueProof {
	   cp := *p
	  return &cp
  }

// applyLeasePatch 将 patch 中非零字段覆盖到 target（UpdateLeaseState 辅助）。

  func applyLeasePatch(target, patch *LendOccupyLease) {
	   if patch.SettledAt != nil {
		  t := *patch.SettledAt
		  target.SettledAt = &t
	   }
	  if patch.CancelledAt != nil {
		   t := *patch.CancelledAt
		  target.CancelledAt = &t
	  }
	   if patch.ResidueClosedAt != nil {
		   t := *patch.ResidueClosedAt
		   target.ResidueClosedAt = &t
	   }
	  if patch.ResidueProofID != "" {
		   target.ResidueProofID = patch.ResidueProofID
	   }
  }
