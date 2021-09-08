package occupbook

import (
	"context"
	"sync"
)

// MemStore 进程内占用账本。

type MemStore struct {
	mu   sync.RWMutex
	lots map[string]*Lot
	holds map[string]*Hold
	holdIdem map[string]string
	rels map[string]*Release
	relIdem map[string]string
	recs    map[string]*Reclass
	recIdem map[string]string
	audits  []*AuditEvent
}

// NewMemStore 构造空仓储。

   func NewMemStore() *MemStore {
	  return &MemStore{
		  lots: map[string]*Lot{},
		   holds:    map[string]*Hold{},
		   holdIdem: map[string]string{},
		  rels:    map[string]*Release{},
		   relIdem: map[string]string{},
		  recs: map[string]*Reclass{},
		  recIdem: map[string]string{},
	   }
  }

  func (s *MemStore) SaveLot(_ context.Context, lot *Lot) error {
	  if s == nil || lot == nil {
		  return ErrUnavailable
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   if _, ok := s.lots[lot.LotID]; ok {
		   return ErrLotExists
	   }
	   cp := *lot
	   cp.Buckets = lot.Buckets.Clone()
	   s.lots[lot.LotID] = &cp
	  return nil
   }

   func (s *MemStore) GetLot(_ context.Context, lotID string) (*Lot, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	   lot, ok := s.lots[lotID]
	   if !ok {
		  return nil, ErrLotNotFound
	  }
	  cp := *lot
	   cp.Buckets = lot.Buckets.Clone()
	   return &cp, nil
  }

   func (s *MemStore) UpdateLot(_ context.Context, lot *Lot) error {
	   if s == nil || lot == nil {
		   return ErrUnavailable
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   if _, ok := s.lots[lot.LotID]; !ok {
		   return ErrLotNotFound
	   }
	   cp := *lot
	   cp.Buckets = lot.Buckets.Clone()
	   s.lots[lot.LotID] = &cp
	  return nil
  }

  func (s *MemStore) ListOpenLots(_ context.Context, limit int) ([]*Lot, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	  }
	  if limit <= 0 {
		   limit = 200
	   }
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	  out := make([]*Lot, 0)
	  for _, lot := range s.lots {
		  if lot.State.Terminal() {
			   continue
		  }
		  cp := *lot
		  cp.Buckets = lot.Buckets.Clone()
		   out = append(out, &cp)
		  if len(out) >= limit {
			   break
		   }
	   }
	   return out, nil
  }

  func (s *MemStore) SaveHold(_ context.Context, h *Hold) error {
	   if s == nil || h == nil {
		   return ErrUnavailable
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   if _, ok := s.holds[h.HoldID]; ok {
		  return ErrIdemConflict
	   }
	   key := h.LotID + "|" + h.IdemKey
	  if exist, ok := s.holdIdem[key]; ok && exist != h.HoldID {
		  return ErrIdemConflict
	   }
	  cp := *h
	  s.holds[h.HoldID] = &cp
	  if h.IdemKey != "" {
		   s.holdIdem[key] = h.HoldID
	  }
	   return nil
  }

  func (s *MemStore) GetHold(_ context.Context, holdID string) (*Hold, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	  }
	  s.mu.RLock()
	  defer s.mu.RUnlock()
	  h, ok := s.holds[holdID]
	   if !ok {
		   return nil, ErrHoldNotFound
	   }
	   cp := *h
	   return &cp, nil
  }

   func (s *MemStore) GetHoldByIdem(_ context.Context, lotID, idemKey string) (*Hold, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	  s.mu.RLock()
	  defer s.mu.RUnlock()
	   id, ok := s.holdIdem[lotID+"|"+idemKey]
	  if !ok {
		  return nil, ErrHoldNotFound
	  }
	   h := s.holds[id]
	   cp := *h
	   return &cp, nil
  }

   func (s *MemStore) UpdateHold(_ context.Context, h *Hold) error {
	  if s == nil || h == nil {
		  return ErrUnavailable
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   if _, ok := s.holds[h.HoldID]; !ok {
		  return ErrHoldNotFound
	  }
	  cp := *h
	  s.holds[h.HoldID] = &cp
	  return nil
  }

   func (s *MemStore) ListHoldsByLot(_ context.Context, lotID string) ([]*Hold, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	   out := make([]*Hold, 0)
	   for _, h := range s.holds {
		  if h.LotID != lotID {
			  continue
		  }
		   cp := *h
		  out = append(out, &cp)
	   }
	  return out, nil
   }

  func (s *MemStore) SaveRelease(_ context.Context, r *Release) error {
	  if s == nil || r == nil {
		   return ErrUnavailable
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  if _, ok := s.rels[r.ReleaseID]; ok {
		  return nil
	  }
	   key := r.LotID + "|" + r.IdemKey
	  if exist, ok := s.relIdem[key]; ok && exist != r.ReleaseID {
		  return ErrIdemConflict
	   }
	   cp := *r
	   s.rels[r.ReleaseID] = &cp
	  if r.IdemKey != "" {
		   s.relIdem[key] = r.ReleaseID
	   }
	  return nil
   }

  func (s *MemStore) GetRelease(_ context.Context, releaseID string) (*Release, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	  }
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	   r, ok := s.rels[releaseID]
	   if !ok {
		   return nil, ErrReleaseNotFound
	   }
	   cp := *r
	   return &cp, nil
   }

   func (s *MemStore) GetReleaseByIdem(_ context.Context, lotID, idemKey string) (*Release, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	  }
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	  id, ok := s.relIdem[lotID+"|"+idemKey]
	  if !ok {
		  return nil, ErrReleaseNotFound
	   }
	   r := s.rels[id]
	   cp := *r
	  return &cp, nil
   }

   func (s *MemStore) SaveReclass(_ context.Context, r *Reclass) error {
	  if s == nil || r == nil {
		   return ErrUnavailable
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  if _, ok := s.recs[r.ReclassID]; ok {
		   return nil
	  }
	  key := r.LotID + "|" + r.IdemKey
	   if exist, ok := s.recIdem[key]; ok && exist != r.ReclassID {
		   return ErrIdemConflict
	  }
	  cp := *r
	   s.recs[r.ReclassID] = &cp
	   if r.IdemKey != "" {
		   s.recIdem[key] = r.ReclassID
	   }
	  return nil
   }

   func (s *MemStore) GetReclass(_ context.Context, reclassID string) (*Reclass, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	   r, ok := s.recs[reclassID]
	   if !ok {
		   return nil, ErrReclassNotFound
	   }
	  cp := *r
	   return &cp, nil
   }

   func (s *MemStore) GetReclassByIdem(_ context.Context, lotID, idemKey string) (*Reclass, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	   }
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	   id, ok := s.recIdem[lotID+"|"+idemKey]
	   if !ok {
		  return nil, ErrReclassNotFound
	  }
	   r := s.recs[id]
	   cp := *r
	  return &cp, nil
  }

   func (s *MemStore) AppendAudit(_ context.Context, ev *AuditEvent) error {
	  if s == nil || ev == nil {
		   return ErrUnavailable
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   cp := *ev
	  s.audits = append(s.audits, &cp)
	  return nil
   }

   func (s *MemStore) ListAudit(_ context.Context, lotID string, limit int) ([]*AuditEvent, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	  }
	   if limit <= 0 {
		   limit = 100
	   }
	  s.mu.RLock()
	  defer s.mu.RUnlock()
	  out := make([]*AuditEvent, 0)
	  for i := len(s.audits) - 1; i >= 0 && len(out) < limit; i-- {
		  if lotID != "" && s.audits[i].LotID != lotID {
			  continue
		  }
		   cp := *s.audits[i]
		  out = append(out, &cp)
	   }
	   return out, nil
  }
