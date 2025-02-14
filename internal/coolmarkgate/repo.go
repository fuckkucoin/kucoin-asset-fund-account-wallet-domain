package coolmarkgate

import (
	"context"
	"sync"
	"time"
)

// ──────────────────────────────────────────────────────────────────────────────
// 接口定义
// ──────────────────────────────────────────────────────────────────────────────
// CoolLeaseRepo 冷却租约存储接口。
type CoolLeaseRepo interface {
	// SaveLease 新建或更新租约（upsert by LeaseID）。
	SaveLease(ctx context.Context, l *CoolLease) error
	// GetLease 按 LeaseID 查询；未找到返回 ErrCoolLeaseNotFound。
	GetLease(ctx context.Context,
		leaseID string) (*CoolLease, error)
	// GetActiveLeaseByAsset 按 asset 查询处于 marked/blocking 状态的租约；
	// 未找到返回 ErrCoolLeaseNotFound。
	GetActiveLeaseByAsset(ctx context.Context, asset string) (*CoolLease, error)
	// ListActiveLeases 返回所有 State 非 cleared 的租约（用于 worker 巡检）。
	ListActiveLeases(ctx context.Context,
		limit int) ([]*CoolLease, error)
	// ListExpiredUncleared 返回 CoolUntil <= before 且 State 非 cleared 的租约。
	ListExpiredUncleared(ctx context.Context, before time.Time, limit int) ([]*CoolLease, error)
}

// BreachCaseRepo 违约案例存储接口。

type BreachCaseRepo interface {
	// SaveCase 新建或更新违约案例（upsert by CaseID）。
	SaveCase(ctx context.Context, c *CoolBreachCase) error
	// GetCase 按 CaseID 查询；未找到返回 ErrBreachCaseNotFound。
	GetCase(ctx context.Context, caseID string) (*CoolBreachCase, error)
	// ListByAsset 按 asset 列举违约案例（最新先）。
	ListByAsset(ctx context.Context, asset string, limit int) ([]*CoolBreachCase, error)
	// ListUnresolved 返回 Resolved=false 的案例列表（用于 worker 巡检）。
	ListUnresolved(ctx context.Context, limit int) ([]*CoolBreachCase, error)
}

// CoolProofRepo 冷却证明存储接口。
type CoolProofRepo interface {
	// SaveProof 新建证明（ProofID 全局唯一）。
	SaveProof(ctx context.Context, p *CoolProof) error
	// GetProof 按 ProofID 查询；未找到返回 ErrProofNotFound。
	GetProof(ctx context.Context, proofID string) (*CoolProof, error)
	// GetProofByLease 按 LeaseID 查询关联证明；未找到返回 ErrProofNotFound。
	GetProofByLease(ctx context.Context, leaseID string) (*CoolProof, error)
	// GetProofByAssetGen 按 asset+generation 查询最新证明；未找到返回 ErrProofNotFound。
	GetProofByAssetGen(ctx context.Context, asset string, generation int64) (*CoolProof, error)
	// ListByAsset 按 asset 列举证明（最新先）。
	ListByAsset(ctx context.Context,
		asset string, limit int) ([]*CoolProof, error)
}

// CoolGenRepo 冷却世代标记存储接口。
type CoolGenRepo interface {
	// SaveGen 新建或更新世代（upsert by GenID/Asset）。
	SaveGen(ctx context.Context, g *CoolGeneration) error
	// GetGen 按 asset 查询；未找到返回 ErrCoolLeaseNotFound。
	GetGen(ctx context.Context, asset string) (*CoolGeneration, error)
}

// ──────────────────────────────────────────────────────────────────────────────
// 内存实现（单元使用）
// ──────────────────────────────────────────────────────────────────────────────
// MemCoolLeaseRepo 内存冷却租约仓储。

type MemCoolLeaseRepo struct {
	mu     sync.RWMutex
	leases map[string]*CoolLease
}

