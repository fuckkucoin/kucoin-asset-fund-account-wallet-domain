package memoeffectgate

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

type MemStore struct {
	mu      sync.Mutex
	effects map[string]MemoEffect
	fences map[string]TicketCreditFence
	proofs map[string]MemoHalfCreditProof
	audits []EffectAudit
}

  func NewMemStore() *MemStore {
	   return &MemStore{
		  effects: map[string]MemoEffect{},
		   fences: map[string]TicketCreditFence{},
		  proofs: map[string]MemoHalfCreditProof{},
	  }
  }

  func (m *MemStore) SaveEffect(_ context.Context, e MemoEffect) (MemoEffect, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   if strings.TrimSpace(e.ID) == "" {
		   return MemoEffect{}, ErrEffectNotFound
	  }
	   m.effects[e.ID] = e
	   return e, nil
  }

  func (m *MemStore) GetEffect(_ context.Context, id string) (MemoEffect, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  e, ok := m.effects[strings.TrimSpace(id)]
	   if !ok {
		   return MemoEffect{}, ErrEffectNotFound
	   }
	  return e, nil
  }

  func (m *MemStore) GetEffectByTicket(_ context.Context, ticketID string) (MemoEffect, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   ticketID = strings.TrimSpace(ticketID)
	  var newest MemoEffect
	  found := false
	   for _, e := range m.effects {
		  if e.TicketID == ticketID {
			  if !found || e.UpdatedAt.After(newest.UpdatedAt) {
				  newest = e
				   found = true
			   }
		   }
	   }
	   if !found {
		   return MemoEffect{}, ErrEffectNotFound
	   }
	  return newest, nil
   }

  func (m *MemStore) GetEffectByIdem(_ context.Context, idem string) (MemoEffect, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  idem = strings.TrimSpace(idem)
	  if idem == "" {
		   return MemoEffect{}, ErrEffectNotFound
	  }
	   for _, e := range m.effects {
		   if e.Idem == idem {
			  return e, nil
		  }
	  }
	   return MemoEffect{},
		   ErrEffectNotFound
  }

   func (m *MemStore) GetEffectByOutput(_ context.Context, outputKey string) (MemoEffect, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   for _, e := range m.effects {
		   if e.OutputKey == outputKey {
			   return e, nil
		   }
	   }
	   return MemoEffect{}, ErrEffectNotFound
  }

   func (m *MemStore) ListEffectsByState(_ context.Context, state EffectState, limit int) ([]MemoEffect, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   out := make([]MemoEffect, 0)
	  for _, e := range m.effects {
		   if e.State == state {
			  out = append(out, e)
		   }
	   }
	   sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	  if limit > 0 && len(out) > limit {
		  out = out[:limit]
	  }
	  return out, nil
   }

  func (m *MemStore) ListExpiredBound(_ context.Context, before time.Time, limit int) ([]MemoEffect, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  out := make([]MemoEffect, 0)
	  for _, e := range m.effects {
		  if e.State == EffectBound && !e.ExpiresAt.IsZero() && !e.ExpiresAt.After(before) {
			   out = append(out, e)
		  }
	  }
	  sort.Slice(out, func(i, j int) bool { return out[i].ExpiresAt.Before(out[j].ExpiresAt) })
	   if limit > 0 && len(out) > limit {
		   out = out[:limit]
	  }
	   return out, nil
  }

   func (m *MemStore) ListEffects(_ context.Context, limit int) ([]MemoEffect, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   out := make([]MemoEffect, 0, len(m.effects))
	  for _, e := range m.effects {
		   out = append(out, e)
	   }
	  sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	   if limit > 0 && len(out) > limit {
		   out = out[:limit]
	  }
	   return out, nil
  }

   func (m *MemStore) CountEffects(_ context.Context, state EffectState) (int, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   n := 0
	   for _, e := range m.effects {
		  if state == "" || e.State == state {
			  n++
		   }
	  }
	  return n, nil
   }

  func (m *MemStore) SaveFence(_ context.Context, f TicketCreditFence) (TicketCreditFence, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   if strings.TrimSpace(f.ID) == "" {
		   return TicketCreditFence{}, ErrFenceUnavailable
	  }
	   m.fences[f.ID] = f
	   return f, nil
   }

  func (m *MemStore) GetFence(_ context.Context, id string) (TicketCreditFence, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   f, ok := m.fences[strings.TrimSpace(id)]
	   if !ok {
		   return TicketCreditFence{}, ErrFenceUnavailable
	   }
	  return f, nil
   }

   func (m *MemStore) GetFenceByTicket(_ context.Context, ticketID string) (TicketCreditFence, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  var newest TicketCreditFence
	  found := false
	   for _, f := range m.fences {
		   if f.TicketID == ticketID {
			   if !found || f.UpdatedAt.After(newest.UpdatedAt) {
				   newest = f
				   found = true
			   }
		   }
	  }
	  if !found {
		   return TicketCreditFence{}, ErrFenceUnavailable
	   }
	   return newest, nil
  }

   func (m *MemStore) ListHalfCutFences(_ context.Context, limit int) ([]TicketCreditFence, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  out := make([]TicketCreditFence, 0)
	  for _, f := range m.fences {
		   if f.HalfCut && f.Phase != FenceSealed {
			  out = append(out, f)
		   }
	   }
	  sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.Before(out[j].UpdatedAt) })
	  if limit > 0 && len(out) > limit {
		   out = out[:limit]
	   }
	  return out, nil
  }

  func (m *MemStore) ListFences(_ context.Context, limit int) ([]TicketCreditFence, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  out := make([]TicketCreditFence, 0, len(m.fences))
	   for _, f := range m.fences {
		  out = append(out, f)
	  }
	   if limit > 0 && len(out) > limit {
		   out = out[:limit]
	   }
	   return out, nil
   }

  func (m *MemStore) SaveProof(_ context.Context, p MemoHalfCreditProof) (MemoHalfCreditProof, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if strings.TrimSpace(p.ID) == "" {
		   return MemoHalfCreditProof{}, ErrProofNotFound
	  }
	  m.proofs[p.ID] = p
	  return p, nil
   }

  func (m *MemStore) GetProof(_ context.Context, id string) (MemoHalfCreditProof, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  p, ok := m.proofs[strings.TrimSpace(id)]
	  if !ok {
		  return MemoHalfCreditProof{}, ErrProofNotFound
	  }
	   return p, nil
  }

  func (m *MemStore) ListProofsByTicket(_ context.Context, ticketID string, limit int) ([]MemoHalfCreditProof, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  out := make([]MemoHalfCreditProof, 0)
	   for _, p := range m.proofs {
		   if p.TicketID == ticketID {
			  out = append(out, p)
		  }
	  }
	  sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	  if limit > 0 && len(out) > limit {
		   out = out[:limit]
	   }
	   return out, nil
   }

   func (m *MemStore) ListProofsByState(_ context.Context, state HalfState, limit int) ([]MemoHalfCreditProof, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   out := make([]MemoHalfCreditProof,
		   0)
	  for _, p := range m.proofs {
		   if p.State == state {
			   out = append(out, p)
		   }
	   }
	   if limit > 0 && len(out) > limit {
		  out = out[:limit]
	   }
	   return out, nil
   }

  func (m *MemStore) ListProofs(_ context.Context, kind ProofKind, limit int) ([]MemoHalfCreditProof, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   out := make([]MemoHalfCreditProof, 0)
	   for _, p := range m.proofs {
		  if kind == "" || p.Kind == kind {
			  out = append(out, p)
		  }
	   }
	  sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	  if limit > 0 && len(out) > limit {
		   out = out[:limit]
	  }
	   return out, nil
  }

   func (m *MemStore) CountProofs(_ context.Context) (int, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  return len(m.proofs), nil
  }

   func (m *MemStore) SaveAudit(_ context.Context, a EffectAudit) (EffectAudit, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   m.audits = append(m.audits, a)
	  return a, nil
   }

   func (m *MemStore) ListAudit(_ context.Context, effectID string, limit int) ([]EffectAudit, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  out := make([]EffectAudit, 0)
	   for _, a := range m.audits {
		  if effectID == "" || a.EffectID == effectID {
			  out = append(out, a)
		   }
	  }
	  if limit > 0 && len(out) > limit {
		   out = out[:limit]
	   }
	  return out, nil
  }

   func (m *MemStore) CountAudits(_ context.Context) (int, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   return len(m.audits), nil
   }
