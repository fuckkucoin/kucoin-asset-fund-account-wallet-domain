package fxmarkleasegate

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

type Store interface {
	SaveLease(ctx context.Context, l MarkLease) (MarkLease, error)
	GetLease(ctx context.Context, id string) (MarkLease, error)
	GetLeaseByIdem(ctx context.Context, idem string) (MarkLease, error)
	ListLeasesByPair(ctx context.Context,
		pairKey string, limit int) ([]MarkLease, error)
	ListLeasesByState(ctx context.Context, state LeaseState, limit int) ([]MarkLease, error)
	ListLeasesByAsset(ctx context.Context, asset string, limit int) ([]MarkLease, error)
	ListExpiredLive(ctx context.Context, before time.Time, limit int) ([]MarkLease, error)
	ListLive(ctx context.Context,
		limit int) ([]MarkLease, error)
	CountByState(ctx context.Context, state LeaseState) (int, error)
	SaveProof(ctx context.Context, p FxMarkDriftProof) (FxMarkDriftProof, error)
	GetProof(ctx context.Context, id string) (FxMarkDriftProof, error)
	ListProofsByLease(ctx context.Context, leaseID string, limit int) ([]FxMarkDriftProof, error)
	ListProofs(ctx context.Context,
		kind DriftKind, limit int) ([]FxMarkDriftProof, error)
	CountProofs(ctx context.Context) (int, error)
	SaveAudit(ctx context.Context, a MarkLeaseAudit) (MarkLeaseAudit, error)
	ListAudit(ctx context.Context, leaseID string, limit int) ([]MarkLeaseAudit, error)
	CountAudits(ctx context.Context) (int, error)
	SaveGeneration(ctx context.Context, g MarkGeneration) (MarkGeneration, error)
	GetGeneration(ctx context.Context,
		asset string) (MarkGeneration, error)
	ListGenerations(ctx context.Context) ([]MarkGeneration, error)
	CountGenerations(ctx context.Context) (int, error)
}

