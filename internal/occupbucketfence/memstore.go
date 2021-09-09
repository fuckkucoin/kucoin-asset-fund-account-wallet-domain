package occupbucketfence

import (
	"context"
	"sync"
	"time"
)

// MemStore 进程内围栏仓。
type MemStore struct {
	mu     sync.Mutex
	fences map[string]*BucketFence
	proofs map[string]*OccupancyBreakProof
	halves map[string]*HalfBucketCase
	lotOp map[string]string
}

// NewMemStore 空仓。
   func NewMemStore() *MemStore {
	  return &MemStore{
		  fences: map[string]*BucketFence{},
		  proofs: map[string]*OccupancyBreakProof{},
		  halves: map[string]*HalfBucketCase{},
		   lotOp:  map[string]string{},
	   }
   }

   func (s *MemStore) SaveFence(_ context.Context, f *BucketFence) error {
	   if s == nil || f == nil {
		  return ErrUnavailable
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  if _, ok := s.fences[f.FenceID]; ok {
		  return nil
	   }
	  cp := *f
	   s.fences[f.FenceID] = &cp
	  return nil
   }

  func (s *MemStore) GetFence(_ context.Context, fenceID string) (*BucketFence, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  f, ok := s.fences[fenceID]
	   if !ok {
		   return nil, ErrFenceNotFound
	  }
	  cp := *f
	   return &cp, nil
  }

   func (s *MemStore) GetActiveFenceByLot(_ context.Context, lotID string) (*BucketFence, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   var found *BucketFence
	   for _, f := range s.fences {
		   if f.LotID != lotID || f.State.Terminal() {
			   continue
		   }
		   cp := *f
		  found = &cp
	  }
	   if found == nil {
		   return nil, ErrFenceNotFound
	   }
	  return found, nil
   }

   func (s *MemStore) UpdateFence(_ context.Context, f *BucketFence) error {
	  if s == nil || f == nil {
		  return ErrUnavailable
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  if _, ok := s.fences[f.FenceID]; !ok {
		   return ErrFenceNotFound
	   }
	   cp := *f
	  s.fences[f.FenceID] = &cp
	   return nil
   }

  func (s *MemStore) CASFenceState(_ context.Context, fenceID string, from, to FenceState, patch *BucketFence) error {
	  if s == nil {
		   return ErrUnavailable
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   f, ok := s.fences[fenceID]
	  if !ok {
		   return ErrFenceNotFound
	   }
	  if f.State != from {
		  return ErrInvalidTransition
	  }
	   f.State = to
	   if patch != nil {
		  if patch.BoundHoldID != "" {
			  f.BoundHoldID = patch.BoundHoldID
		  }
		  if patch.ConsumedBy != "" {
			  f.ConsumedBy = patch.ConsumedBy
		   }
		  if patch.BucketFingerprint != "" {
			  f.BucketFingerprint = patch.BucketFingerprint
		  }
		  if patch.Generation != 0 {
			  f.Generation = patch.Generation
		   }
		  if patch.BoundAt != nil {
			   t := *patch.BoundAt
			  f.BoundAt = &t
		  }
		  if patch.ConsumedAt != nil {
			  t := *patch.ConsumedAt
			  f.ConsumedAt = &t
		   }
	   }
	  return nil
   }

  func (s *MemStore) ListExpiredBound(_ context.Context, now time.Time, limit int) ([]*BucketFence, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	  }
	   if limit <= 0 {
		   limit = 200
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   out := make([]*BucketFence, 0)
	  for _, f := range s.fences {
		   if f.State != FenceBound && f.State != FenceIssued {
			   continue
		   }
		  if !now.After(f.ExpiresAt) {
			   continue
		  }
		  cp := *f
		   out = append(out, &cp)
		  if len(out) >= limit {
			   break
		  }
	  }
	   return out, nil
  }

   func (s *MemStore) ListFencesByLot(_ context.Context, lotID string) ([]*BucketFence, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   out := make([]*BucketFence, 0)
	  for _, f := range s.fences {
		  if f.LotID != lotID {
			   continue
		   }
		  cp := *f
		  out = append(out, &cp)
	  }
	  return out, nil
   }

  func (s *MemStore) SaveProof(_ context.Context, p *OccupancyBreakProof) error {
	   if s == nil || p == nil {
		  return ErrUnavailable
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   if _, ok := s.proofs[p.ProofID]; ok {
		  return ErrAlreadyProven
	   }
	   cp := *p
	  s.proofs[p.ProofID] = &cp
	   return nil
   }

   func (s *MemStore) GetProof(_ context.Context, proofID string) (*OccupancyBreakProof, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   p, ok := s.proofs[proofID]
	  if !ok {
		   return nil, ErrProofNotFound
	  }
	   cp := *p
	   return &cp, nil
   }

   func (s *MemStore) GetOpenProofByLot(_ context.Context, lotID string) (*OccupancyBreakProof, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  for _, p := range s.proofs {
		   if p.LotID == lotID && !p.State.Terminal() {
			   cp := *p
			   return &cp, nil
		  }
	  }
	   return nil, ErrProofNotFound
  }

   func (s *MemStore) ListProofsByLot(_ context.Context, lotID string) ([]*OccupancyBreakProof, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   out := make([]*OccupancyBreakProof,
		   0)
	  for _, p := range s.proofs {
		   if p.LotID != lotID {
			  continue
		  }
		   cp := *p
		  out = append(out, &cp)
	   }
	  return out, nil
  }

   func (s *MemStore) UpdateProof(_ context.Context, p *OccupancyBreakProof) error {
	  if s == nil || p == nil {
		   return ErrUnavailable
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   if _, ok := s.proofs[p.ProofID]; !ok {
		  return ErrProofNotFound
	  }
	  cp := *p
	  s.proofs[p.ProofID] = &cp
	  return nil
   }

   func (s *MemStore) SaveHalf(_ context.Context, h *HalfBucketCase) error {
	   if s == nil || h == nil {
		  return ErrUnavailable
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  cp := *h
	  s.halves[h.CaseID] = &cp
	  return nil
   }

   func (s *MemStore) GetHalf(_ context.Context, caseID string) (*HalfBucketCase, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  h, ok := s.halves[caseID]
	  if !ok {
		  return nil, ErrHalfNotFound
	   }
	  cp := *h
	  return &cp, nil
   }

   func (s *MemStore) GetOpenHalfByLot(_ context.Context, lotID string) (*HalfBucketCase, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   for _, h := range s.halves {
		  if h.LotID == lotID && h.State == HalfOpen {
			  cp := *h
			   return &cp, nil
		  }
	  }
	  return nil, ErrHalfNotFound
   }

   func (s *MemStore) UpdateHalf(_ context.Context, h *HalfBucketCase) error {
	   if s == nil || h == nil {
		  return ErrUnavailable
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  if _, ok := s.halves[h.CaseID]; !ok {
		   return ErrHalfNotFound
	  }
	   cp := *h
	   s.halves[h.CaseID] = &cp
	  return nil
   }

  func (s *MemStore) ListOpenHalves(_ context.Context, limit int) ([]*HalfBucketCase, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	  }
	  if limit <= 0 {
		   limit = 200
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  out := make([]*HalfBucketCase, 0)
	   for _, h := range s.halves {
		  if h.State != HalfOpen {
			   continue
		   }
		  cp := *h
		   out = append(out, &cp)
		  if len(out) >= limit {
			  break
		   }
	  }
	  return out, nil
  }

   func (s *MemStore) TryLockLotOp(_ context.Context, lotID, op string) error {
	  if s == nil {
		   return ErrUnavailable
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  if cur, ok := s.lotOp[lotID]; ok && cur != "" {
		  return ErrReclassMutex
	   }
	  s.lotOp[lotID] = op
	  return nil
   }

   func (s *MemStore) UnlockLotOp(_ context.Context, lotID string) {
	   if s == nil {
		  return
	  }
	  s.mu.Lock()
	   delete(s.lotOp, lotID)
	  s.mu.Unlock()
  }