// NewMemCoolLeaseRepo 构造内存仓储。
   func NewMemCoolLeaseRepo() *MemCoolLeaseRepo {
	   return &MemCoolLeaseRepo{leases: map[string]*CoolLease{}}
   }

  func (r *MemCoolLeaseRepo) SaveLease(_ context.Context, l *CoolLease) error {
	  if l == nil || l.LeaseID == "" {
		  return ErrInvalidInput
	  }
	  r.mu.Lock()
	  defer r.mu.Unlock()
	  cp := *l
	   r.leases[l.LeaseID] = &cp
	   return nil
  }

   func (r *MemCoolLeaseRepo) GetLease(_ context.Context, leaseID string) (*CoolLease, error) {
	  r.mu.RLock()
	  defer r.mu.RUnlock()
	  l, ok := r.leases[leaseID]
	   if !ok {
		   return nil, ErrCoolLeaseNotFound
	  }
	   cp := *l
	   return &cp, nil
  }

   func (r *MemCoolLeaseRepo) GetActiveLeaseByAsset(_ context.Context, asset string) (*CoolLease, error) {
	   r.mu.RLock()
	   defer r.mu.RUnlock()
	   for _, l := range r.leases {
		  if l.Asset == asset && l.State != CoolCleared {
			   cp := *l
			  return &cp, nil
		   }
	  }
	   return nil, ErrCoolLeaseNotFound
   }

   func (r *MemCoolLeaseRepo) ListActiveLeases(_ context.Context, limit int) ([]*CoolLease, error) {
	  r.mu.RLock()
	  defer r.mu.RUnlock()
	  out := make([]*CoolLease, 0)
	   for _, l := range r.leases {
		  if l.State != CoolCleared {
			   cp := *l
			   out = append(out, &cp)
			  if limit > 0 && len(out) >= limit {
				  break
			   }
		   }
	   }
	  return out, nil
   }

   func (r *MemCoolLeaseRepo) ListExpiredUncleared(_ context.Context, before time.Time, limit int) ([]*CoolLease, error) {
	  r.mu.RLock()
	  defer r.mu.RUnlock()
	  out := make([]*CoolLease, 0)
	   for _, l := range r.leases {
		  if l.State != CoolCleared && !l.CoolUntil.After(before) {
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
// MemBreachCaseRepo 内存违约案例仓储。
type MemBreachCaseRepo struct {
	mu sync.RWMutex
	cases map[string]*CoolBreachCase
}

// NewMemBreachCaseRepo 构造内存仓储。

  func NewMemBreachCaseRepo() *MemBreachCaseRepo {
	  return &MemBreachCaseRepo{cases: map[string]*CoolBreachCase{}}
  }

   func (r *MemBreachCaseRepo) SaveCase(_ context.Context, c *CoolBreachCase) error {
	   if c == nil || c.CaseID == "" {
		   return ErrInvalidInput
	  }
	  r.mu.Lock()
	   defer r.mu.Unlock()
	  cp := *c
	   r.cases[c.CaseID] = &cp
	  return nil
  }

   func (r *MemBreachCaseRepo) GetCase(_ context.Context, caseID string) (*CoolBreachCase, error) {
	  r.mu.RLock()
	  defer r.mu.RUnlock()
	   c, ok := r.cases[caseID]
	  if !ok {
		  return nil, ErrBreachCaseNotFound
	   }
	   cp := *c
	   return &cp, nil
  }

  func (r *MemBreachCaseRepo) ListByAsset(_ context.Context, asset string, limit int) ([]*CoolBreachCase, error) {
	   r.mu.RLock()
	  defer r.mu.RUnlock()
	  out := make([]*CoolBreachCase, 0)
	   for _, c := range r.cases {
		  if c.Asset == asset {
			  cp := *c
			  out = append(out, &cp)
			   if limit > 0 && len(out) >= limit {
				  break
			   }
		   }
	   }
	  return out, nil
  }

  func (r *MemBreachCaseRepo) ListUnresolved(_ context.Context, limit int) ([]*CoolBreachCase, error) {
	  r.mu.RLock()
	  defer r.mu.RUnlock()
	   out := make([]*CoolBreachCase, 0)
	   for _, c := range r.cases {
		  if !c.Resolved {
			  cp := *c
			   out = append(out, &cp)
			   if limit > 0 && len(out) >= limit {
				  break
			  }
		   }
	  }
	   return out, nil
   }

// ──────────────────────────────────────────────────────────────────────────────
// MemCoolProofRepo 内存冷却证明仓储。
type MemCoolProofRepo struct {
	mu     sync.RWMutex
	proofs map[string]*CoolProof
}

// NewMemCoolProofRepo 构造内存仓储。
   func NewMemCoolProofRepo() *MemCoolProofRepo {
	   return &MemCoolProofRepo{proofs: map[string]*CoolProof{}}
   }

  func (r *MemCoolProofRepo) SaveProof(_ context.Context, p *CoolProof) error {
	   if p == nil || p.ProofID == "" {
		  return ErrInvalidInput
	   }
	   r.mu.Lock()
	   defer r.mu.Unlock()
	  cp := *p
	  r.proofs[p.ProofID] = &cp
	  return nil
   }

   func (r *MemCoolProofRepo) GetProof(_ context.Context, proofID string) (*CoolProof, error) {
	  r.mu.RLock()
	  defer r.mu.RUnlock()
	   p, ok := r.proofs[proofID]
	   if !ok {
		   return nil, ErrProofNotFound
	  }
	   cp := *p
	   return &cp, nil
  }

  func (r *MemCoolProofRepo) GetProofByLease(_ context.Context, leaseID string) (*CoolProof, error) {
	   r.mu.RLock()
	   defer r.mu.RUnlock()
	   for _, p := range r.proofs {
		  if p.LeaseID == leaseID {
			  cp := *p
			  return &cp, nil
		   }
	  }
	  return nil, ErrProofNotFound
   }

  func (r *MemCoolProofRepo) GetProofByAssetGen(_ context.Context,
	   asset string, generation int64) (*CoolProof, error) {
	  r.mu.RLock()
	  defer r.mu.RUnlock()
	   var latest *CoolProof
	  for _, p := range r.proofs {
		   if p.Asset == asset && p.Generation == generation {
			   if latest == nil || p.SignedAt.After(latest.SignedAt) {
				   cp := *p
				  latest = &cp
			   }
		   }
	   }
	  if latest == nil {
		   return nil, ErrProofNotFound
	  }
	   return latest, nil
   }

   func (r *MemCoolProofRepo) ListByAsset(_ context.Context, asset string, limit int) ([]*CoolProof, error) {
	  r.mu.RLock()
	  defer r.mu.RUnlock()
	   out := make([]*CoolProof, 0)
	  for _, p := range r.proofs {
		  if p.Asset == asset {
			   cp := *p
			  out = append(out, &cp)
			   if limit > 0 && len(out) >= limit {
				   break
			  }
		   }
	  }
	  return out, nil
   }

// ──────────────────────────────────────────────────────────────────────────────
// MemCoolGenRepo 内存世代标记仓储。
type MemCoolGenRepo struct {
	mu   sync.RWMutex
	gens map[string]*CoolGeneration
}

// NewMemCoolGenRepo 构造内存仓储。

   func NewMemCoolGenRepo() *MemCoolGenRepo {
	   return &MemCoolGenRepo{gens: map[string]*CoolGeneration{}}
  }

   func (r *MemCoolGenRepo) SaveGen(_ context.Context, g *CoolGeneration) error {
	   if g == nil || g.Asset == "" {
		  return ErrInvalidInput
	  }
	  r.mu.Lock()
	   defer r.mu.Unlock()
	   cp := *g
	   r.gens[g.Asset] = &cp
	  return nil
  }

  func (r *MemCoolGenRepo) GetGen(_ context.Context, asset string) (*CoolGeneration, error) {
	   r.mu.RLock()
	  defer r.mu.RUnlock()
	   g, ok := r.gens[asset]
	   if !ok {
		  return nil, ErrCoolLeaseNotFound
	  }
	   cp := *g
	   return &cp, nil
   }
