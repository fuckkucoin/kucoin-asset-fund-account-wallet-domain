package booksnapgate

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Store 持久化接口。走 SQL 实现；走 MemStore。
type Store interface {
	SaveLease(ctx context.Context,
		l BookSnapLease) (BookSnapLease, error)
	GetLease(ctx context.Context, id string) (BookSnapLease, error)
	GetActiveLease(ctx context.Context, userID, asset string) (BookSnapLease, error)
	GetSealedByWindow(ctx context.Context, userID, asset, window string) (BookSnapLease, error)
	ListLeasesByUser(ctx context.Context, userID string, limit int) ([]BookSnapLease, error)
	ListLeasesByState(ctx context.Context,
		state LeaseState, limit int) ([]BookSnapLease, error)
	ListExpiredLeases(ctx context.Context, before time.Time, limit int) ([]BookSnapLease, error)
	ListAbortableLeases(ctx context.Context, before time.Time, limit int) ([]BookSnapLease, error)
	LatestSnapGen(ctx context.Context, userID, asset string) (int64, error)
	CountByState(ctx context.Context, state LeaseState) (int, error)
	SaveDriftProof(ctx context.Context,
		p BookSnapDriftProof) (BookSnapDriftProof, error)
	GetDriftProof(ctx context.Context, id string) (BookSnapDriftProof, error)
	ListDriftProofsByLease(ctx context.Context, leaseID string, limit int) ([]BookSnapDriftProof, error)
	ListDriftProofsByUser(ctx context.Context, userID string, limit int) ([]BookSnapDriftProof, error)
	CountDriftProofs(ctx context.Context) (int, error)
}

// ──────────────────────────────────────────
// MemStore 内存实现（用）
// ──────────────────────────────────────────

