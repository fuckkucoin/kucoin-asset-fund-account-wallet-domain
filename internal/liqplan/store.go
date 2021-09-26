package liqplan

import (
	"sync"
	"time"
)

// ─────────────────────────────────────────────
// PlanStore
// ─────────────────────────────────────────────

type PlanStore struct {
	mu       sync.Mutex
	plans    map[string]*LiqPlan
	byBorrow map[string]string // borrowID → planID (active)
	byIdem map[string]string // idempotencyKey → planID
}

   func NewPlanStore() *PlanStore {
	   return &PlanStore{
		   plans:    make(map[string]*LiqPlan),
		  byBorrow: make(map[string]string),
		   byIdem:   make(map[string]string),
	   }
   }

  func (s *PlanStore) Put(p LiqPlan) {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  cp := p
	   cp.UpdatedAt = time.Now().UTC()
	  s.plans[p.ID] = &cp
	   if p.BorrowID != "" && !PlanTerminal(p.State) {
		  s.byBorrow[p.BorrowID] = p.ID
	  } else if PlanTerminal(p.State) {
		   delete(s.byBorrow, p.BorrowID)
	  }
	   if p.IdempotencyKey != "" {
		  s.byIdem[p.IdempotencyKey] = p.ID
	   }
  }

   func (s *PlanStore) Get(id string) (LiqPlan, bool) {
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   p, ok := s.plans[id]
	  if !ok {
		   return LiqPlan{}, false
	  }
	   return *p, true
  }

   func (s *PlanStore) ByBorrow(borrowID string) (LiqPlan, bool) {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  pid, ok := s.byBorrow[borrowID]
	  if !ok {
		   return LiqPlan{}, false
	  }
	   p := s.plans[pid]
	   if p == nil {
		  return LiqPlan{}, false
	   }
	   return *p, true
   }

   func (s *PlanStore) ByIdem(key string) (LiqPlan, bool) {
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  pid, ok := s.byIdem[key]
	  if !ok {
		  return LiqPlan{}, false
	   }
	  p := s.plans[pid]
	   if p == nil {
		  return LiqPlan{}, false
	   }
	   return *p, true
   }

// List 返回所有计划（快照副本）。
  func (s *PlanStore) List() []LiqPlan {
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   out := make([]LiqPlan, 0, len(s.plans))
	  for _, p := range s.plans {
		   out = append(out, *p)
	   }
	  return out
  }

// ListByState 按状态过滤。
  func (s *PlanStore) ListByState(st PlanState) []LiqPlan {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  var out []LiqPlan
	   for _, p := range s.plans {
		  if p.State == st {
			  out = append(out, *p)
		   }
	  }
	   return out
   }

// ListByUser 按用户过滤（含所有状态）。
   func (s *PlanStore) ListByUser(userID string) []LiqPlan {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   var out []LiqPlan
	   for _, p := range s.plans {
		  if p.UserID == userID {
			  out = append(out, *p)
		   }
	  }
	   return out
   }

// ─────────────────────────────────────────────
// LegStore
// ─────────────────────────────────────────────

type LegStore struct {
	mu sync.Mutex
	legs   map[string]*SeizeLeg
	byPlan map[string][]string // planID → []legID
	byIdem map[string]string // idempotency → legID
}

   func NewLegStore() *LegStore {
	  return &LegStore{
		   legs:   make(map[string]*SeizeLeg),
		   byPlan: make(map[string][]string),
		   byIdem: make(map[string]string),
	  }
   }

   func (s *LegStore) Put(l SeizeLeg) {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   cp := l
	   cp.UpdatedAt = time.Now().UTC()
	  s.legs[l.ID] = &cp
	  s.byPlan[l.PlanID] = appendUniq(s.byPlan[l.PlanID], l.ID)
	  if l.IdempotencyKey != "" {
		  s.byIdem[l.IdempotencyKey] = l.ID
	  }
  }

  func (s *LegStore) Get(id string) (SeizeLeg, bool) {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   l, ok := s.legs[id]
	   if !ok {
		   return SeizeLeg{}, false
	  }
	  return *l, true
  }

   func (s *LegStore) ByIdem(key string) (SeizeLeg, bool) {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  id, ok := s.byIdem[key]
	   if !ok {
		  return SeizeLeg{}, false
	   }
	   l := s.legs[id]
	   if l == nil {
		  return SeizeLeg{}, false
	   }
	   return *l, true
   }

   func (s *LegStore) ByPlan(planID string) []SeizeLeg {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   ids := s.byPlan[planID]
	  out := make([]SeizeLeg,
		  0, len(ids))
	   for _, id := range ids {
		  if l := s.legs[id]; l != nil {
			  out = append(out, *l)
		  }
	   }
	   return out
   }

