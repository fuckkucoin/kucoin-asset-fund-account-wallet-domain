package signpipegate

import (
	"context"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
)

// Store 引擎全部实体的持久化契约。

type Store interface {
	CurrentGeneration(ctx context.Context, movementID, jobID string) (int64, error)
	BumpGeneration(ctx context.Context, movementID, jobID string, expected int64, at time.Time) (int64, error)
	GetGeneration(ctx context.Context, movementID, jobID string) (*RetryGeneration, error)
	SaveLease(ctx context.Context,
		l *InflightLease) error
	GetLease(ctx context.Context, leaseID string) (*InflightLease, error)
	GetLeaseByNonce(ctx context.Context,
		movementID, nonce string) (*InflightLease, error)
	FindActiveLease(ctx context.Context, movementID string) (*InflightLease, error)
	ListLeasesByStatus(ctx context.Context, status LeaseStatus, limit int) ([]*InflightLease, error)
	ListLeasesByMovement(ctx context.Context, movementID string) ([]*InflightLease, error)
	ListExpiredArmed(ctx context.Context, now time.Time, limit int) ([]*InflightLease, error)
	SaveFault(ctx context.Context, f *FaultCase) error
	GetFault(ctx context.Context, id string) (*FaultCase, error)
	ListBlockingFaults(ctx context.Context, movementID string) ([]*FaultCase, error)
	ListOpenFaults(ctx context.Context, limit int) ([]*FaultCase, error)
	SaveCloseout(ctx context.Context, c *CloseoutProof) error
	GetCloseout(ctx context.Context, id string) (*CloseoutProof, error)
	FindOpenCloseout(ctx context.Context, leaseID string) (*CloseoutProof, error)
	ListBlockingCloseouts(ctx context.Context,
		movementID string) ([]*CloseoutProof, error)
	ListOpenCloseouts(ctx context.Context, limit int) ([]*CloseoutProof, error)
	SaveArtifact(ctx context.Context, a *SignedArtifact) error
	GetArtifactByMovement(ctx context.Context, movementID string) (*SignedArtifact, error)
	GetArtifact(ctx context.Context, id string) (*SignedArtifact, error)
}

// MemStore 线程安全内存仓。

