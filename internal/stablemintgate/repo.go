package stablemintgate

import (
	"context"
	"sync"
	"time"
)

// LeaseRepo 铸赎租约仓。
type LeaseRepo interface {
	Save(ctx context.Context, l *MintLease) error
	Get(ctx context.Context, leaseID string) (*MintLease, error)
	GetByIdem(ctx context.Context, idem string) (*MintLease, error)
	ListByScope(ctx context.Context,
		scope string, limit int) ([]*MintLease, error)
	ListLiveExpired(ctx context.Context, now time.Time, limit int) ([]*MintLease, error)
	ListByState(ctx context.Context, state LeaseState, limit int) ([]*MintLease, error)
}

// HalfRepo 半截证明仓。

type HalfRepo interface {
	Save(ctx context.Context, p *StableHalfProof) error
	Get(ctx context.Context,
		halfID string) (*StableHalfProof, error)
	GetByIdem(ctx context.Context, idem string) (*StableHalfProof, error)
	GetByJournal(ctx context.Context, journalID string) (*StableHalfProof, error)
	ListOpen(ctx context.Context, limit int) ([]*StableHalfProof, error)
	ListByLease(ctx context.Context, leaseID string, limit int) ([]*StableHalfProof, error)
}

// AuditRepo 审计仓。
type AuditRepo interface {
	Save(ctx context.Context, a *MintAudit) error
	ListByLease(ctx context.Context, leaseID string, limit int) ([]*MintAudit, error)
	ListByScope(ctx context.Context, scope string, limit int) ([]*MintAudit, error)
}

// GenRepo 世代仓。

type GenRepo interface {
	Save(ctx context.Context, g *MintGeneration) error
	Get(ctx context.Context, scope string) (*MintGeneration, error)
}

// ProofRepo 拒铸证明仓。

type ProofRepo interface {
	Save(ctx context.Context,
		p *DenyProof) error
	Get(ctx context.Context, proofID string) (*DenyProof, error)
	ListByScope(ctx context.Context, scope string, limit int) ([]*DenyProof, error)
	ListByOutcome(ctx context.Context, outcome ProofOutcome, limit int) ([]*DenyProof, error)
}

  func cloneLease(l *MintLease) *MintLease {
	  if l == nil {
		  return nil
	  }
	   cp := l.Copy()
	  return &cp
  }

  func cloneHalf(p *StableHalfProof) *StableHalfProof {
	   if p == nil {
		   return nil
	   }
	  cp := p.Copy()
	  return &cp
  }

// MemLeaseRepo 内存租约仓。

type MemLeaseRepo struct {
	mu     sync.RWMutex
	byID   map[string]*MintLease
	byIdem map[string]string
}

  func NewMemLeaseRepo() *MemLeaseRepo {
	   return &MemLeaseRepo{byID: map[string]*MintLease{},
		  byIdem: map[string]string{}}
   }

  func (r *MemLeaseRepo) Save(_ context.Context, l *MintLease) error {
	  if l == nil || l.LeaseID == "" {
		   return ErrInvalidInput
	  }
	  r.mu.Lock()
	  defer r.mu.Unlock()
	  cp := l.Copy()
	  r.byID[l.LeaseID] = &cp
	   if l.Idempotency != "" {
		  r.byIdem[l.Idempotency] = l.LeaseID
	   }
	   return nil
  }

  func (r *MemLeaseRepo) Get(_ context.Context, leaseID string) (*MintLease, error) {
	   r.mu.RLock()
	  defer r.mu.RUnlock()
	  cur, ok := r.byID[leaseID]
	   if !ok {
		  return nil, ErrLeaseNotFound
	  }
	  return cloneLease(cur), nil
   }

  func (r *MemLeaseRepo) GetByIdem(_ context.Context, idem string) (*MintLease, error) {
	  r.mu.RLock()
	  defer r.mu.RUnlock()
	   id, ok := r.byIdem[idem]
	   if !ok {
		  return nil, ErrLeaseNotFound
	  }
	  return cloneLease(r.byID[id]), nil
  }

   func (r *MemLeaseRepo) ListByScope(_ context.Context, scope string, limit int) ([]*MintLease, error) {
	  r.mu.RLock()
	   defer r.mu.RUnlock()
	   if limit <= 0 {
		  limit = 50
	   }
	   out := make([]*MintLease, 0, limit)
	  for _, l := range r.byID {
		  if ScopeKey(l.Kind, l.Asset) != scope {
			   continue
		   }
		   out = append(out, cloneLease(l))
		  if len(out) >= limit {
			  break
		   }
	  }
	   return out, nil
  }

   func (r *MemLeaseRepo) ListLiveExpired(_ context.Context, now time.Time, limit int) ([]*MintLease, error) {
	   r.mu.RLock()
	   defer r.mu.RUnlock()
	  if limit <= 0 {
		  limit = 100
	   }
	  out := make([]*MintLease, 0, limit)
	   for _, l := range r.byID {
		  if !l.State.Live() {
			  continue
		   }
		  if now.Before(l.ExpireAt) {
			   continue
		   }
		  out = append(out, cloneLease(l))
		   if len(out) >= limit {
			  break
		  }
	  }
	  return out, nil
  }

   func (r *MemLeaseRepo) ListByState(_ context.Context, state LeaseState, limit int) ([]*MintLease, error) {
	  r.mu.RLock()
	  defer r.mu.RUnlock()
	   if limit <= 0 {
		  limit = 100
	  }
	   out := make([]*MintLease, 0, limit)
	  for _, l := range r.byID {
		  if l.State != state {
			   continue
		  }
		   out = append(out, cloneLease(l))
		  if len(out) >= limit {
			  break
		   }
	  }
	  return out, nil
  }

