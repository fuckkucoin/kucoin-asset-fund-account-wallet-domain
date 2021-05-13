package fiatbalgate

import (
	"context"
	"sort"
	"sync"
	"time"
)

// MemStore 基于内存的 Store 实现（专用）。

type MemStore struct {
	mu sync.RWMutex
	cases       map[string]*FiatImbalanceCase  // key=CaseID
	proofs      map[string]*FiatImbalanceProof // key=ProofID
	proofByCase map[string]*FiatImbalanceProof // key=CaseID
	gens      map[uint64]*JournalGeneration // key=Seq
	approvals map[string][]*ClearApproval   // key=CaseID
	clock     func() time.Time
}

// NewMemStore 构造内存 Store。

   func NewMemStore() *MemStore {
	   return &MemStore{
		  cases:  make(map[string]*FiatImbalanceCase),
		   proofs: make(map[string]*FiatImbalanceProof),
		   proofByCase: make(map[string]*FiatImbalanceProof),
		   gens:        make(map[uint64]*JournalGeneration),
		  approvals: make(map[string][]*ClearApproval),
		  clock:     func() time.Time { return time.Now().UTC() },
	   }
  }

// WithClock 注入时钟。

  func (m *MemStore) WithClock(fn func() time.Time) *MemStore {
	   if fn != nil {
		   m.clock = fn
	  }
	  return m
  }

  // ── JournalGeneration ─────────────────────────────────────────────────────────

   func (m *MemStore) SaveGeneration(_ context.Context, g *JournalGeneration) error {
	   if m == nil || g == nil {
		   return ErrUnavailable
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   if _, exists := m.gens[g.Seq]; exists {
		  return nil // 幂等：同 Seq 已存在，静默忽略
	  }
	  cp := *g
	  m.gens[g.Seq] = &cp
	   return nil
   }

  func (m *MemStore) GetGenerationBySeq(_ context.Context, seq uint64) (*JournalGeneration, error) {
	   if m == nil {
		  return nil, ErrUnavailable
	  }
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  g, ok := m.gens[seq]
	  if !ok {
		  return nil, ErrGenNotFound
	   }
	   cp := *g
	   return &cp, nil
   }

   func (m *MemStore) MaxGenSeq(_ context.Context) (uint64, error) {
	  if m == nil {
		   return 0, ErrUnavailable
	   }
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	   var max uint64
	   for seq := range m.gens {
		  if seq > max {
			  max = seq
		  }
	  }
	   return max, nil
  }

// ── FiatImbalanceCase ─────────────────────────────────────────────────────────

   func (m *MemStore) SaveCase(_ context.Context, c *FiatImbalanceCase) error {
	  if m == nil || c == nil {
		   return ErrUnavailable
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   cp := c.Copy()
	  m.cases[c.CaseID] = &cp
	  return nil
   }

  func (m *MemStore) GetCase(_ context.Context, caseID string) (*FiatImbalanceCase, error) {
	  if m == nil {
		  return nil, ErrUnavailable
	   }
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  c, ok := m.cases[caseID]
	  if !ok {
		  return nil, ErrCaseNotFound
	  }
	   cp := c.Copy()
	   return &cp, nil
  }

  func (m *MemStore) UpdateCaseState(_ context.Context, caseID string, from, to CaseState, patch *FiatImbalanceCase) error {
	   if m == nil {
		  return ErrUnavailable
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   c, ok := m.cases[caseID]
	   if !ok {
		  return ErrCaseNotFound
	  }
	  if c.State != from {
		  return ErrInvalidTransition
	   }
	  c.State = to
	  c.UpdatedAt = m.clock()
	  if patch != nil {
		   if !patch.BlockingAt.IsZero() || patch.BlockingAt != nil {
			   c.BlockingAt = patch.BlockingAt
		   }
		  if patch.ClearedAt != nil {
			   c.ClearedAt = patch.ClearedAt
		   }
		  if patch.ClearedBy != "" {
			  c.ClearedBy = patch.ClearedBy
		   }
		  if patch.ClearNote != "" {
			   c.ClearNote = patch.ClearNote
		  }
		   if patch.ProofID != "" {
			   c.ProofID = patch.ProofID
		   }
	  }
	   return nil
   }

  func (m *MemStore) ListOpenOrBlockingCases(_ context.Context, asset string, limit int) ([]*FiatImbalanceCase, error) {
	   if m == nil {
		   return nil, ErrUnavailable
	   }
	  if limit <= 0 {
		  limit = 100
	   }
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  var out []*FiatImbalanceCase
	   for _, c := range m.cases {
		   if c.State != CaseOpen && c.State != CaseBlocking {
			   continue
		  }
		   if asset != "" && c.Asset != asset {
			  continue
		   }
		  cp := c.Copy()
		   out = append(out, &cp)
	  }
	  sort.Slice(out, func(i, j int) bool {
		  return out[i].OpenedAt.After(out[j].OpenedAt)
	   })
	   if len(out) > limit {
		  out = out[:limit]
	   }
	  return out, nil
  }

  func (m *MemStore) ListCasesByAsset(_ context.Context, asset string, limit int) ([]*FiatImbalanceCase, error) {
	  if m == nil {
		  return nil, ErrUnavailable
	  }
	  if limit <= 0 {
		  limit = 50
	   }
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   var out []*FiatImbalanceCase
	  for _, c := range m.cases {
		   if asset != "" && c.Asset != asset {
			  continue
		  }
		   cp := c.Copy()
		   out = append(out, &cp)
	   }
	   sort.Slice(out, func(i, j int) bool {
		  return out[i].OpenedAt.After(out[j].OpenedAt)
	  })
	  if len(out) > limit {
		  out = out[:limit]
	   }
	  return out, nil
   }

   func (m *MemStore) ListExpiredOpenCases(_ context.Context, staleThreshold time.Time, limit int) ([]*FiatImbalanceCase, error) {
	   if m == nil {
		  return nil, ErrUnavailable
	  }
	   if limit <= 0 {
		   limit = 100
	   }
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   var out []*FiatImbalanceCase
	  for _, c := range m.cases {
		   if c.State != CaseOpen {
			  continue
		   }
		  if c.OpenedAt.After(staleThreshold) {
			   continue
		   }
		  cp := c.Copy()
		  out = append(out, &cp)
		  if len(out) >= limit {
			   break
		  }
	   }
	   return out, nil
  }

// ── FiatImbalanceProof ────────────────────────────────────────────────────────

  func (m *MemStore) SaveProof(_ context.Context, p *FiatImbalanceProof) error {
	  if m == nil || p == nil {
		   return ErrUnavailable
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if _, exists := m.proofByCase[p.CaseID]; exists {
		   return ErrAlreadyProven
	  }
	   cp := p.Copy()
	  m.proofs[p.ProofID] = &cp
	   m.proofByCase[p.CaseID] = &cp
	   return nil
  }

  func (m *MemStore) GetProofByCase(_ context.Context, caseID string) (*FiatImbalanceProof, error) {
	  if m == nil {
		  return nil, ErrUnavailable
	  }
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	   p, ok := m.proofByCase[caseID]
	   if !ok {
		  return nil, ErrProofNotFound
	  }
	   cp := p.Copy()
	  return &cp, nil
   }

   func (m *MemStore) GetProof(_ context.Context, proofID string) (*FiatImbalanceProof, error) {
	   if m == nil {
		   return nil, ErrUnavailable
	  }
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  p, ok := m.proofs[proofID]
	  if !ok {
		  return nil, ErrProofNotFound
	  }
	  cp := p.Copy()
	  return &cp, nil
   }

   func (m *MemStore) UpdateProofState(_ context.Context, proofID string, from, to ProofState, patch *FiatImbalanceProof) error {
	   if m == nil {
		  return ErrUnavailable
	  }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  p, ok := m.proofs[proofID]
	  if !ok {
		   return ErrProofNotFound
	  }
	  if p.State != from {
		   return ErrInvalidTransition
	   }
	  p.State = to
	   p.UpdatedAt = m.clock()
	  if patch != nil {
		   if patch.ExportedAt != nil {
			   p.ExportedAt = patch.ExportedAt
		   }
		   if patch.VerifiedAt != nil {
			  p.VerifiedAt = patch.VerifiedAt
		  }
		  if patch.VerifiedBy != "" {
			   p.VerifiedBy = patch.VerifiedBy
		  }
	   }
	  // 同步 proofByCase 指针
	   if pb, ok2 := m.proofByCase[p.CaseID]; ok2 {
		   *pb = *p
	  }
	  return nil
   }

// ── ClearApproval ─────────────────────────────────────────────────────────────

  func (m *MemStore) SaveApproval(_ context.Context, a *ClearApproval) error {
	  if m == nil || a == nil {
		   return ErrUnavailable
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   cp := *a
	   m.approvals[a.CaseID] = append(m.approvals[a.CaseID], &cp)
	   return nil
   }

  func (m *MemStore) ListApprovalsByCase(_ context.Context, caseID string) ([]*ClearApproval, error) {
	   if m == nil {
		   return nil, ErrUnavailable
	  }
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  src := m.approvals[caseID]
	   out := make([]*ClearApproval, len(src))
	   for i, a := range src {
		   cp := *a
		  out[i] = &cp
	   }
	   return out, nil
  }

// ── 辅助 ──────────────────────────────────────────────────────────────────────

   func (m *MemStore) HasActiveBlockingForAsset(_ context.Context, asset string) (bool, error) {
	  if m == nil {
		   return false, ErrUnavailable
	   }
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	   for _, c := range m.cases {
		   if c.Asset == asset && (c.State == CaseOpen || c.State == CaseBlocking) {
			   return true, nil
		   }
	   }
	   return false, nil
   }
