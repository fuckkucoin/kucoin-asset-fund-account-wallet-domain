package occseggate

import (
	"context"
	"sort"
	"sync"
)

// Store 覆盖世代、证明、吊销、审计、互斥占位。
type Store interface {
	InsertSeg(ctx context.Context, s *OccSegGen) error
	GetSegByID(ctx context.Context, id string) (*OccSegGen, error)
	GetSegByGrantIdem(ctx context.Context, grantIdem string) (*OccSegGen, error)
	UpdateSeg(ctx context.Context, s *OccSegGen) error
	ListSegsByTicket(ctx context.Context, ticketID string, limit int) ([]*OccSegGen, error)
	ListSegsByState(ctx context.Context,
		st SegState, limit int) ([]*OccSegGen, error)
	ListActiveSegs(ctx context.Context, limit int) ([]*OccSegGen, error)
	GetCursor(ctx context.Context, ticketID string) (*TicketCursor, error)
	UpsertCursor(ctx context.Context,
		c *TicketCursor) error
	InsertProof(ctx context.Context, p *GhostOccupancyProof) error
	GetProofByID(ctx context.Context, id string) (*GhostOccupancyProof, error)
	GetProofByIdem(ctx context.Context,
		idem string) (*GhostOccupancyProof, error)
	UpdateProof(ctx context.Context, p *GhostOccupancyProof) error
	ListProofsByState(ctx context.Context, st ProofState, limit int) ([]*GhostOccupancyProof, error)
	ListProofsByTicket(ctx context.Context, ticketID string, limit int) ([]*GhostOccupancyProof, error)
	ListProofsByKind(ctx context.Context,
		kind ProofKind, limit int) ([]*GhostOccupancyProof, error)
	InsertRevocation(ctx context.Context, r *ExpireRevocation) error
	GetRevocation(ctx context.Context, id string) (*ExpireRevocation, error)
	GetRevocationByIdem(ctx context.Context,
		idem string) (*ExpireRevocation, error)
	UpdateRevocation(ctx context.Context, r *ExpireRevocation) error
	ListRevocations(ctx context.Context, limit int) ([]*ExpireRevocation, error)
	ListRevocationsByTicket(ctx context.Context, ticketID string, limit int) ([]*ExpireRevocation, error)
	InsertAudit(ctx context.Context, a *OccAudit) error
	ListAuditByTicket(ctx context.Context, ticketID string, limit int) ([]*OccAudit, error)
	TryAcquireMutex(ctx context.Context, rec *MutexRecord) error
	ReleaseMutex(ctx context.Context, grantIdem, owner string) error
	GetMutex(ctx context.Context, grantIdem string) (*MutexRecord, error)
}

// OccMemStore 线程安全内存实现。读写深拷贝。

type OccMemStore struct {
	mu sync.RWMutex
	segs    map[string]*OccSegGen
	segIdem map[string]string
	cursors map[string]*TicketCursor
	proofs  map[string]*GhostOccupancyProof
	proofIdem map[string]string
	revs      map[string]*ExpireRevocation
	revIdem   map[string]string
	audits    []OccAudit
	mutexes map[string]*MutexRecord
}

