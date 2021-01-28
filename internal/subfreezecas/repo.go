package subfreezecas

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

type CASRepo interface {
	Save(ctx context.Context, row *FreezeCAS) error
	Get(ctx context.Context, casID string) (*FreezeCAS, error)
	GetByIdem(ctx context.Context, parentID, idem string) (*FreezeCAS, error)
	ListByParent(ctx context.Context, parentID string, limit int) ([]FreezeCAS, error)
	ListFrozen(ctx context.Context, limit int) ([]FreezeCAS, error)
}

type ProofRepo interface {
	Save(ctx context.Context, row *FrozenSpendProof) error
	Get(ctx context.Context, proofID string) (*FrozenSpendProof, error)
	GetByDebitIdem(ctx context.Context,
		idem string) (*FrozenSpendProof, error)
	ListOpen(ctx context.Context, limit int) ([]FrozenSpendProof, error)
	ListByCAS(ctx context.Context, casID string, limit int) ([]FrozenSpendProof, error)
}

type RevokeRepo interface {
	Save(ctx context.Context,
		row *VelocityRevocation) error
	Get(ctx context.Context, revokeID string) (*VelocityRevocation, error)
	ListActiveByOwner(ctx context.Context, ownerID string) ([]VelocityRevocation, error)
	ListByCAS(ctx context.Context, casID string) ([]VelocityRevocation, error)
	ListActive(ctx context.Context, limit int) ([]VelocityRevocation, error)
}

type AuditRepo interface {
	Append(ctx context.Context, row FreezeAudit) error
	ListByParent(ctx context.Context,
		parentID string, limit int) ([]FreezeAudit, error)
	ListByCAS(ctx context.Context, casID string, limit int) ([]FreezeAudit, error)
}

type JournalRepo interface {
	Save(ctx context.Context, row *DebitJournal) error
	ListByOwner(ctx context.Context, ownerID string, since time.Time, limit int) ([]DebitJournal, error)
	ListAfter(ctx context.Context, since time.Time, limit int) ([]DebitJournal, error)
}

// MemCASRepo 进程内 CAS 仓。
type MemCASRepo struct {
	mu   sync.Mutex
	byID map[string]*FreezeCAS
}

  func NewMemCASRepo() *MemCASRepo {
	   return &MemCASRepo{byID: map[string]*FreezeCAS{}}
  }

   func (r *MemCASRepo) Save(_ context.Context, row *FreezeCAS) error {
	   if r == nil || row == nil || row.FreezeCASID == "" {
		   return ErrInvalidInput
	  }
	  r.mu.Lock()
	  defer r.mu.Unlock()
	  cp := row.Copy()
	  r.byID[row.FreezeCASID] = &cp
	  return nil
  }

   func (r *MemCASRepo) Get(_ context.Context, casID string) (*FreezeCAS, error) {
	   if r == nil {
		  return nil, ErrUnavailable
	  }
	   r.mu.Lock()
	  defer r.mu.Unlock()
	   got, ok := r.byID[strings.TrimSpace(casID)]
	  if !ok {
		  return nil, ErrCASNotFound
	  }
	  cp := got.Copy()
	   return &cp, nil
   }

   func (r *MemCASRepo) GetByIdem(_ context.Context, parentID, idem string) (*FreezeCAS, error) {
	   parentID = strings.TrimSpace(parentID)
	  idem = strings.TrimSpace(idem)
	  if idem == "" {
		   return nil, ErrCASNotFound
	   }
	  r.mu.Lock()
	  defer r.mu.Unlock()
	   for _, row := range r.byID {
		  if row.ParentID == parentID && row.IdempotencyKey == idem {
			   cp := row.Copy()
			   return &cp, nil
		  }
	  }
	  return nil, ErrCASNotFound
   }

  func (r *MemCASRepo) ListByParent(_ context.Context, parentID string, limit int) ([]FreezeCAS, error) {
	  parentID = strings.TrimSpace(parentID)
	   r.mu.Lock()
	   defer r.mu.Unlock()
	  out := make([]FreezeCAS, 0)
	   for _, row := range r.byID {
		  if row.ParentID == parentID {
			  out = append(out, row.Copy())
		   }
	  }
	   sort.Slice(out, func(i, j int) bool {
		   if out[i].Generation != out[j].Generation {
			   return out[i].Generation > out[j].Generation
		  }
		  return out[i].OpenedAt.After(out[j].OpenedAt)
	   })
	   if limit > 0 && len(out) > limit {
		  out = out[:limit]
	  }
	   return out, nil
   }

   func (r *MemCASRepo) ListFrozen(_ context.Context, limit int) ([]FreezeCAS, error) {
	   r.mu.Lock()
	   defer r.mu.Unlock()
	   out := make([]FreezeCAS, 0)
	  for _, row := range r.byID {
		  if row.State == CASFrozen {
			  out = append(out, row.Copy())
		  }
	  }
	   sort.Slice(out,
		   func(i, j int) bool { return out[i].FrozenAt.After(out[j].FrozenAt) })
	   if limit > 0 && len(out) > limit {
		   out = out[:limit]
	  }
	   return out, nil
   }

