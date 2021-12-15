package trexecgate

import (
	"context"
	"sync"
	"time"
)

// LeaseRepo 计划执行租约存储接口。
type LeaseRepo interface {
	// SaveLease 新建或更新租约（upsert by LeaseID）。
	SaveLease(ctx context.Context, l *PlanExecLease) error
	// GetLease 按 LeaseID 查询；未找到返回 ErrLeaseNotFound。
	GetLease(ctx context.Context, leaseID string) (*PlanExecLease, error)
	// GetLeaseByIdemKey 按幂等键查询活跃（未终态）租约；未找到返回 ErrLeaseNotFound。
	GetLeaseByIdemKey(ctx context.Context, idemKey string) (*PlanExecLease, error)
	// GetLeaseByExecID 按 ExecID 查询；未找到返回 ErrLeaseNotFound。
	GetLeaseByExecID(ctx context.Context, execID string) (*PlanExecLease, error)
	// ListExpiredLeases 返回 ExpiresAt <= before 且 State 仍为 bound/rechecked/sent 的租约列表。
	ListExpiredLeases(ctx context.Context, before time.Time, limit int) ([]*PlanExecLease, error)
	// ListByState 按状态列举（用于巡检 worker）。
	ListByState(ctx context.Context, state LeaseState, limit int) ([]*PlanExecLease, error)
}

// DriftProofRepo 漂移证明存储接口。
type DriftProofRepo interface {
	// SaveProof 保存漂移证明；ProofID 全局唯一。
	SaveProof(ctx context.Context, p *TreasDriftProof) error
	// GetProof 按 ProofID 查询。
	GetProof(ctx context.Context, proofID string) (*TreasDriftProof, error)
	// ListProofsByLease 列举同一租约的所有漂移证明（用于证明导出 API）。
	ListProofsByLease(ctx context.Context, leaseID string) ([]*TreasDriftProof, error)
	// ListProofsByExec 列举同一 ExecID 的所有漂移证明。
	ListProofsByExec(ctx context.Context, execID string) ([]*TreasDriftProof, error)
}

// GenRepo ExecGeneration 世代标记存储接口。
type GenRepo interface {
	// SaveGen 新建或更新世代（upsert by IdemKey）。
	SaveGen(ctx context.Context, g *ExecGeneration) error
	// GetGen 按 IdemKey 查询；未找到返回 ErrLeaseNotFound。
	GetGen(ctx context.Context, idemKey string) (*ExecGeneration, error)
}

// ──────────────────────────────────────────────────────────────────────────────
// 内存实现（与单机开发使用）
// ──────────────────────────────────────────────────────────────────────────────
// MemLeaseRepo 内存租约存储，并发安全。
type MemLeaseRepo struct {
	mu sync.Mutex
	byID     map[string]*PlanExecLease
	byIdem   map[string]*PlanExecLease
	byExecID map[string]*PlanExecLease
}

  func NewMemLeaseRepo() *MemLeaseRepo {
	  return &MemLeaseRepo{
		  byID:     map[string]*PlanExecLease{},
		  byIdem:   map[string]*PlanExecLease{},
		   byExecID: map[string]*PlanExecLease{},
	   }
   }

  func (r *MemLeaseRepo) SaveLease(_ context.Context, l *PlanExecLease) error {
	  if l == nil || l.LeaseID == "" {
		  return ErrInvalidInput
	   }
	   r.mu.Lock()
	  defer r.mu.Unlock()
	   cp := *l
	   r.byID[l.LeaseID] = &cp
	   if l.IdemKey != "" {
		  r.byIdem[l.IdemKey] = &cp
	  }
	   if l.ExecID != "" {
		   r.byExecID[l.ExecID] = &cp
	   }
	   return nil
   }

  func (r *MemLeaseRepo) GetLease(_ context.Context, leaseID string) (*PlanExecLease, error) {
	  r.mu.Lock()
	   defer r.mu.Unlock()
	   l, ok := r.byID[leaseID]
	  if !ok {
		   return nil, ErrLeaseNotFound
	  }
	  cp := *l
	  return &cp, nil
  }

   func (r *MemLeaseRepo) GetLeaseByIdemKey(_ context.Context, idemKey string) (*PlanExecLease, error) {
	   r.mu.Lock()
	   defer r.mu.Unlock()
	  l, ok := r.byIdem[idemKey]
	  if !ok {
		   return nil, ErrLeaseNotFound
	  }
	   cp := *l
	  return &cp, nil
  }

  func (r *MemLeaseRepo) GetLeaseByExecID(_ context.Context, execID string) (*PlanExecLease, error) {
	   r.mu.Lock()
	  defer r.mu.Unlock()
	   l, ok := r.byExecID[execID]
	   if !ok {
		  return nil, ErrLeaseNotFound
	   }
	  cp := *l
	   return &cp, nil
   }

  func (r *MemLeaseRepo) ListExpiredLeases(_ context.Context, before time.Time, limit int) ([]*PlanExecLease, error) {
	  r.mu.Lock()
	  defer r.mu.Unlock()
	   if limit <= 0 {
		   limit = 50
	  }
	   var out []*PlanExecLease
	   for _, l := range r.byID {
		   if !l.IsTerminal() && !before.Before(l.ExpiresAt) {
			   cp := *l
			  out = append(out, &cp)
			   if len(out) >= limit {
				  break
			   }
		  }
	   }
	   return out, nil
  }

  func (r *MemLeaseRepo) ListByState(_ context.Context, state LeaseState, limit int) ([]*PlanExecLease, error) {
	  r.mu.Lock()
	   defer r.mu.Unlock()
	  if limit <= 0 {
		   limit = 50
	  }
	   var out []*PlanExecLease
	   for _, l := range r.byID {
		   if l.State == state {
			  cp := *l
			  out = append(out, &cp)
			   if len(out) >= limit {
				   break
			   }
		   }
	   }
	   return out, nil
  }

