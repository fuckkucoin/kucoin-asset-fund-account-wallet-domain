package wdtriplegate

import (
	"sync"
	"time"
)

// MemStore Store 接口的内存实现。
type MemStore struct {
	mu         sync.Mutex
	leases     map[string]*TripleLease     // key = triple_lease_id
	binds      map[string]*PreflightBind   // key = movement_id
	driftCases map[string]*TripleDriftCase // key = drift_case_id
}

// NewMemStore 创建内存存储。

  func NewMemStore() *MemStore {
	   return &MemStore{
		   leases: make(map[string]*TripleLease),
		  binds:      make(map[string]*PreflightBind),
		  driftCases: make(map[string]*TripleDriftCase),
	   }
   }

// ── TripleLease ──────────────────────────────────────────────────────────────

  func (s *MemStore) SaveLease(lease TripleLease) error {
	  if s == nil {
		  return ErrUnavailable
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   // 检查同 movement_id 是否已存在活跃租约
	  for _, l := range s.leases {
		   if l.MovementID == lease.MovementID &&
			  (l.State == LeaseIssued || l.State == LeaseBound) {
			  return ErrBindConflict
		  }
	   }
	  cp := CopyLease(lease)
	   s.leases[lease.TripleLeaseID] = &cp
	  return nil
  }

   func (s *MemStore) GetLease(leaseID string) (*TripleLease, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   l, ok := s.leases[leaseID]
	  if !ok {
		  return nil, ErrLeaseNotFound
	  }
	   cp := CopyLease(*l)
	   return &cp, nil
   }

  func (s *MemStore) GetActiveLeaseByMovement(movementID string) (*TripleLease, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  for _, l := range s.leases {
		   if l.MovementID == movementID &&
			  (l.State == LeaseIssued || l.State == LeaseBound) {
			  cp := CopyLease(*l)
			   return &cp, nil
		  }
	   }
	   return nil, ErrLeaseNotFound
  }

  func (s *MemStore) UpdateLeaseStateCAS(leaseID string, from, to LeaseState, now time.Time, extra *LeaseExtra) (*TripleLease, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	   }
	  if err := LeaseTransition(from, to); err != nil {
		   return nil, ErrInvalidTransition
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  l, ok := s.leases[leaseID]
	   if !ok {
		  return nil, ErrLeaseNotFound
	   }
	  if l.State != from {
		  return nil, ErrInvalidTransition
	   }
	   l.State = to
	   l.UpdatedAt = now
	  switch to {
	   case LeaseBound:
		   l.BoundAt = now
	  case LeaseConsumed:
		  l.ConsumedAt = now
	  case LeaseExpired:
		   l.ExpiredAt = now
	  case LeaseBlocked:
		  l.BlockedAt = now
		   if extra != nil {
			   l.DriftCaseID = extra.DriftCaseID
			  l.BlockReason = extra.BlockReason
		   }
	  }
	   cp := CopyLease(*l)
	  return &cp, nil
   }

   func (s *MemStore) ListExpiredLeases(before time.Time, limit int) ([]*TripleLease, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   var out []*TripleLease
	   for _, l := range s.leases {
		   if (l.State == LeaseIssued || l.State == LeaseBound) && l.ExpiresAt.Before(before) {
			   cp := CopyLease(*l)
			   out = append(out, &cp)
		  }
		  if limit > 0 && len(out) >= limit {
			   break
		   }
	   }
	  return out, nil
   }

  func (s *MemStore) ListLeasesByMovement(movementID string) ([]*TripleLease, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  var out []*TripleLease
	   for _, l := range s.leases {
		   if l.MovementID == movementID {
			   cp := CopyLease(*l)
			   out = append(out, &cp)
		   }
	   }
	  return out, nil
  }

// ── PreflightBind ────────────────────────────────────────────────────────────

   func (s *MemStore) AppendBind(bind PreflightBind) error {
	  if s == nil {
		   return ErrUnavailable
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   // 幂等：同 movement_id 已存在则忽略
	   if _, ok := s.binds[bind.MovementID]; ok {
		  return nil
	   }
	  cp := CopyBind(bind)
	  s.binds[bind.MovementID] = &cp
	   return nil
   }

   func (s *MemStore) GetBindByMovement(movementID string) (*PreflightBind, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   b, ok := s.binds[movementID]
	   if !ok {
		   return nil, ErrBindNotFound
	  }
	   cp := CopyBind(*b)
	   return &cp, nil
  }

// ── TripleDriftCase ──────────────────────────────────────────────────────────

  func (s *MemStore) AppendDriftCase(dc TripleDriftCase) error {
	   if s == nil {
		  return ErrUnavailable
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   // 幂等：同 drift_case_id 已存在则忽略
	  if _, ok := s.driftCases[dc.DriftCaseID]; ok {
		  return nil
	   }
	  cp := CopyDriftCase(dc)
	  s.driftCases[dc.DriftCaseID] = &cp
	   return nil
   }

   func (s *MemStore) GetDriftCase(driftCaseID string) (*TripleDriftCase, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  dc, ok := s.driftCases[driftCaseID]
	   if !ok {
		   return nil, ErrDriftCaseNotFound
	   }
	  cp := CopyDriftCase(*dc)
	  return &cp, nil
  }

  func (s *MemStore) ListOpenDriftCases(limit int) ([]*TripleDriftCase, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   var out []*TripleDriftCase
	   for _, dc := range s.driftCases {
		   if dc.State == DriftOpen {
			   cp := CopyDriftCase(*dc)
			  out = append(out, &cp)
		   }
		   if limit > 0 && len(out) >= limit {
			  break
		  }
	  }
	  return out, nil
  }

  func (s *MemStore) ListDriftCasesByMovement(movementID string) ([]*TripleDriftCase, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  var out []*TripleDriftCase
	   for _, dc := range s.driftCases {
		   if dc.MovementID == movementID {
			  cp := CopyDriftCase(*dc)
			  out = append(out, &cp)
		  }
	  }
	  return out, nil
   }

   func (s *MemStore) UpdateDriftCaseState(driftCaseID string, from, to DriftCaseState, now time.Time, extra *DriftCaseExtra) (*TripleDriftCase, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	   if err := DriftCaseTransition(from, to); err != nil {
		  return nil, ErrInvalidTransition
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   dc, ok := s.driftCases[driftCaseID]
	   if !ok {
		   return nil, ErrDriftCaseNotFound
	  }
	  if dc.State != from {
		   return nil, ErrInvalidTransition
	   }
	  dc.State = to
	  dc.UpdatedAt = now
	   switch to {
	  case DriftProven:
		  dc.ProvenAt = now
		   if extra != nil {
			   dc.ProofChecksum = extra.ProofChecksum
		   }
	  case DriftClosed:
		  dc.ClosedAt = now
		   if extra != nil {
			   dc.ConfirmedBy = extra.ConfirmedBy
			   dc.CoConfirmedBy = extra.CoConfirmedBy
			   dc.CloseNote = extra.CloseNote
		   }
	  }
	  cp := CopyDriftCase(*dc)
	   return &cp, nil
  }
