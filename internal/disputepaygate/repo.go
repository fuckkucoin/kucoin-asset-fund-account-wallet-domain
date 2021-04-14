package disputepaygate

import (
	"context"
	"sync"
	"time"
)

// PayLeaseRepo 放款租约仓。
type PayLeaseRepo interface {
	Save(ctx context.Context, l *DisputePayLease) error
	Get(ctx context.Context, leaseID string) (*DisputePayLease, error)
	GetByCase(ctx context.Context, caseID string) (*DisputePayLease, error)
	GetByIdem(ctx context.Context, idem string) (*DisputePayLease, error)
	ListIssuedExpired(ctx context.Context, now time.Time, limit int) ([]*DisputePayLease, error)
	ListByState(ctx context.Context, state LeaseState, limit int) ([]*DisputePayLease, error)
}

// ClawLeaseRepo 扣回租约仓。
type ClawLeaseRepo interface {
	Save(ctx context.Context,
		l *ClawLease) error
	Get(ctx context.Context, leaseID string) (*ClawLease, error)
	GetByCase(ctx context.Context, caseID string) (*ClawLease, error)
	ListIssuedExpired(ctx context.Context,
		now time.Time, limit int) ([]*ClawLease, error)
	ListByState(ctx context.Context, state LeaseState, limit int) ([]*ClawLease, error)
}

// ProofRepo 只增证明仓。

type ProofRepo interface {
	Save(ctx context.Context, p *DisputePayProof) error
	Get(ctx context.Context, proofID string) (*DisputePayProof, error)
	ListByCase(ctx context.Context, caseID string, limit int) ([]*DisputePayProof, error)
	ListByOutcome(ctx context.Context,
		outcome ProofOutcome, limit int) ([]*DisputePayProof, error)
}

// AuditRepo 审计仓。
type AuditRepo interface {
	Save(ctx context.Context,
		a *AuditRow) error
	ListByCase(ctx context.Context, caseID string, limit int) ([]*AuditRow, error)
}

// GenRepo 世代镜像仓。
type GenRepo interface {
	Save(ctx context.Context, g *CaseGeneration) error
	Get(ctx context.Context,
		caseID string) (*CaseGeneration, error)
}

// MemPayLeaseRepo 内存放款仓。
type MemPayLeaseRepo struct {
	mu sync.RWMutex
	byID   map[string]*DisputePayLease
	byCase map[string]string
	byIdem map[string]string
}

  func NewMemPayLeaseRepo() *MemPayLeaseRepo {
	   return &MemPayLeaseRepo{
		   byID: map[string]*DisputePayLease{}, byCase: map[string]string{}, byIdem: map[string]string{},
	   }
  }

  func (r *MemPayLeaseRepo) Save(_ context.Context, l *DisputePayLease) error {
	  if l == nil || l.LeaseID == "" {
		   return ErrInvalidInput
	  }
	   r.mu.Lock()
	  defer r.mu.Unlock()
	  cp := *l
	  r.byID[l.LeaseID] = &cp
	   r.byCase[l.CaseID] = l.LeaseID
	   if l.Idempotency != "" {
		  r.byIdem[l.Idempotency] = l.LeaseID
	  }
	  return nil
   }

   func (r *MemPayLeaseRepo) Get(_ context.Context, leaseID string) (*DisputePayLease, error) {
	   r.mu.RLock()
	  defer r.mu.RUnlock()
	   l, ok := r.byID[leaseID]
	   if !ok {
		   return nil, ErrLeaseNotFound
	   }
	  cp := *l
	  return &cp, nil
  }

   func (r *MemPayLeaseRepo) GetByCase(_ context.Context, caseID string) (*DisputePayLease, error) {
	   r.mu.RLock()
	   defer r.mu.RUnlock()
	   id, ok := r.byCase[caseID]
	   if !ok {
		  return nil, ErrLeaseNotFound
	   }
	   l := r.byID[id]
	  cp := *l
	   return &cp, nil
   }

  func (r *MemPayLeaseRepo) GetByIdem(_ context.Context, idem string) (*DisputePayLease, error) {
	   r.mu.RLock()
	  defer r.mu.RUnlock()
	   id, ok := r.byIdem[idem]
	  if !ok {
		  return nil, ErrLeaseNotFound
	  }
	  l := r.byID[id]
	   cp := *l
	  return &cp, nil
  }

   func (r *MemPayLeaseRepo) ListIssuedExpired(_ context.Context, now time.Time, limit int) ([]*DisputePayLease, error) {
	   r.mu.RLock()
	   defer r.mu.RUnlock()
	  var out []*DisputePayLease
	  for _, l := range r.byID {
		  if l.State == PayIssued && !now.Before(l.ExpireAt) {
			   cp := *l
			  out = append(out, &cp)
			  if limit > 0 && len(out) >= limit {
				  break
			  }
		  }
	  }
	   return out, nil
  }

   func (r *MemPayLeaseRepo) ListByState(_ context.Context, state LeaseState, limit int) ([]*DisputePayLease, error) {
	   r.mu.RLock()
	  defer r.mu.RUnlock()
	   var out []*DisputePayLease
	   for _, l := range r.byID {
		   if l.State == state {
			  cp := *l
			   out = append(out, &cp)
			  if limit > 0 && len(out) >= limit {
				  break
			   }
		  }
	  }
	   return out, nil
   }

