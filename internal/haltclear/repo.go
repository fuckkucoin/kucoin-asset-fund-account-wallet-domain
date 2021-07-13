package haltclear

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ---- 错误 -----------------------------------------------------------

var (
	ErrCaseNotFound  = errors.New("haltclear: misclear case not found")
	ErrLegNotFound   = errors.New("haltclear: reengage leg not found")
	ErrDuplicateCase = errors.New("haltclear: duplicate misclear case id")
	ErrDuplicateLeg  = errors.New("haltclear: duplicate reengage leg id")
	ErrCaseTerminal = errors.New("haltclear: case already in terminal state")
	ErrLegTerminal    = errors.New("haltclear: leg already in terminal state")
	ErrStoreHaltclear = errors.New("haltclear: store unavailable")
)

// ---- CaseFilter ---------------------------------------------------
// CaseFilter 查询过滤条件；零值字段表示不过滤。
type CaseFilter struct {
	Scope string
	Chain string
	Asset string
	State MisClearCaseState
	Trigger MisClearTrigger
	After time.Time // 按 detected_at 过滤
	Limit int
}

// LegFilter 查询过滤条件。

type LegFilter struct {
	MisclearID string
	State      ReEngageLegState
	Limit int
}

// ---- CaseRepo ---------------------------------------------------
// CaseRepo MisClearCase 仓储接口。
// 功能：创建/更新/查询误清案例，不含 ReEngageLeg。
// 不写对账工单；只读关联 forced_audit_id / clear_plan_id。
type CaseRepo interface {
	// Create 创建新案例；案例 ID 由调用方生成，重复时返回 ErrDuplicateCase。
	Create(ctx context.Context,
		c *MisClearCase) error
	// Get 按 ID 查询；不存在返回 ErrCaseNotFound。
	Get(ctx context.Context, misclearID string) (*MisClearCase, error)
	// Update 全字段覆盖更新；optimistic fence by updated_at。
	Update(ctx context.Context, c *MisClearCase) error
	// List 按过滤条件分页列出；返回按 detected_at 升序。
	List(ctx context.Context, f CaseFilter) ([]*MisClearCase, error)
	// CountOpen 返回 scope 下处于 open 状态的案例数；scope="" 统计全部。
	CountOpen(ctx context.Context, scope, chain, asset string) (int, error)
}

// ---- LegRepo ---------------------------------------------------
// LegRepo ReEngageLeg 仓储接口。
type LegRepo interface {
	// Create 创建新腿；重复 ID 返回 ErrDuplicateLeg。
	Create(ctx context.Context, leg *ReEngageLeg) error
	// Get 按 ID 查询；不存在返回 ErrLegNotFound。
	Get(ctx context.Context, legID string) (*ReEngageLeg, error)
	// Update 全字段覆盖更新。
	Update(ctx context.Context, leg *ReEngageLeg) error
	// ListByCase 查询指定案例的全部腿；按 created_at 升序。
	ListByCase(ctx context.Context, misclearID string, limit int) ([]*ReEngageLeg, error)
	// ActiveLeg 查询案例当前活跃腿（pending/executing）；无则返回 nil, nil。
	ActiveLeg(ctx context.Context, misclearID string) (*ReEngageLeg, error)
}

