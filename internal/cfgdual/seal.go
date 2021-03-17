package cfgdual

import (
	"fmt"
	"sync"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// SealGate 单发布封堵闸
// ─────────────────────────────────────────────────────────────────────────────
// SealGate 管理一个 GrayRelease 的封堵闸状态。

type SealGate struct {
	mu        sync.Mutex
	releaseID string
	state     SealState
	actions   []*SealAction
	clock     func() time.Time
	idgen func() string
}

// NewSealGate 构造封堵闸（初始态 SealOpen）。
   func NewSealGate(releaseID string, idgen func() string) *SealGate {
	  return &SealGate{
		  releaseID: releaseID,
		   state:     SealOpen,
		   clock:     func() time.Time { return time.Now().UTC() },
		idgen: idgen,
	}
}

// WithClock 注入时钟（用）。
   func (g *SealGate) WithClock(c func() time.Time) *SealGate {
	  if c != nil {
		   g.clock = c
	  }
	  return g
  }

   // State 当前封堵状态（只读）。
  func (g *SealGate) State() SealState {
	   g.mu.Lock()
	   defer g.mu.Unlock()
	   return g.state
  }

// IsEnforced 是否已强制封堵。

   func (g *SealGate) IsEnforced() bool {
	  return g.State() == SealEnforced
  }

// Enforce 触发封堵（open → enforced）。
// 已是 enforced 时幂等返回 nil。
// 已是 waived 时返回 ErrInvalidState（需运营重置）。
  func (g *SealGate) Enforce(splitDiffID, reason, actor string) (*SealAction, error) {
	   g.mu.Lock()
	   defer g.mu.Unlock()
	  if g.state == SealEnforced {
		  return nil, nil // 幂等
	   }
	   if g.state == SealWaived {
		   return nil, fmt.Errorf("%w: cannot enforce a waived seal without reset", ErrInvalidState)
	   }
	   from := g.state
	  g.state = SealEnforced
	  act := g.newAction(from,
		   SealEnforced, splitDiffID, reason, actor)
	   g.actions = append(g.actions, act)
	   return act, nil
  }

// Waive 运营豁免（open | enforced → waived）。

  func (g *SealGate) Waive(reason, actor string) (*SealAction, error) {
	  g.mu.Lock()
	   defer g.mu.Unlock()
	   if g.state == SealWaived {
		  return nil, nil // 幂等
	   }
	  from := g.state
	   g.state = SealWaived
	  act := g.newAction(from, SealWaived, "", reason, actor)
	  g.actions = append(g.actions, act)
	  return act, nil
   }

// Reset 重置回 open（用于运营取消豁免或取消封堵后重新观察）。
  func (g *SealGate) Reset(reason, actor string) (*SealAction, error) {
	   g.mu.Lock()
	  defer g.mu.Unlock()
	  if g.state == SealOpen {
		  return nil, nil // 幂等
	   }
	   from := g.state
	   g.state = SealOpen
	   act := g.newAction(from, SealOpen, "", reason, actor)
	  g.actions = append(g.actions, act)
	   return act, nil
  }

// Actions 返回所有审计动作（只读副本）。
   func (g *SealGate) Actions() []*SealAction {
	  g.mu.Lock()
	  defer g.mu.Unlock()
	   out := make([]*SealAction, len(g.actions))
	  copy(out, g.actions)
	  return out
  }

  func (g *SealGate) newAction(from, to SealState, splitDiffID, reason, actor string) *SealAction {
	  id := ""
	  if g.idgen != nil {
		   id = g.idgen()
	  }
	  return &SealAction{
		  ID:            id,
		  GrayReleaseID: g.releaseID,
		   SplitDiffID:   splitDiffID,
		  FromState:     from,
		  ToState: to,
		  Reason:  reason,
		   Actor: actor,
		  CreatedAt: g.clock(),
	   }
   }

// ─────────────────────────────────────────────────────────────────────────────
// SealRegistry 进程级封堵闸注册表
// ─────────────────────────────────────────────────────────────────────────────
// SealRegistry 管理多个 GrayRelease 对应的 SealGate。
type SealRegistry struct {
	mu    sync.RWMutex
	gates map[string]*SealGate // key = releaseID
	idgen func() string
}

// NewSealRegistry 构造空注册表。
   func NewSealRegistry(idgen func() string) *SealRegistry {
	   return &SealRegistry{gates: map[string]*SealGate{}, idgen: idgen}
   }

   // GetOrCreate 按 releaseID 找到或新建 SealGate。

  func (r *SealRegistry) GetOrCreate(releaseID string) *SealGate {
	   r.mu.Lock()
	  defer r.mu.Unlock()
	   if g, ok := r.gates[releaseID]; ok {
		   return g
	  }
	   g := NewSealGate(releaseID, r.idgen)
	  r.gates[releaseID] = g
	   return g
  }

// Get 按 releaseID 查找 SealGate；不存在返回 nil。
  func (r *SealRegistry) Get(releaseID string) *SealGate {
	  r.mu.RLock()
	  defer r.mu.RUnlock()
	   return r.gates[releaseID]
   }

// Remove 发布终结后移除。
  func (r *SealRegistry) Remove(releaseID string) {
	   r.mu.Lock()
	  delete(r.gates, releaseID)
	   r.mu.Unlock()
   }

// IsEnforced 快速判断指定发布是否已封堵（Engine 用）。

  func (r *SealRegistry) IsEnforced(releaseID string) bool {
	   g := r.Get(releaseID)
	   if g == nil {
		   return false
	   }
	   return g.IsEnforced()
  }

// ─────────────────────────────────────────────────────────────────────────────
// SealPolicy 封堵触发策略（可配置阈值）
// ─────────────────────────────────────────────────────────────────────────────
// SealPolicy 封堵闸触发判断策略。
type SealPolicy struct {
	// MaxAllowDenyCount allow_deny 类型差分累计超过此值立即触发封堵；0 表示任意 1 次。
	MaxAllowDenyCount int64
	// MaxDiffRatio 差分比例超过此值（百分比）触发封堵；0 表示不按比例触发。
	MaxDiffRatio float64
}

// DefaultSealPolicy 默认封堵策略：allow_deny 出现即封堵；比例超过 20% 封堵。

  func DefaultSealPolicy() SealPolicy {
	   return SealPolicy{
		   MaxAllowDenyCount: 1,
		   MaxDiffRatio: 20.0,
	   }
  }

// ShouldEnforce 判断给定 SplitDiff 是否应触发封堵。

  func (p SealPolicy) ShouldEnforce(diff *SplitDiff) bool {
	   if diff == nil {
		   return false
	  }
	   // allow_deny 脑裂：最危险，任意 1 次（或超阈值）立即封堵。
	  threshold := p.MaxAllowDenyCount
	   if threshold <= 0 {
		   threshold = 1
	  }
	   if diff.AllowDenyCount >= threshold {
		  return true
	   }
	  // kill switch 任一侧出现。
	  if diff.KillSwitchCount > 0 {
		  return true
	   }
	  // 差分比例超限（严格大于阈值）。
	   if p.MaxDiffRatio > 0 && diff.DiffRatio > p.MaxDiffRatio {
		  return true
	   }
	   // ExceedsPolicy 由聚合 worker 综合判断。
	  return diff.ExceedsPolicy
   }
