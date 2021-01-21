package wrfreeze

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MemStore 线程安全的内存存储。

type MemStore struct {
	mu sync.RWMutex
	plans   map[string]*CascadePlan
	waves   map[string]*FreezeWave
	entries map[string]*WaveUserEntry
	phases  map[string]*UnfreezePhase
	proofs     map[string]*CoverageProof
	legs       map[string]*RollbackLeg
	waveByPlan map[string][]string // planID → []waveID
}

// NewMemStore 构造内存存储。
  func NewMemStore() *MemStore {
	   return &MemStore{
		  plans: map[string]*CascadePlan{},
		  waves:   map[string]*FreezeWave{},
		  entries: map[string]*WaveUserEntry{},
		   phases: map[string]*UnfreezePhase{},
		   proofs: map[string]*CoverageProof{},
		  legs:   map[string]*RollbackLeg{},
		   waveByPlan: map[string][]string{},
	  }
   }

// ── CascadePlan ──

   func (m *MemStore) SavePlan(_ context.Context, plan *CascadePlan) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   cp := *plan
	  users := make([]string,
		   len(plan.UserSet))
	   copy(users, plan.UserSet)
	  cp.UserSet = users
	  m.plans[plan.ID] = &cp
	  return nil
   }

   func (m *MemStore) GetPlan(_ context.Context, planID string) (*CascadePlan, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  p, ok := m.plans[planID]
	   if !ok {
		  return nil, ErrPlanNotFound
	   }
	   cp := *p
	  users := make([]string, len(p.UserSet))
	  copy(users, p.UserSet)
	  cp.UserSet = users
	   return &cp, nil
  }

   func (m *MemStore) ListPlans(_ context.Context, status string, limit int) ([]*CascadePlan, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  out := make([]*CascadePlan, 0)
	   for _, p := range m.plans {
		  if status != "" && string(p.Status) != status {
			  continue
		  }
		  cp := *p
		  out = append(out, &cp)
		   if limit > 0 && len(out) >= limit {
			   break
		  }
	  }
	   return out, nil
  }

  func (m *MemStore) ListOverduePlans(_ context.Context, now time.Time) ([]*CascadePlan, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  out := make([]*CascadePlan, 0)
	   for _, p := range m.plans {
		  if p.Status.Terminal() {
			   continue
		  }
		  if now.After(p.ExpiresAt) {
			  cp := *p
			   out = append(out, &cp)
		   }
	  }
	   return out, nil
   }

// ── FreezeWave ──

  func (m *MemStore) SaveWave(_ context.Context, wave *FreezeWave) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   cp := *wave
	  users := make([]string, len(wave.TargetUsers))
	   copy(users, wave.TargetUsers)
	  cp.TargetUsers = users
	  m.waves[wave.ID] = &cp
	  // 维护 planID → waveID 索引
	  wids := m.waveByPlan[wave.PlanID]
	  for _, w := range wids {
		   if w == wave.ID {
			   return nil
		   }
	   }
	   m.waveByPlan[wave.PlanID] = append(wids, wave.ID)
	  return nil
   }

   func (m *MemStore) GetWave(_ context.Context, waveID string) (*FreezeWave, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	   w, ok := m.waves[waveID]
	  if !ok {
		   return nil, ErrWaveNotFound
	  }
	   cp := *w
	  users := make([]string, len(w.TargetUsers))
	   copy(users, w.TargetUsers)
	   cp.TargetUsers = users
	   return &cp, nil
  }

   func (m *MemStore) ListWaves(_ context.Context, planID string) ([]*FreezeWave, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  wids := m.waveByPlan[planID]
	   out := make([]*FreezeWave, 0, len(wids))
	  for _, wid := range wids {
		   w, ok := m.waves[wid]
		   if !ok {
			   continue
		  }
		   cp := *w
		   users := make([]string,
			   len(w.TargetUsers))
		  copy(users, w.TargetUsers)
		  cp.TargetUsers = users
		   out = append(out, &cp)
	  }
	  return out, nil
   }

  func (m *MemStore) SaveWaveEntry(_ context.Context, entry *WaveUserEntry) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  cp := *entry
	   m.entries[entry.ID] = &cp
	  return nil
   }

// ── UnfreezePhase ──

  func (m *MemStore) SavePhase(_ context.Context, phase *UnfreezePhase) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  cp := *phase
	  m.phases[phase.ID] = &cp
	   return nil
  }

   func (m *MemStore) GetPhase(_ context.Context, phaseID string) (*UnfreezePhase, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	  p, ok := m.phases[phaseID]
	   if !ok {
		   return nil, ErrPhaseNotFound
	   }
	   cp := *p
	   return &cp, nil
   }

  func (m *MemStore) ListPhases(_ context.Context, planID string) ([]*UnfreezePhase, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	   out := make([]*UnfreezePhase, 0)
	  for _, p := range m.phases {
		  if p.PlanID == planID {
			   cp := *p
			   out = append(out, &cp)
		   }
	  }
	   return out, nil
  }

// ── CoverageProof ──

  func (m *MemStore) SaveProof(_ context.Context, proof *CoverageProof) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  cp := *proof
	  delta := make([]string,
		  len(proof.DeltaUsers))
	   copy(delta, proof.DeltaUsers)
	   cp.DeltaUsers = delta
	   m.proofs[proof.ID] = &cp
	  return nil
   }

   func (m *MemStore) GetProof(_ context.Context, proofID string) (*CoverageProof, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   p, ok := m.proofs[proofID]
	  if !ok {
		   return nil, fmt.Errorf("wrfreeze: proof not found")
	  }
	   cp := *p
	  return &cp, nil
  }

   func (m *MemStore) LatestProof(_ context.Context, planID string) (*CoverageProof, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	   var latest *CoverageProof
	   for _, p := range m.proofs {
		  if p.PlanID != planID {
			   continue
		   }
		  if latest == nil || p.CheckedAt.After(latest.CheckedAt) {
			  latest = p
		  }
	   }
	  if latest == nil {
		  return nil, nil
	  }
	  cp := *latest
	  return &cp, nil
  }

  func (m *MemStore) ListProofs(_ context.Context, planID string) ([]*CoverageProof, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	  out := make([]*CoverageProof, 0)
	   for _, p := range m.proofs {
		  if p.PlanID == planID {
			   cp := *p
			  out = append(out, &cp)
		  }
	   }
	   return out, nil
   }

// ── RollbackLeg ──

   func (m *MemStore) SaveLeg(_ context.Context, leg *RollbackLeg) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  cp := *leg
	  m.legs[leg.ID] = &cp
	   return nil
  }

  func (m *MemStore) GetLeg(_ context.Context, legID string) (*RollbackLeg, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   l, ok := m.legs[legID]
	   if !ok {
		   return nil, ErrLegNotFound
	   }
	  cp := *l
	   return &cp, nil
  }

  func (m *MemStore) ListLegs(_ context.Context, planID string) ([]*RollbackLeg, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	  out := make([]*RollbackLeg, 0)
	   for _, l := range m.legs {
		   if l.PlanID == planID {
			  cp := *l
			   out = append(out, &cp)
		  }
	  }
	   return out, nil
  }
