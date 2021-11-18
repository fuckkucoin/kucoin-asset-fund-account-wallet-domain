package shardholdgen

import (
	"context"
	"sync"
	"time"
)

// MemStore 进程内仓储。

type MemStore struct {
	mu sync.Mutex
	gens map[string]LegHoldGen
	idem   map[string]string
	legIdx map[string]string
	proofs map[string]ShardHoldProof
	halfs  map[string]HalfLegCase
	audits []GenAudit
	locks  map[string]string
}

  func NewMemStore() *MemStore {
	   return &MemStore{
		  gens: make(map[string]LegHoldGen),
		  idem:   make(map[string]string),
		  legIdx: make(map[string]string),
		  proofs: make(map[string]ShardHoldProof),
		   halfs: make(map[string]HalfLegCase),
		   locks: make(map[string]string),
	  }
  }

   func lk(planID, legID string) string { return planID + "/" + legID }

   func (m *MemStore) SaveGen(_ context.Context, g LegHoldGen) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  if g.GenID == "" {
		   return ErrInvalidReq
	  }
	  m.gens[g.GenID] = g
	   if g.IdempotencyKey != "" {
		   m.idem[g.IdempotencyKey] = g.GenID
	  }
	  m.legIdx[lk(g.PlanID, g.LegID)] = g.GenID
	  return nil
   }

   func (m *MemStore) GetGen(_ context.Context, genID string) (LegHoldGen, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   g, ok := m.gens[genID]
	  if !ok {
		   return LegHoldGen{}, ErrGenNotFound
	  }
	   return g, nil
  }

   func (m *MemStore) GetGenByIdem(_ context.Context, key string) (LegHoldGen, bool, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  id, ok := m.idem[key]
	  if !ok {
		  return LegHoldGen{}, false, nil
	   }
	  g, ok := m.gens[id]
	  return g, ok, nil
   }

  func (m *MemStore) GetGenByLeg(_ context.Context, planID, legID string) (LegHoldGen, bool, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  id, ok := m.legIdx[lk(planID, legID)]
	   if !ok {
		   return LegHoldGen{}, false, nil
	   }
	  g, ok := m.gens[id]
	  return g, ok, nil
  }

   func (m *MemStore) UpdateGen(_ context.Context, g LegHoldGen) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if _, ok := m.gens[g.GenID]; !ok {
		   return ErrGenNotFound
	   }
	   m.gens[g.GenID] = g
	  return nil
   }

   func (m *MemStore) ListGensByState(_ context.Context, state GenState, limit int) ([]LegHoldGen, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   out := make([]LegHoldGen, 0)
	  for _, g := range m.gens {
		  if g.State == state {
			   out = append(out, g)
			  if limit > 0 && len(out) >= limit {
				  break
			  }
		   }
	   }
	  return out, nil
  }

  func (m *MemStore) ListExpiredActive(_ context.Context, now time.Time, limit int) ([]LegHoldGen, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   out := make([]LegHoldGen, 0)
	  for _, g := range m.gens {
		  if (g.State == GenIssued || g.State == GenHeld) && now.After(g.ExpiresAt()) {
			   out = append(out, g)
			   if limit > 0 && len(out) >= limit {
				   break
			   }
		   }
	  }
	   return out, nil
   }

  func (m *MemStore) TryLockLeg(_ context.Context, planID, legID, owner string) (bool, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  k := lk(planID, legID)
	   if cur, ok := m.locks[k]; ok && cur != "" && cur != owner {
		   return false, nil
	   }
	  m.locks[k] = owner
	  return true, nil
  }

  func (m *MemStore) UnlockLeg(_ context.Context, planID, legID, owner string) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   k := lk(planID, legID)
	  if cur, ok := m.locks[k]; ok && (cur == owner || owner == "") {
		   delete(m.locks, k)
	   }
	  return nil
   }

  func (m *MemStore) SaveProof(_ context.Context, p ShardHoldProof) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  if p.ProofID == "" {
		   return ErrInvalidReq
	   }
	   m.proofs[p.ProofID] = p
	  return nil
   }

  func (m *MemStore) GetProof(_ context.Context, proofID string) (ShardHoldProof, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   p, ok := m.proofs[proofID]
	  if !ok {
		   return ShardHoldProof{}, ErrProofNotFound
	  }
	  return p, nil
   }

  func (m *MemStore) ListProofs(_ context.Context, filter ExportFilter) ([]ShardHoldProof, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  out := make([]ShardHoldProof, 0)
	   for _, p := range m.proofs {
		   if filter.Kind != "" && p.Kind != filter.Kind {
			   continue
		  }
		  if filter.GenID != "" && p.GenID != filter.GenID {
			  continue
		   }
		   if filter.LegID != "" && p.LegID != filter.LegID {
			  continue
		  }
		  if filter.UnexportedOnly && p.ExportedAt != nil {
			   continue
		   }
		   out = append(out, p)
		  if filter.Limit > 0 && len(out) >= filter.Limit {
			  break
		  }
	  }
	  return out, nil
   }

   func (m *MemStore) MarkProofsExported(_ context.Context, ids []string, at time.Time) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  for _, id := range ids {
		   if p, ok := m.proofs[id]; ok {
			   t := at
			   p.ExportedAt = &t
			   m.proofs[id] = p
		  }
	   }
	   return nil
   }

  func (m *MemStore) SaveHalf(_ context.Context, c HalfLegCase) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  if c.CaseID == "" {
		   return ErrInvalidReq
	   }
	   m.halfs[c.CaseID] = c
	  return nil
   }

  func (m *MemStore) GetHalf(_ context.Context, caseID string) (HalfLegCase, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  c, ok := m.halfs[caseID]
	   if !ok {
		  return HalfLegCase{}, ErrHalfNotFound
	   }
	   return c, nil
  }

   func (m *MemStore) GetOpenHalfByGen(_ context.Context, genID string) (HalfLegCase, bool, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  for _, c := range m.halfs {
		  if c.GenID == genID && c.State == HalfOpen {
			  return c, true, nil
		   }
	   }
	  return HalfLegCase{}, false, nil
   }

  func (m *MemStore) UpdateHalf(_ context.Context, c HalfLegCase) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   if _, ok := m.halfs[c.CaseID]; !ok {
		  return ErrHalfNotFound
	  }
	  m.halfs[c.CaseID] = c
	  return nil
   }

   func (m *MemStore) ListOpenHalf(_ context.Context, limit int) ([]HalfLegCase, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  out := make([]HalfLegCase, 0)
	   for _, c := range m.halfs {
		   if c.State == HalfOpen {
			   out = append(out, c)
			  if limit > 0 && len(out) >= limit {
				   break
			  }
		  }
	  }
	  return out, nil
  }

   func (m *MemStore) SaveAudit(_ context.Context, a GenAudit) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   m.audits = append(m.audits, a)
	   return nil
   }

  func (m *MemStore) ListAudit(_ context.Context, genID string, limit int) ([]GenAudit, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   out := make([]GenAudit, 0)
	   for i := len(m.audits) - 1; i >= 0; i-- {
		   if m.audits[i].GenID == genID || genID == "" {
			  out = append(out, m.audits[i])
			  if limit > 0 && len(out) >= limit {
				   break
			  }
		   }
	   }
	  return out, nil
   }
