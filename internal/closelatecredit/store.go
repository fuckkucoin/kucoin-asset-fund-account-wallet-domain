package closelatecredit

import (
	"context"
	"sort"
	"sync"
)

// Store 覆盖世代、迟贷证明、争议仓、审计、迟贷扫描队列。

type Store interface {
	InsertEpoch(ctx context.Context,
		e *CloseEpoch) error
	GetEpoch(ctx context.Context, id string) (*CloseEpoch, error)
	GetEpochByUserBook(ctx context.Context, userID, book string) (*CloseEpoch, error)
	UpdateEpoch(ctx context.Context, e *CloseEpoch) error
	ListEpochsByState(ctx context.Context, st EpochState, limit int) ([]*CloseEpoch, error)
	ListEpochsByUser(ctx context.Context,
		userID string, limit int) ([]*CloseEpoch, error)
	GetCursor(ctx context.Context, userID, book string) (*EpochCursor, error)
	UpsertCursor(ctx context.Context, c *EpochCursor) error
	InsertProof(ctx context.Context,
		p *CloseLateCreditProof) error
	GetProofByID(ctx context.Context, id string) (*CloseLateCreditProof, error)
	GetProofByIdem(ctx context.Context,
		idem string) (*CloseLateCreditProof, error)
	UpdateProof(ctx context.Context, p *CloseLateCreditProof) error
	ListProofsByState(ctx context.Context, st LateState, limit int) ([]*CloseLateCreditProof, error)
	ListProofsByUser(ctx context.Context, userID string, limit int) ([]*CloseLateCreditProof, error)
	ListProofsBySource(ctx context.Context, src CreditSource, limit int) ([]*CloseLateCreditProof, error)
	ListOpenProofs(ctx context.Context, limit int) ([]*CloseLateCreditProof, error)
	InsertHold(ctx context.Context, h *DisputeHoldCase) error
	GetHoldByID(ctx context.Context, id string) (*DisputeHoldCase, error)
	GetHoldByIdem(ctx context.Context, idem string) (*DisputeHoldCase, error)
	GetHoldByProof(ctx context.Context, proofID string) (*DisputeHoldCase, error)
	UpdateHold(ctx context.Context, h *DisputeHoldCase) error
	ListHoldsByState(ctx context.Context,
		st HoldState, limit int) ([]*DisputeHoldCase, error)
	ListHoldsByUser(ctx context.Context, userID string, limit int) ([]*DisputeHoldCase, error)
	InsertAudit(ctx context.Context,
		a *EpochAudit) error
	ListAuditByUser(ctx context.Context, userID string, limit int) ([]*EpochAudit, error)
	InsertPending(ctx context.Context, p *PendingCredit) error
	GetPending(ctx context.Context, id string) (*PendingCredit, error)
	UpdatePending(ctx context.Context, p *PendingCredit) error
	ListUnscannedPending(ctx context.Context, limit int) ([]*PendingCredit, error)
}

// MemStore 线程安全内存实现。读写深拷贝。
type MemStore struct {
	mu sync.RWMutex
	epochs map[string]*CloseEpoch
	epochUB   map[string]string
	cursors   map[string]*EpochCursor
	proofs    map[string]*CloseLateCreditProof
	proofIdem map[string]string
	holds map[string]*DisputeHoldCase
	holdIdem map[string]string
	holdProof map[string]string
	audits    []EpochAudit
	pending   map[string]*PendingCredit
}

