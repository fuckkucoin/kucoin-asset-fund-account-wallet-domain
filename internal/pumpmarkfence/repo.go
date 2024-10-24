package pumpmarkfence

import (
	"context"
	"sort"
	"strings"
	"sync"
)

type GenRepo interface {
	Save(ctx context.Context,
		row *DeliverEffectGen) error
	Get(ctx context.Context, genID string) (*DeliverEffectGen, error)
	GetByOutbox(ctx context.Context, outboxID string) (*DeliverEffectGen, error)
	GetByEffect(ctx context.Context, effectKey string) (*DeliverEffectGen, error)
	ListLive(ctx context.Context, limit int) ([]DeliverEffectGen, error)
	ListByState(ctx context.Context, state GenState, limit int) ([]DeliverEffectGen, error)
}

type HalfRepo interface {
	Save(ctx context.Context, row *HalfMarkCase) error
	Get(ctx context.Context, caseID string) (*HalfMarkCase, error)
	GetByGen(ctx context.Context, genID string) (*HalfMarkCase, error)
	GetOpenByOutbox(ctx context.Context, outboxID string) (*HalfMarkCase, error)
	ListOpen(ctx context.Context, limit int) ([]HalfMarkCase, error)
}

type ProofRepo interface {
	Save(ctx context.Context, row *OutboxDupProof) error
	Get(ctx context.Context, proofID string) (*OutboxDupProof, error)
	ListUnexported(ctx context.Context, limit int) ([]OutboxDupProof, error)
	ListByOutbox(ctx context.Context, outboxID string, limit int) ([]OutboxDupProof, error)
	ListByKind(ctx context.Context, kind ProofKind, limit int) ([]OutboxDupProof, error)
}

type AppliedRepo interface {
	Put(ctx context.Context, row *AppliedEffect) error
	Get(ctx context.Context, effectKey string) (*AppliedEffect, error)
}

type AuditRepo interface {
	Append(ctx context.Context, row FenceAudit) error
	ListByGen(ctx context.Context,
		genID string, limit int) ([]FenceAudit, error)
	ListByOutbox(ctx context.Context, outboxID string, limit int) ([]FenceAudit, error)
}

