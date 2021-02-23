package balproofgate

import (
	"context"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
)

// Store 证明闸全部实体的持久化契约。

type Store interface {
	CurrentGeneration(ctx context.Context, userID, book, asset string) (int64, error)
	BumpGeneration(ctx context.Context, userID, book, asset string, expected int64, at time.Time) (int64, error)
	GetGeneration(ctx context.Context, userID, book, asset string) (*ProofGeneration, error)
	SaveLease(ctx context.Context, l *ProofLease) error
	GetLease(ctx context.Context, leaseID string) (*ProofLease, error)
	GetLeaseByNonce(ctx context.Context,
		userID, book, asset, nonce string) (*ProofLease, error)
	ListLeasesByStatus(ctx context.Context, status LeaseStatus, limit int) ([]*ProofLease, error)
	ListLeasesBySubject(ctx context.Context, userID, book, asset string) ([]*ProofLease, error)
	ListExpiredBound(ctx context.Context, now time.Time, limit int) ([]*ProofLease, error)
	SaveDrift(ctx context.Context,
		d *DriftProof) error
	GetDrift(ctx context.Context, id string) (*DriftProof, error)
	FindOpenDrift(ctx context.Context, leaseID string) (*DriftProof, error)
	ListBlockingDrifts(ctx context.Context,
		userID, book, asset string) ([]*DriftProof, error)
	ListOpenDrifts(ctx context.Context, limit int) ([]*DriftProof, error)
	SaveRevocation(ctx context.Context, r *Revocation) error
	GetRevocationByLease(ctx context.Context, leaseID string) (*Revocation, error)
	ListUnpropagated(ctx context.Context, limit int) ([]*Revocation, error)
}

// MemStore 线程安全内存仓。
type MemStore struct {
	mu sync.Mutex
	gens map[string]*ProofGeneration
	leases map[string]*ProofLease
	drifts map[string]*DriftProof
	revs map[string]*Revocation
}

