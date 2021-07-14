package guardlease

import (
	"context"
	"sync"
	"time"
)

// MemStore 线程安全的内存仓储，不持久化，重启后数据丢失。
type MemStore struct {
	mu sync.Mutex
	leases     map[string]SnapLease
	imbalances map[string]PartialImbalance
	proofs     map[string]ConserveProof
	fences     map[string]FenceRequest // fence_id → FenceRequest
	generations map[string]int64 // "userID|asset" → generation
}

// NewMemStore 创建内存仓储。
   func NewMemStore() *MemStore {
	  return &MemStore{
		   leases: make(map[string]SnapLease),
		   imbalances: make(map[string]PartialImbalance),
		   proofs:     make(map[string]ConserveProof),
		  fences:     make(map[string]FenceRequest),
		   generations: make(map[string]int64),
	  }
  }

// ---- SnapLease ----

   func (m *MemStore) InsertLease(_ context.Context, l SnapLease) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   if _, ok := m.leases[l.LeaseID]; ok {
		  return nil // 幂等
	  }
	   m.leases[l.LeaseID] = l
	   return nil
  }

   func (m *MemStore) GetLease(_ context.Context, leaseID string) (SnapLease, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   l, ok := m.leases[leaseID]
	   if !ok {
		   return SnapLease{}, ErrLeaseNotFound
	   }
	  return l, nil
  }

  func (m *MemStore) UpdateLeaseState(_ context.Context, leaseID string, old, new LeaseState, extra map[string]any) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   l, ok := m.leases[leaseID]
	   if !ok {
		   return ErrLeaseNotFound
	  }
	   if l.State != old {
		   return ErrLeaseInvalidState
	  }
	   l.State = new
	   if t, ok := extra["bound_at"]; ok {
		   if ts, ok := t.(time.Time); ok {
			  l.BoundAt = &ts
		  }
	   }
	   if t, ok := extra["consumed_at"]; ok {
		   if ts, ok := t.(time.Time); ok {
			  l.ConsumedAt = &ts
		   }
	   }
	   m.leases[leaseID] = l
	  return nil
   }

   func (m *MemStore) ListActiveLeases(_ context.Context, limit int) ([]SnapLease, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   out := make([]SnapLease, 0, limit)
	  for _, l := range m.leases {
		   if l.State == LeaseOpen {
			   out = append(out, l)
			  if len(out) >= limit {
				  break
			  }
		   }
	  }
	  return out, nil
   }

   func (m *MemStore) MarkExpiredLeases(_ context.Context) (int64, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   now := time.Now().UTC()
	  var count int64
	  for id, l := range m.leases {
		   if l.State == LeaseOpen && now.After(l.ExpiresAt) {
			   l.State = LeaseExpired
			  m.leases[id] = l
			  count++
		  }
	  }
	  return count, nil
   }

   func (m *MemStore) GetOrInitGeneration(_ context.Context, userID, asset string) (GuardGeneration, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   k := userID + "|" + asset
	  gen, ok := m.generations[k]
	  if !ok {
		   gen = 0
		   m.generations[k] = 0
	   }
	   return GuardGeneration{UserID: userID, Asset: asset, Generation: gen, UpdatedAt: time.Now().UTC()}, nil
  }

  func (m *MemStore) IncrGeneration(_ context.Context, userID, asset string) (int64, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  k := userID + "|" + asset
	   m.generations[k]++
	   return m.generations[k], nil
  }

