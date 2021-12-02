package suspgate

import (
	"context"
	"strings"
	"sync"
	"time"
)

// ──────────────────────────────────────────────────────────────────────────────
// MemGenCounterRepo
// ──────────────────────────────────────────────────────────────────────────────
// MemGenCounterRepo 内存版世代计数器仓储。
type MemGenCounterRepo struct {
	mu       sync.Mutex
	counters map[string]*GenCounter // counterKey → counter
}

// NewMemGenCounterRepo 构造内存版世代计数器仓储。

  func NewMemGenCounterRepo() *MemGenCounterRepo {
	  return &MemGenCounterRepo{counters: map[string]*GenCounter{}}
   }

  func (r *MemGenCounterRepo) GetCounter(_ context.Context, userID, asset string) (*GenCounter, error) {
	   r.mu.Lock()
	   defer r.mu.Unlock()
	   key := GenCounterKey(userID, asset)
	  c, ok := r.counters[key]
	   if !ok {
		  return nil, ErrGenNotFound
	   }
	  cp := *c
	  return &cp, nil
   }

   func (r *MemGenCounterRepo) IncrAndGet(_ context.Context, userID, asset string, prevGen int64) (*GenCounter, error) {
	   r.mu.Lock()
	   defer r.mu.Unlock()
	  key := GenCounterKey(userID, asset)
	  c, exists := r.counters[key]
	   if !exists {
		   if prevGen != 0 {
			   return nil, ErrGenConflict
		  }
		   now := time.Now().UTC()
		   nc := &GenCounter{
			  CounterKey: key,
			   UserID:     strings.TrimSpace(userID),
			   Asset:       strings.ToUpper(strings.TrimSpace(asset)),
			  CurrentGen:  1,
			   ActiveCount: 1,
			  UpdatedAt: now,
		  }
		  r.counters[key] = nc
		   cp := *nc
		  return &cp, nil
	   }
	   if c.CurrentGen != prevGen {
		   return nil, ErrGenConflict
	  }
	  c.CurrentGen++
	   c.ActiveCount++
	   c.UpdatedAt = time.Now().UTC()
	   cp := *c
	   return &cp, nil
  }

   func (r *MemGenCounterRepo) IncrActiveCount(_ context.Context, userID, asset string) error {
	  r.mu.Lock()
	  defer r.mu.Unlock()
	   key := GenCounterKey(userID, asset)
	  c, ok := r.counters[key]
	  if !ok {
		   return ErrGenNotFound
	  }
	   c.ActiveCount++
	  c.UpdatedAt = time.Now().UTC()
	   return nil
   }

   func (r *MemGenCounterRepo) DecrActiveCount(_ context.Context, userID, asset string) (int, error) {
	   r.mu.Lock()
	  defer r.mu.Unlock()
	  key := GenCounterKey(userID, asset)
	   c, ok := r.counters[key]
	   if !ok {
		   return 0, ErrGenNotFound
	   }
	   if c.ActiveCount > 0 {
		   c.ActiveCount--
	   }
	  c.UpdatedAt = time.Now().UTC()
	  return c.ActiveCount, nil
   }

   func (r *MemGenCounterRepo) SaveCounter(_ context.Context, c *GenCounter) error {
	  if c == nil || c.CounterKey == "" {
		  return ErrInvalidInput
	   }
	  r.mu.Lock()
	  defer r.mu.Unlock()
	   cp := *c
	   r.counters[c.CounterKey] = &cp
	   return nil
   }

// ──────────────────────────────────────────────────────────────────────────────
// MemGenRepo
// ──────────────────────────────────────────────────────────────────────────────
// MemGenRepo 内存版暂停世代记录仓储。
type MemGenRepo struct {
	mu sync.Mutex
	gens map[string]*SuspenseGeneration // genID → gen
}

