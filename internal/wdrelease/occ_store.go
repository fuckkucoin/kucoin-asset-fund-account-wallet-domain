package wdrelease

import (
	"context"
	"sort"
	"sync"
)

// Store 票据与 Grant 仓。
type OccStore interface {
	InsertTicket(ctx context.Context,
		t *OccTicket) error
	GetTicket(ctx context.Context, id string) (*OccTicket, error)
	UpdateTicket(ctx context.Context, t *OccTicket) error
	ListTickets(ctx context.Context, limit int) ([]*OccTicket, error)
	ListTicketsByState(ctx context.Context,
		st OccTicketState, limit int) ([]*OccTicket, error)
	InsertGrant(ctx context.Context, g *OccGrant) error
	GetGrant(ctx context.Context, id string) (*OccGrant, error)
	GetGrantByIdem(ctx context.Context, idem string) (*OccGrant, error)
	UpdateGrant(ctx context.Context,
		g *OccGrant) error
	ListGrantsByTicket(ctx context.Context, ticketID string, limit int) ([]*OccGrant, error)
	ListGrantsByState(ctx context.Context, st OccGrantState, limit int) ([]*OccGrant, error)
	InsertAudit(ctx context.Context, a *OccAudit) error
	ListAuditByTicket(ctx context.Context,
		ticketID string, limit int) ([]*OccAudit, error)
}

// OccMemStore 线程安全内存实现。
type OccMemStore struct {
	mu      sync.RWMutex
	tickets map[string]*OccTicket
	grants  map[string]*OccGrant
	grantIdem map[string]string
	audits    []OccAudit
}

// NewOccMemStore 空仓。

   func NewOccMemStore() *OccMemStore {
	   return &OccMemStore{
		  tickets: map[string]*OccTicket{},
		  grants: map[string]*OccGrant{},
		   grantIdem: map[string]string{},
	   }
  }

   func cloneOccTicket(t *OccTicket) *OccTicket {
	  if t == nil {
		  return nil
	   }
	  c := *t
	  return &c
  }

   func cloneOccGrant(g *OccGrant) *OccGrant {
	   if g == nil {
		   return nil
	   }
	   c := *g
	  return &c
  }

  func (s *OccMemStore) InsertTicket(_ context.Context, t *OccTicket) error {
	   if s == nil {
		  return ErrUnavailable
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  if _, ok := s.tickets[t.ID]; ok {
		   return ErrDuplicate
	   }
	   s.tickets[t.ID] = cloneOccTicket(t)
	   return nil
  }

  func (s *OccMemStore) GetTicket(_ context.Context, id string) (*OccTicket, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	  }
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	   t, ok := s.tickets[id]
	   if !ok {
		  return nil, ErrNotFound
	   }
	  return cloneOccTicket(t), nil
   }

   func (s *OccMemStore) UpdateTicket(_ context.Context, t *OccTicket) error {
	   if s == nil {
		   return ErrUnavailable
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  if _, ok := s.tickets[t.ID]; !ok {
		   return ErrNotFound
	   }
	  s.tickets[t.ID] = cloneOccTicket(t)
	  return nil
  }

   func (s *OccMemStore) ListTickets(_ context.Context, limit int) ([]*OccTicket, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	  s.mu.RLock()
	  defer s.mu.RUnlock()
	   out := make([]*OccTicket, 0, len(s.tickets))
	   for _, t := range s.tickets {
		   out = append(out, cloneOccTicket(t))
	  }
	  sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	  if limit > 0 && len(out) > limit {
		  out = out[:limit]
	   }
	  return out, nil
   }

   func (s *OccMemStore) ListTicketsByState(_ context.Context, st OccTicketState, limit int) ([]*OccTicket, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	  out := make([]*OccTicket, 0)
	  for _, t := range s.tickets {
		   if t.State == st {
			  out = append(out, cloneOccTicket(t))
		  }
	   }
	   sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	   if limit > 0 && len(out) > limit {
		   out = out[:limit]
	  }
	   return out, nil
   }

   func (s *OccMemStore) InsertGrant(_ context.Context, g *OccGrant) error {
	   if s == nil {
		  return ErrUnavailable
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  if _, ok := s.grants[g.ID]; ok {
		   return ErrDuplicate
	  }
	   if g.GrantIdem != "" {
		   if _, ok := s.grantIdem[g.GrantIdem]; ok {
			   return ErrDuplicate
		   }
		   s.grantIdem[g.GrantIdem] = g.ID
	  }
	  s.grants[g.ID] = cloneOccGrant(g)
	   return nil
   }

  func (s *OccMemStore) GetGrant(_ context.Context, id string) (*OccGrant, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	   g, ok := s.grants[id]
	   if !ok {
		  return nil, ErrNotFound
	   }
	  return cloneOccGrant(g), nil
  }

   func (s *OccMemStore) GetGrantByIdem(_ context.Context, idem string) (*OccGrant, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	   id, ok := s.grantIdem[idem]
	   if !ok {
		  return nil, ErrNotFound
	   }
	  return cloneOccGrant(s.grants[id]), nil
  }

  func (s *OccMemStore) UpdateGrant(_ context.Context, g *OccGrant) error {
	  if s == nil {
		  return ErrUnavailable
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   if _, ok := s.grants[g.ID]; !ok {
		  return ErrNotFound
	  }
	  s.grants[g.ID] = cloneOccGrant(g)
	  return nil
   }

   func (s *OccMemStore) ListGrantsByTicket(_ context.Context, ticketID string, limit int) ([]*OccGrant, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	  }
	  s.mu.RLock()
	  defer s.mu.RUnlock()
	  out := make([]*OccGrant, 0)
	  for _, g := range s.grants {
		   if g.TicketID == ticketID {
			   out = append(out, cloneOccGrant(g))
		   }
	   }
	   sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	  if limit > 0 && len(out) > limit {
		   out = out[:limit]
	  }
	  return out, nil
  }

   func (s *OccMemStore) ListGrantsByState(_ context.Context, st OccGrantState, limit int) ([]*OccGrant, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	  out := make([]*OccGrant, 0)
	   for _, g := range s.grants {
		  if g.State == st {
			   out = append(out, cloneOccGrant(g))
		   }
	  }
	   sort.Slice(out,
		   func(i, j int) bool { return out[i].ID < out[j].ID })
	  if limit > 0 && len(out) > limit {
		  out = out[:limit]
	  }
	   return out, nil
   }

  func (s *OccMemStore) InsertAudit(_ context.Context, a *OccAudit) error {
	   if s == nil {
		  return ErrUnavailable
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  s.audits = append(s.audits, *a)
	  return nil
  }

   func (s *OccMemStore) ListAuditByTicket(_ context.Context, ticketID string, limit int) ([]*OccAudit, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	  }
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	   out := make([]*OccAudit, 0)
	  for i := range s.audits {
		   if s.audits[i].TicketID == ticketID {
			  c := s.audits[i]
			   out = append(out, &c)
		  }
	  }
	  if limit > 0 && len(out) > limit {
		   out = out[:limit]
	  }
	  return out, nil
  }