// MemClawLeaseRepo 内存扣回仓。

type MemClawLeaseRepo struct {
	mu sync.RWMutex
	byID   map[string]*ClawLease
	byCase map[string]string
}

  func NewMemClawLeaseRepo() *MemClawLeaseRepo {
	  return &MemClawLeaseRepo{byID: map[string]*ClawLease{}, byCase: map[string]string{}}
  }

   func (r *MemClawLeaseRepo) Save(_ context.Context, l *ClawLease) error {
	  if l == nil || l.LeaseID == "" {
		   return ErrInvalidInput
	   }
	  r.mu.Lock()
	  defer r.mu.Unlock()
	  cp := *l
	   r.byID[l.LeaseID] = &cp
	   r.byCase[l.CaseID] = l.LeaseID
	   return nil
   }

  func (r *MemClawLeaseRepo) Get(_ context.Context, leaseID string) (*ClawLease, error) {
	  r.mu.RLock()
	  defer r.mu.RUnlock()
	  l, ok := r.byID[leaseID]
	  if !ok {
		  return nil, ErrLeaseNotFound
	  }
	  cp := *l
	   return &cp, nil
  }

  func (r *MemClawLeaseRepo) GetByCase(_ context.Context, caseID string) (*ClawLease, error) {
	  r.mu.RLock()
	  defer r.mu.RUnlock()
	   id, ok := r.byCase[caseID]
	   if !ok {
		   return nil, ErrLeaseNotFound
	   }
	  l := r.byID[id]
	   cp := *l
	  return &cp, nil
   }

  func (r *MemClawLeaseRepo) ListIssuedExpired(_ context.Context, now time.Time, limit int) ([]*ClawLease, error) {
	   r.mu.RLock()
	  defer r.mu.RUnlock()
	   var out []*ClawLease
	  for _, l := range r.byID {
		  if l.State == ClawIssued && !now.Before(l.ExpireAt) {
			  cp := *l
			   out = append(out, &cp)
			  if limit > 0 && len(out) >= limit {
				  break
			   }
		  }
	   }
	  return out, nil
   }

   func (r *MemClawLeaseRepo) ListByState(_ context.Context, state LeaseState, limit int) ([]*ClawLease, error) {
	   r.mu.RLock()
	  defer r.mu.RUnlock()
	   var out []*ClawLease
	   for _, l := range r.byID {
		  if l.State == state {
			   cp := *l
			  out = append(out, &cp)
			   if limit > 0 && len(out) >= limit {
				  break
			   }
		   }
	  }
	   return out, nil
  }