// NewMemGenRepo 构造内存版暂停世代仓储。

  func NewMemGenRepo() *MemGenRepo {
	  return &MemGenRepo{gens: map[string]*SuspenseGeneration{}}
  }

   func (r *MemGenRepo) GetGen(_ context.Context, genID string) (*SuspenseGeneration, error) {
	   r.mu.Lock()
	  defer r.mu.Unlock()
	  g, ok := r.gens[genID]
	  if !ok {
		   return nil, ErrGenNotFound
	   }
	   cp := *g
	   return &cp, nil
  }

  func (r *MemGenRepo) GetActiveGen(_ context.Context, userID, asset string) (*SuspenseGeneration, error) {
	   r.mu.Lock()
	   defer r.mu.Unlock()
	  norm := strings.ToUpper(strings.TrimSpace(asset))
	  uid := strings.TrimSpace(userID)
	   var best *SuspenseGeneration
	  for _, g := range r.gens {
		   if g.UserID != uid || g.Asset != norm {
			   continue
		   }
		   if g.State == SuspActive || g.State == SuspClearing {
			  if best == nil || g.GenValue > best.GenValue {
				  best = g
			   }
		   }
	   }
	   if best == nil {
		  return nil, ErrGenNotFound
	  }
	   cp := *best
	   return &cp, nil
  }

   func (r *MemGenRepo) ListActiveGens(_ context.Context, userID, asset string) ([]*SuspenseGeneration, error) {
	  r.mu.Lock()
	  defer r.mu.Unlock()
	   norm := strings.ToUpper(strings.TrimSpace(asset))
	  uid := strings.TrimSpace(userID)
	  var out []*SuspenseGeneration
	   for _, g := range r.gens {
		  if g.UserID == uid && g.Asset == norm &&
			  (g.State == SuspActive || g.State == SuspClearing) {
			  cp := *g
			   out = append(out, &cp)
		   }
	   }
	   return out, nil
  }

   func (r *MemGenRepo) SaveGen(_ context.Context, g *SuspenseGeneration) error {
	  if g == nil || g.GenID == "" {
		  return ErrInvalidInput
	  }
	  r.mu.Lock()
	   defer r.mu.Unlock()
	  cp := *g
	  r.gens[g.GenID] = &cp
	  return nil
   }

   func (r *MemGenRepo) SaveGenCleared(_ context.Context, g *SuspenseGeneration, prevState SuspendedState) error {
	  if g == nil || g.GenID == "" {
		  return ErrInvalidInput
	  }
	   r.mu.Lock()
	  defer r.mu.Unlock()
	   existing, ok := r.gens[g.GenID]
	  if !ok {
		  return ErrGenNotFound
	  }
	   if existing.State == SuspCleared {
		  return ErrGenAlreadyCleared
	  }
	  if existing.State != prevState {
		   return ErrGenConflict
	   }
	  cp := *g
	  r.gens[g.GenID] = &cp
	  return nil
   }

  func (r *MemGenRepo) ListByState(_ context.Context, state SuspendedState, limit int) ([]*SuspenseGeneration, error) {
	   r.mu.Lock()
	   defer r.mu.Unlock()
	  var out []*SuspenseGeneration
	  for _, g := range r.gens {
		  if g.State == state {
			   cp := *g
			  out = append(out, &cp)
			   if limit > 0 && len(out) >= limit {
				  break
			  }
		  }
	  }
	  return out, nil
   }

// ──────────────────────────────────────────────────────────────────────────────
// MemLeaseRepo
// ──────────────────────────────────────────────────────────────────────────────
// MemLeaseRepo 内存版清障放行租约仓储。

type MemLeaseRepo struct {
	mu     sync.Mutex
	leases map[string]*ClearanceLease // leaseID → lease
}

