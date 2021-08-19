package legmovefence

import (
	"context"
	"sync"
	"time"
)

// MemStore 进程内仓储。
type MemStore struct {
	mu sync.RWMutex
	fences map[string]LegMoveFence
	idem   map[string]string
	legIdx map[string]string
	proofs map[string]GeoDoubleMoveProof
	halfs  map[string]HalfMoveCase
	audits []FenceAudit
}

   func NewMemStore() *MemStore {
	   return &MemStore{
		  fences: make(map[string]LegMoveFence),
		  idem:   make(map[string]string),
		   legIdx: make(map[string]string),
		   proofs: make(map[string]GeoDoubleMoveProof),
		  halfs:  make(map[string]HalfMoveCase),
	   }
   }

  func legKey(planID, legID string) string { return planID + "/" + legID }

  func (m *MemStore) SaveFence(_ context.Context, f LegMoveFence) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  if f.FenceID == "" {
		  return ErrInvalidReq
	  }
	   m.fences[f.FenceID] = f
	   if f.IdempotencyKey != "" {
		   m.idem[f.IdempotencyKey] = f.FenceID
	  }
	   m.legIdx[legKey(f.PlanID, f.LegID)] = f.FenceID
	   return nil
   }

  func (m *MemStore) GetFence(_ context.Context, fenceID string) (LegMoveFence, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	  f, ok := m.fences[fenceID]
	   if !ok {
		   return LegMoveFence{}, ErrFenceNotFound
	  }
	  return f, nil
   }

  func (m *MemStore) GetFenceByIdem(_ context.Context, key string) (LegMoveFence, bool, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	   id, ok := m.idem[key]
	  if !ok {
		   return LegMoveFence{}, false, nil
	   }
	  f, ok := m.fences[id]
	   if !ok {
		  return LegMoveFence{}, false, nil
	   }
	  return f, true, nil
  }

  func (m *MemStore) GetFenceByLeg(_ context.Context, planID, legID string) (LegMoveFence, bool, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   id, ok := m.legIdx[legKey(planID, legID)]
	   if !ok {
		  return LegMoveFence{}, false, nil
	   }
	   f, ok := m.fences[id]
	  return f, ok, nil
   }

   func (m *MemStore) UpdateFence(_ context.Context, f LegMoveFence) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if _, ok := m.fences[f.FenceID]; !ok {
		  return ErrFenceNotFound
	  }
	  m.fences[f.FenceID] = f
	   return nil
  }

  func (m *MemStore) ListFencesByState(_ context.Context, state FenceState, limit int) ([]LegMoveFence, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  if limit <= 0 {
		  limit = 200
	   }
	  out := make([]LegMoveFence, 0)
	   for _, f := range m.fences {
		   if f.State == state {
			  out = append(out, f)
			  if len(out) >= limit {
				   break
			  }
		  }
	  }
	   return out, nil
  }

  func (m *MemStore) ListExpiredActive(_ context.Context, now time.Time, limit int) ([]LegMoveFence, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	  if limit <= 0 {
		  limit = 200
	  }
	   out := make([]LegMoveFence, 0)
	   for _, f := range m.fences {
		  if (f.State == FenceIssued || f.State == FenceEnqueued) && now.After(f.ExpiresAt()) {
			   out = append(out, f)
			   if len(out) >= limit {
				  break
			  }
		   }
	  }
	   return out, nil
   }

   func (m *MemStore) ListByRegion(_ context.Context, regionID string, limit int) ([]LegMoveFence, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   if limit <= 0 {
		   limit = 200
	  }
	  out := make([]LegMoveFence, 0)
	   for _, f := range m.fences {
		  if f.SrcRegion == regionID || f.DstRegion == regionID {
			  out = append(out, f)
			   if len(out) >= limit {
				   break
			   }
		  }
	  }
	  return out, nil
  }

   func (m *MemStore) SaveProof(_ context.Context, p GeoDoubleMoveProof) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   m.proofs[p.ProofID] = p
	  return nil
   }

  func (m *MemStore) GetProof(_ context.Context, proofID string) (GeoDoubleMoveProof, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   p, ok := m.proofs[proofID]
	  if !ok {
		   return GeoDoubleMoveProof{}, ErrProofNotFound
	  }
	   return p, nil
   }

   func (m *MemStore) ListProofs(_ context.Context, filter ExportFilter) ([]GeoDoubleMoveProof, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	   limit := filter.Limit
	   if limit <= 0 {
		   limit = 200
	   }
	   out := make([]GeoDoubleMoveProof, 0)
	   for _, p := range m.proofs {
		   if filter.Kind != "" && p.Kind != filter.Kind {
			   continue
		  }
		   if filter.FenceID != "" && p.FenceID != filter.FenceID {
			  continue
		   }
		  if filter.LegID != "" && p.LegID != filter.LegID {
			   continue
		   }
		   if filter.UnexportedOnly && p.ExportedAt != nil {
			  continue
		  }
		  out = append(out, p)
		   if len(out) >= limit {
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

  func (m *MemStore) SaveHalf(_ context.Context, c HalfMoveCase) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   m.halfs[c.CaseID] = c
	   return nil
   }

   func (m *MemStore) GetHalf(_ context.Context, caseID string) (HalfMoveCase, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   c, ok := m.halfs[caseID]
	  if !ok {
		   return HalfMoveCase{}, ErrHalfNotFound
	  }
	  return c, nil
   }

   func (m *MemStore) GetOpenHalfByFence(_ context.Context, fenceID string) (HalfMoveCase, bool, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   for _, c := range m.halfs {
		   if c.FenceID == fenceID && c.State == HalfOpen {
			  return c, true, nil
		  }
	  }
	  return HalfMoveCase{}, false, nil
   }

   func (m *MemStore) UpdateHalf(_ context.Context, c HalfMoveCase) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   if _, ok := m.halfs[c.CaseID]; !ok {
		   return ErrHalfNotFound
	   }
	  m.halfs[c.CaseID] = c
	   return nil
   }

   func (m *MemStore) ListOpenHalf(_ context.Context, limit int) ([]HalfMoveCase, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  if limit <= 0 {
		   limit = 200
	  }
	   out := make([]HalfMoveCase, 0)
	   for _, c := range m.halfs {
		  if c.State == HalfOpen {
			   out = append(out, c)
			  if len(out) >= limit {
				  break
			   }
		  }
	   }
	  return out, nil
  }

  func (m *MemStore) SaveAudit(_ context.Context, a FenceAudit) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   m.audits = append(m.audits, a)
	   return nil
   }

  func (m *MemStore) ListAudit(_ context.Context, fenceID string, limit int) ([]FenceAudit, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  if limit <= 0 {
		  limit = 100
	  }
	   out := make([]FenceAudit, 0)
	  for i := len(m.audits) - 1; i >= 0; i-- {
		  if fenceID == "" || m.audits[i].FenceID == fenceID {
			   out = append(out, m.audits[i])
			  if len(out) >= limit {
				  break
			  }
		  }
	   }
	   return out, nil
   }
