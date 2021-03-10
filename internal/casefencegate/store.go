package casefencegate

import (
	"sort"
	"sync"
	"time"
	"github.com/google/uuid"
)

// ──────────────────────────────────────────
// TicketStore 仓储接口
// ──────────────────────────────────────────

type TicketStore interface {
	SaveTicket(t *CaseReleaseTicket) error
	GetTicket(id string) (*CaseReleaseTicket, error)
	GetTicketByCase(caseID string) (*CaseReleaseTicket, error)
	GetTicketByMovement(movementID string) (*CaseReleaseTicket,
		error)
	ListActiveTickets() ([]*CaseReleaseTicket, error)
	ListTicketsByUser(userID string, limit int) ([]*CaseReleaseTicket, error)
	SaveFenceLease(l *CaseFenceLease) error
	GetFenceLease(id string) (*CaseFenceLease, error)
	GetFenceLeaseByCase(caseID string) (*CaseFenceLease, error)
	ListActiveFenceLeases() ([]*CaseFenceLease, error)
	SaveProof(p *CaseDualDecideProof) error
	GetProof(id string) (*CaseDualDecideProof, error)
	ListProofs(caseID string,
		limit int) ([]*CaseDualDecideProof, error)
	ListProofsByMovement(movementID string, limit int) ([]*CaseDualDecideProof, error)
	SaveBareAlert(a *BareFenceAlert) error
	ListBareAlerts(caseID string, limit int) ([]*BareFenceAlert, error)
}

// ──────────────────────────────────────────
// MemTicketStore 内存实现
// ──────────────────────────────────────────