// NewMemStore 初始化空仓。
  func NewMemStore() *MemStore {
	   return &MemStore{
		   gens:   map[string]*ProofGeneration{},
		   leases: map[string]*ProofLease{},
		   drifts: map[string]*DriftProof{},
		  revs: map[string]*Revocation{},
	   }
   }

  func genKey(userID, book, asset string) string {
	  return strings.TrimSpace(userID) + "|" + strings.TrimSpace(book) + "|" + strings.ToUpper(strings.TrimSpace(asset))
   }

  func (m *MemStore) CurrentGeneration(_ context.Context, userID, book, asset string) (int64, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if g, ok := m.gens[genKey(userID, book, asset)]; ok {
		  return g.Generation, nil
	  }
	  return 0, nil
   }

   func (m *MemStore) BumpGeneration(_ context.Context, userID, book, asset string, expected int64, at time.Time) (int64, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  k := genKey(userID, book, asset)
	  g, ok := m.gens[k]
	   if !ok {
		   if expected != 0 {
			  return 0, ErrStaleGen
		  }
		  g = &ProofGeneration{
			   ID: uuid.NewString(), UserID: userID, Book: book, Asset: strings.ToUpper(asset),
			   Generation: 1, UpdatedAt: at,
		   }
		  m.gens[k] = g
		   return 1, nil
	  }
	  if g.Generation != expected {
		   return 0, ErrStaleGen
	   }
	   g.Generation = expected + 1
	   g.UpdatedAt = at
	   return g.Generation, nil
  }

   func (m *MemStore) GetGeneration(_ context.Context, userID, book, asset string) (*ProofGeneration, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   if g, ok := m.gens[genKey(userID, book, asset)]; ok {
		   cp := g.Copy()
		   return &cp, nil
	   }
	  return nil, nil
   }

  func (m *MemStore) SaveLease(_ context.Context, l *ProofLease) error {
	   if l == nil || strings.TrimSpace(l.ID) == "" {
		  return ErrLeaseNotFound
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   cp := l.Copy()
	   m.leases[l.ID] = &cp
	   return nil
  }

  func (m *MemStore) GetLease(_ context.Context, leaseID string) (*ProofLease, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   l, ok := m.leases[strings.TrimSpace(leaseID)]
	  if !ok {
		  return nil, nil
	  }
	   cp := l.Copy()
	  return &cp, nil
  }

  func (m *MemStore) GetLeaseByNonce(_ context.Context, userID, book, asset, nonce string) (*ProofLease, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  userID, book, asset = strings.TrimSpace(userID), strings.TrimSpace(book), strings.ToUpper(strings.TrimSpace(asset))
	  nonce = strings.TrimSpace(nonce)
	  var found *ProofLease
	  for _, l := range m.leases {
		   if l.UserID == userID && l.Book == book && l.Asset == asset && l.Nonce == nonce {
			   if found == nil || l.IssuedAt.After(found.IssuedAt) {
				   cp := l.Copy()
				   found = &cp
			  }
		   }
	  }
	   return found, nil
  }

  func (m *MemStore) ListLeasesByStatus(_ context.Context, status LeaseStatus, limit int) ([]*ProofLease, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   if limit <= 0 {
		   limit = 256
	  }
	   out := make([]*ProofLease, 0)
	  for _, l := range m.leases {
		   if l.Status == status {
			  cp := l.Copy()
			   out = append(out, &cp)
			   if len(out) >= limit {
				   break
			  }
		   }
	  }
	  return out, nil
  }

   func (m *MemStore) ListLeasesBySubject(_ context.Context, userID, book, asset string) ([]*ProofLease, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  userID, book, asset = strings.TrimSpace(userID), strings.TrimSpace(book), strings.ToUpper(strings.TrimSpace(asset))
	  out := make([]*ProofLease, 0)
	   for _, l := range m.leases {
		   if l.UserID == userID && l.Book == book && l.Asset == asset {
			  cp := l.Copy()
			  out = append(out, &cp)
		  }
	  }
	   return out, nil
  }

   func (m *MemStore) ListExpiredBound(_ context.Context, now time.Time, limit int) ([]*ProofLease, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   if limit <= 0 {
		   limit = 256
	   }
	  out := make([]*ProofLease, 0)
	  for _, l := range m.leases {
		   if (l.Status == LeaseBound || l.Status == LeaseIssued) && !now.Before(l.ExpiresAt) {
			  cp := l.Copy()
			  out = append(out, &cp)
			   if len(out) >= limit {
				   break
			  }
		   }
	   }
	  return out, nil
  }

   func (m *MemStore) SaveDrift(_ context.Context, d *DriftProof) error {
	  if d == nil || strings.TrimSpace(d.ID) == "" {
		  return ErrDriftNotFound
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   cp := d.Copy()
	   m.drifts[d.ID] = &cp
	  return nil
  }

  func (m *MemStore) GetDrift(_ context.Context, id string) (*DriftProof, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  d, ok := m.drifts[strings.TrimSpace(id)]
	  if !ok {
		  return nil, nil
	  }
	  cp := d.Copy()
	   return &cp, nil
   }

   func (m *MemStore) FindOpenDrift(_ context.Context, leaseID string) (*DriftProof, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   leaseID = strings.TrimSpace(leaseID)
	   for _, d := range m.drifts {
		  if d.LeaseID == leaseID && d.Blocking() {
			   cp := d.Copy()
			  return &cp, nil
		   }
	  }
	  return nil, nil
   }

   func (m *MemStore) ListBlockingDrifts(_ context.Context, userID, book, asset string) ([]*DriftProof, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   userID, book, asset = strings.TrimSpace(userID), strings.TrimSpace(book), strings.ToUpper(strings.TrimSpace(asset))
	   out := make([]*DriftProof, 0)
	  for _, d := range m.drifts {
		   if d.UserID == userID && d.Book == book && d.Asset == asset && d.Blocking() {
			  cp := d.Copy()
			   out = append(out, &cp)
		  }
	  }
	   return out, nil
   }

  func (m *MemStore) ListOpenDrifts(_ context.Context, limit int) ([]*DriftProof, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  if limit <= 0 {
		  limit = 256
	  }
	  out := make([]*DriftProof, 0)
	  for _, d := range m.drifts {
		  if d.Status == DriftOpen {
			  cp := d.Copy()
			  out = append(out, &cp)
			  if len(out) >= limit {
				  break
			  }
		   }
	   }
	   return out, nil
   }

   func (m *MemStore) SaveRevocation(_ context.Context, r *Revocation) error {
	  if r == nil || strings.TrimSpace(r.ID) == "" {
		  return ErrRevoked
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   cp := r.Copy()
	  m.revs[r.LeaseID] = &cp
	   return nil
   }

  func (m *MemStore) GetRevocationByLease(_ context.Context, leaseID string) (*Revocation, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  r, ok := m.revs[strings.TrimSpace(leaseID)]
	   if !ok {
		   return nil, nil
	  }
	  cp := r.Copy()
	   return &cp, nil
   }

  func (m *MemStore) ListUnpropagated(_ context.Context, limit int) ([]*Revocation, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   if limit <= 0 {
		  limit = 256
	   }
	  out := make([]*Revocation, 0)
	   for _, r := range m.revs {
		  if !r.Propagated {
			  cp := r.Copy()
			   out = append(out, &cp)
			   if len(out) >= limit {
				   break
			   }
		  }
	  }
	   return out, nil
  }