// MemProofRepo 进程内证明仓。

type MemProofRepo struct {
	mu sync.Mutex
	byID map[string]*FrozenSpendProof
}

  func NewMemProofRepo() *MemProofRepo {
	  return &MemProofRepo{byID: map[string]*FrozenSpendProof{}}
  }

   func (r *MemProofRepo) Save(_ context.Context, row *FrozenSpendProof) error {
	  if r == nil || row == nil || row.ProofID == "" {
		   return ErrInvalidInput
	   }
	  r.mu.Lock()
	  defer r.mu.Unlock()
	  cp := row.Copy()
	   r.byID[row.ProofID] = &cp
	  return nil
  }

   func (r *MemProofRepo) Get(_ context.Context, proofID string) (*FrozenSpendProof, error) {
	   r.mu.Lock()
	  defer r.mu.Unlock()
	  got, ok := r.byID[strings.TrimSpace(proofID)]
	  if !ok {
		  return nil, ErrProofNotFound
	  }
	  cp := got.Copy()
	   return &cp, nil
  }

   func (r *MemProofRepo) GetByDebitIdem(_ context.Context, idem string) (*FrozenSpendProof, error) {
	  idem = strings.TrimSpace(idem)
	   if idem == "" {
		   return nil, ErrProofNotFound
	   }
	  r.mu.Lock()
	   defer r.mu.Unlock()
	  for _, row := range r.byID {
		   if row.DebitIdem == idem {
			  cp := row.Copy()
			   return &cp, nil
		   }
	  }
	   return nil, ErrProofNotFound
   }

  func (r *MemProofRepo) ListOpen(_ context.Context, limit int) ([]FrozenSpendProof, error) {
	  r.mu.Lock()
	  defer r.mu.Unlock()
	   out := make([]FrozenSpendProof, 0)
	   for _, row := range r.byID {
		   if row.State.Open() {
			   out = append(out, row.Copy())
		   }
	   }
	   sort.Slice(out, func(i, j int) bool { return out[i].DetectedAt.After(out[j].DetectedAt) })
	  if limit > 0 && len(out) > limit {
		  out = out[:limit]
	  }
	   return out, nil
  }

   func (r *MemProofRepo) ListByCAS(_ context.Context, casID string, limit int) ([]FrozenSpendProof, error) {
	   casID = strings.TrimSpace(casID)
	  r.mu.Lock()
	   defer r.mu.Unlock()
	   out := make([]FrozenSpendProof, 0)
	  for _, row := range r.byID {
		   if row.FreezeCASID == casID {
			  out = append(out, row.Copy())
		  }
	  }
	  sort.Slice(out, func(i, j int) bool { return out[i].DetectedAt.After(out[j].DetectedAt) })
	  if limit > 0 && len(out) > limit {
		  out = out[:limit]
	   }
	   return out, nil
   }

// MemRevokeRepo 进程内速度窗吊销仓。

type MemRevokeRepo struct {
	mu   sync.Mutex
	byID map[string]*VelocityRevocation
}

  func NewMemRevokeRepo() *MemRevokeRepo {
	   return &MemRevokeRepo{byID: map[string]*VelocityRevocation{}}
  }

  func (r *MemRevokeRepo) Save(_ context.Context, row *VelocityRevocation) error {
	  if r == nil || row == nil || row.RevokeID == "" {
		  return ErrInvalidInput
	   }
	   r.mu.Lock()
	  defer r.mu.Unlock()
	  cp := row.Copy()
	   r.byID[row.RevokeID] = &cp
	  return nil
   }

   func (r *MemRevokeRepo) Get(_ context.Context, revokeID string) (*VelocityRevocation, error) {
	   r.mu.Lock()
	   defer r.mu.Unlock()
	  got, ok := r.byID[strings.TrimSpace(revokeID)]
	   if !ok {
		   return nil, ErrRevokeNotFound
	  }
	  cp := got.Copy()
	  return &cp, nil
  }

  func (r *MemRevokeRepo) ListActiveByOwner(_ context.Context, ownerID string) ([]VelocityRevocation, error) {
	   ownerID = strings.TrimSpace(ownerID)
	  r.mu.Lock()
	  defer r.mu.Unlock()
	  out := make([]VelocityRevocation, 0)
	  for _, row := range r.byID {
		  if row.OwnerID == ownerID && row.Active {
			  out = append(out, row.Copy())
		   }
	  }
	  sort.Slice(out, func(i, j int) bool { return out[i].FrozenAt.After(out[j].FrozenAt) })
	  return out, nil
   }

   func (r *MemRevokeRepo) ListByCAS(_ context.Context, casID string) ([]VelocityRevocation, error) {
	  casID = strings.TrimSpace(casID)
	   r.mu.Lock()
	   defer r.mu.Unlock()
	  out := make([]VelocityRevocation, 0)
	   for _, row := range r.byID {
		  if row.FreezeCASID == casID {
			   out = append(out, row.Copy())
		  }
	  }
	  return out, nil
  }

  func (r *MemRevokeRepo) ListActive(_ context.Context, limit int) ([]VelocityRevocation, error) {
	   r.mu.Lock()
	   defer r.mu.Unlock()
	  out := make([]VelocityRevocation, 0)
	   for _, row := range r.byID {
		  if row.Active {
			   out = append(out, row.Copy())
		   }
	   }
	  sort.Slice(out, func(i, j int) bool { return out[i].FrozenAt.After(out[j].FrozenAt) })
	   if limit > 0 && len(out) > limit {
		  out = out[:limit]
	  }
	  return out, nil
   }