type MemTicketStore struct {
	mu      sync.RWMutex
	tickets map[string]*CaseReleaseTicket
	byCase map[string]string
	byMovement map[string]string
	leases      map[string]*CaseFenceLease
	leaseByCase map[string]string
	proofs map[string]*CaseDualDecideProof
	alerts map[string]*BareFenceAlert
}

  func NewMemTicketStore() *MemTicketStore {
	  return &MemTicketStore{
		  tickets: make(map[string]*CaseReleaseTicket),
		   byCase:  make(map[string]string),
		  byMovement:  make(map[string]string),
		   leases:      make(map[string]*CaseFenceLease),
		   leaseByCase: make(map[string]string),
		  proofs:      make(map[string]*CaseDualDecideProof),
		  alerts: make(map[string]*BareFenceAlert),
	  }
   }

  func (s *MemTicketStore) SaveTicket(t *CaseReleaseTicket) error {
	  if s == nil {
		   return ErrStoreNil
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   if t.ID == "" {
		  t.ID = uuid.NewString()
	   }
	  now := time.Now().UTC()
	  if t.CreatedAt.IsZero() {
		  t.CreatedAt = now
	  }
	   t.UpdatedAt = now
	   cp := *t
	  s.tickets[t.ID] = &cp
	  if t.CaseID != "" {
		   s.byCase[t.CaseID] = t.ID
	  }
	   if t.MovementID != "" {
		  s.byMovement[t.MovementID] = t.ID
	   }
	  return nil
  }

  func (s *MemTicketStore) GetTicket(id string) (*CaseReleaseTicket, error) {
	   if s == nil {
		   return nil, ErrStoreNil
	   }
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	  t, ok := s.tickets[id]
	  if !ok {
		  return nil, ErrTicketNotFound
	   }
	  cp := *t
	   return &cp, nil
   }

   func (s *MemTicketStore) GetTicketByCase(caseID string) (*CaseReleaseTicket, error) {
	  if s == nil {
		   return nil, ErrStoreNil
	   }
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	   id, ok := s.byCase[caseID]
	   if !ok {
		  return nil, ErrTicketNotFound
	   }
	   t, ok := s.tickets[id]
	   if !ok {
		  return nil, ErrTicketNotFound
	   }
	  cp := *t
	   return &cp, nil
   }

  func (s *MemTicketStore) GetTicketByMovement(movementID string) (*CaseReleaseTicket, error) {
	   if s == nil {
		  return nil, ErrStoreNil
	   }
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	  id, ok := s.byMovement[movementID]
	   if !ok {
		  return nil, ErrTicketNotFound
	  }
	  t, ok := s.tickets[id]
	  if !ok {
		  return nil, ErrTicketNotFound
	  }
	  cp := *t
	  return &cp, nil
   }

  func (s *MemTicketStore) ListActiveTickets() ([]*CaseReleaseTicket, error) {
	  if s == nil {
		  return nil, ErrStoreNil
	   }
	  s.mu.RLock()
	  defer s.mu.RUnlock()
	   var out []*CaseReleaseTicket
	   for _, t := range s.tickets {
		   if t.State.Active() {
			  cp := *t
			   out = append(out, &cp)
		  }
	  }
	  sort.Slice(out, func(i, j int) bool { return out[i].IssuedAt.Before(out[j].IssuedAt) })
	  return out, nil
  }

  func (s *MemTicketStore) ListTicketsByUser(userID string, limit int) ([]*CaseReleaseTicket, error) {
	  if s == nil {
		   return nil, ErrStoreNil
	  }
	  if limit <= 0 {
		   limit = 100
	  }
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	  var out []*CaseReleaseTicket
	   for _, t := range s.tickets {
		  if t.UserID == userID {
			   cp := *t
			   out = append(out, &cp)
		   }
	   }
	   sort.Slice(out, func(i, j int) bool { return out[i].IssuedAt.After(out[j].IssuedAt) })
	   if len(out) > limit {
		   out = out[:limit]
	  }
	   return out, nil
   }

  func (s *MemTicketStore) SaveFenceLease(l *CaseFenceLease) error {
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
	   if l.CaseID != "" {
		   s.leaseByCase[l.CaseID] = l.ID
	   }
	  return nil
  }

   func (s *MemTicketStore) GetFenceLease(id string) (*CaseFenceLease, error) {
	  if s == nil {
		  return nil, ErrStoreNil
	  }
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	   l, ok := s.leases[id]
	   if !ok {
		   return nil, ErrTicketNotFound
	  }
	   cp := *l
	  return &cp, nil
   }

  func (s *MemTicketStore) GetFenceLeaseByCase(caseID string) (*CaseFenceLease, error) {
	  if s == nil {
		  return nil, ErrStoreNil
	  }
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	  id, ok := s.leaseByCase[caseID]
	   if !ok {
		  return nil, ErrTicketNotFound
	   }
	   l, ok := s.leases[id]
	  if !ok {
		  return nil, ErrTicketNotFound
	  }
	  cp := *l
	   return &cp, nil
   }

   func (s *MemTicketStore) ListActiveFenceLeases() ([]*CaseFenceLease, error) {
	   if s == nil {
		  return nil, ErrStoreNil
	   }
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	  var out []*CaseFenceLease
	   for _, l := range s.leases {
		   if !l.Released {
			   cp := *l
			  out = append(out, &cp)
		   }
	   }
	   sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	  return out, nil
  }

  func (s *MemTicketStore) SaveProof(p *CaseDualDecideProof) error {
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

   func (s *MemTicketStore) GetProof(id string) (*CaseDualDecideProof, error) {
	  if s == nil {
		  return nil, ErrStoreNil
	   }
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	  p, ok := s.proofs[id]
	   if !ok {
		  return nil, ErrProofNotFound
	  }
	  cp := *p
	   return &cp, nil
  }

   func (s *MemTicketStore) ListProofs(caseID string, limit int) ([]*CaseDualDecideProof, error) {
	  if s == nil {
		   return nil, ErrStoreNil
	  }
	  if limit <= 0 {
		   limit = 100
	  }
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	  var out []*CaseDualDecideProof
	  for _, p := range s.proofs {
		   if caseID == "" || p.CaseID == caseID {
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

   func (s *MemTicketStore) ListProofsByMovement(movementID string, limit int) ([]*CaseDualDecideProof, error) {
	   if s == nil {
		  return nil, ErrStoreNil
	   }
	   if limit <= 0 {
		   limit = 100
	   }
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	  var out []*CaseDualDecideProof
	   for _, p := range s.proofs {
		  if p.MovementID == movementID {
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

  func (s *MemTicketStore) SaveBareAlert(a *BareFenceAlert) error {
	   if s == nil {
		  return ErrStoreNil
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  if a.ID == "" {
		   a.ID = uuid.NewString()
	  }
	  if a.CreatedAt.IsZero() {
		   a.CreatedAt = time.Now().UTC()
	   }
	  cp := *a
	  s.alerts[a.ID] = &cp
	  return nil
  }

   func (s *MemTicketStore) ListBareAlerts(caseID string, limit int) ([]*BareFenceAlert, error) {
	   if s == nil {
		  return nil, ErrStoreNil
	   }
	   if limit <= 0 {
		   limit = 100
	   }
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	  var out []*BareFenceAlert
	   for _, a := range s.alerts {
		  if caseID == "" || a.CaseID == caseID {
			  cp := *a
			  out = append(out, &cp)
		   }
	   }
	  sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	  if len(out) > limit {
		   out = out[:limit]
	  }
	   return out, nil
   }