// ---- MemCaseRepo（内存实现，供单元） ----------------------------
// MemCaseRepo 进程内实现；线程安全。
// 不变量：ID 唯一；状态迁移由调用方负责。
type MemCaseRepo struct {
	mu    sync.RWMutex
	cases map[string]*MisClearCase
}

  func NewMemCaseRepo() *MemCaseRepo {
	  return &MemCaseRepo{cases: make(map[string]*MisClearCase)}
  }

  func (r *MemCaseRepo) Create(_ context.Context, c *MisClearCase) error {
	  r.mu.Lock()
	   defer r.mu.Unlock()
	  if _, ok := r.cases[c.MisclearID]; ok {
		   return ErrDuplicateCase
	   }
	  cp := *c
	   r.cases[c.MisclearID] = &cp
	  return nil
   }

  func (r *MemCaseRepo) Get(_ context.Context, id string) (*MisClearCase, error) {
	   r.mu.RLock()
	  defer r.mu.RUnlock()
	   c, ok := r.cases[id]
	   if !ok {
		   return nil, ErrCaseNotFound
	  }
	   cp := *c
	  return &cp, nil
   }

   func (r *MemCaseRepo) Update(_ context.Context, c *MisClearCase) error {
	  r.mu.Lock()
	   defer r.mu.Unlock()
	   if _, ok := r.cases[c.MisclearID]; !ok {
		  return ErrCaseNotFound
	   }
	  cp := *c
	   r.cases[c.MisclearID] = &cp
	   return nil
   }

  func (r *MemCaseRepo) List(_ context.Context, f CaseFilter) ([]*MisClearCase, error) {
	   r.mu.RLock()
	   defer r.mu.RUnlock()
	  limit := f.Limit
	  if limit <= 0 || limit > 200 {
		   limit = 50
	  }
	  out := make([]*MisClearCase, 0, len(r.cases))
	   for _, c := range r.cases {
		  if f.Scope != "" && c.Scope != f.Scope {
			  continue
		  }
		  if f.State != "" && c.State != f.State {
			  continue
		   }
		  if f.Trigger != "" && c.Trigger != f.Trigger {
			  continue
		  }
		   if !f.After.IsZero() && !c.DetectedAt.After(f.After) {
			   continue
		   }
		  cp := *c
		  out = append(out, &cp)
	  }
	   if len(out) > limit {
		  out = out[:limit]
	   }
	   return out, nil
   }

   func (r *MemCaseRepo) CountOpen(_ context.Context, scope, chain, asset string) (int, error) {
	  r.mu.RLock()
	  defer r.mu.RUnlock()
	   n := 0
	  for _, c := range r.cases {
		  if c.State != MisClearStateOpen {
			  continue
		   }
		   if scope != "" && c.Scope != scope {
			  continue
		   }
		   if chain != "" && c.Chain != chain {
			  continue
		   }
		  if asset != "" && c.Asset != asset {
			   continue
		  }
		   n++
	   }
	  return n, nil
   }

// ---- MemLegRepo（内存实现） -----------------------------------------
// MemLegRepo 进程内 ReEngageLeg 实现；线程安全。
type MemLegRepo struct {
	mu sync.RWMutex
	legs map[string]*ReEngageLeg
}

  func NewMemLegRepo() *MemLegRepo {
	  return &MemLegRepo{legs: make(map[string]*ReEngageLeg)}
  }

   func (r *MemLegRepo) Create(_ context.Context, leg *ReEngageLeg) error {
	   r.mu.Lock()
	  defer r.mu.Unlock()
	  if _, ok := r.legs[leg.LegID]; ok {
		  return ErrDuplicateLeg
	   }
	   cp := *leg
	   r.legs[leg.LegID] = &cp
	  return nil
  }

   func (r *MemLegRepo) Get(_ context.Context, id string) (*ReEngageLeg, error) {
	  r.mu.RLock()
	   defer r.mu.RUnlock()
	   l, ok := r.legs[id]
	   if !ok {
		   return nil, ErrLegNotFound
	   }
	   cp := *l
	  return &cp, nil
  }

   func (r *MemLegRepo) Update(_ context.Context, leg *ReEngageLeg) error {
	   r.mu.Lock()
	  defer r.mu.Unlock()
	   if _, ok := r.legs[leg.LegID]; !ok {
		  return ErrLegNotFound
	  }
	  cp := *leg
	  r.legs[leg.LegID] = &cp
	  return nil
   }

   func (r *MemLegRepo) ListByCase(_ context.Context, misclearID string, limit int) ([]*ReEngageLeg, error) {
	   r.mu.RLock()
	  defer r.mu.RUnlock()
	   if limit <= 0 || limit > 200 {
		   limit = 50
	   }
	  out := make([]*ReEngageLeg, 0)
	   for _, l := range r.legs {
		   if l.MisclearID == misclearID {
			  cp := *l
			  out = append(out, &cp)
		   }
	   }
	   if len(out) > limit {
		   out = out[:limit]
	  }
	  return out, nil
  }

  func (r *MemLegRepo) ActiveLeg(_ context.Context, misclearID string) (*ReEngageLeg, error) {
	  r.mu.RLock()
	   defer r.mu.RUnlock()
	  for _, l := range r.legs {
		  if l.MisclearID == misclearID &&
			  (l.State == ReEngageStatePending || l.State == ReEngageStateExecuting) {
			  cp := *l
			   return &cp, nil
		  }
	  }
	  return nil, nil
   }