// MemGenRepo 进程内世代仓。
type MemGenRepo struct {
	mu sync.Mutex
	byID map[string]*DeliverEffectGen
}

   func NewMemGenRepo() *MemGenRepo { return &MemGenRepo{byID: map[string]*DeliverEffectGen{}} }

   func (r *MemGenRepo) Save(_ context.Context, row *DeliverEffectGen) error {
	  if r == nil || row == nil || strings.TrimSpace(row.GenID) == "" {
		   return ErrInvalidInput
	  }
	  r.mu.Lock()
	  defer r.mu.Unlock()
	  cp := row.Copy()
	   r.byID[row.GenID] = &cp
	   return nil
  }

  func (r *MemGenRepo) Get(_ context.Context, genID string) (*DeliverEffectGen, error) {
	  if r == nil {
		   return nil, ErrUnavailable
	   }
	   r.mu.Lock()
	   defer r.mu.Unlock()
	  row := r.byID[strings.TrimSpace(genID)]
	  if row == nil {
		  return nil, ErrGenNotFound
	  }
	   cp := row.Copy()
	   return &cp, nil
   }

   func (r *MemGenRepo) GetByOutbox(_ context.Context, outboxID string) (*DeliverEffectGen, error) {
	   if r == nil {
		   return nil, ErrUnavailable
	  }
	   outboxID = strings.TrimSpace(outboxID)
	  r.mu.Lock()
	   defer r.mu.Unlock()
	   var best *DeliverEffectGen
	  for _, row := range r.byID {
		  if row.OutboxID != outboxID {
			   continue
		  }
		   if best == nil || row.Generation > best.Generation || (row.Generation == best.Generation && row.IssuedAt.After(best.IssuedAt)) {
			  cp := row.Copy()
			  best = &cp
		  }
	  }
	   if best == nil {
		   return nil, ErrGenNotFound
	   }
	   return best, nil
   }

  func (r *MemGenRepo) GetByEffect(_ context.Context, effectKey string) (*DeliverEffectGen, error) {
	  if r == nil {
		  return nil, ErrUnavailable
	  }
	  effectKey = strings.TrimSpace(effectKey)
	  r.mu.Lock()
	  defer r.mu.Unlock()
	  var best *DeliverEffectGen
	  for _, row := range r.byID {
		   if row.EffectKey != effectKey {
			   continue
		   }
		   if best == nil || row.Generation > best.Generation {
			  cp := row.Copy()
			  best = &cp
		  }
	  }
	   if best == nil {
		   return nil, ErrGenNotFound
	  }
	   return best, nil
  }

   func (r *MemGenRepo) ListLive(_ context.Context, limit int) ([]DeliverEffectGen, error) {
	   return r.listPred(limit,
		   func(g DeliverEffectGen) bool { return g.State.Live() })
  }

   func (r *MemGenRepo) ListByState(_ context.Context, state GenState, limit int) ([]DeliverEffectGen, error) {
	   return r.listPred(limit, func(g DeliverEffectGen) bool { return g.State == state })
   }

  func (r *MemGenRepo) listPred(limit int, pred func(DeliverEffectGen) bool) ([]DeliverEffectGen, error) {
	  if r == nil {
		  return nil, ErrUnavailable
	   }
	   if limit <= 0 {
		   limit = 80
	  }
	   r.mu.Lock()
	  defer r.mu.Unlock()
	  out := make([]DeliverEffectGen, 0)
	   for _, row := range r.byID {
		  if pred(*row) {
			  out = append(out, row.Copy())
		  }
	  }
	  sort.Slice(out, func(i, j int) bool {
		   if out[i].IssuedAt.Equal(out[j].IssuedAt) {
			  return out[i].GenID < out[j].GenID
		  }
		   return out[i].IssuedAt.Before(out[j].IssuedAt)
	   })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// MemHalfRepo 半截案仓。
type MemHalfRepo struct {
	mu   sync.Mutex
	byID map[string]*HalfMarkCase
}

   func NewMemHalfRepo() *MemHalfRepo { return &MemHalfRepo{byID: map[string]*HalfMarkCase{}} }

  func (r *MemHalfRepo) Save(_ context.Context, row *HalfMarkCase) error {
	   if r == nil || row == nil || strings.TrimSpace(row.CaseID) == "" {
		  return ErrInvalidInput
	  }
	  r.mu.Lock()
	   defer r.mu.Unlock()
	   cp := row.Copy()
	  r.byID[row.CaseID] = &cp
	  return nil
   }

  func (r *MemHalfRepo) Get(_ context.Context, caseID string) (*HalfMarkCase, error) {
	  if r == nil {
		   return nil, ErrUnavailable
	   }
	   r.mu.Lock()
	   defer r.mu.Unlock()
	   row := r.byID[strings.TrimSpace(caseID)]
	   if row == nil {
		   return nil, ErrHalfMarkNotFound
	   }
	   cp := row.Copy()
	  return &cp, nil
   }

   func (r *MemHalfRepo) GetByGen(_ context.Context, genID string) (*HalfMarkCase, error) {
	   if r == nil {
		  return nil, ErrUnavailable
	   }
	   genID = strings.TrimSpace(genID)
	   r.mu.Lock()
	   defer r.mu.Unlock()
	  var open *HalfMarkCase
	  var any *HalfMarkCase
	  for _, row := range r.byID {
		   if row.GenID != genID {
			   continue
		   }
		  cp := row.Copy()
		  any = &cp
		   if row.State.Open() {
			   open = &cp
		   }
	   }
	   if open != nil {
		  return open, nil
	  }
	   if any != nil {
		   return any, nil
	   }
	   return nil, ErrHalfMarkNotFound
   }

   func (r *MemHalfRepo) GetOpenByOutbox(_ context.Context, outboxID string) (*HalfMarkCase, error) {
	   if r == nil {
		  return nil, ErrUnavailable
	   }
	   outboxID = strings.TrimSpace(outboxID)
	  r.mu.Lock()
	  defer r.mu.Unlock()
	   for _, row := range r.byID {
		   if row.OutboxID == outboxID && row.State.Open() {
			  cp := row.Copy()
			  return &cp, nil
		  }
	   }
	  return nil, ErrHalfMarkNotFound
  }

   func (r *MemHalfRepo) ListOpen(_ context.Context, limit int) ([]HalfMarkCase, error) {
	   if r == nil {
		   return nil, ErrUnavailable
	   }
	   if limit <= 0 {
		   limit = 80
	  }
	   r.mu.Lock()
	  defer r.mu.Unlock()
	   out := make([]HalfMarkCase, 0)
	   for _, row := range r.byID {
		  if row.State.Open() {
			   out = append(out, row.Copy())
		   }
	  }
	   sort.Slice(out, func(i, j int) bool { return out[i].OpenedAt.Before(out[j].OpenedAt) })
	   if len(out) > limit {
		  out = out[:limit]
	   }
	  return out, nil
   }

// MemProofRepo 双效果证明仓。
type MemProofRepo struct {
	mu sync.Mutex
	byID map[string]*OutboxDupProof
}

  func NewMemProofRepo() *MemProofRepo { return &MemProofRepo{byID: map[string]*OutboxDupProof{}} }

   func (r *MemProofRepo) Save(_ context.Context, row *OutboxDupProof) error {
	  if r == nil || row == nil || strings.TrimSpace(row.ProofID) == "" {
		   return ErrInvalidInput
	   }
	   r.mu.Lock()
	  defer r.mu.Unlock()
	  cp := row.Copy()
	  r.byID[row.ProofID] = &cp
	   return nil
   }

   func (r *MemProofRepo) Get(_ context.Context, proofID string) (*OutboxDupProof, error) {
	   if r == nil {
		  return nil, ErrUnavailable
	  }
	  r.mu.Lock()
	   defer r.mu.Unlock()
	   row := r.byID[strings.TrimSpace(proofID)]
	   if row == nil {
		   return nil, ErrProofNotFound
	  }
	   cp := row.Copy()
	  return &cp, nil
   }

  func (r *MemProofRepo) ListUnexported(_ context.Context, limit int) ([]OutboxDupProof, error) {
	   return r.listPred(limit, func(p OutboxDupProof) bool { return !p.Exported })
   }

  func (r *MemProofRepo) ListByOutbox(_ context.Context, outboxID string, limit int) ([]OutboxDupProof, error) {
	  outboxID = strings.TrimSpace(outboxID)
	   return r.listPred(limit, func(p OutboxDupProof) bool { return p.OutboxID == outboxID })
   }

   func (r *MemProofRepo) ListByKind(_ context.Context, kind ProofKind, limit int) ([]OutboxDupProof, error) {
	   return r.listPred(limit, func(p OutboxDupProof) bool { return p.Kind == kind })
  }

  func (r *MemProofRepo) listPred(limit int, pred func(OutboxDupProof) bool) ([]OutboxDupProof, error) {
	  if r == nil {
		   return nil, ErrUnavailable
	  }
	   if limit <= 0 {
		  limit = 80
	   }
	  r.mu.Lock()
	  defer r.mu.Unlock()
	  out := make([]OutboxDupProof, 0)
	  for _, row := range r.byID {
		  if pred(*row) {
			  out = append(out, row.Copy())
		  }
	   }
	  sort.Slice(out, func(i, j int) bool { return out[i].DetectedAt.Before(out[j].DetectedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// MemAppliedRepo 已落地效果键。

type MemAppliedRepo struct {
	mu  sync.Mutex
	byK map[string]*AppliedEffect
}

   func NewMemAppliedRepo() *MemAppliedRepo { return &MemAppliedRepo{byK: map[string]*AppliedEffect{}} }

   func (r *MemAppliedRepo) Put(_ context.Context, row *AppliedEffect) error {
	  if r == nil || row == nil || strings.TrimSpace(row.EffectKey) == "" {
		   return ErrInvalidInput
	  }
	  r.mu.Lock()
	  defer r.mu.Unlock()
	   if prev := r.byK[row.EffectKey]; prev != nil {
		  return ErrAlreadyApplied
	  }
	   cp := row.Copy()
	   r.byK[row.EffectKey] = &cp
	  return nil
   }

  func (r *MemAppliedRepo) Get(_ context.Context, effectKey string) (*AppliedEffect, error) {
	   if r == nil {
		   return nil, ErrUnavailable
	   }
	   r.mu.Lock()
	   defer r.mu.Unlock()
	   row := r.byK[strings.TrimSpace(effectKey)]
	  if row == nil {
		  return nil, ErrEffectKey
	  }
	  cp := row.Copy()
	   return &cp, nil
  }

// MemAuditRepo 审计。
type MemAuditRepo struct {
	mu   sync.Mutex
	rows []FenceAudit
}

   func NewMemAuditRepo() *MemAuditRepo { return &MemAuditRepo{} }

  func (r *MemAuditRepo) Append(_ context.Context, row FenceAudit) error {
	  if r == nil || strings.TrimSpace(row.AuditID) == "" {
		  return ErrInvalidInput
	  }
	  r.mu.Lock()
	  defer r.mu.Unlock()
	   r.rows = append(r.rows, row.Copy())
	  return nil
   }

  func (r *MemAuditRepo) ListByGen(_ context.Context, genID string, limit int) ([]FenceAudit, error) {
	   return r.list(limit, func(a FenceAudit) bool { return a.GenID == strings.TrimSpace(genID) })
  }

   func (r *MemAuditRepo) ListByOutbox(_ context.Context, outboxID string, limit int) ([]FenceAudit, error) {
	  return r.list(limit, func(a FenceAudit) bool { return a.OutboxID == strings.TrimSpace(outboxID) })
   }

  func (r *MemAuditRepo) list(limit int, pred func(FenceAudit) bool) ([]FenceAudit, error) {
	  if r == nil {
		  return nil, ErrUnavailable
	   }
	  if limit <= 0 {
		   limit = 80
	  }
	   r.mu.Lock()
	  defer r.mu.Unlock()
	  out := make([]FenceAudit, 0)
	   for _, row := range r.rows {
		  if pred(row) {
			  out = append(out, row.Copy())
		   }
	   }
	  sort.Slice(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
