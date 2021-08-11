package latecreditgate

import (
	"context"
	"sync"
	"time"
)

// MemStore 内存实现。
type MemStore struct {
	mu    sync.Mutex
	gens  map[string]*LateCreditGeneration // genID
	byOut map[string]string
	events map[string]*RaceEvent
	proofs map[string]*LateCreditRaceProof
}

// NewMemStore 创建空仓。
   func NewMemStore() *MemStore {
	  return &MemStore{
		  gens:   make(map[string]*LateCreditGeneration),
		   byOut:  make(map[string]string),
		   events: make(map[string]*RaceEvent),
		  proofs: make(map[string]*LateCreditRaceProof),
	  }
   }

// Snapshot 深拷贝当前仓，进程重启后从持久化恢复。
   func (m *MemStore) Snapshot() *MemStore {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   out := NewMemStore()
	   for k, v := range m.gens {
		   cp := *v
		   out.gens[k] = &cp
	  }
	   for k, v := range m.byOut {
		  out.byOut[k] = v
	   }
	  for k, v := range m.events {
		   cp := *v
		   if v.AckedAt != nil {
			   t := *v.AckedAt
			   cp.AckedAt = &t
		   }
		  out.events[k] = &cp
	   }
	  for k, v := range m.proofs {
		  cp := *v
		   if v.ExportedAt != nil {
			  t := *v.ExportedAt
			   cp.ExportedAt = &t
		   }
		  out.proofs[k] = &cp
	  }
	   return out
  }

   func (m *MemStore) GetGen(_ context.Context, key OutputKey) (*LateCreditGeneration, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   id, ok := m.byOut[key.String()]
	  if !ok {
		   return nil, ErrGenNotFound
	   }
	   g, ok := m.gens[id]
	   if !ok {
		  return nil, ErrGenNotFound
	   }
	  cp := *g
	  return &cp, nil
   }

  func (m *MemStore) GetGenByID(_ context.Context, genID string) (*LateCreditGeneration, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  g, ok := m.gens[genID]
	   if !ok {
		   return nil, ErrGenNotFound
	  }
	  cp := *g
	  return &cp, nil
  }

  func (m *MemStore) InsertGen(_ context.Context, g LateCreditGeneration) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   k := g.OutputKey.String()
	   if _, exists := m.byOut[k]; exists {
		  return ErrOccupyConflict
	  }
	   cp := g
	  m.gens[g.GenID] = &cp
	  m.byOut[k] = g.GenID
	  return nil
  }

  func (m *MemStore) CASUpdateGen(_ context.Context, genID string, expectedGen int64, expectedState GenState, next LateCreditGeneration) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  g, ok := m.gens[genID]
	   if !ok {
		  return ErrGenNotFound
	   }
	  if g.Generation != expectedGen || g.State != expectedState {
		   return ErrGenMismatch
	   }
	  if next.Generation < g.Generation {
		  return ErrGenRollback
	   }
	  cp := next
	  cp.GenID = g.GenID
	   cp.OutputKey = g.OutputKey
	   m.gens[genID] = &cp
	   return nil
   }

  func (m *MemStore) ListGensByState(_ context.Context, state GenState, limit int) ([]LateCreditGeneration, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  var out []LateCreditGeneration
	  for _, g := range m.gens {
		  if g.State == state {
			  out = append(out, *g)
		  }
		   if limit > 0 && len(out) >= limit {
			  break
		   }
	   }
	   return out, nil
   }

  func (m *MemStore) ListOccupiedExpired(_ context.Context, now time.Time, ttl time.Duration, limit int) ([]LateCreditGeneration, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   var out []LateCreditGeneration
	   for _, g := range m.gens {
		  if g.State == GenOccupied && now.Sub(g.UpdatedAt) > ttl {
			  out = append(out, *g)
		   }
		   if limit > 0 && len(out) >= limit {
			  break
		  }
	   }
	  return out, nil
   }

  func (m *MemStore) InsertRaceEvent(_ context.Context, ev RaceEvent) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   cp := ev
	   m.events[ev.EventID] = &cp
	   return nil
  }

  func (m *MemStore) GetRaceEvent(_ context.Context, eventID string) (*RaceEvent, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  ev, ok := m.events[eventID]
	   if !ok {
		  return nil, ErrRaceEventNotFound
	   }
	  cp := *ev
	   if ev.AckedAt != nil {
		   t := *ev.AckedAt
		  cp.AckedAt = &t
	   }
	   return &cp, nil
  }

   func (m *MemStore) ListOpenRaceEvents(_ context.Context, limit int) ([]RaceEvent, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  var out []RaceEvent
	  for _, ev := range m.events {
		  if ev.State == RaceOpen {
			  cp := *ev
			   out = append(out, cp)
		   }
		  if limit > 0 && len(out) >= limit {
			   break
		   }
	   }
	  return out, nil
  }

   func (m *MemStore) ListOpenRaceEventsByKey(_ context.Context, key OutputKey) ([]RaceEvent, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  var out []RaceEvent
	   ks := key.String()
	   for _, ev := range m.events {
		   if ev.State == RaceOpen && ev.OutputKey.String() == ks {
			   out = append(out, *ev)
		   }
	  }
	   return out, nil
  }

   func (m *MemStore) AckRaceEvent(_ context.Context, eventID string, now time.Time) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  ev, ok := m.events[eventID]
	  if !ok {
		   return ErrRaceEventNotFound
	  }
	  if ev.State == RaceAcked {
		   return ErrRaceEventAlreadyAcked
	  }
	   ev.State = RaceAcked
	  t := now
	   ev.AckedAt = &t
	  return nil
  }

  func (m *MemStore) InsertProof(_ context.Context, p LateCreditRaceProof) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   cp := p
	   m.proofs[p.ProofID] = &cp
	  return nil
  }

  func (m *MemStore) GetProof(_ context.Context, proofID string) (*LateCreditRaceProof, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  p, ok := m.proofs[proofID]
	  if !ok {
		  return nil, ErrProofNotFound
	  }
	   cp := *p
	  return &cp, nil
  }

   func (m *MemStore) ListProofs(_ context.Context, limit int) ([]LateCreditRaceProof, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  var out []LateCreditRaceProof
	   for _, p := range m.proofs {
		  out = append(out, *p)
		  if limit > 0 && len(out) >= limit {
			  break
		  }
	   }
	   return out, nil
  }

   func (m *MemStore) ListProofsByKey(_ context.Context, key OutputKey) ([]LateCreditRaceProof, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  var out []LateCreditRaceProof
	   ks := key.String()
	   for _, p := range m.proofs {
		   if p.OutputKey.String() == ks {
			   out = append(out, *p)
		   }
	   }
	   return out, nil
   }

  func (m *MemStore) MarkProofExported(_ context.Context, proofID string, now time.Time) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   p, ok := m.proofs[proofID]
	   if !ok {
		   return ErrProofNotFound
	   }
	  t := now
	   p.ExportedAt = &t
	  return nil
  }