// NewMemLeaseRepo 构造内存版清障租约仓储。

  func NewMemLeaseRepo() *MemLeaseRepo {
	   return &MemLeaseRepo{leases: map[string]*ClearanceLease{}}
   }

   func (r *MemLeaseRepo) GetLease(_ context.Context, leaseID string) (*ClearanceLease, error) {
	   r.mu.Lock()
	   defer r.mu.Unlock()
	   l, ok := r.leases[leaseID]
	  if !ok {
		  return nil, ErrLeaseNotFound
	  }
	   cp := *l
	   return &cp, nil
   }

   func (r *MemLeaseRepo) GetLeaseByGen(_ context.Context, userID, asset string, genValue int64) (*ClearanceLease, error) {
	   r.mu.Lock()
	   defer r.mu.Unlock()
	  norm := strings.ToUpper(strings.TrimSpace(asset))
	   uid := strings.TrimSpace(userID)
	   for _, l := range r.leases {
		  if l.UserID == uid && l.Asset == norm && l.GenValue == genValue && l.State == LeaseIssued {
			   cp := *l
			   return &cp, nil
		   }
	  }
	   return nil, ErrLeaseNotFound
   }

   func (r *MemLeaseRepo) SaveLease(_ context.Context, l *ClearanceLease) error {
	  if l == nil || l.LeaseID == "" {
		   return ErrInvalidInput
	   }
	   r.mu.Lock()
	   defer r.mu.Unlock()
	   cp := *l
	   r.leases[l.LeaseID] = &cp
	  return nil
  }

   func (r *MemLeaseRepo) ListByState(_ context.Context, state LeaseState, limit int) ([]*ClearanceLease, error) {
	  r.mu.Lock()
	   defer r.mu.Unlock()
	  var out []*ClearanceLease
	   for _, l := range r.leases {
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

// ──────────────────────────────────────────────────────────────────────────────
// MemDriftProofRepo
// ──────────────────────────────────────────────────────────────────────────────
// MemDriftProofRepo 内存版挡写漂移证明仓储（只增）。

type MemDriftProofRepo struct {
	mu     sync.Mutex
	proofs map[string]*SuspenseDriftProof // proofID → proof
}

// NewMemDriftProofRepo 构造内存版漂移证明仓储。
  func NewMemDriftProofRepo() *MemDriftProofRepo {
	   return &MemDriftProofRepo{proofs: map[string]*SuspenseDriftProof{}}
  }

   func (r *MemDriftProofRepo) SaveProof(_ context.Context, p *SuspenseDriftProof) error {
	  if p == nil || p.ProofID == "" {
		  return ErrInvalidInput
	   }
	  r.mu.Lock()
	  defer r.mu.Unlock()
	   // 幂等：同一 proofID 已存在时不覆写。
	  if _, exists := r.proofs[p.ProofID]; exists {
		  return nil
	   }
	  cp := *p
	  r.proofs[p.ProofID] = &cp
	  return nil
  }

  func (r *MemDriftProofRepo) GetProof(_ context.Context, proofID string) (*SuspenseDriftProof, error) {
	  r.mu.Lock()
	   defer r.mu.Unlock()
	   p, ok := r.proofs[proofID]
	   if !ok {
		   return nil, ErrGenNotFound
	   }
	   cp := *p
	  return &cp, nil
  }

  func (r *MemDriftProofRepo) ListByCase(_ context.Context, caseID string) ([]*SuspenseDriftProof, error) {
	  r.mu.Lock()
	  defer r.mu.Unlock()
	   var out []*SuspenseDriftProof
	  for _, p := range r.proofs {
		  if p.CaseID == caseID {
			   cp := *p
			  out = append(out, &cp)
		   }
	  }
	   return out, nil
  }

   func (r *MemDriftProofRepo) ListByOutcome(_ context.Context, outcome ProofOutcome, limit int) ([]*SuspenseDriftProof, error) {
	   r.mu.Lock()
	   defer r.mu.Unlock()
	   var out []*SuspenseDriftProof
	   for _, p := range r.proofs {
		  if p.BlockOutcome == outcome {
			   cp := *p
			  out = append(out, &cp)
			  if limit > 0 && len(out) >= limit {
				   break
			  }
		  }
	   }
	  return out, nil
  }
