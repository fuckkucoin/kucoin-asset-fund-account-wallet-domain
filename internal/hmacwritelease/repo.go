package hmacwritelease

import (
	"context"
	"sync"
)

// ──────────────────────────────────────────────────────────────────────────────
// 仓储接口
// ──────────────────────────────────────────────────────────────────────────────
// LeaseRepo HmacWriteLease 持久化接口。

type LeaseRepo interface {
	// SaveLease 保存或更新租约（upsert by lease_id）。
	SaveLease(ctx context.Context, l *HmacWriteLease) error
	// GetLease 按 lease_id 查询；不存在返回 ErrLeaseNotFound。
	GetLease(ctx context.Context, leaseID string) (*HmacWriteLease, error)
	// ListByState 按状态查询租约（用于 worker 批量扫描）。
	ListByState(ctx context.Context,
		state LeaseState, limit int) ([]*HmacWriteLease, error)
	// ListIssuedExpired 查询已超过 ExpireAt 且状态为 issued 或 bound 的租约。
	ListIssuedExpired(ctx context.Context, limit int) ([]*HmacWriteLease, error)
	// ListByHaltGenLT 查询 HaltGen < currentGen 且状态非终态的租约（Halt 吊销批量扫描）。
	ListByHaltGenLT(ctx context.Context, currentGen int64, limit int) ([]*HmacWriteLease, error)
}

// NonceSeenRepo HmacNonceSeen 持久化接口。
type NonceSeenRepo interface {
	// SaveNonceSeen 保存 nonce 防重放记录；seen_id 唯一，重复写幂等。
	SaveNonceSeen(ctx context.Context, s *HmacNonceSeen) error
	// GetNonceSeen 按 seen_id 查询；不存在返回 ErrLeaseNotFound。
	GetNonceSeen(ctx context.Context,
		seenID string) (*HmacNonceSeen, error)
	// DeleteExpiredNonces 删除 ExpireAt 已过的 nonce 记录；由 worker 周期调用。
	DeleteExpiredNonces(ctx context.Context, limit int) (int, error)
}

// DriftProofRepo HmacWriteDriftProof 持久化接口（只增表）。
type DriftProofRepo interface {
	// SaveProof 保存漂移证明；禁止更新（只增）。
	SaveProof(ctx context.Context, p *HmacWriteDriftProof) error
	// GetProof 按 proof_id 查询；不存在返回 ErrLeaseNotFound。
	GetProof(ctx context.Context, proofID string) (*HmacWriteDriftProof, error)
	// ListByLeaseID 按 lease_id 查询漂移证明列表。
	ListByLeaseID(ctx context.Context, leaseID string, limit int) ([]*HmacWriteDriftProof, error)
	// ListByReason 按 reason 查询漂移证明列表（审计导出）。
	ListByReason(ctx context.Context, reason DriftReason, limit int) ([]*HmacWriteDriftProof, error)
}

// HaltGenRepo HaltGeneration 持久化接口。

type HaltGenRepo interface {
	// SaveHaltGen 保存或更新 HaltGeneration（upsert by gen_id）。
	SaveHaltGen(ctx context.Context, g *HaltGeneration) error
	// GetHaltGen 查询 HaltGeneration；不存在时返回零值（Generation=0, Halted=false）。
	GetHaltGen(ctx context.Context) (*HaltGeneration, error)
}

// ──────────────────────────────────────────────────────────────────────────────
// 内存实现（仅用于与内存模式）
// ──────────────────────────────────────────────────────────────────────────────
// MemLeaseRepo 基于内存 map 的 LeaseRepo 实现。

type MemLeaseRepo struct {
	mu     sync.RWMutex
	leases map[string]*HmacWriteLease
}

