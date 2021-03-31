package cvquotelease

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

type Store interface {
	SaveLease(ctx context.Context, l CvQuoteLease) (CvQuoteLease, error)
	GetLease(ctx context.Context, id string) (CvQuoteLease, error)
	GetLeaseByQuote(ctx context.Context, quoteID string) (CvQuoteLease, error)
	ListIssuedByPair(ctx context.Context, pair string, limit int) ([]CvQuoteLease, error)
	ListExpiredIssued(ctx context.Context, before time.Time, limit int) ([]CvQuoteLease, error)
	ListIssued(ctx context.Context, limit int) ([]CvQuoteLease, error)
	ListLeasesByUser(ctx context.Context, userID string, limit int) ([]CvQuoteLease, error)
	ListLeasesByState(ctx context.Context, state LeaseState, limit int) ([]CvQuoteLease, error)
	CountByState(ctx context.Context, state LeaseState) (int, error)
	CASConsume(ctx context.Context, id, token string, now time.Time, orderID string) (CvQuoteLease, bool, error)
	SaveMark(ctx context.Context, m MarkGeneration) (MarkGeneration, error)
	GetMark(ctx context.Context, pair string) (MarkGeneration, error)
	ListMarks(ctx context.Context) ([]MarkGeneration, error)
	CountMarks(ctx context.Context) (int, error)
	SaveQuote(ctx context.Context,
		q QuoteSnapshot) (QuoteSnapshot, error)
	GetQuote(ctx context.Context, id string) (QuoteSnapshot, error)
	SaveProof(ctx context.Context,
		p CvQuoteDriftProof) (CvQuoteDriftProof, error)
	GetProof(ctx context.Context, id string) (CvQuoteDriftProof, error)
	ListProofsByLease(ctx context.Context, leaseID string, limit int) ([]CvQuoteDriftProof, error)
	ListProofsByQuote(ctx context.Context, quoteID string, limit int) ([]CvQuoteDriftProof, error)
	ListProofsByUser(ctx context.Context,
		userID string, limit int) ([]CvQuoteDriftProof, error)
	CountProofs(ctx context.Context) (int, error)
}