type MemStore struct {
	mu     sync.Mutex
	leases map[string]MarkLease
	proofs map[string]FxMarkDriftProof
	audits []MarkLeaseAudit
	gens   map[string]MarkGeneration
}

  func NewMemStore() *MemStore {
	   return &MemStore{
		  leases: make(map[string]MarkLease),
		  proofs: make(map[string]FxMarkDriftProof),
		  gens:   make(map[string]MarkGeneration),
	   }
   }

   func (m *MemStore) SaveLease(_ context.Context, l MarkLease) (MarkLease, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if strings.TrimSpace(l.ID) == "" {
		  return MarkLease{},
			   ErrLeaseNotFound
	  }
	  m.leases[l.ID] = l
	  return l, nil
  }

  func (m *MemStore) GetLease(_ context.Context, id string) (MarkLease, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   l, ok := m.leases[strings.TrimSpace(id)]
	  if !ok {
		   return MarkLease{}, ErrLeaseNotFound
	  }
	   return l, nil
  }

   func (m *MemStore) GetLeaseByIdem(_ context.Context, idem string) (MarkLease, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   idem = strings.TrimSpace(idem)
	   if idem == "" {
		   return MarkLease{}, ErrLeaseNotFound
	  }
	  var best MarkLease
	   found := false
	  for _, l := range m.leases {
		   if l.Idem != idem {
			   continue
		  }
		  if !found || l.IssuedAt.After(best.IssuedAt) {
			   best = l
			  found = true
		   }
	   }
	  if !found {
		   return MarkLease{}, ErrLeaseNotFound
	  }
	   return best, nil
  }

  func (m *MemStore) ListLeasesByPair(_ context.Context, pairKey string, limit int) ([]MarkLease, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   if limit <= 0 {
		  limit = 50
	   }
	   out := make([]MarkLease, 0)
	   for _, l := range m.leases {
		  if PairKey(l.From, l.To) == pairKey {
			  out = append(out, l)
		  }
	   }
	   sortIssued(out)
	   if len(out) > limit {
		  out = out[:limit]
	  }
	  return out, nil
   }

   func (m *MemStore) ListLeasesByState(_ context.Context, state LeaseState, limit int) ([]MarkLease, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   if limit <= 0 {
		  limit = 50
	   }
	   out := make([]MarkLease, 0)
	   for _, l := range m.leases {
		   if l.State == state {
			   out = append(out, l)
		  }
	  }
	  sortIssued(out)
	   if len(out) > limit {
		  out = out[:limit]
	  }
	   return out, nil
  }

   func (m *MemStore) ListLeasesByAsset(_ context.Context, asset string, limit int) ([]MarkLease, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  if limit <= 0 {
		   limit = 80
	   }
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	   out := make([]MarkLease, 0)
	   for _, l := range m.leases {
		  if AssetTouched(l, asset) {
			   out = append(out, l)
		  }
	  }
	  sortIssued(out)
	   if len(out) > limit {
		   out = out[:limit]
	   }
	   return out, nil
  }

  func (m *MemStore) ListExpiredLive(_ context.Context, before time.Time, limit int) ([]MarkLease, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  if limit <= 0 {
		   limit = DefaultExpiryBatch
	  }
	  out := make([]MarkLease, 0)
	   for _, l := range m.leases {
		   if IsLive(l.State) && !l.ExpiresAt.After(before) {
			  out = append(out, l)
		  }
	   }
	  sortIssued(out)
	  if len(out) > limit {
		  out = out[:limit]
	  }
	   return out, nil
  }

   func (m *MemStore) ListLive(_ context.Context, limit int) ([]MarkLease, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  if limit <= 0 {
		   limit = DefaultPatrolBatch
	   }
	  out := make([]MarkLease, 0)
	  for _, l := range m.leases {
		   if IsLive(l.State) {
			   out = append(out, l)
		  }
	   }
	  sortIssued(out)
	   if len(out) > limit {
		   out = out[:limit]
	  }
	  return out, nil
   }

   func (m *MemStore) CountByState(_ context.Context, state LeaseState) (int, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  n := 0
	   for _, l := range m.leases {
		  if l.State == state {
			  n++
		   }
	  }
	   return n, nil
   }

  func (m *MemStore) SaveProof(_ context.Context, p FxMarkDriftProof) (FxMarkDriftProof, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if strings.TrimSpace(p.ID) == "" {
		  return FxMarkDriftProof{},
			   ErrProofNotFound
	   }
	  m.proofs[p.ID] = p
	   return p, nil
   }

   func (m *MemStore) GetProof(_ context.Context, id string) (FxMarkDriftProof, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   p, ok := m.proofs[strings.TrimSpace(id)]
	  if !ok {
		  return FxMarkDriftProof{}, ErrProofNotFound
	   }
	  return p, nil
   }

   func (m *MemStore) ListProofsByLease(_ context.Context, leaseID string, limit int) ([]FxMarkDriftProof, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   if limit <= 0 {
		  limit = 50
	   }
	  out := make([]FxMarkDriftProof, 0)
	   for _, p := range m.proofs {
		   if p.LeaseID == leaseID {
			  out = append(out, p)
		   }
	   }
	  sortProofs(out)
	   if len(out) > limit {
		   out = out[:limit]
	  }
	   return out, nil
   }

  func (m *MemStore) ListProofs(_ context.Context, kind DriftKind, limit int) ([]FxMarkDriftProof, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   if limit <= 0 {
		  limit = 50
	   }
	   out := make([]FxMarkDriftProof, 0)
	   for _, p := range m.proofs {
		   if kind == "" || p.Kind == kind {
			  out = append(out, p)
		  }
	   }
	   sortProofs(out)
	   if len(out) > limit {
		   out = out[:limit]
	   }
	   return out, nil
   }

  func (m *MemStore) CountProofs(_ context.Context) (int, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  return len(m.proofs), nil
  }

   func (m *MemStore) SaveAudit(_ context.Context, a MarkLeaseAudit) (MarkLeaseAudit, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  m.audits = append(m.audits, a)
	  return a, nil
   }

  func (m *MemStore) ListAudit(_ context.Context, leaseID string, limit int) ([]MarkLeaseAudit, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if limit <= 0 {
		   limit = 50
	  }
	   out := make([]MarkLeaseAudit, 0)
	   for _, a := range m.audits {
		   if leaseID == "" || a.LeaseID == leaseID {
			  out = append(out, a)
		   }
	  }
	   sort.Slice(out,
		   func(i, j int) bool { return out[i].At.After(out[j].At) })
	   if len(out) > limit {
		  out = out[:limit]
	   }
	  return out, nil
   }

   func (m *MemStore) CountAudits(_ context.Context) (int, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  return len(m.audits), nil
   }

  func (m *MemStore) SaveGeneration(_ context.Context, g MarkGeneration) (MarkGeneration, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  g.Asset = strings.ToUpper(strings.TrimSpace(g.Asset))
	  if g.Asset == "" {
		   return MarkGeneration{}, ErrPairRequired
	   }
	   m.gens[g.Asset] = g
	  return g, nil
   }

  func (m *MemStore) GetGeneration(_ context.Context, asset string) (MarkGeneration, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   g, ok := m.gens[strings.ToUpper(strings.TrimSpace(asset))]
	  if !ok {
		   return MarkGeneration{}, ErrLeaseNotFound
	  }
	  return g, nil
   }

  func (m *MemStore) ListGenerations(_ context.Context) ([]MarkGeneration, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   out := make([]MarkGeneration, 0, len(m.gens))
	  for _, g := range m.gens {
		  out = append(out, g)
	  }
	   sort.Slice(out,
		  func(i, j int) bool { return out[i].Asset < out[j].Asset })
	   return out, nil
  }

   func (m *MemStore) CountGenerations(_ context.Context) (int, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   return len(m.gens), nil
   }

  func sortIssued(out []MarkLease) {
	   sort.Slice(out,
		  func(i, j int) bool { return out[i].IssuedAt.After(out[j].IssuedAt) })
  }

  func sortProofs(out []FxMarkDriftProof) {
	   sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
   }
