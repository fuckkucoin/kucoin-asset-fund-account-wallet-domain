package usageleasegate

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

type Store interface {
	SaveLease(ctx context.Context, l UsageLease) (UsageLease, error)
	GetLease(ctx context.Context,
		id string) (UsageLease, error)
	ListLeasesByUser(ctx context.Context, userID string, limit int) ([]UsageLease, error)
	ListLeasesByState(ctx context.Context, state LeaseState, limit int) ([]UsageLease, error)
	ListLeasesByWindow(ctx context.Context, windowKey string, limit int) ([]UsageLease, error)
	ListExpiredLive(ctx context.Context, before time.Time, limit int) ([]UsageLease, error)
	CountByState(ctx context.Context, state LeaseState) (int, error)
	GetLeaseByIdem(ctx context.Context, idem string) (UsageLease, error)
	SaveReservation(ctx context.Context, r UsageReservation) (UsageReservation, error)
	GetReservation(ctx context.Context, id string) (UsageReservation, error)
	GetReservationByLease(ctx context.Context, leaseID string) (UsageReservation, error)
	ListHeldByWindow(ctx context.Context, windowKey string) ([]UsageReservation, error)
	ListExpiredHeld(ctx context.Context, before time.Time, limit int) ([]UsageReservation, error)
	CountReservations(ctx context.Context,
		state ReservationState) (int, error)
	NextSeq(ctx context.Context, windowKey string) (int64, error)
	SaveProof(ctx context.Context, p UsageDriftProof) (UsageDriftProof, error)
	GetProof(ctx context.Context,
		id string) (UsageDriftProof, error)
	ListProofsByLease(ctx context.Context, leaseID string, limit int) ([]UsageDriftProof, error)
	ListProofsByUser(ctx context.Context, userID string, limit int) ([]UsageDriftProof, error)
	ListProofs(ctx context.Context, kind DriftKind, limit int) ([]UsageDriftProof, error)
	CountProofs(ctx context.Context) (int, error)
	SaveAudit(ctx context.Context, a UsageLeaseAudit) (UsageLeaseAudit, error)
	ListAudit(ctx context.Context, leaseID string, limit int) ([]UsageLeaseAudit, error)
	CountAudits(ctx context.Context) (int, error)
}

