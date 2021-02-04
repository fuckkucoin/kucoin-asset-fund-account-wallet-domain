package allowleasegate

import (
	"sort"
	"sync"
	"time"
	"github.com/google/uuid"
)

// ──────────────────────────────────────────
// LeaseStore 仓储接口
// ──────────────────────────────────────────

type LeaseStore interface {
	SaveLease(l *AllowLease) error
	GetLease(id string) (*AllowLease, error)
	GetLeaseByMovement(movementID string) (*AllowLease,
		error)
	ListActive() ([]*AllowLease, error)
	ListByUser(userID string,
		limit int) ([]*AllowLease, error)
	SaveProof(p *AllowDriftProof) error
	GetProof(id string) (*AllowDriftProof, error)
	ListProofs(movementID string, limit int) ([]*AllowDriftProof, error)
	ListProofsByUser(userID string, limit int) ([]*AllowDriftProof, error)
	ListProofsByLease(leaseID string,
		limit int) ([]*AllowDriftProof, error)
	SavePolicyBind(b *PolicyBind) error
	GetPolicyBind(leaseID string) (*PolicyBind, error)
}

// ──────────────────────────────────────────
// MemStore 内存实现
// ──────────────────────────────────────────

type MemStore struct {
	mu     sync.RWMutex
	leases map[string]*AllowLease
	byMove map[string]string
	proofs map[string]*AllowDriftProof
	binds  map[string]*PolicyBind
}

   func NewMemStore() *MemStore {
	  return &MemStore{
		  leases: make(map[string]*AllowLease),
		   byMove: make(map[string]string),
		   proofs: make(map[string]*AllowDriftProof),
		  binds: make(map[string]*PolicyBind),
	   }
  }

  func (s *MemStore) SaveLease(l *AllowLease) error {
	  if s == nil {
		  return ErrStoreNil
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  if l.ID == "" {
		  l.ID = uuid.NewString()
	   }
	   now := time.Now().UTC()
	   if l.CreatedAt.IsZero() {
		   l.CreatedAt = now
	  }
	  l.UpdatedAt = now
	   cp := *l
	  s.leases[l.ID] = &cp
	   if l.MovementID != "" {
		   s.byMove[l.MovementID] = l.ID
	   }
	  return nil
   }

  func (s *MemStore) GetLease(id string) (*AllowLease, error) {
	   if s == nil {
		   return nil, ErrStoreNil
	   }
	  s.mu.RLock()
	  defer s.mu.RUnlock()
	  l, ok := s.leases[id]
	   if !ok {
		  return nil, ErrLeaseNotFound
	  }
	   cp := *l
	   return &cp, nil
  }

   func (s *MemStore) GetLeaseByMovement(movementID string) (*AllowLease, error) {
	   if s == nil {
		   return nil, ErrStoreNil
	  }
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	   id, ok := s.byMove[movementID]
	   if !ok {
		   return nil, ErrLeaseNotFound
	  }
	   l, ok := s.leases[id]
	   if !ok {
		   return nil, ErrLeaseNotFound
	  }
	  cp := *l
	  return &cp, nil
  }

  func (s *MemStore) ListActive() ([]*AllowLease, error) {
	   if s == nil {
		  return nil, ErrStoreNil
	   }
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	  var out []*AllowLease
	   for _, l := range s.leases {
		   if l.State.Active() {
			   cp := *l
			  out = append(out, &cp)
		  }
	  }
	   sort.Slice(out, func(i, j int) bool { return out[i].IssuedAt.Before(out[j].IssuedAt) })
	   return out, nil
  }

   func (s *MemStore) ListByUser(userID string, limit int) ([]*AllowLease, error) {
	  if s == nil {
		  return nil, ErrStoreNil
	   }
	   if limit <= 0 {
		   limit = 100
	   }
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	   var out []*AllowLease
	  for _, l := range s.leases {
		   if l.UserID == userID {
			   cp := *l
			   out = append(out, &cp)
		  }
	  }
	   sort.Slice(out, func(i, j int) bool { return out[i].IssuedAt.After(out[j].IssuedAt) })
	   if len(out) > limit {
		   out = out[:limit]
	  }
	   return out, nil
  }

  func (s *MemStore) SaveProof(p *AllowDriftProof) error {
	  if s == nil {
		   return ErrStoreNil
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   if p.ID == "" {
		   p.ID = uuid.NewString()
	  }
	   if p.CreatedAt.IsZero() {
		   p.CreatedAt = time.Now().UTC()
	   }
	   cp := *p
	  s.proofs[p.ID] = &cp
	   return nil
   }

  func (s *MemStore) GetProof(id string) (*AllowDriftProof, error) {
	  if s == nil {
		   return nil, ErrStoreNil
	  }
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	  p, ok := s.proofs[id]
	  if !ok {
		   return nil, ErrLeaseNotFound
	   }
	  cp := *p
	  return &cp, nil
  }

  func (s *MemStore) ListProofs(movementID string, limit int) ([]*AllowDriftProof, error) {
	  if s == nil {
		   return nil, ErrStoreNil
	  }
	  if limit <= 0 {
		   limit = 100
	  }
	  s.mu.RLock()
	  defer s.mu.RUnlock()
	   var out []*AllowDriftProof
	   for _, p := range s.proofs {
		  if movementID == "" || p.MovementID == movementID {
			  cp := *p
			  out = append(out, &cp)
		   }
	   }
	   sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	  if len(out) > limit {
		   out = out[:limit]
	   }
	  return out, nil
  }

  func (s *MemStore) ListProofsByUser(userID string, limit int) ([]*AllowDriftProof, error) {
	  if s == nil {
		   return nil, ErrStoreNil
	  }
	  if limit <= 0 {
		  limit = 100
	  }
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	   var out []*AllowDriftProof
	   for _, p := range s.proofs {
		  if p.UserID == userID {
			   cp := *p
			   out = append(out, &cp)
		   }
	   }
	   sort.Slice(out,
		   func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	  if len(out) > limit {
		  out = out[:limit]
	   }
	  return out, nil
  }

  func (s *MemStore) ListProofsByLease(leaseID string, limit int) ([]*AllowDriftProof, error) {
	  if s == nil {
		   return nil, ErrStoreNil
	  }
	  if limit <= 0 {
		  limit = 100
	  }
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	  var out []*AllowDriftProof
	   for _, p := range s.proofs {
		   if p.LeaseID == leaseID {
			  cp := *p
			   out = append(out, &cp)
		  }
	  }
	  sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	  if len(out) > limit {
		  out = out[:limit]
	   }
	   return out, nil
  }

  func (s *MemStore) SavePolicyBind(b *PolicyBind) error {
	   if s == nil {
		  return ErrStoreNil
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   cp := *b
	  s.binds[b.LeaseID] = &cp
	   return nil
  }

   func (s *MemStore) GetPolicyBind(leaseID string) (*PolicyBind, error) {
	   if s == nil {
		  return nil, ErrStoreNil
	  }
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	  b, ok := s.binds[leaseID]
	   if !ok {
		   return nil, ErrLeaseNotFound
	  }
	   cp := *b
	   return &cp, nil
   }