// NewMemStore 创建空存储。

  func NewMemStore() *MemStore {
	   return &MemStore{
		   epochs: map[string]*CloseEpoch{},
		  epochUB: map[string]string{},
		  cursors: map[string]*EpochCursor{},
		  proofs:  map[string]*CloseLateCreditProof{},
		   proofIdem: map[string]string{},
		  holds:     map[string]*DisputeHoldCase{},
		   holdIdem:  map[string]string{},
		   holdProof: map[string]string{},
		  pending: map[string]*PendingCredit{},
	  }
  }

   func ubKey(userID, book string) string { return trim(userID) + "|" + BookDefault(book) }

  func (s *MemStore) InsertEpoch(_ context.Context, e *CloseEpoch) error {
	  if s == nil {
		   return ErrUnavailable
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   if _, ok := s.epochs[e.ID]; ok {
		  return ErrDuplicate
	  }
	  k := ubKey(e.UserID, e.Book)
	   if _, ok := s.epochUB[k]; ok {
		   return ErrDuplicate
	   }
	   s.epochs[e.ID] = e.Clone()
	  s.epochUB[k] = e.ID
	  return nil
  }

   func (s *MemStore) GetEpoch(_ context.Context, id string) (*CloseEpoch, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	  }
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	   e, ok := s.epochs[id]
	   if !ok {
		  return nil, ErrNotFound
	   }
	   return e.Clone(), nil
  }

  func (s *MemStore) GetEpochByUserBook(_ context.Context, userID, book string) (*CloseEpoch, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	   }
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	   id,
		  ok := s.epochUB[ubKey(userID, book)]
	   if !ok {
		  return nil, ErrNotFound
	  }
	   return s.epochs[id].Clone(), nil
  }

  func (s *MemStore) UpdateEpoch(_ context.Context, e *CloseEpoch) error {
	  if s == nil {
		  return ErrUnavailable
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   if _, ok := s.epochs[e.ID]; !ok {
		   return ErrNotFound
	  }
	  s.epochs[e.ID] = e.Clone()
	   s.epochUB[ubKey(e.UserID, e.Book)] = e.ID
	   return nil
   }

  func (s *MemStore) ListEpochsByState(_ context.Context, st EpochState, limit int) ([]*CloseEpoch, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	  }
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	  out := make([]*CloseEpoch, 0)
	  for _, e := range s.epochs {
		  if e.State == st {
			   out = append(out, e.Clone())
		  }
	  }
	  sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.Before(out[j].UpdatedAt) })
	   if limit > 0 && len(out) > limit {
		   out = out[:limit]
	   }
	   return out, nil
  }

   func (s *MemStore) ListEpochsByUser(_ context.Context, userID string, limit int) ([]*CloseEpoch, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	  }
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	  out := make([]*CloseEpoch, 0)
	  for _, e := range s.epochs {
		  if e.UserID == userID {
			   out = append(out, e.Clone())
		  }
	  }
	  sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.Before(out[j].UpdatedAt) })
	  if limit > 0 && len(out) > limit {
		   out = out[:limit]
	  }
	  return out, nil
   }

   func (s *MemStore) GetCursor(_ context.Context, userID, book string) (*EpochCursor, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	   }
	  s.mu.RLock()
	  defer s.mu.RUnlock()
	  c, ok := s.cursors[ubKey(userID, book)]
	   if !ok {
		   return nil, ErrNotFound
	   }
	  return c.Clone(), nil
   }

   func (s *MemStore) UpsertCursor(_ context.Context, c *EpochCursor) error {
	  if s == nil {
		  return ErrUnavailable
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  s.cursors[ubKey(c.UserID, c.Book)] = c.Clone()
	   return nil
  }

  func (s *MemStore) InsertProof(_ context.Context, p *CloseLateCreditProof) error {
	  if s == nil {
		  return ErrUnavailable
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  if _, ok := s.proofs[p.ID]; ok {
		   return ErrDuplicate
	   }
	  if p.IdemKey != "" {
		  if _, ok := s.proofIdem[p.IdemKey]; ok {
			   return ErrDuplicate
		   }
		   s.proofIdem[p.IdemKey] = p.ID
	   }
	   s.proofs[p.ID] = p.Clone()
	   return nil
   }

   func (s *MemStore) GetProofByID(_ context.Context, id string) (*CloseLateCreditProof, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	   p, ok := s.proofs[id]
	  if !ok {
		  return nil, ErrNotFound
	   }
	   return p.Clone(), nil
   }

   func (s *MemStore) GetProofByIdem(_ context.Context, idem string) (*CloseLateCreditProof, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	   }
	  s.mu.RLock()
	  defer s.mu.RUnlock()
	   id, ok := s.proofIdem[idem]
	   if !ok {
		  return nil, ErrNotFound
	   }
	   return s.proofs[id].Clone(), nil
   }

   func (s *MemStore) UpdateProof(_ context.Context, p *CloseLateCreditProof) error {
	   if s == nil {
		  return ErrUnavailable
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   if _, ok := s.proofs[p.ID]; !ok {
		  return ErrNotFound
	   }
	  s.proofs[p.ID] = p.Clone()
	  if p.IdemKey != "" {
		   s.proofIdem[p.IdemKey] = p.ID
	   }
	  return nil
  }

   func (s *MemStore) listProofs(match func(*CloseLateCreditProof) bool, limit int) []*CloseLateCreditProof {
	   out := make([]*CloseLateCreditProof, 0)
	  for _, p := range s.proofs {
		   if match(p) {
			  out = append(out, p.Clone())
		  }
	   }
	  sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

   func (s *MemStore) ListProofsByState(_ context.Context, st LateState, limit int) ([]*CloseLateCreditProof, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	   }
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	  return s.listProofs(func(p *CloseLateCreditProof) bool { return p.State == st }, limit), nil
  }

   func (s *MemStore) ListProofsByUser(_ context.Context, userID string, limit int) ([]*CloseLateCreditProof, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	   return s.listProofs(func(p *CloseLateCreditProof) bool { return p.UserID == userID }, limit), nil
   }

  func (s *MemStore) ListProofsBySource(_ context.Context, src CreditSource, limit int) ([]*CloseLateCreditProof, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	  s.mu.RLock()
	  defer s.mu.RUnlock()
	   return s.listProofs(func(p *CloseLateCreditProof) bool { return p.Source == src }, limit), nil
   }

  func (s *MemStore) ListOpenProofs(_ context.Context, limit int) ([]*CloseLateCreditProof, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	   }
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	  return s.listProofs(func(p *CloseLateCreditProof) bool {
		  return p.State == LateOpen || p.State == LateDisputed
	   }, limit), nil
  }

   func (s *MemStore) InsertHold(_ context.Context, h *DisputeHoldCase) error {
	  if s == nil {
		   return ErrUnavailable
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  if _, ok := s.holds[h.ID]; ok {
		  return ErrDuplicate
	  }
	   if h.IdemKey != "" {
		   if _, ok := s.holdIdem[h.IdemKey]; ok {
			   return ErrDuplicate
		  }
		   s.holdIdem[h.IdemKey] = h.ID
	  }
	   if h.ProofID != "" {
		   s.holdProof[h.ProofID] = h.ID
	  }
	  s.holds[h.ID] = h.Clone()
	  return nil
  }

   func (s *MemStore) GetHoldByID(_ context.Context, id string) (*DisputeHoldCase, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	   h, ok := s.holds[id]
	  if !ok {
		   return nil, ErrNotFound
	   }
	  return h.Clone(), nil
  }

  func (s *MemStore) GetHoldByIdem(_ context.Context, idem string) (*DisputeHoldCase, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	  id, ok := s.holdIdem[idem]
	  if !ok {
		   return nil, ErrNotFound
	   }
	   return s.holds[id].Clone(), nil
  }

  func (s *MemStore) GetHoldByProof(_ context.Context, proofID string) (*DisputeHoldCase, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	  }
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	  id, ok := s.holdProof[proofID]
	  if !ok {
		   return nil, ErrNotFound
	   }
	  return s.holds[id].Clone(), nil
  }

   func (s *MemStore) UpdateHold(_ context.Context, h *DisputeHoldCase) error {
	   if s == nil {
		   return ErrUnavailable
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  if _, ok := s.holds[h.ID]; !ok {
		  return ErrNotFound
	   }
	   s.holds[h.ID] = h.Clone()
	  return nil
   }

  func (s *MemStore) ListHoldsByState(_ context.Context, st HoldState, limit int) ([]*DisputeHoldCase, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	  }
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	   out := make([]*DisputeHoldCase, 0)
	  for _, h := range s.holds {
		  if h.State == st {
			   out = append(out, h.Clone())
		   }
	   }
	   sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.Before(out[j].UpdatedAt) })
	   if limit > 0 && len(out) > limit {
		  out = out[:limit]
	   }
	   return out, nil
   }

   func (s *MemStore) ListHoldsByUser(_ context.Context, userID string, limit int) ([]*DisputeHoldCase, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	   out := make([]*DisputeHoldCase, 0)
	   for _, h := range s.holds {
		   if h.UserID == userID {
			  out = append(out, h.Clone())
		  }
	   }
	   sort.Slice(out,
		  func(i, j int) bool { return out[i].UpdatedAt.Before(out[j].UpdatedAt) })
	  if limit > 0 && len(out) > limit {
		  out = out[:limit]
	  }
	  return out, nil
  }

  func (s *MemStore) InsertAudit(_ context.Context, a *EpochAudit) error {
	   if s == nil {
		  return ErrUnavailable
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   s.audits = append(s.audits,
		   *a.Clone())
	  return nil
   }

   func (s *MemStore) ListAuditByUser(_ context.Context, userID string, limit int) ([]*EpochAudit, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	  out := make([]*EpochAudit, 0)
	  for i := range s.audits {
		  if s.audits[i].UserID == userID {
			   c := s.audits[i]
			  out = append(out, &c)
		  }
	  }
	  if limit > 0 && len(out) > limit {
		  out = out[len(out)-limit:]
	  }
	   return out, nil
  }

   func (s *MemStore) InsertPending(_ context.Context, p *PendingCredit) error {
	   if s == nil {
		   return ErrUnavailable
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  if _, ok := s.pending[p.ID]; ok {
		   return ErrDuplicate
	   }
	  s.pending[p.ID] = p.Clone()
	   return nil
   }

  func (s *MemStore) GetPending(_ context.Context, id string) (*PendingCredit, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	  }
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	   p, ok := s.pending[id]
	   if !ok {
		   return nil, ErrNotFound
	  }
	   return p.Clone(), nil
  }

  func (s *MemStore) UpdatePending(_ context.Context, p *PendingCredit) error {
	   if s == nil {
		   return ErrUnavailable
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  if _, ok := s.pending[p.ID]; !ok {
		   return ErrNotFound
	  }
	  s.pending[p.ID] = p.Clone()
	   return nil
  }

   func (s *MemStore) ListUnscannedPending(_ context.Context, limit int) ([]*PendingCredit, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	  }
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	   out := make([]*PendingCredit, 0)
	  for _, p := range s.pending {
		  if !p.Scanned {
			  out = append(out, p.Clone())
		  }
	   }
	   sort.Slice(out,
		  func(i, j int) bool { return out[i].SeenAt.Before(out[j].SeenAt) })
	  if limit > 0 && len(out) > limit {
		  out = out[:limit]
	  }
	   return out, nil
   }
