package wdpartial

import (
	"context"
	"sync"
	"time"
)

// ──────────────────────────────────────────────
// MemHoldStore
// ──────────────────────────────────────────────
// MemHoldStore Hold 内存存储。

type MemHoldStore struct {
	mu sync.RWMutex
	rows map[string]*HoldRecord
}

// NewMemHoldStore 构造内存 HoldStore。
   func NewMemHoldStore() *MemHoldStore {
	   return &MemHoldStore{rows: map[string]*HoldRecord{}}
   }

  func (s *MemHoldStore) Create(_ context.Context, h *HoldRecord) error {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   if _, ok := s.rows[h.ID]; ok {
		   return nil // 幂等
	   }
	   cp := *h
	  s.rows[h.ID] = &cp
	  return nil
  }

   func (s *MemHoldStore) Get(_ context.Context, id string) (*HoldRecord, error) {
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	  r, ok := s.rows[id]
	  if !ok {
		  return nil, ErrHoldNotFound
	  }
	  cp := *r
	  return &cp, nil
  }

  func (s *MemHoldStore) GetByPlan(_ context.Context, planID string) ([]*HoldRecord, error) {
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	   var out []*HoldRecord
	   for _, r := range s.rows {
		  if r.PlanID == planID {
			   cp := *r
			   out = append(out, &cp)
		   }
	  }
	  return out, nil
  }

  func (s *MemHoldStore) Update(_ context.Context, h *HoldRecord) error {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  if _, ok := s.rows[h.ID]; !ok {
		   return ErrHoldNotFound
	   }
	   cp := *h
	   s.rows[h.ID] = &cp
	   return nil
  }

   func (s *MemHoldStore) ListOpenByPlan(_ context.Context, planID string) ([]*HoldRecord, error) {
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	   var out []*HoldRecord
	   for _, r := range s.rows {
		   if r.PlanID == planID && r.Status == HoldStatusOpen {
			  cp := *r
			   out = append(out, &cp)
		   }
	  }
	   return out, nil
  }

// ──────────────────────────────────────────────
// MemFillStore
// ──────────────────────────────────────────────
// MemFillStore Fill 内存存储。

type MemFillStore struct {
	mu sync.RWMutex
	rows map[string]*FillRecord
}

// NewMemFillStore 构造内存 FillStore。
  func NewMemFillStore() *MemFillStore {
	  return &MemFillStore{rows: map[string]*FillRecord{}}
   }

  func (s *MemFillStore) Create(_ context.Context, f *FillRecord) error {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  if _, ok := s.rows[f.ID]; ok {
		   return nil
	   }
	  cp := *f
	   s.rows[f.ID] = &cp
	   return nil
  }

   func (s *MemFillStore) Get(_ context.Context, id string) (*FillRecord, error) {
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	  r, ok := s.rows[id]
	   if !ok {
		  return nil, ErrFillNotFound
	   }
	   cp := *r
	  return &cp, nil
  }

  func (s *MemFillStore) GetByPlan(_ context.Context, planID string) ([]*FillRecord, error) {
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	   var out []*FillRecord
	   for _, r := range s.rows {
		   if r.PlanID == planID {
			   cp := *r
			   out = append(out, &cp)
		   }
	   }
	   return out, nil
   }

  func (s *MemFillStore) Update(_ context.Context, f *FillRecord) error {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   if _, ok := s.rows[f.ID]; !ok {
		  return ErrFillNotFound
	  }
	  cp := *f
	  s.rows[f.ID] = &cp
	   return nil
   }

  func (s *MemFillStore) ListConfirmedByPlan(_ context.Context, planID string) ([]*FillRecord, error) {
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	  var out []*FillRecord
	   for _, r := range s.rows {
		  if r.PlanID == planID && r.Status == FillStatusConfirmed {
			   cp := *r
			  out = append(out, &cp)
		  }
	   }
	  return out, nil
  }

// ──────────────────────────────────────────────
// MemLeaseStore
// ──────────────────────────────────────────────
// MemLeaseStore RemReleaseLease 内存存储。
type MemLeaseStore struct {
	mu   sync.Mutex
	rows map[string]*RemReleaseLease
}

// NewMemLeaseStore 构造内存 LeaseStore。
  func NewMemLeaseStore() *MemLeaseStore {
	   return &MemLeaseStore{rows: map[string]*RemReleaseLease{}}
   }

  func (s *MemLeaseStore) Create(_ context.Context, l *RemReleaseLease) error {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  if _, ok := s.rows[l.ID]; ok {
		   return nil
	  }
	  cp := *l
	   s.rows[l.ID] = &cp
	  return nil
   }

  func (s *MemLeaseStore) Get(_ context.Context, id string) (*RemReleaseLease, error) {
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  r, ok := s.rows[id]
	   if !ok {
		   return nil, ErrLeaseNotFound
	  }
	   cp := *r
	  return &cp, nil
  }

  func (s *MemLeaseStore) GetOpenByPlan(_ context.Context, planID string) (*RemReleaseLease, error) {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   for _, r := range s.rows {
		  if r.PlanID == planID && r.Status == LeaseStatusOpen {
			   cp := *r
			  return &cp, nil
		  }
	   }
	   return nil, ErrLeaseNotFound
   }

  func (s *MemLeaseStore) Update(_ context.Context, l *RemReleaseLease) error {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  if _, ok := s.rows[l.ID]; !ok {
		   return ErrLeaseNotFound
	   }
	  cp := *l
	  s.rows[l.ID] = &cp
	  return nil
  }

  func (s *MemLeaseStore) ListExpired(_ context.Context, limit int) ([]*RemReleaseLease, error) {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   now := time.Now().UTC()
	   var out []*RemReleaseLease
	  for _, r := range s.rows {
		  if r.Status.IsTerminal() {
			  continue
		  }
		  if !r.ExpiresAt.IsZero() && now.After(r.ExpiresAt) {
			   cp := *r
			   out = append(out, &cp)
			   if len(out) >= limit {
				  break
			   }
		  }
	  }
	  return out, nil
   }