// MemAuditRepo 进程内审计仓。
type MemAuditRepo struct {
	mu sync.Mutex
	rows []FreezeAudit
}

   func NewMemAuditRepo() *MemAuditRepo { return &MemAuditRepo{} }

   func (r *MemAuditRepo) Append(_ context.Context, row FreezeAudit) error {
	   if r == nil || row.AuditID == "" {
		  return ErrInvalidInput
	   }
	   r.mu.Lock()
	   defer r.mu.Unlock()
	  r.rows = append(r.rows, row.Copy())
	   return nil
   }

  func (r *MemAuditRepo) ListByParent(_ context.Context, parentID string, limit int) ([]FreezeAudit, error) {
	   parentID = strings.TrimSpace(parentID)
	  r.mu.Lock()
	   defer r.mu.Unlock()
	  out := make([]FreezeAudit, 0)
	  for i := len(r.rows) - 1; i >= 0; i-- {
		  if r.rows[i].ParentID == parentID {
			   out = append(out, r.rows[i].Copy())
			   if limit > 0 && len(out) >= limit {
				   break
			  }
		  }
	  }
	  return out, nil
  }

  func (r *MemAuditRepo) ListByCAS(_ context.Context, casID string, limit int) ([]FreezeAudit, error) {
	  casID = strings.TrimSpace(casID)
	   r.mu.Lock()
	  defer r.mu.Unlock()
	   out := make([]FreezeAudit, 0)
	  for i := len(r.rows) - 1; i >= 0; i-- {
		   if r.rows[i].FreezeCASID == casID {
			   out = append(out, r.rows[i].Copy())
			  if limit > 0 && len(out) >= limit {
				   break
			   }
		  }
	  }
	  return out, nil
  }

// MemJournalRepo 进程内借记轨迹。

type MemJournalRepo struct {
	mu sync.Mutex
	rows []DebitJournal
}

   func NewMemJournalRepo() *MemJournalRepo { return &MemJournalRepo{} }

  func (r *MemJournalRepo) Save(_ context.Context, row *DebitJournal) error {
	   if r == nil || row == nil || row.JournalID == "" {
		   return ErrInvalidInput
	   }
	  r.mu.Lock()
	  defer r.mu.Unlock()
	  r.rows = append(r.rows,
		  row.Copy())
	  return nil
  }

   func (r *MemJournalRepo) ListByOwner(_ context.Context, ownerID string, since time.Time, limit int) ([]DebitJournal, error) {
	   ownerID = strings.TrimSpace(ownerID)
	   r.mu.Lock()
	  defer r.mu.Unlock()
	   out := make([]DebitJournal, 0)
	  for i := len(r.rows) - 1; i >= 0; i-- {
		   if r.rows[i].OwnerID != ownerID {
			  continue
		  }
		   if !since.IsZero() && r.rows[i].At.Before(since) {
			   continue
		  }
		  out = append(out, r.rows[i].Copy())
		  if limit > 0 && len(out) >= limit {
			   break
		   }
	  }
	  return out, nil
   }

   func (r *MemJournalRepo) ListAfter(_ context.Context, since time.Time, limit int) ([]DebitJournal, error) {
	  r.mu.Lock()
	  defer r.mu.Unlock()
	   out := make([]DebitJournal, 0)
	   for i := len(r.rows) - 1; i >= 0; i-- {
		  if !since.IsZero() && r.rows[i].At.Before(since) {
			  continue
		   }
		   out = append(out, r.rows[i].Copy())
		  if limit > 0 && len(out) >= limit {
			  break
		   }
	   }
	  return out, nil
  }