type MemStore struct {
	mu     sync.Mutex
	leases map[string]UsageLease
	resv   map[string]UsageReservation
	proofs map[string]UsageDriftProof
	audits []UsageLeaseAudit
	seq    map[string]int64
}

   func NewMemStore() *MemStore {
	  return &MemStore{
		  leases: make(map[string]UsageLease),
		  resv:   make(map[string]UsageReservation),
		   proofs: make(map[string]UsageDriftProof),
		   seq:    make(map[string]int64),
	  }
   }

   func (m *MemStore) SaveLease(_ context.Context, l UsageLease) (UsageLease, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   if strings.TrimSpace(l.ID) == "" {
		  return UsageLease{}, ErrLeaseNotFound
	   }
	   m.leases[l.ID] = l
	  return l, nil
  }

  func (m *MemStore) GetLease(_ context.Context, id string) (UsageLease, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  l, ok := m.leases[strings.TrimSpace(id)]
	  if !ok {
		  return UsageLease{}, ErrLeaseNotFound
	  }
	  return l, nil
   }

   func (m *MemStore) GetLeaseByIdem(_ context.Context, idem string) (UsageLease, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  idem = strings.TrimSpace(idem)
	   if idem == "" {
		  return UsageLease{}, ErrLeaseNotFound
	   }
	   var best UsageLease
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
		  return UsageLease{}, ErrLeaseNotFound
	  }
	  return best, nil
  }

   func (m *MemStore) ListLeasesByUser(_ context.Context, userID string, limit int) ([]UsageLease, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  if limit <= 0 {
		   limit = 50
	  }
	  out := make([]UsageLease, 0)
	   for _, l := range m.leases {
		  if l.UserID == userID {
			  out = append(out, l)
		   }
	   }
	   sort.Slice(out, func(i, j int) bool { return out[i].IssuedAt.After(out[j].IssuedAt) })
	   if len(out) > limit {
		  out = out[:limit]
	   }
	  return out, nil
   }

   func (m *MemStore) ListLeasesByState(_ context.Context, state LeaseState, limit int) ([]UsageLease, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  if limit <= 0 {
		   limit = DefaultExpiryBatch
	   }
	   out := make([]UsageLease, 0)
	   for _, l := range m.leases {
		   if l.State == state {
			  out = append(out, l)
		  }
	   }
	  sort.Slice(out, func(i, j int) bool { return out[i].IssuedAt.Before(out[j].IssuedAt) })
	  if len(out) > limit {
		  out = out[:limit]
	  }
	   return out, nil
   }

  func (m *MemStore) ListLeasesByWindow(_ context.Context, windowKey string, limit int) ([]UsageLease, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   if limit <= 0 {
		   limit = DefaultPatrolBatch
	  }
	   out := make([]UsageLease, 0)
	   for _, l := range m.leases {
		   if WindowKey(l.Kind, l.UserID, l.Asset, l.Dest) != windowKey {
			   continue
		  }
		  out = append(out, l)
	  }
	   sort.Slice(out, func(i, j int) bool { return out[i].IssuedAt.Before(out[j].IssuedAt) })
	   if len(out) > limit {
		  out = out[:limit]
	   }
	  return out, nil
   }

   func (m *MemStore) ListExpiredLive(_ context.Context, before time.Time, limit int) ([]UsageLease, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   if limit <= 0 {
		  limit = DefaultExpiryBatch
	   }
	  out := make([]UsageLease, 0)
	  for _, l := range m.leases {
		  if !IsLive(l.State) {
			  continue
		  }
		  if !l.ExpiresAt.After(before) {
			   out = append(out, l)
		   }
	  }
	  sort.Slice(out, func(i, j int) bool { return out[i].ExpiresAt.Before(out[j].ExpiresAt) })
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

  func (m *MemStore) SaveReservation(_ context.Context, r UsageReservation) (UsageReservation, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   if strings.TrimSpace(r.ID) == "" {
		   return UsageReservation{}, ErrReservationMissing
	   }
	  m.resv[r.ID] = r
	  return r, nil
  }

   func (m *MemStore) GetReservation(_ context.Context, id string) (UsageReservation, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   r, ok := m.resv[strings.TrimSpace(id)]
	   if !ok {
		   return UsageReservation{}, ErrReservationMissing
	   }
	  return r, nil
  }

   func (m *MemStore) GetReservationByLease(_ context.Context, leaseID string) (UsageReservation, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   leaseID = strings.TrimSpace(leaseID)
	  var best UsageReservation
	   found := false
	  for _, r := range m.resv {
		  if r.LeaseID != leaseID {
			   continue
		   }
		   if !found || r.HeldAt.After(best.HeldAt) {
			   best = r
			   found = true
		   }
	   }
	   if !found {
		  return UsageReservation{}, ErrReservationMissing
	   }
	   return best, nil
  }

   func (m *MemStore) ListHeldByWindow(_ context.Context, windowKey string) ([]UsageReservation, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  out := make([]UsageReservation, 0)
	   for _, r := range m.resv {
		   if r.WindowKey == windowKey && r.State == ResHeld {
			   out = append(out, r)
		   }
	   }
	  sort.Slice(out, func(i, j int) bool { return rSeq(out[i]) < rSeq(out[j]) })
	   return out, nil
   }

  func rSeq(r UsageReservation) int64 { return r.Seq }

   func (m *MemStore) ListExpiredHeld(_ context.Context, before time.Time, limit int) ([]UsageReservation, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   if limit <= 0 {
		   limit = DefaultExpiryBatch
	  }
	   out := make([]UsageReservation,
		   0)
	  for _, r := range m.resv {
		   if r.State != ResHeld {
			  continue
		  }
		   if !r.ExpiresAt.After(before) {
			   out = append(out, r)
		   }
	   }
	  sort.Slice(out, func(i, j int) bool { return out[i].ExpiresAt.Before(out[j].ExpiresAt) })
	   if len(out) > limit {
		   out = out[:limit]
	   }
	   return out, nil
   }

  func (m *MemStore) CountReservations(_ context.Context, state ReservationState) (int, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  n := 0
	  for _, r := range m.resv {
		   if r.State == state {
			   n++
		   }
	   }
	  return n, nil
  }

   func (m *MemStore) NextSeq(_ context.Context, windowKey string) (int64, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   m.seq[windowKey]++
	  return m.seq[windowKey], nil
  }

  func (m *MemStore) SaveProof(_ context.Context, p UsageDriftProof) (UsageDriftProof, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  if strings.TrimSpace(p.ID) == "" {
		   return UsageDriftProof{}, ErrProofNotFound
	   }
	   m.proofs[p.ID] = p
	   return p, nil
  }

   func (m *MemStore) GetProof(_ context.Context, id string) (UsageDriftProof, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  p, ok := m.proofs[strings.TrimSpace(id)]
	   if !ok {
		  return UsageDriftProof{},
			  ErrProofNotFound
	   }
	   return p, nil
   }

   func (m *MemStore) ListProofsByLease(_ context.Context, leaseID string, limit int) ([]UsageDriftProof, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   if limit <= 0 {
		   limit = 50
	  }
	   out := make([]UsageDriftProof, 0)
	  for _, p := range m.proofs {
		  if p.LeaseID == leaseID {
			   out = append(out, p)
		   }
	  }
	  sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	   if len(out) > limit {
		  out = out[:limit]
	  }
	   return out, nil
  }

  func (m *MemStore) ListProofsByUser(_ context.Context, userID string, limit int) ([]UsageDriftProof, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if limit <= 0 {
		  limit = 50
	  }
	  out := make([]UsageDriftProof, 0)
	  for _, p := range m.proofs {
		  if p.UserID == userID {
			  out = append(out, p)
		  }
	  }
	   sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	  if len(out) > limit {
		  out = out[:limit]
	  }
	   return out, nil
  }

  func (m *MemStore) ListProofs(_ context.Context, kind DriftKind, limit int) ([]UsageDriftProof, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if limit <= 0 {
		   limit = 50
	  }
	   out := make([]UsageDriftProof, 0)
	  for _, p := range m.proofs {
		   if kind == "" || p.Kind == kind {
			  out = append(out, p)
		   }
	   }
	   sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
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

  func (m *MemStore) SaveAudit(_ context.Context, a UsageLeaseAudit) (UsageLeaseAudit, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   m.audits = append(m.audits, a)
	   return a, nil
  }

   func (m *MemStore) ListAudit(_ context.Context, leaseID string, limit int) ([]UsageLeaseAudit, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   if limit <= 0 {
		  limit = 50
	  }
	  out := make([]UsageLeaseAudit, 0)
	  for _, a := range m.audits {
		  if a.LeaseID == leaseID {
			  out = append(out, a)
		  }
	  }
	   if len(out) > limit {
		  out = out[len(out)-limit:]
	   }
	  return out, nil
  }

   func (m *MemStore) CountAudits(_ context.Context) (int, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  return len(m.audits), nil
  }