// ──────────────────────────────────────────────
// MemConserveCaseStore
// ──────────────────────────────────────────────
// MemConserveCaseStore ConserveCase 内存存储。
type MemConserveCaseStore struct {
	mu   sync.RWMutex
	rows map[string]*ConserveCase
}

// NewMemConserveCaseStore 构造内存 ConserveCaseStore。

  func NewMemConserveCaseStore() *MemConserveCaseStore {
	  return &MemConserveCaseStore{rows: map[string]*ConserveCase{}}
  }

   func (s *MemConserveCaseStore) Create(_ context.Context, c *ConserveCase) error {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  if _, ok := s.rows[c.ID]; ok {
		   return nil
	   }
	   cp := *c
	   s.rows[c.ID] = &cp
	  return nil
   }

   func (s *MemConserveCaseStore) Get(_ context.Context, id string) (*ConserveCase, error) {
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	   r, ok := s.rows[id]
	   if !ok {
		   return nil, ErrConserveCaseNotFound
	   }
	  cp := *r
	   return &cp, nil
  }

   func (s *MemConserveCaseStore) GetOpenByPlan(_ context.Context, planID string) (*ConserveCase, error) {
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	   for _, r := range s.rows {
		  if r.PlanID == planID && r.Status == ConserveCaseOpen {
			   cp := *r
			  return &cp, nil
		  }
	   }
	  return nil,
		  ErrConserveCaseNotFound
   }

   func (s *MemConserveCaseStore) Update(_ context.Context, c *ConserveCase) error {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  existing, ok := s.rows[c.ID]
	   if !ok {
		  return ErrConserveCaseNotFound
	   }
	   cp := *c
	   // conserve_snapshot 落库后禁止修改（内存实现同等限制）。
	  cp.ConserveSnapshot = existing.ConserveSnapshot
	   s.rows[c.ID] = &cp
	  return nil
  }

  func (s *MemConserveCaseStore) ListOpen(_ context.Context, limit int) ([]*ConserveCase, error) {
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	  var out []*ConserveCase
	   for _, r := range s.rows {
		  if r.Status == ConserveCaseOpen {
			  cp := *r
			  out = append(out, &cp)
			  if len(out) >= limit {
				   break
			   }
		  }
	   }
	   return out, nil
   }

// ──────────────────────────────────────────────
// MemGuardGenStore
// ──────────────────────────────────────────────
// MemGuardGenStore BroadcastGuardGen 内存存储。
type MemGuardGenStore struct {
	mu sync.RWMutex
	rows map[string]*BroadcastGuardGen
}

// NewMemGuardGenStore 构造内存 GuardGenStore。

  func NewMemGuardGenStore() *MemGuardGenStore {
	   return &MemGuardGenStore{rows: map[string]*BroadcastGuardGen{}}
   }

   func (s *MemGuardGenStore) Create(_ context.Context, g *BroadcastGuardGen) error {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  if _, ok := s.rows[g.ID]; ok {
		  return nil
	  }
	   cp := *g
	  s.rows[g.ID] = &cp
	   return nil
  }

   func (s *MemGuardGenStore) Get(_ context.Context, id string) (*BroadcastGuardGen, error) {
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	  r, ok := s.rows[id]
	   if !ok {
		  return nil, ErrGuardGenNotFound
	  }
	  cp := *r
	   return &cp, nil
  }

  func (s *MemGuardGenStore) GetActiveByPlan(_ context.Context, planID string) (*BroadcastGuardGen, error) {
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	   for _, r := range s.rows {
		  if r.PlanID == planID && r.Status == GuardGenActive {
			  cp := *r
			   return &cp, nil
		   }
	  }
	  return nil, ErrGuardGenNotFound
   }

   func (s *MemGuardGenStore) Update(_ context.Context, g *BroadcastGuardGen) error {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   if _, ok := s.rows[g.ID]; !ok {
		   return ErrGuardGenNotFound
	  }
	  cp := *g
	   s.rows[g.ID] = &cp
	   return nil
   }

  func (s *MemGuardGenStore) ListActive(_ context.Context, limit int) ([]*BroadcastGuardGen, error) {
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	   var out []*BroadcastGuardGen
	  for _, r := range s.rows {
		   if r.Status == GuardGenActive {
			  cp := *r
			   out = append(out, &cp)
			  if len(out) >= limit {
				   break
			  }
		  }
	   }
	   return out, nil
   }
