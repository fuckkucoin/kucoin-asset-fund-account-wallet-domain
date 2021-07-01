package georebal

import (
	"context"
	"errors"
	"sync"
	"time"
)

// MemStore 内存驱动，专用。
type MemStore struct {
	mu sync.Mutex
	plans map[string]GeoRebalPlan // id→Plan
	byIem  map[string]string      // idem→id
	legs   map[string]GeoRebalLeg // id→Leg
	byPlan map[string][]string    // planID→[]legID
	clock func() time.Time
}

   func NewMemStore() *MemStore {
	   return &MemStore{
		   plans: map[string]GeoRebalPlan{},
		   byIem: map[string]string{},
		   legs:   map[string]GeoRebalLeg{},
		  byPlan: map[string][]string{},
		  clock: func() time.Time { return time.Now().UTC() },
	  }
  }

  func (m *MemStore) SetClock(fn func() time.Time) {
	   if fn != nil {
		  m.mu.Lock()
		  m.clock = fn
		   m.mu.Unlock()
	   }
   }

   func (m *MemStore) SavePlan(_ context.Context, p GeoRebalPlan) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  m.plans[p.ID] = p.Clone()
	  m.byIem[p.Idem] = p.ID
	   return nil
   }

   func (m *MemStore) GetPlan(_ context.Context, id string) (GeoRebalPlan, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   p, ok := m.plans[id]
	   if !ok {
		  return GeoRebalPlan{},
			   ErrNotFound
	   }
	   return p.Clone(), nil
  }

  func (m *MemStore) FindPlanByIdem(_ context.Context, idem string) (GeoRebalPlan, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  id, ok := m.byIem[idem]
	  if !ok {
		   return GeoRebalPlan{}, ErrNotFound
	  }
	   p, ok := m.plans[id]
	  if !ok {
		  return GeoRebalPlan{}, ErrNotFound
	  }
	  return p.Clone(), nil
   }

   func (m *MemStore) ListPlans(_ context.Context, states []string) ([]GeoRebalPlan, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   stateSet := map[string]bool{}
	  for _, s := range states {
		  stateSet[s] = true
	   }
	  var out []GeoRebalPlan
	   for _, p := range m.plans {
		  if len(stateSet) == 0 || stateSet[p.State] {
			  out = append(out, p.Clone())
		  }
	  }
	  return out, nil
   }

  func (m *MemStore) SaveLeg(_ context.Context, leg GeoRebalLeg) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   m.legs[leg.ID] = leg
	  found := false
	   for _, id := range m.byPlan[leg.PlanID] {
		   if id == leg.ID {
			   found = true
			  break
		  }
	  }
	  if !found {
		   m.byPlan[leg.PlanID] = append(m.byPlan[leg.PlanID], leg.ID)
	   }
	   return nil
   }

   func (m *MemStore) GetLeg(_ context.Context, id string) (GeoRebalLeg, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  l, ok := m.legs[id]
	  if !ok {
		   return GeoRebalLeg{},
			  ErrNotFound
	   }
	   return l, nil
  }

  func (m *MemStore) ListLegs(_ context.Context, planID string) ([]GeoRebalLeg, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  ids := m.byPlan[planID]
	  legs := make([]GeoRebalLeg, 0, len(ids))
	   for _, id := range ids {
		  if l, ok := m.legs[id]; ok {
			   legs = append(legs, l)
		   }
	   }
	   return legs, nil
  }

   func (m *MemStore) UpdateLegState(_ context.Context, id, state, sweepIntentID, reason string) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   l, ok := m.legs[id]
	  if !ok {
		   return errors.New("georebal: leg not found")
	   }
	   if !AllowLegTransition(l.State, state) {
		  return ErrBadTransition
	   }
	  l.State = state
	  if sweepIntentID != "" {
		   l.SweepIntentID = sweepIntentID
	  }
	  if reason != "" {
		   l.Reason = reason
	  }
	   m.legs[id] = l
	  return nil
  }