// NewOccMemStore 创建空存储。

   func NewOccMemStore() *OccMemStore {
	  return &OccMemStore{
		   segs: map[string]*OccSegGen{},
		   segIdem: map[string]string{},
		   cursors:   map[string]*TicketCursor{},
		  proofs:    map[string]*GhostOccupancyProof{},
		  proofIdem: map[string]string{},
		  revs:      map[string]*ExpireRevocation{},
		  revIdem: map[string]string{},
		   mutexes: map[string]*MutexRecord{},
	   }
   }

  func cloneSeg(s *OccSegGen) *OccSegGen {
	  if s == nil {
		   return nil
	  }
	  c := *s
	   return &c
  }

   func cloneProof(p *GhostOccupancyProof) *GhostOccupancyProof {
	  if p == nil {
		   return nil
	  }
	  c := *p
	  if p.AckedAt != nil {
		  t := *p.AckedAt
		  c.AckedAt = &t
	  }
	  return &c
   }

  func cloneRev(r *ExpireRevocation) *ExpireRevocation {
	  if r == nil {
		  return nil
	   }
	   c := *r
	   return &c
   }

   func cloneMux(m *MutexRecord) *MutexRecord {
	   if m == nil {
		  return nil
	   }
	  c := *m
	   return &c
   }

   func (s *OccMemStore) InsertSeg(_ context.Context, seg *OccSegGen) error {
	  if s == nil {
		  return ErrUnavailable
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  if _, ok := s.segs[seg.ID]; ok {
		  return ErrDuplicate
	  }
	   if seg.GrantIdem != "" {
		   if _, ok := s.segIdem[seg.GrantIdem]; ok {
			   return ErrDuplicate
		  }
		  s.segIdem[seg.GrantIdem] = seg.ID
	   }
	  s.segs[seg.ID] = cloneSeg(seg)
	   return nil
  }

   func (s *OccMemStore) GetSegByID(_ context.Context, id string) (*OccSegGen, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	  seg, ok := s.segs[id]
	  if !ok {
		  return nil, ErrNotFound
	  }
	   return cloneSeg(seg), nil
  }

   func (s *OccMemStore) GetSegByGrantIdem(_ context.Context, grantIdem string) (*OccSegGen, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	   }
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	   id, ok := s.segIdem[grantIdem]
	  if !ok {
		  return nil, ErrNotFound
	   }
	  return cloneSeg(s.segs[id]), nil
   }

  func (s *OccMemStore) UpdateSeg(_ context.Context, seg *OccSegGen) error {
	  if s == nil {
		  return ErrUnavailable
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   if _, ok := s.segs[seg.ID]; !ok {
		  return ErrNotFound
	   }
	   s.segs[seg.ID] = cloneSeg(seg)
	  return nil
  }

  func (s *OccMemStore) listSegsLocked(pred func(*OccSegGen) bool, limit int) []*OccSegGen {
	  out := make([]*OccSegGen, 0)
	  for _, seg := range s.segs {
		   if pred == nil || pred(seg) {
			  out = append(out,
				cloneSeg(seg))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

   func (s *OccMemStore) ListSegsByTicket(_ context.Context, ticketID string, limit int) ([]*OccSegGen, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	  }
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	   return s.listSegsLocked(func(seg *OccSegGen) bool { return seg.TicketID == ticketID }, limit), nil
   }

   func (s *OccMemStore) ListSegsByState(_ context.Context, st SegState, limit int) ([]*OccSegGen, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	   }
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	  return s.listSegsLocked(func(seg *OccSegGen) bool { return seg.State == st }, limit), nil
   }

   func (s *OccMemStore) ListActiveSegs(_ context.Context, limit int) ([]*OccSegGen, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	  }
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	  return s.listSegsLocked(func(seg *OccSegGen) bool { return !seg.State.Closed() }, limit), nil
   }

  func (s *OccMemStore) GetCursor(_ context.Context, ticketID string) (*TicketCursor, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	  s.mu.RLock()
	  defer s.mu.RUnlock()
	  c, ok := s.cursors[ticketID]
	   if !ok {
		   return nil, ErrNotFound
	  }
	   cp := *c
	  return &cp, nil
   }

   func (s *OccMemStore) UpsertCursor(_ context.Context, c *TicketCursor) error {
	  if s == nil {
		  return ErrUnavailable
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   cp := *c
	  s.cursors[c.TicketID] = &cp
	   return nil
  }

   func (s *OccMemStore) InsertProof(_ context.Context, p *GhostOccupancyProof) error {
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
	   s.proofs[p.ID] = cloneProof(p)
	  return nil
  }

  func (s *OccMemStore) GetProofByID(_ context.Context, id string) (*GhostOccupancyProof, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	  p, ok := s.proofs[id]
	   if !ok {
		   return nil, ErrNotFound
	  }
	   return cloneProof(p), nil
  }

  func (s *OccMemStore) GetProofByIdem(_ context.Context, idem string) (*GhostOccupancyProof, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	  }
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	  id, ok := s.proofIdem[idem]
	   if !ok {
		   return nil, ErrNotFound
	  }
	   return cloneProof(s.proofs[id]), nil
   }

  func (s *OccMemStore) UpdateProof(_ context.Context, p *GhostOccupancyProof) error {
	  if s == nil {
		  return ErrUnavailable
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   if _, ok := s.proofs[p.ID]; !ok {
		   return ErrNotFound
	   }
	   s.proofs[p.ID] = cloneProof(p)
	   return nil
   }

   func (s *OccMemStore) listProofsLocked(pred func(*GhostOccupancyProof) bool, limit int) []*GhostOccupancyProof {
	   out := make([]*GhostOccupancyProof,
		0)
	for _, p := range s.proofs {
		if pred == nil || pred(p) {
			out = append(out, cloneProof(p))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

   func (s *OccMemStore) ListProofsByState(_ context.Context, st ProofState, limit int) ([]*GhostOccupancyProof, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	   }
	  s.mu.RLock()
	  defer s.mu.RUnlock()
	   return s.listProofsLocked(func(p *GhostOccupancyProof) bool { return p.State == st }, limit), nil
   }

  func (s *OccMemStore) ListProofsByTicket(_ context.Context, ticketID string, limit int) ([]*GhostOccupancyProof, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	  }
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	   return s.listProofsLocked(func(p *GhostOccupancyProof) bool { return p.TicketID == ticketID }, limit), nil
  }

  func (s *OccMemStore) ListProofsByKind(_ context.Context, kind ProofKind, limit int) ([]*GhostOccupancyProof, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	  }
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	  return s.listProofsLocked(func(p *GhostOccupancyProof) bool { return p.Kind == kind }, limit), nil
  }

  func (s *OccMemStore) InsertRevocation(_ context.Context, r *ExpireRevocation) error {
	   if s == nil {
		   return ErrUnavailable
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   if _, ok := s.revs[r.ID]; ok {
		  return ErrDuplicate
	   }
	  if r.IdemKey != "" {
		  if _, ok := s.revIdem[r.IdemKey]; ok {
			  return ErrDuplicate
		  }
		  s.revIdem[r.IdemKey] = r.ID
	  }
	   s.revs[r.ID] = cloneRev(r)
	   return nil
  }

  func (s *OccMemStore) GetRevocation(_ context.Context, id string) (*ExpireRevocation, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	  }
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	  r, ok := s.revs[id]
	  if !ok {
		  return nil, ErrNotFound
	   }
	  return cloneRev(r), nil
  }

  func (s *OccMemStore) GetRevocationByIdem(_ context.Context, idem string) (*ExpireRevocation, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	  id, ok := s.revIdem[idem]
	  if !ok {
		  return nil, ErrNotFound
	   }
	  return cloneRev(s.revs[id]), nil
   }

  func (s *OccMemStore) UpdateRevocation(_ context.Context, r *ExpireRevocation) error {
	  if s == nil {
		  return ErrUnavailable
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  if _, ok := s.revs[r.ID]; !ok {
		  return ErrNotFound
	  }
	   s.revs[r.ID] = cloneRev(r)
	  return nil
   }

   func (s *OccMemStore) ListRevocations(_ context.Context, limit int) ([]*ExpireRevocation, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	  s.mu.RLock()
	  defer s.mu.RUnlock()
	  out := make([]*ExpireRevocation, 0, len(s.revs))
	  for _, r := range s.revs {
		   out = append(out, cloneRev(r))
	  }
	   sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	   if limit > 0 && len(out) > limit {
		  out = out[:limit]
	   }
	   return out, nil
  }

   func (s *OccMemStore) ListRevocationsByTicket(_ context.Context, ticketID string, limit int) ([]*ExpireRevocation, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	  s.mu.RLock()
	  defer s.mu.RUnlock()
	  out := make([]*ExpireRevocation, 0)
	   for _, r := range s.revs {
		  if r.TicketID == ticketID {
			   out = append(out, cloneRev(r))
		  }
	  }
	   sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
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

   func (s *OccMemStore) TryAcquireMutex(_ context.Context, rec *MutexRecord) error {
	   if s == nil {
		   return ErrUnavailable
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   if cur, ok := s.mutexes[rec.GrantIdem]; ok {
		   if cur.Owner == rec.Owner {
			   return nil
		  }
		   return ErrMutexHeld
	  }
	  s.mutexes[rec.GrantIdem] = cloneMux(rec)
	  return nil
   }

   func (s *OccMemStore) ReleaseMutex(_ context.Context, grantIdem, owner string) error {
	  if s == nil {
		   return ErrUnavailable
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   cur, ok := s.mutexes[grantIdem]
	  if !ok {
		  return nil
	  }
	  if cur.Owner != owner {
		   return ErrMutexHeld
	   }
	  delete(s.mutexes, grantIdem)
	  return nil
  }

   func (s *OccMemStore) GetMutex(_ context.Context, grantIdem string) (*MutexRecord, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	   }
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	  m, ok := s.mutexes[grantIdem]
	  if !ok {
		   return nil, ErrNotFound
	  }
	   return cloneMux(m), nil
   }

// LockedKinds 当前互斥种类（巡检用）。

   func (s *OccMemStore) LockedKinds() []string {
	  s.mu.RLock()
	  defer s.mu.RUnlock()
	  out := make([]string, 0, len(s.mutexes))
	   for _, m := range s.mutexes {
		   out = append(out, m.Kind)
	   }
	   return out
  }

var _ Store = (*OccMemStore)(nil)