// MemProofRepo 内存证明仓。
type MemProofRepo struct {
	mu   sync.RWMutex
	rows map[string]*DisputePayProof
}

   func NewMemProofRepo() *MemProofRepo {
	   return &MemProofRepo{rows: map[string]*DisputePayProof{}}
  }

   func (r *MemProofRepo) Save(_ context.Context, p *DisputePayProof) error {
	   if p == nil || p.ProofID == "" {
		  return ErrInvalidInput
	   }
	   r.mu.Lock()
	   defer r.mu.Unlock()
	   if _, ok := r.rows[p.ProofID]; ok {
		  return nil
	   }
	  cp := *p
	  r.rows[p.ProofID] = &cp
	  return nil
   }

   func (r *MemProofRepo) Get(_ context.Context, proofID string) (*DisputePayProof, error) {
	  r.mu.RLock()
	  defer r.mu.RUnlock()
	   p, ok := r.rows[proofID]
	   if !ok {
		  return nil, ErrProofNotFound
	  }
	   cp := *p
	  return &cp, nil
  }

   func (r *MemProofRepo) ListByCase(_ context.Context, caseID string, limit int) ([]*DisputePayProof, error) {
	  r.mu.RLock()
	  defer r.mu.RUnlock()
	  var out []*DisputePayProof
	   for _, p := range r.rows {
		   if p.CaseID == caseID {
			  cp := *p
			   out = append(out, &cp)
			   if limit > 0 && len(out) >= limit {
				   break
			  }
		  }
	  }
	  return out, nil
   }

  func (r *MemProofRepo) ListByOutcome(_ context.Context, outcome ProofOutcome, limit int) ([]*DisputePayProof, error) {
	   r.mu.RLock()
	   defer r.mu.RUnlock()
	  var out []*DisputePayProof
	   for _, p := range r.rows {
		  if p.Outcome == outcome {
			   cp := *p
			  out = append(out, &cp)
			  if limit > 0 && len(out) >= limit {
				   break
			   }
		  }
	  }
	  return out, nil
  }

// MemAuditRepo 内存审计仓。
type MemAuditRepo struct {
	mu sync.RWMutex
	rows []AuditRow
}

  func NewMemAuditRepo() *MemAuditRepo { return &MemAuditRepo{} }

   func (r *MemAuditRepo) Save(_ context.Context, a *AuditRow) error {
	  if a == nil || a.AuditID == "" {
		   return ErrInvalidInput
	   }
	   r.mu.Lock()
	   defer r.mu.Unlock()
	  r.rows = append(r.rows, *a)
	   return nil
   }

  func (r *MemAuditRepo) ListByCase(_ context.Context, caseID string, limit int) ([]*AuditRow, error) {
	   r.mu.RLock()
	   defer r.mu.RUnlock()
	  var out []*AuditRow
	   for i := range r.rows {
		   if r.rows[i].CaseID == caseID {
			  cp := r.rows[i]
			   out = append(out, &cp)
			  if limit > 0 && len(out) >= limit {
				   break
			   }
		   }
	   }
	   return out, nil
  }

// MemGenRepo 内存世代仓。
type MemGenRepo struct {
	mu   sync.RWMutex
	rows map[string]*CaseGeneration
}

   func NewMemGenRepo() *MemGenRepo { return &MemGenRepo{rows: map[string]*CaseGeneration{}} }

   func (r *MemGenRepo) Save(_ context.Context, g *CaseGeneration) error {
	  if g == nil || g.CaseID == "" {
		   return ErrInvalidInput
	   }
	  r.mu.Lock()
	   defer r.mu.Unlock()
	  cp := *g
	  r.rows[g.CaseID] = &cp
	   return nil
   }

  func (r *MemGenRepo) Get(_ context.Context, caseID string) (*CaseGeneration, error) {
	   r.mu.RLock()
	   defer r.mu.RUnlock()
	  g, ok := r.rows[caseID]
	  if !ok {
		   return nil, ErrCaseNotFound
	  }
	  cp := *g
	  return &cp, nil
  }