// NewMemLeaseRepo 构造内存租约仓储。
   func NewMemLeaseRepo() *MemLeaseRepo {
	   return &MemLeaseRepo{leases: map[string]*HmacWriteLease{}}
  }

  func (r *MemLeaseRepo) SaveLease(_ context.Context, l *HmacWriteLease) error {
	  if l == nil || l.LeaseID == "" {
		  return ErrInvalidInput
	   }
	  r.mu.Lock()
	   defer r.mu.Unlock()
	   cp := *l
	   r.leases[l.LeaseID] = &cp
	   return nil
  }

   func (r *MemLeaseRepo) GetLease(_ context.Context, leaseID string) (*HmacWriteLease, error) {
	  r.mu.RLock()
	   defer r.mu.RUnlock()
	  l, ok := r.leases[leaseID]
	  if !ok {
		   return nil, ErrLeaseNotFound
	  }
	   cp := *l
	   return &cp, nil
  }

   func (r *MemLeaseRepo) ListByState(_ context.Context, state LeaseState, limit int) ([]*HmacWriteLease, error) {
	   r.mu.RLock()
	  defer r.mu.RUnlock()
	   var out []*HmacWriteLease
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

   func (r *MemLeaseRepo) ListIssuedExpired(_ context.Context, limit int) ([]*HmacWriteLease, error) {
	   r.mu.RLock()
	  defer r.mu.RUnlock()
	   var out []*HmacWriteLease
	  for _, l := range r.leases {
		  if l.State == LeaseIssued || l.State == LeaseBound {
			  cp := *l
			   out = append(out, &cp)
			   if limit > 0 && len(out) >= limit {
				   break
			  }
		   }
	   }
	   return out, nil
  }

  func (r *MemLeaseRepo) ListByHaltGenLT(_ context.Context, currentGen int64, limit int) ([]*HmacWriteLease, error) {
	   r.mu.RLock()
	  defer r.mu.RUnlock()
	  var out []*HmacWriteLease
	  for _, l := range r.leases {
		  if !l.State.IsTerminal() && l.HaltGen < currentGen {
			  cp := *l
			   out = append(out, &cp)
			   if limit > 0 && len(out) >= limit {
				   break
			  }
		   }
	   }
	   return out, nil
  }

// MemNonceSeenRepo 基于内存 map 的 NonceSeenRepo 实现。

type MemNonceSeenRepo struct {
	mu     sync.RWMutex
	nonces map[string]*HmacNonceSeen
}

// NewMemNonceSeenRepo 构造内存 nonce 防重放仓储。
  func NewMemNonceSeenRepo() *MemNonceSeenRepo {
	   return &MemNonceSeenRepo{nonces: map[string]*HmacNonceSeen{}}
  }

   func (r *MemNonceSeenRepo) SaveNonceSeen(_ context.Context, s *HmacNonceSeen) error {
	  if s == nil || s.SeenID == "" {
		   return ErrInvalidInput
	  }
	   r.mu.Lock()
	  defer r.mu.Unlock()
	  if _, exists := r.nonces[s.SeenID]; !exists {
		   cp := *s
		  r.nonces[s.SeenID] = &cp
	  }
	   return nil
  }

   func (r *MemNonceSeenRepo) GetNonceSeen(_ context.Context, seenID string) (*HmacNonceSeen, error) {
	  r.mu.RLock()
	  defer r.mu.RUnlock()
	  s, ok := r.nonces[seenID]
	  if !ok {
		  return nil, ErrLeaseNotFound
	  }
	   cp := *s
	  return &cp, nil
  }

   func (r *MemNonceSeenRepo) DeleteExpiredNonces(_ context.Context, _ int) (int, error) {
	   return 0, nil
   }

// MemDriftProofRepo 基于内存 map 的 DriftProofRepo 实现。
type MemDriftProofRepo struct {
	mu     sync.RWMutex
	proofs map[string]*HmacWriteDriftProof
}

// NewMemDriftProofRepo 构造内存漂移证明仓储。
   func NewMemDriftProofRepo() *MemDriftProofRepo {
	   return &MemDriftProofRepo{proofs: map[string]*HmacWriteDriftProof{}}
  }

  func (r *MemDriftProofRepo) SaveProof(_ context.Context, p *HmacWriteDriftProof) error {
	   if p == nil || p.ProofID == "" {
		   return ErrInvalidInput
	  }
	   r.mu.Lock()
	   defer r.mu.Unlock()
	   cp := *p
	   r.proofs[p.ProofID] = &cp
	  return nil
   }

  func (r *MemDriftProofRepo) GetProof(_ context.Context, proofID string) (*HmacWriteDriftProof, error) {
	   r.mu.RLock()
	  defer r.mu.RUnlock()
	   p, ok := r.proofs[proofID]
	   if !ok {
		  return nil, ErrLeaseNotFound
	  }
	   cp := *p
	  return &cp, nil
   }

   func (r *MemDriftProofRepo) ListByLeaseID(_ context.Context, leaseID string, limit int) ([]*HmacWriteDriftProof, error) {
	  r.mu.RLock()
	   defer r.mu.RUnlock()
	  var out []*HmacWriteDriftProof
	  for _, p := range r.proofs {
		  if p.LeaseID == leaseID {
			  cp := *p
			   out = append(out, &cp)
			   if limit > 0 && len(out) >= limit {
				   break
			   }
		  }
	   }
	   return out, nil
  }

  func (r *MemDriftProofRepo) ListByReason(_ context.Context, reason DriftReason, limit int) ([]*HmacWriteDriftProof, error) {
	  r.mu.RLock()
	   defer r.mu.RUnlock()
	  var out []*HmacWriteDriftProof
	  for _, p := range r.proofs {
		   if p.Reason == reason {
			   cp := *p
			  out = append(out, &cp)
			  if limit > 0 && len(out) >= limit {
				  break
			   }
		   }
	  }
	  return out, nil
  }

// MemHaltGenRepo 基于内存的 HaltGenRepo 实现。
type MemHaltGenRepo struct {
	mu sync.RWMutex
	gen *HaltGeneration
}

// NewMemHaltGenRepo 构造内存 HaltGeneration 仓储。
  func NewMemHaltGenRepo() *MemHaltGenRepo {
	   return &MemHaltGenRepo{}
   }

  func (r *MemHaltGenRepo) SaveHaltGen(_ context.Context, g *HaltGeneration) error {
	  if g == nil {
		   return ErrInvalidInput
	   }
	   r.mu.Lock()
	  defer r.mu.Unlock()
	  cp := *g
	  r.gen = &cp
	  return nil
  }

  func (r *MemHaltGenRepo) GetHaltGen(_ context.Context) (*HaltGeneration, error) {
	  r.mu.RLock()
	   defer r.mu.RUnlock()
	  if r.gen == nil {
		   return &HaltGeneration{
			  GenID:      HaltGenGlobalID,
			  Generation: 0,
			   Halted: false,
		  }, nil
	  }
	   cp := *r.gen
	   return &cp, nil
  }