// MemHalfRepo 内存半截仓。
type MemHalfRepo struct {
	mu sync.RWMutex
	byID map[string]*StableHalfProof
	byIdem    map[string]string
	byJournal map[string]string
}

  func NewMemHalfRepo() *MemHalfRepo {
	   return &MemHalfRepo{
		  byID: map[string]*StableHalfProof{}, byIdem: map[string]string{}, byJournal: map[string]string{},
	  }
   }

   func (r *MemHalfRepo) Save(_ context.Context, p *StableHalfProof) error {
	   if p == nil || p.HalfID == "" {
		   return ErrInvalidInput
	   }
	   r.mu.Lock()
	  defer r.mu.Unlock()
	   cp := p.Copy()
	  r.byID[p.HalfID] = &cp
	   if p.IdemKey != "" {
		   r.byIdem[p.IdemKey] = p.HalfID
	   }
	  if p.JournalID != "" {
		   r.byJournal[p.JournalID] = p.HalfID
	   }
	   return nil
   }

  func (r *MemHalfRepo) Get(_ context.Context, halfID string) (*StableHalfProof, error) {
	  r.mu.RLock()
	   defer r.mu.RUnlock()
	  cur, ok := r.byID[halfID]
	  if !ok {
		  return nil, ErrHalfNotFound
	   }
	   return cloneHalf(cur), nil
  }

  func (r *MemHalfRepo) GetByIdem(_ context.Context, idem string) (*StableHalfProof, error) {
	   r.mu.RLock()
	   defer r.mu.RUnlock()
	   id, ok := r.byIdem[idem]
	  if !ok {
		  return nil, ErrHalfNotFound
	   }
	  return cloneHalf(r.byID[id]), nil
   }

   func (r *MemHalfRepo) GetByJournal(_ context.Context, journalID string) (*StableHalfProof, error) {
	   r.mu.RLock()
	   defer r.mu.RUnlock()
	   id, ok := r.byJournal[journalID]
	  if !ok {
		  return nil, ErrHalfNotFound
	  }
	   return cloneHalf(r.byID[id]), nil
  }

  func (r *MemHalfRepo) ListOpen(_ context.Context, limit int) ([]*StableHalfProof, error) {
	   r.mu.RLock()
	   defer r.mu.RUnlock()
	  if limit <= 0 {
		   limit = 100
	   }
	  out := make([]*StableHalfProof,
		   0, limit)
	   for _, p := range r.byID {
		  if p.State != HalfOpen {
			   continue
		   }
		  out = append(out, cloneHalf(p))
		  if len(out) >= limit {
			  break
		   }
	   }
	   return out, nil
  }

   func (r *MemHalfRepo) ListByLease(_ context.Context, leaseID string, limit int) ([]*StableHalfProof, error) {
	   r.mu.RLock()
	  defer r.mu.RUnlock()
	   if limit <= 0 {
		  limit = 50
	  }
	  out := make([]*StableHalfProof,
		  0, limit)
	  for _, p := range r.byID {
		   if p.LeaseID != leaseID {
			   continue
		   }
		   out = append(out, cloneHalf(p))
		   if len(out) >= limit {
			   break
		  }
	   }
	   return out, nil
   }

// MemAuditRepo 内存审计。