type MemStore struct {
	mu     sync.Mutex
	leases map[string]BookSnapLease
	proofs map[string]BookSnapDriftProof
}

  func NewMemStore() *MemStore {
	   return &MemStore{
		  leases: make(map[string]BookSnapLease),
		   proofs: make(map[string]BookSnapDriftProof),
	   }
   }

  func (m *MemStore) SaveLease(_ context.Context, l BookSnapLease) (BookSnapLease, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   if strings.TrimSpace(l.ID) == "" {
		   return BookSnapLease{}, fmt.Errorf("lease id empty")
	  }
	   m.leases[l.ID] = l
	   return l, nil
   }

  func (m *MemStore) GetLease(_ context.Context, id string) (BookSnapLease, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   l, ok := m.leases[id]
	  if !ok {
		  return BookSnapLease{}, ErrLeaseNotFound
	  }
	   return l, nil
   }

   func (m *MemStore) GetActiveLease(_ context.Context, userID, asset string) (BookSnapLease, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  userID = strings.TrimSpace(userID)
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	  var best BookSnapLease
	   found := false
	   for _, l := range m.leases {
		   if l.UserID != userID || strings.ToUpper(l.Asset) != asset {
			   continue
		   }
		   if !IsActive(l.State) {
			   continue
		  }
		   if !found || l.CreatedAt.After(best.CreatedAt) {
			   best = l
			  found = true
		  }
	  }
	   if !found {
		  return BookSnapLease{}, ErrLeaseNotFound
			  return l, nil
		   }
	   }
	  return BookSnapLease{}, ErrLeaseNotFound
  }

  func (m *MemStore) ListLeasesByUser(_ context.Context, userID string, limit int) ([]BookSnapLease, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   var out []BookSnapLease
	   for _, l := range m.leases {
		  if l.UserID == userID {
			   out = append(out, l)
		   }
	   }
	   sortLeases(out)
	  if limit > 0 && len(out) > limit {
		   out = out[:limit]
	  }
	   return out, nil
   }

  func (m *MemStore) ListLeasesByState(_ context.Context, state LeaseState, limit int) ([]BookSnapLease, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  var out []BookSnapLease
	   for _, l := range m.leases {
		  if l.State == state {
			   out = append(out, l)
		   }
	   }
	  sortLeases(out)
	   if limit > 0 && len(out) > limit {
		  out = out[:limit]
	   }
	  return out, nil
   }

  func (m *MemStore) ListExpiredLeases(_ context.Context, before time.Time, limit int) ([]BookSnapLease, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  var out []BookSnapLease
	   for _, l := range m.leases {
		  if IsActive(l.State) && l.ExpiresAt.Before(before) {
			  out = append(out, l)
		   }
	  }
	   sortLeases(out)
	  if limit > 0 && len(out) > limit {
		   out = out[:limit]
	  }
	  return out, nil
   }

  func (m *MemStore) ListAbortableLeases(_ context.Context, before time.Time, limit int) ([]BookSnapLease, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  var out []BookSnapLease
	  for _, l := range m.leases {
		   if l.State == LeaseAssembling && l.AssembleStart != nil && l.AssembleStart.Before(before) {
			   out = append(out, l)
		   }
	  }
	   sortLeases(out)
	  if limit > 0 && len(out) > limit {
		   out = out[:limit]
	  }
	   return out, nil
  }

  func (m *MemStore) LatestSnapGen(_ context.Context, userID, asset string) (int64, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  userID = strings.TrimSpace(userID)
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	   var maxGen int64
	   for _, l := range m.leases {
		   if l.UserID == userID && strings.ToUpper(l.Asset) == asset && l.SnapGen > maxGen {
			  maxGen = l.SnapGen
		  }
	  }
	   return maxGen, nil
  }

   func (m *MemStore) CountByState(_ context.Context, state LeaseState) (int, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  count := 0
	   for _, l := range m.leases {
		   if l.State == state {
			  count++
		  }
	  }
	  return count, nil
  }

  func (m *MemStore) SaveDriftProof(_ context.Context, p BookSnapDriftProof) (BookSnapDriftProof, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  if strings.TrimSpace(p.ID) == "" {
		  return BookSnapDriftProof{}, fmt.Errorf("drift proof id empty")
	   }
	  m.proofs[p.ID] = p
	   return p, nil
  }

   func (m *MemStore) GetDriftProof(_ context.Context, id string) (BookSnapDriftProof, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   p, ok := m.proofs[id]
	  if !ok {
		   return BookSnapDriftProof{},
			  ErrProofNotFound
	   }
	   return p, nil
  }

  func (m *MemStore) ListDriftProofsByLease(_ context.Context, leaseID string, limit int) ([]BookSnapDriftProof, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  var out []BookSnapDriftProof
	   for _, p := range m.proofs {
		  if p.LeaseID == leaseID {
			  out = append(out, p)
		  }
	   }
	  sortProofs(out)
	   if limit > 0 && len(out) > limit {
		  out = out[:limit]
	   }
	  return out, nil
  }

   func (m *MemStore) ListDriftProofsByUser(_ context.Context, userID string, limit int) ([]BookSnapDriftProof, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   var out []BookSnapDriftProof
	  for _, p := range m.proofs {
		  if p.UserID == userID {
			   out = append(out, p)
		  }
	  }
	   sortProofs(out)
	  if limit > 0 && len(out) > limit {
		  out = out[:limit]
	  }
	   return out, nil
   }

  func (m *MemStore) CountDriftProofs(_ context.Context) (int, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  return len(m.proofs), nil
   }

  func sortLeases(ls []BookSnapLease) {
	  sort.SliceStable(ls, func(i, j int) bool {
		   return ls[i].CreatedAt.After(ls[j].CreatedAt)
	  })
  }

  func sortProofs(ps []BookSnapDriftProof) {
	   sort.SliceStable(ps, func(i, j int) bool {
		  return ps[i].CreatedAt.After(ps[j].CreatedAt)
	  })
  }
