package availcas

import (
	"context"
	"sync"
	"time"
	"github.com/shopspring/decimal"
)

// MemStore 线程安全的内存 Store 实现。

type MemStore struct {
	mu      sync.Mutex
	leases  map[string]*AvailLease
	intents map[string]*DebitIntent
	idem   map[string]*DebitIntent // idemKey → intent
	proofs map[string]*RaceProof
}

// NewMemStore 构造内存 Store。

  func NewMemStore() *MemStore {
	  return &MemStore{
		  leases: make(map[string]*AvailLease),
		  intents: make(map[string]*DebitIntent),
		   idem: make(map[string]*DebitIntent),
		   proofs: make(map[string]*RaceProof),
	  }
   }

// ── 租约 ───────────────────────────────────────────────────────────────────

  func (m *MemStore) SaveLease(_ context.Context, l *AvailLease) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   cp := *l
	  m.leases[l.ID] = &cp
	  return nil
  }

  func (m *MemStore) GetLease(_ context.Context, id string) (*AvailLease, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  l, ok := m.leases[id]
	   if !ok {
		   return nil, ErrLeaseNotFound
	  }
	   cp := *l
	   return &cp, nil
  }

   func (m *MemStore) UpdateLeaseState(_ context.Context, id, state string, now time.Time) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  l, ok := m.leases[id]
	   if !ok {
		  return ErrLeaseNotFound
	   }
	   l.State = LeaseState(state)
	  l.UpdatedAt = now
	   return nil
   }

  func (m *MemStore) ListExpirableLeases(_ context.Context, now time.Time, limit int) ([]*AvailLease, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   var out []*AvailLease
	  for _, l := range m.leases {
		  if l.State == LeaseStateOpen && !now.Before(l.ExpiresAt) {
			   cp := *l
			   out = append(out, &cp)
			  if len(out) >= limit {
				   break
			   }
		  }
	  }
	  return out, nil
  }

   func (m *MemStore) MaxLeaseGen(_ context.Context, userID, asset, book string) (int64, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  var maxGen int64
	   for _, l := range m.leases {
		   if l.UserID == userID && l.Asset == asset && l.Book == book {
			   if l.Gen > maxGen {
				  maxGen = l.Gen
			   }
		  }
	  }
	   return maxGen, nil
  }

// ── 意图 ───────────────────────────────────────────────────────────────────

   func (m *MemStore) SaveIntent(_ context.Context, d *DebitIntent) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  cp := *d
	  m.intents[d.ID] = &cp
	  if d.IdemKey != "" {
		   m.idem[d.IdemKey] = &cp
	   }
	   return nil
  }

  func (m *MemStore) GetIntent(_ context.Context, id string) (*DebitIntent, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   d, ok := m.intents[id]
	   if !ok {
		  return nil, ErrIntentNotFound
	   }
	   cp := *d
	   return &cp, nil
   }

   func (m *MemStore) FindIntentByIdem(_ context.Context, idemKey string) (*DebitIntent, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   d, ok := m.idem[idemKey]
	  if !ok {
		  return nil, nil
	   }
	   cp := *d
	  return &cp, nil
  }

  func (m *MemStore) FindAcceptedIntentByLease(_ context.Context, leaseID string) (*DebitIntent, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  for _, d := range m.intents {
		   if d.LeaseID == leaseID && d.State == IntentStateAccepted {
			  cp := *d
			  return &cp, nil
		   }
	  }
	   return nil, nil
   }

  func (m *MemStore) SumInFlightIntents(_ context.Context, userID, asset, book string) (decimal.Decimal, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   sum := decimal.Zero
	   for _, d := range m.intents {
		   if d.UserID == userID && d.Asset == asset && d.Book == book &&
			  d.State == IntentStateAccepted {
			  sum = sum.Add(d.Amount)
		  }
	  }
	   return sum, nil
   }

// ── 竞态证明 ───────────────────────────────────────────────────────────────

  func (m *MemStore) SaveProof(_ context.Context, p *RaceProof) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  cp := *p
	  m.proofs[p.ID] = &cp
	   return nil
  }

  func (m *MemStore) GetProof(_ context.Context, id string) (*RaceProof, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   p, ok := m.proofs[id]
	   if !ok {
		   return nil, ErrProofNotFound
	   }
	  cp := *p
	  return &cp, nil
  }

  func (m *MemStore) ListProofs(_ context.Context, userID, asset string, limit int) ([]*RaceProof, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   var out []*RaceProof
	  for _, p := range m.proofs {
		  if p.UserID == userID && p.Asset == asset {
			   cp := *p
			  out = append(out, &cp)
			   if len(out) >= limit {
				   break
			  }
		  }
	   }
	  return out, nil
   }

// ── 辅助 ──────────────────────────────────────────────────────────────
// IntentCount 返回内存中意图总数（断言用）。
   func (m *MemStore) IntentCount() int {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  return len(m.intents)
  }

// ProofCount 返回内存中竞态证明总数（断言用）。
  func (m *MemStore) ProofCount() int {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   return len(m.proofs)
   }

// AllLeases 返回所有租约副本（断言用）。

   func (m *MemStore) AllLeases() []*AvailLease {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   out := make([]*AvailLease, 0, len(m.leases))
	  for _, l := range m.leases {
		   cp := *l
		   out = append(out, &cp)
	  }
	  return out
  }