type MemAuditRepo struct {
	mu   sync.RWMutex
	rows []*MintAudit
}

   func NewMemAuditRepo() *MemAuditRepo { return &MemAuditRepo{} }

  func (r *MemAuditRepo) Save(_ context.Context, a *MintAudit) error {
	   if a == nil || a.AuditID == "" {
		  return ErrInvalidInput
	   }
	  r.mu.Lock()
	  cp := *a
	  r.rows = append(r.rows, &cp)
	   r.mu.Unlock()
	   return nil
   }

   func (r *MemAuditRepo) ListByLease(_ context.Context, leaseID string, limit int) ([]*MintAudit, error) {
	  return r.list(func(a *MintAudit) bool { return a.LeaseID == leaseID }, limit)
   }

   func (r *MemAuditRepo) ListByScope(_ context.Context, scope string, limit int) ([]*MintAudit, error) {
	   return r.list(func(a *MintAudit) bool { return a.Scope == scope },
		  limit)
  }

   func (r *MemAuditRepo) list(match func(*MintAudit) bool, limit int) ([]*MintAudit, error) {
	  r.mu.RLock()
	   defer r.mu.RUnlock()
	  if limit <= 0 {
		   limit = 50
	   }
	  out := make([]*MintAudit, 0, limit)
	   for i := len(r.rows) - 1; i >= 0; i-- {
		  if !match(r.rows[i]) {
			  continue
		  }
		  cp := *r.rows[i]
		  out = append(out, &cp)
		   if len(out) >= limit {
			   break
		  }
	   }
	   return out, nil
   }

  // MemGenRepo 内存世代。

   type MemGenRepo struct {
	   mu sync.RWMutex
	  rows map[string]*MintGeneration
  }

   func NewMemGenRepo() *MemGenRepo { return &MemGenRepo{rows: map[string]*MintGeneration{}} }

   func (r *MemGenRepo) Save(_ context.Context, g *MintGeneration) error {
	   if g == nil || g.Scope == "" {
		   return ErrInvalidInput
	  }
	  r.mu.Lock()
	  cp := *g
	   r.rows[g.Scope] = &cp
	   r.mu.Unlock()
	  return nil
   }

   func (r *MemGenRepo) Get(_ context.Context, scope string) (*MintGeneration, error) {
	   r.mu.RLock()
	  defer r.mu.RUnlock()
	  cur, ok := r.rows[scope]
	  if !ok {
		  return &MintGeneration{Scope: scope, MintGen: 0}, nil
	  }
	   cp := *cur
	  return &cp, nil
  }

// MemProofRepo 内存拒铸证明。
type MemProofRepo struct {
	mu   sync.RWMutex
	byID map[string]*DenyProof
}

   func NewMemProofRepo() *MemProofRepo { return &MemProofRepo{byID: map[string]*DenyProof{}} }

  func (r *MemProofRepo) Save(_ context.Context, p *DenyProof) error {
	  if p == nil || p.ProofID == "" {
		  return ErrInvalidInput
	   }
	  r.mu.Lock()
	   cp := *p
	  r.byID[p.ProofID] = &cp
	   r.mu.Unlock()
	   return nil
   }

   func (r *MemProofRepo) Get(_ context.Context, proofID string) (*DenyProof, error) {
	  r.mu.RLock()
	   defer r.mu.RUnlock()
	  cur, ok := r.byID[proofID]
	  if !ok {
		  return nil, ErrProofNotFound
	   }
	  cp := *cur
	  return &cp, nil
  }

   func (r *MemProofRepo) ListByScope(_ context.Context, scope string, limit int) ([]*DenyProof, error) {
	  return r.list(func(p *DenyProof) bool { return p.Scope == scope }, limit)
   }

   func (r *MemProofRepo) ListByOutcome(_ context.Context, outcome ProofOutcome, limit int) ([]*DenyProof, error) {
	  return r.list(func(p *DenyProof) bool { return p.Outcome == outcome },
		   limit)
   }

   func (r *MemProofRepo) list(match func(*DenyProof) bool, limit int) ([]*DenyProof, error) {
	  r.mu.RLock()
	  defer r.mu.RUnlock()
	   if limit <= 0 {
		  limit = 50
	  }
	  out := make([]*DenyProof, 0, limit)
	   for _, p := range r.byID {
		  if !match(p) {
			  continue
		  }
		  cp := *p
		  out = append(out, &cp)
		  if len(out) >= limit {
			   break
		   }
	   }
	   return out, nil
   }
