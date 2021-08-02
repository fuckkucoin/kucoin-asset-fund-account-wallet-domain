package isodebt

import (
	"context"
	"sort"
	"sync"
	"time"
)

// MemStore 线程安全的内存 Store。
type MemStore struct {
	mu          sync.Mutex
	generations map[string][]PairGeneration // key=PairKey
	leases map[string]RatioLease
	gates map[string]TransferGate
	drifts map[string]RatioDriftCase
	proofs    map[string]RatioDriftProof
	mutexRecs map[string]MutexRecord
}

// NewMemStore 构造内存 Store。

  func NewMemStore() *MemStore {
	  return &MemStore{
		  generations: map[string][]PairGeneration{},
		   leases:      map[string]RatioLease{},
		   gates:  map[string]TransferGate{},
		   drifts: map[string]RatioDriftCase{},
		  proofs: map[string]RatioDriftProof{},
		   mutexRecs: map[string]MutexRecord{},
	   }
   }

// ─────────────────────────────────────────────
// PairGeneration
// ─────────────────────────────────────────────

   func (m *MemStore) GetGeneration(_ context.Context, userID, symbol string) (PairGeneration, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   key := PairKey(userID, symbol)
	   gs := m.generations[key]
	   if len(gs) == 0 {
		  return PairGeneration{UserID: userID, Symbol: symbol}, nil
	  }
	   return gs[len(gs)-1], nil
   }

  func (m *MemStore) SaveGeneration(_ context.Context, g PairGeneration) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  key := PairKey(g.UserID, g.Symbol)
	   gs := m.generations[key]
	   // 替换相同 Generation 或追加
	   for i, existing := range gs {
		   if existing.Generation == g.Generation {
			  gs[i] = g
			  m.generations[key] = gs
			  return nil
		   }
	  }
	   m.generations[key] = append(gs, g)
	   return nil
   }

  func (m *MemStore) ListGenerations(_ context.Context, userID, symbol string, limit int) ([]PairGeneration, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   key := PairKey(userID, symbol)
	  gs := append([]PairGeneration{}, m.generations[key]...)
	   sort.Slice(gs, func(i, j int) bool { return gs[i].ComputedAt.After(gs[j].ComputedAt) })
	   if limit > 0 && len(gs) > limit {
		  gs = gs[:limit]
	  }
	  return gs, nil
  }

// ─────────────────────────────────────────────
// RatioLease
// ─────────────────────────────────────────────

  func (m *MemStore) GetLease(_ context.Context, id string) (RatioLease, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   l, ok := m.leases[id]
	  if !ok {
		  return RatioLease{}, ErrNotFound
	   }
	   return l, nil
  }

  func (m *MemStore) SaveLease(_ context.Context, l RatioLease) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   m.leases[l.ID] = l
	  return nil
  }

   func (m *MemStore) ListActiveLeases(_ context.Context, userID, symbol string, now time.Time) ([]RatioLease, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   var out []RatioLease
	   for _, l := range m.leases {
		   if l.UserID == userID && l.Symbol == symbol &&
			   l.State == LeaseStateFresh && l.ExpiresAt.After(now) {
			  out = append(out, l)
		   }
	   }
	   return out, nil
  }

   func (m *MemStore) ExpireStaleLeases(_ context.Context, userID, symbol string, currentGen int64, now time.Time) (int64, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  var count int64
	   for id, l := range m.leases {
		   if l.UserID != userID || l.Symbol != symbol || l.State != LeaseStateFresh {
			  continue
		  }
		   if l.Generation != currentGen || now.After(l.ExpiresAt) {
			   t := now
			  l.State = LeaseStateStale
			   l.StaledAt = &t
			   m.leases[id] = l
			   count++
		   }
	   }
	   return count, nil
  }

// ─────────────────────────────────────────────
// TransferGate
// ─────────────────────────────────────────────

  func (m *MemStore) GetGate(_ context.Context, id string) (TransferGate, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  g, ok := m.gates[id]
	  if !ok {
		   return TransferGate{}, ErrNotFound
	   }
	  return g, nil
   }

  func (m *MemStore) SaveGate(_ context.Context, g TransferGate) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   m.gates[g.ID] = g
	   return nil
  }

   func (m *MemStore) ListPendingGates(_ context.Context, limit int) ([]TransferGate, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  var out []TransferGate
	   for _, g := range m.gates {
		   if g.State == GateStatePending {
			  out = append(out, g)
			   if limit > 0 && len(out) >= limit {
				   break
			  }
		  }
	   }
	  return out, nil
   }

// ─────────────────────────────────────────────
// RatioDriftCase
// ─────────────────────────────────────────────

  func (m *MemStore) GetDriftCase(_ context.Context, id string) (RatioDriftCase, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  d, ok := m.drifts[id]
	   if !ok {
		  return RatioDriftCase{},
			  ErrNotFound
	   }
	   return d, nil
   }

   func (m *MemStore) SaveDriftCase(_ context.Context, d RatioDriftCase) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  m.drifts[d.ID] = d
	   return nil
   }

   func (m *MemStore) ListOpenDriftCases(_ context.Context, userID, symbol string) ([]RatioDriftCase, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  var out []RatioDriftCase
	   for _, d := range m.drifts {
		   // open 与 proven 均视为未关闭，均阻断 TransferIn
		   if d.UserID == userID && d.Symbol == symbol &&
			  (d.State == DriftStateOpen || d.State == DriftStateProven) {
			   out = append(out, d)
		   }
	   }
	  return out, nil
   }

  func (m *MemStore) ListAllOpenDriftCases(_ context.Context, limit int) ([]RatioDriftCase, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   var out []RatioDriftCase
	  for _, d := range m.drifts {
		   if d.State == DriftStateOpen || d.State == DriftStateProven {
			  out = append(out, d)
			   if limit > 0 && len(out) >= limit {
				  break
			  }
		  }
	  }
	   return out, nil
  }

// ─────────────────────────────────────────────
// RatioDriftProof
// ─────────────────────────────────────────────

  func (m *MemStore) SaveProof(_ context.Context, p RatioDriftProof) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  m.proofs[p.ID] = p
	  return nil
  }

   func (m *MemStore) GetProof(_ context.Context, id string) (RatioDriftProof, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   p, ok := m.proofs[id]
	  if !ok {
		   return RatioDriftProof{},
			  ErrNotFound
	   }
	  return p, nil
  }

  func (m *MemStore) ListProofsByCase(_ context.Context, driftCaseID string) ([]RatioDriftProof, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  var out []RatioDriftProof
	  for _, p := range m.proofs {
		  if p.DriftCaseID == driftCaseID {
			  out = append(out, p)
		   }
	  }
	  return out, nil
  }

// ─────────────────────────────────────────────
// MutexRecord
// ─────────────────────────────────────────────

   func (m *MemStore) SaveMutexRecord(_ context.Context, rec MutexRecord) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  m.mutexRecs[rec.ID] = rec
	   return nil
  }

   func (m *MemStore) ListMutexRecords(_ context.Context, userID, symbol string, limit int) ([]MutexRecord, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  var out []MutexRecord
	  for _, r := range m.mutexRecs {
		  if r.UserID == userID && r.Symbol == symbol {
			  out = append(out, r)
			   if limit > 0 && len(out) >= limit {
				   break
			   }
		   }
	   }
	  return out, nil
  }

// 确认 *MemStore 实现 Store 接口
var _ Store = (*MemStore)(nil)