// IsTerminal 代理到模型方法，避免调用方重复导入。

   func (l *PlanExecLease) IsTerminal() bool {
	   return l.State.IsTerminal()
   }

// MemDriftProofRepo 内存漂移证明存储，并发安全。
type MemDriftProofRepo struct {
	mu sync.Mutex
	byID    map[string]*TreasDriftProof
	byLease map[string][]*TreasDriftProof
	byExec map[string][]*TreasDriftProof
}

   func NewMemDriftProofRepo() *MemDriftProofRepo {
	   return &MemDriftProofRepo{
		   byID: map[string]*TreasDriftProof{},
		   byLease: map[string][]*TreasDriftProof{},
		   byExec: map[string][]*TreasDriftProof{},
	  }
   }

   func (r *MemDriftProofRepo) SaveProof(_ context.Context, p *TreasDriftProof) error {
	   if p == nil || p.ProofID == "" {
		  return ErrInvalidInput
	  }
	  r.mu.Lock()
	   defer r.mu.Unlock()
	   cp := *p
	   r.byID[p.ProofID] = &cp
	  r.byLease[p.LeaseID] = append(r.byLease[p.LeaseID], &cp)
	   r.byExec[p.ExecID] = append(r.byExec[p.ExecID], &cp)
	   return nil
   }

  func (r *MemDriftProofRepo) GetProof(_ context.Context, proofID string) (*TreasDriftProof, error) {
	  r.mu.Lock()
	  defer r.mu.Unlock()
	  p, ok := r.byID[proofID]
	  if !ok {
		  return nil, ErrLeaseNotFound
	  }
	  cp := *p
	   return &cp, nil
   }

   func (r *MemDriftProofRepo) ListProofsByLease(_ context.Context, leaseID string) ([]*TreasDriftProof, error) {
	   r.mu.Lock()
	   defer r.mu.Unlock()
	  src := r.byLease[leaseID]
	   out := make([]*TreasDriftProof, len(src))
	  for i, p := range src {
		  cp := *p
		   out[i] = &cp
	  }
	   return out, nil
   }

   func (r *MemDriftProofRepo) ListProofsByExec(_ context.Context, execID string) ([]*TreasDriftProof, error) {
	   r.mu.Lock()
	  defer r.mu.Unlock()
	   src := r.byExec[execID]
	  out := make([]*TreasDriftProof,
		  len(src))
	   for i, p := range src {
		   cp := *p
		   out[i] = &cp
	  }
	   return out, nil
  }

// MemGenRepo 内存世代标记存储。

type MemGenRepo struct {
	mu sync.Mutex
	rows map[string]*ExecGeneration
}

  func NewMemGenRepo() *MemGenRepo {
	   return &MemGenRepo{rows: map[string]*ExecGeneration{}}
  }

  func (r *MemGenRepo) SaveGen(_ context.Context, g *ExecGeneration) error {
	  if g == nil || g.IdemKey == "" {
		  return ErrInvalidInput
	   }
	  r.mu.Lock()
	   defer r.mu.Unlock()
	  cp := *g
	   r.rows[g.IdemKey] = &cp
	   return nil
   }

  func (r *MemGenRepo) GetGen(_ context.Context, idemKey string) (*ExecGeneration, error) {
	  r.mu.Lock()
	   defer r.mu.Unlock()
	   g, ok := r.rows[idemKey]
	   if !ok {
		  return nil, ErrLeaseNotFound
	  }
	  cp := *g
	  return &cp, nil
  }