type MemStore struct {
	mu   sync.Mutex
	gens map[string]*RetryGeneration
	leases map[string]*InflightLease
	faults map[string]*FaultCase
	closeouts map[string]*CloseoutProof
	arts      map[string]*SignedArtifact
}

  func NewMemStore() *MemStore {
	   return &MemStore{
		   gens:   map[string]*RetryGeneration{},
		   leases: map[string]*InflightLease{},
		   faults:    map[string]*FaultCase{},
		  closeouts: map[string]*CloseoutProof{},
		  arts: map[string]*SignedArtifact{},
	   }
   }

   func genKey(movementID, jobID string) string {
	  return NormalizeMovement(movementID) + "|" + strings.TrimSpace(jobID)
  }

   func (m *MemStore) CurrentGeneration(_ context.Context, movementID, jobID string) (int64, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   if g, ok := m.gens[genKey(movementID, jobID)]; ok {
		   return g.Generation, nil
	  }
	  return 0, nil
   }

   func (m *MemStore) BumpGeneration(_ context.Context, movementID, jobID string, expected int64, at time.Time) (int64, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   k := genKey(movementID, jobID)
	   g, ok := m.gens[k]
	   if !ok {
		   if expected != 0 {
			  return 0, ErrStaleGen
		   }
		   g = &RetryGeneration{
			  ID: uuid.NewString(), MovementID: NormalizeMovement(movementID),
			  JobID: strings.TrimSpace(jobID), Generation: 1, UpdatedAt: at,
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

  func (m *MemStore) GetGeneration(_ context.Context, movementID, jobID string) (*RetryGeneration, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   if g, ok := m.gens[genKey(movementID, jobID)]; ok {
		  cp := g.Copy()
		  return &cp, nil
	  }
	   return nil, nil
  }

  func (m *MemStore) SaveLease(_ context.Context, l *InflightLease) error {
	  if l == nil || strings.TrimSpace(l.ID) == "" {
		   return ErrLeaseNotFound
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  cp := l.Copy()
	  m.leases[l.ID] = &cp
	  return nil
   }

  func (m *MemStore) GetLease(_ context.Context, leaseID string) (*InflightLease, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   l,
		   ok := m.leases[strings.TrimSpace(leaseID)]
	  if !ok {
		   }
	  }
	   return best, nil
   }

   func (m *MemStore) FindActiveLease(_ context.Context, movementID string) (*InflightLease, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   movementID = NormalizeMovement(movementID)
	   for _, l := range m.leases {
		  if l.MovementID == movementID && (l.Status == LeaseArmed || l.Status == LeaseInflight) {
			  cp := l.Copy()
			   return &cp, nil
		  }
	  }
	   return nil, nil
   }

   func (m *MemStore) ListLeasesByStatus(_ context.Context, status LeaseStatus, limit int) ([]*InflightLease, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if limit <= 0 {
		   limit = 64
	  }
	   out := make([]*InflightLease,
		   0)
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

  func (m *MemStore) ListLeasesByMovement(_ context.Context, movementID string) ([]*InflightLease, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   movementID = NormalizeMovement(movementID)
	   out := make([]*InflightLease,
		  0)
	  for _, l := range m.leases {
		   if l.MovementID == movementID {
			  cp := l.Copy()
			  out = append(out, &cp)
		  }
	   }
	   return out, nil
  }

  func (m *MemStore) ListExpiredArmed(_ context.Context, now time.Time, limit int) ([]*InflightLease, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  if limit <= 0 {
		  limit = 64
	  }
	  out := make([]*InflightLease, 0)
	  for _, l := range m.leases {
		   if (l.Status == LeaseArmed || l.Status == LeaseInflight) && !now.Before(l.ExpiresAt) {
			   cp := l.Copy()
			  out = append(out, &cp)
			   if len(out) >= limit {
				  break
			  }
		  }
	  }
	   return out, nil
   }

  func (m *MemStore) SaveFault(_ context.Context, f *FaultCase) error {
	   if f == nil || strings.TrimSpace(f.ID) == "" {
		  return ErrFaultNotFound
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   cp := f.Copy()
	  m.faults[f.ID] = &cp
	  return nil
  }

   func (m *MemStore) GetFault(_ context.Context, id string) (*FaultCase, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   f, ok := m.faults[strings.TrimSpace(id)]
	   if !ok {
		   return nil, ErrFaultNotFound
	  }
	  cp := f.Copy()
	  return &cp, nil
  }

   func (m *MemStore) ListBlockingFaults(_ context.Context, movementID string) ([]*FaultCase, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  movementID = NormalizeMovement(movementID)
	  out := make([]*FaultCase, 0)
	   for _, f := range m.faults {
		   if f.MovementID == movementID && f.Blocking() {
			   cp := f.Copy()
			   out = append(out, &cp)
		   }
	  }
	   return out, nil
   }

  func (m *MemStore) ListOpenFaults(_ context.Context, limit int) ([]*FaultCase, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  if limit <= 0 {
		   limit = 64
	   }
	  out := make([]*FaultCase, 0)
	  for _, f := range m.faults {
		  if f.Blocking() {
			   cp := f.Copy()
			  out = append(out, &cp)
			  if len(out) >= limit {
				   break
			   }
		  }
	   }
	  return out, nil
   }

  func (m *MemStore) SaveCloseout(_ context.Context, c *CloseoutProof) error {
	  if c == nil || strings.TrimSpace(c.ID) == "" {
		   return ErrCloseoutNotFound
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   cp := c.Copy()
	   m.closeouts[c.ID] = &cp
	   return nil
   }

   func (m *MemStore) GetCloseout(_ context.Context, id string) (*CloseoutProof, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  c, ok := m.closeouts[strings.TrimSpace(id)]
	  if !ok {
		   return nil, ErrCloseoutNotFound
	  }
	  cp := c.Copy()
	  return &cp, nil
  }

   func (m *MemStore) FindOpenCloseout(_ context.Context, leaseID string) (*CloseoutProof, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  leaseID = strings.TrimSpace(leaseID)
	   for _, c := range m.closeouts {
		  if c.LeaseID == leaseID && c.Blocking() {
			   cp := c.Copy()
			  return &cp, nil
		   }
	  }
	  return nil, nil
  }

  func (m *MemStore) ListBlockingCloseouts(_ context.Context, movementID string) ([]*CloseoutProof, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  movementID = NormalizeMovement(movementID)
	   out := make([]*CloseoutProof, 0)
	   for _, c := range m.closeouts {
		  if c.MovementID == movementID && c.Blocking() {
			   cp := c.Copy()
			  out = append(out, &cp)
		   }
	  }
	  return out, nil
  }

  func (m *MemStore) ListOpenCloseouts(_ context.Context, limit int) ([]*CloseoutProof, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  if limit <= 0 {
		   limit = 64
	   }
	  out := make([]*CloseoutProof, 0)
	   for _, c := range m.closeouts {
		  if c.Blocking() {
			  cp := c.Copy()
			   out = append(out, &cp)
			  if len(out) >= limit {
				  break
			  }
		  }
	  }
	  return out, nil
  }

  func (m *MemStore) SaveArtifact(_ context.Context, a *SignedArtifact) error {
	  if a == nil || strings.TrimSpace(a.ID) == "" {
		  return ErrArtifactNotFound
	  }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  if exist, ok := m.arts[NormalizeMovement(a.MovementID)]; ok && exist.ID != a.ID {
		  return ErrDoubleArtifact
	   }
	  cp := a.Copy()
	   m.arts[NormalizeMovement(a.MovementID)] = &cp
	   return nil
  }

   func (m *MemStore) GetArtifactByMovement(_ context.Context, movementID string) (*SignedArtifact, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   a,
		  ok := m.arts[NormalizeMovement(movementID)]
	  if !ok {
		   return nil, nil
	  }
	   cp := a.Copy()
	   return &cp, nil
  }

   func (m *MemStore) GetArtifact(_ context.Context, id string) (*SignedArtifact, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   for _, a := range m.arts {
		  if a.ID == id {
			   cp := a.Copy()
			  return &cp, nil
		  }
	   }
	  return nil, ErrArtifactNotFound
  }