// ByPlanAndState 按计划+腿状态过滤。
   func (s *LegStore) ByPlanAndState(planID string, st LegState) []SeizeLeg {
	  all := s.ByPlan(planID)
	   var out []SeizeLeg
	   for _, l := range all {
		   if l.State == st {
			  out = append(out, l)
		   }
	   }
	  return out
   }

// HasResidue 检查计划是否有残留腿。
   func (s *LegStore) HasResidue(planID string) bool {
	   all := s.ByPlan(planID)
	  for _, l := range all {
		  if l.HasResidue() {
			  return true
		  }
	  }
	  return false
  }

// ─────────────────────────────────────────────
// MarkGateStore
// ─────────────────────────────────────────────

type MarkGateStore struct {
	mu sync.Mutex
	gates map[string]*MarkGate
}

  func NewMarkGateStore() *MarkGateStore {
	   return &MarkGateStore{gates: make(map[string]*MarkGate)}
   }

  func (s *MarkGateStore) Put(g MarkGate) {
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  cp := g
	   s.gates[g.ID] = &cp
  }

   func (s *MarkGateStore) Get(id string) (MarkGate, bool) {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  g, ok := s.gates[id]
	   if !ok {
		   return MarkGate{}, false
	   }
	  return *g, true
  }

// Latest 返回某资产最新世代（generation 最大）。
  func (s *MarkGateStore) Latest(asset string) (MarkGate, bool) {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  var found *MarkGate
	  for _, g := range s.gates {
		   if g.Asset == asset {
			   if found == nil || g.Generation > found.Generation {
				  found = g
			   }
		   }
	  }
	  if found == nil {
		   return MarkGate{}, false
	  }
	   return *found, true
   }

// ─────────────────────────────────────────────
// ProofStore
// ─────────────────────────────────────────────

type ProofStore struct {
	mu sync.Mutex
	proofs map[string]*LiqProof
	byPlan map[string]string // planID → proofID
}

   func NewProofStore() *ProofStore {
	   return &ProofStore{
		  proofs: make(map[string]*LiqProof),
		   byPlan: make(map[string]string),
	  }
   }

  func (s *ProofStore) Put(p LiqProof) {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  cp := p
	  s.proofs[p.ID] = &cp
	  s.byPlan[p.PlanID] = p.ID
   }

  func (s *ProofStore) Get(id string) (LiqProof, bool) {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  p, ok := s.proofs[id]
	  if !ok {
		  return LiqProof{}, false
	   }
	   return *p, true
   }

  func (s *ProofStore) ByPlan(planID string) (LiqProof, bool) {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  pid, ok := s.byPlan[planID]
	  if !ok {
		   return LiqProof{}, false
	  }
	  p := s.proofs[pid]
	   if p == nil {
		   return LiqProof{}, false
	   }
	  return *p, true
   }

// OccupyStore 追踪活跃占用令牌，防止同一 BorrowID 并发开两个计划。
type OccupyStore struct {
	mu     sync.Mutex
	tokens map[string]*OccupyToken // borrowID → token
}

  func NewOccupyStore() *OccupyStore {
	  return &OccupyStore{tokens: make(map[string]*OccupyToken)}
   }

  func (s *OccupyStore) Acquire(t OccupyToken) error {
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  if existing, ok := s.tokens[t.BorrowID]; ok {
		   if existing.PlanID != t.PlanID {
			  return ErrOccupyConflict
		  }
	   }
	  cp := t
	   s.tokens[t.BorrowID] = &cp
	   return nil
   }

   func (s *OccupyStore) Release(borrowID string) {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  delete(s.tokens, borrowID)
  }

   func (s *OccupyStore) Get(borrowID string) (OccupyToken, bool) {
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   t, ok := s.tokens[borrowID]
	   if !ok {
		   return OccupyToken{}, false
	  }
	  return *t, true
   }

// ─────────────────────────────────────────────
// 辅助函数
// ─────────────────────────────────────────────

   func appendUniq(ids []string, id string) []string {
	   for _, x := range ids {
		   if x == id {
			   return ids
		   }
	   }
	  return append(ids, id)
  }