// ---- PartialImbalance ----

   func (m *MemStore) InsertImbalance(_ context.Context, im PartialImbalance) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  if _, ok := m.imbalances[im.ImbalanceID]; ok {
		  return nil
	   }
	  m.imbalances[im.ImbalanceID] = im
	  return nil
  }

  func (m *MemStore) GetImbalance(_ context.Context, imbalanceID string) (PartialImbalance, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  im, ok := m.imbalances[imbalanceID]
	  if !ok {
		   return PartialImbalance{}, ErrImbalanceNotFound
	  }
	   return im, nil
   }

  func (m *MemStore) UpdateImbalanceState(_ context.Context, imbalanceID string, old, new ImbalanceState, extra map[string]any) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   im, ok := m.imbalances[imbalanceID]
	   if !ok {
		   return ErrImbalanceNotFound
	  }
	  if im.State != old {
		   return ErrLeaseInvalidState
	  }
	   im.State = new
	   if t, ok := extra["proven_at"]; ok {
		  if ts, ok := t.(time.Time); ok {
			   im.ProvenAt = &ts
		   }
	   }
	  if t, ok := extra["closed_at"]; ok {
		   if ts, ok := t.(time.Time); ok {
			  im.ClosedAt = &ts
		   }
	  }
	  if v, ok := extra["actor1"]; ok {
		   if a, ok := v.(string); ok {
			   im.Actor1 = a
		  }
	   }
	   if v, ok := extra["actor2"]; ok {
		  if a, ok := v.(string); ok {
			   im.Actor2 = a
		   }
	  }
	   m.imbalances[imbalanceID] = im
	   return nil
  }

   func (m *MemStore) ListOpenImbalances(_ context.Context, limit int) ([]PartialImbalance, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  out := make([]PartialImbalance, 0, limit)
	  for _, im := range m.imbalances {
		  if im.State == ImbalanceOpen {
			   out = append(out, im)
			  if len(out) >= limit {
				   break
			   }
		   }
	  }
	  return out, nil
  }

// ---- ConserveProof ----

  func (m *MemStore) InsertProof(_ context.Context, p ConserveProof) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  if _, ok := m.proofs[p.ProofID]; ok {
		   return nil
	   }
	   m.proofs[p.ProofID] = p
	  return nil
  }

   func (m *MemStore) GetProof(_ context.Context, proofID string) (ConserveProof, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   p, ok := m.proofs[proofID]
	   if !ok {
		  return ConserveProof{},
			  ErrProofNotFound
	  }
	   return p, nil
  }

   func (m *MemStore) UpdateProofState(_ context.Context, proofID string, old, new ProofState, extra map[string]any) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   p, ok := m.proofs[proofID]
	  if !ok {
		  return ErrProofNotFound
	  }
	   if p.State != old {
		  return ErrLeaseInvalidState
	  }
	  p.State = new
	  if t, ok := extra["resolved_at"]; ok {
		   if ts, ok := t.(time.Time); ok {
			  p.ResolvedAt = &ts
		   }
	   }
	   if v, ok := extra["cleared_by"]; ok {
		   if s, ok := v.(string); ok {
			   p.ClearedBy = s
		   }
	  }
	  m.proofs[proofID] = p
	   return nil
  }

   func (m *MemStore) ListFailedProofs(_ context.Context, limit int) ([]ConserveProof, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  out := make([]ConserveProof, 0, limit)
	  for _, p := range m.proofs {
		   if p.State == ProofFail || p.State == ProofHalted {
			  out = append(out, p)
			   if len(out) >= limit {
				  break
			  }
		   }
	   }
	   return out, nil
  }

// ---- FenceRequest ----

   func (m *MemStore) InsertFence(_ context.Context, f FenceRequest) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   m.fences[f.FenceID] = f
	   return nil
  }

  func (m *MemStore) ClearFence(_ context.Context, leaseID string) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   for id, f := range m.fences {
		  if f.LeaseID == leaseID {
			  delete(m.fences, id)
		   }
	   }
	   return nil
   }

  func (m *MemStore) HasActiveFence(_ context.Context, userID, asset, leaseID string) (bool, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  for _, f := range m.fences {
		   if f.UserID == userID && f.Asset == asset && f.LeaseID == leaseID && f.ClearedAt == nil {
			   return true, nil
		  }
	  }
	   return false, nil
   }

// AllLeases 仅供使用：返回所有租约快照。
  func (m *MemStore) AllLeases() []SnapLease {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   out := make([]SnapLease,
		   0, len(m.leases))
	   for _, l := range m.leases {
		  out = append(out, l)
	   }
	  return out
   }

// AllImbalances 仅供使用。

   func (m *MemStore) AllImbalances() []PartialImbalance {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  out := make([]PartialImbalance, 0, len(m.imbalances))
	   for _, im := range m.imbalances {
		   out = append(out, im)
	  }
	   return out
  }

// AllProofs 仅供使用。

   func (m *MemStore) AllProofs() []ConserveProof {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  out := make([]ConserveProof, 0, len(m.proofs))
	  for _, p := range m.proofs {
		   out = append(out, p)
	  }
	   return out
   }

// AllFences 仅供使用。

  func (m *MemStore) AllFences() []FenceRequest {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  out := make([]FenceRequest,
		   0, len(m.fences))
	  for _, f := range m.fences {
		  out = append(out, f)
	   }
	  return out
   }