type MemStore struct {
	mu sync.Mutex
	leases map[string]CvQuoteLease
	marks  map[string]MarkGeneration
	quotes map[string]QuoteSnapshot
	proofs map[string]CvQuoteDriftProof
}

   func NewMemStore() *MemStore {
	  return &MemStore{
		  leases: make(map[string]CvQuoteLease),
		   marks:  make(map[string]MarkGeneration),
		  quotes: make(map[string]QuoteSnapshot),
		  proofs: make(map[string]CvQuoteDriftProof),
	   }
   }

   func (m *MemStore) SaveLease(_ context.Context, l CvQuoteLease) (CvQuoteLease, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  if strings.TrimSpace(l.ID) == "" {
		   return CvQuoteLease{}, ErrLeaseNotFound
	  }
	  m.leases[l.ID] = l
	  return l, nil
   }

  func (m *MemStore) GetLease(_ context.Context, id string) (CvQuoteLease, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  l, ok := m.leases[strings.TrimSpace(id)]
	  if !ok {
		  return CvQuoteLease{},
			  ErrLeaseNotFound
	   }
	   return l, nil
  }

   func (m *MemStore) GetLeaseByQuote(_ context.Context, quoteID string) (CvQuoteLease, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  quoteID = strings.TrimSpace(quoteID)
	   var best CvQuoteLease
	   found := false
	  for _, l := range m.leases {
		   if l.QuoteID != quoteID {
			   continue
		   }
		   if !found || l.IssuedAt.After(best.IssuedAt) {
			  best = l
			   found = true
		  }
	  }
	  if !found {
		   return CvQuoteLease{}, ErrLeaseNotFound
	   }
	   return best, nil
  }

  func (m *MemStore) ListIssuedByPair(_ context.Context, pair string, limit int) ([]CvQuoteLease, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   if limit <= 0 {
		   limit = DefaultRevokeBatch
	  }
	   out := make([]CvQuoteLease, 0)
	   for _, l := range m.leases {
		  if l.State != LeaseIssued {
			  continue
		  }
		  if PairKey(l.FromAsset, l.ToAsset) != pair {
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

   func (m *MemStore) ListExpiredIssued(_ context.Context, before time.Time, limit int) ([]CvQuoteLease, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   if limit <= 0 {
		   limit = DefaultExpiryBatch
	  }
	  out := make([]CvQuoteLease, 0)
	  for _, l := range m.leases {
		  if l.State != LeaseIssued {
			   continue
		   }
		  if l.ExpiresAt.After(before) {
			   continue
		   }
		   out = append(out, l)
	   }
	   sort.Slice(out, func(i, j int) bool { return out[i].ExpiresAt.Before(out[j].ExpiresAt) })
	   if len(out) > limit {
		   out = out[:limit]
	  }
	  return out, nil
  }

  func (m *MemStore) ListIssued(_ context.Context, limit int) ([]CvQuoteLease, error) {
	   return m.ListLeasesByState(context.Background(), LeaseIssued, limit)
  }

  func (m *MemStore) ListLeasesByUser(_ context.Context, userID string, limit int) ([]CvQuoteLease, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   if limit <= 0 {
		  limit = 50
	   }
	  out := make([]CvQuoteLease, 0)
	   for _, l := range m.leases {
		  if userID != "" && l.UserID != userID {
			   continue
		  }
		   out = append(out, l)
	  }
	   sort.Slice(out, func(i, j int) bool { return out[i].IssuedAt.After(out[j].IssuedAt) })
	   if len(out) > limit {
		   out = out[:limit]
	  }
	   return out, nil
  }

  func (m *MemStore) ListLeasesByState(_ context.Context, state LeaseState, limit int) ([]CvQuoteLease, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   if limit <= 0 {
		   limit = 100
	   }
	  out := make([]CvQuoteLease, 0)
	   for _, l := range m.leases {
		   if l.State == state {
			  out = append(out, l)
		   }
	  }
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

  func (m *MemStore) CASConsume(_ context.Context, id, token string, now time.Time, orderID string) (CvQuoteLease, bool, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  l, ok := m.leases[id]
	  if !ok {
		   return CvQuoteLease{}, false, ErrLeaseNotFound
	   }
	  if l.State == LeaseConsumed && l.Used {
		   return l, false, nil
	  }
	   if l.State != LeaseIssued {
		   return l,
			  false, ErrLeaseConsumed
	  }
	   if l.ConsumeToken != "" && token != "" && l.ConsumeToken != token {
		  return l, false, ErrDualExecute
	   }
	   ts := now
	   l.State = LeaseConsumed
	  l.Used = true
	   l.OrderID = orderID
	   l.ConsumeToken = token
	  l.ConsumedAt = &ts
	  l.UpdatedAt = now
	   m.leases[id] = l
	  if q, ok := m.quotes[l.QuoteID]; ok {
		   q.Used = true
		  q.OrderID = orderID
		   m.quotes[l.QuoteID] = q
	  }
	  return l, true, nil
  }

  func (m *MemStore) SaveMark(_ context.Context, mk MarkGeneration) (MarkGeneration, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  if mk.PairKey == "" {
		  return MarkGeneration{}, ErrMarkPairRequired
	   }
	   m.marks[mk.PairKey] = mk
	  return mk, nil
   }

   func (m *MemStore) GetMark(_ context.Context, pair string) (MarkGeneration, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  mk, ok := m.marks[pair]
	  if !ok {
		   return MarkGeneration{}, ErrMarkGenMissing
	   }
	   return mk, nil
  }

  func (m *MemStore) ListMarks(_ context.Context) ([]MarkGeneration, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   out := make([]MarkGeneration, 0, len(m.marks))
	   for _, mk := range m.marks {
		   out = append(out, mk)
	  }
	  return out, nil
  }

  func (m *MemStore) CountMarks(_ context.Context) (int, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   return len(m.marks), nil
   }

  func (m *MemStore) SaveQuote(_ context.Context, q QuoteSnapshot) (QuoteSnapshot, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  if q.ID == "" {
		   return QuoteSnapshot{},
			  ErrQuoteNotFound
	   }
	  m.quotes[q.ID] = q
	  return q, nil
   }

  func (m *MemStore) GetQuote(_ context.Context, id string) (QuoteSnapshot, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  q, ok := m.quotes[strings.TrimSpace(id)]
	   if !ok {
		   return QuoteSnapshot{},
			  ErrQuoteNotFound
	  }
	   return q, nil
   }

  func (m *MemStore) SaveProof(_ context.Context, p CvQuoteDriftProof) (CvQuoteDriftProof, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  if p.ID == "" {
		   return CvQuoteDriftProof{}, ErrProofNotFound
	  }
	  m.proofs[p.ID] = p
	   return p, nil
   }

   func (m *MemStore) GetProof(_ context.Context, id string) (CvQuoteDriftProof, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  p, ok := m.proofs[id]
	   if !ok {
		  return CvQuoteDriftProof{}, ErrProofNotFound
	  }
	   return p, nil
  }

   func (m *MemStore) ListProofsByLease(_ context.Context, leaseID string, limit int) ([]CvQuoteDriftProof, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   return m.filterProofs(func(p CvQuoteDriftProof) bool { return p.LeaseID == leaseID },
		  limit)
   }

   func (m *MemStore) ListProofsByQuote(_ context.Context, quoteID string, limit int) ([]CvQuoteDriftProof, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  return m.filterProofs(func(p CvQuoteDriftProof) bool { return p.QuoteID == quoteID }, limit)
  }

   func (m *MemStore) ListProofsByUser(_ context.Context, userID string, limit int) ([]CvQuoteDriftProof, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   return m.filterProofs(func(p CvQuoteDriftProof) bool { return p.UserID == userID }, limit)
  }

  func (m *MemStore) CountProofs(_ context.Context) (int, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  return len(m.proofs), nil
  }

   func (m *MemStore) filterProofs(pred func(CvQuoteDriftProof) bool, limit int) ([]CvQuoteDriftProof, error) {
	   if limit <= 0 {
		  limit = 50
	   }
	  out := make([]CvQuoteDriftProof, 0)
	   for _, p := range m.proofs {
		  if pred(p) {
			  out = append(out, p)
		   }
	   }
	  sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
