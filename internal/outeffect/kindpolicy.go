package outeffect

import (
	"context"
	"fmt"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/fxonce"
)

// ────────────────────────────────────────────────────────────────────────────
// KindPolicyManager
// ────────────────────────────────────────────────────────────────────────────
// KindPolicyManager 管理 KindPolicy 的持久化与热加载。

type KindPolicyManager struct {
	registry *fxonce.PolicyRegistry
	storage  fxonce.KindPolicyStore
	mu       sync.Mutex
	lastLoad time.Time
	clock    func() time.Time
}

// NewKindPolicyManager 构造 KindPolicyManager。
// storage 可为 nil（此时策略仅在内存中，不持久化）。
  func NewKindPolicyManager(registry *fxonce.PolicyRegistry, storage fxonce.KindPolicyStore) *KindPolicyManager {
	  if registry == nil {
		   registry = fxonce.NewPolicyRegistry()
	  }
	   return &KindPolicyManager{
		  registry: registry,
		   storage:  storage,
		  clock: func() time.Time { return time.Now().UTC() },
	  }
   }

// Load 从存储加载策略到注册表（幂等；适合启动时调用）。
   func (m *KindPolicyManager) Load(ctx context.Context) error {
	   if m.storage == nil {
		   return nil
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  policies, err := m.storage.LoadPolicies(ctx)
	  if err != nil {
		   return fmt.Errorf("outeffect: load kind policies: %w", err)
	   }
	  for _, p := range policies {
		   if setErr := m.registry.Set(p); setErr != nil {
			  return fmt.Errorf("outeffect: set kind policy %s: %w", p.Kind, setErr)
		  }
	  }
	   m.lastLoad = m.clock()
	  return nil
  }

// Reload 从存储重新加载策略（热更新，不重启进程）。
// 与 Load 的区别：Reload 会清除注册表中现有的显式策略后重载。
// 注意：资金类 kind 的默认策略（GateRequired=true）不会被清除。
  func (m *KindPolicyManager) Reload(ctx context.Context) error {
	   return m.Load(ctx)
  }

// SetPolicy 新增或更新策略（写存储 + 热更新注册表）。

   func (m *KindPolicyManager) SetPolicy(ctx context.Context, p *fxonce.KindPolicy, actor string) error {
	  if p == nil {
		   return fmt.Errorf("outeffect: policy must not be nil")
	  }
	  if actor == "" {
		   return fmt.Errorf("outeffect: actor must not be empty")
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   p.UpdatedBy = actor
	  p.UpdatedAt = m.clock()
	   // 先持久化
	  if m.storage != nil {
		   if err := m.storage.SavePolicy(ctx, p); err != nil {
			   return fmt.Errorf("outeffect: save kind policy %s: %w", p.Kind, err)
		  }
	  }
	  // 热更新注册表
	  if err := m.registry.Set(p); err != nil {
		   return fmt.Errorf("outeffect: registry set kind policy %s: %w", p.Kind, err)
	   }
	   return nil
   }

// GetPolicy 查询指定 kind 的当前策略。

  func (m *KindPolicyManager) GetPolicy(kind string) *fxonce.KindPolicy {
	  return m.registry.Get(kind)
  }

// ListPolicies 返回所有显式配置的策略（快照）。

   func (m *KindPolicyManager) ListPolicies() []*fxonce.KindPolicy {
	   return m.registry.List()
   }

// LastLoadAt 返回最近一次从存储加载策略的时刻。

   func (m *KindPolicyManager) LastLoadAt() time.Time {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  return m.lastLoad
  }

// ────────────────────────────────────────────────────────────────────────────
// PolicyChangeAudit — 策略变更审计记录
// ────────────────────────────────────────────────────────────────────────────
// PolicyChangeAudit 记录策略变更事件，供 desk 审计。

type PolicyChangeAudit struct {
	// ID 唯一标识。
	ID string `json:"id"`
	// Kind 被修改的 kind。
	Kind string `json:"kind"`
	// Before 修改前的策略（nil = 首次创建）。
	Before *fxonce.KindPolicy `json:"before,omitempty"`
	// After 修改后的策略。
	After *fxonce.KindPolicy `json:"after"`
	// Actor 操作人。
	Actor string `json:"actor"`
	// At 操作时刻。
	At time.Time `json:"at"`
}

// AuditedPolicyManager 在 KindPolicyManager 基础上追加审计日志。

type AuditedPolicyManager struct {
	inner *KindPolicyManager
	audits []*PolicyChangeAudit
	mu     sync.Mutex
	clock  func() time.Time
}

// NewAuditedPolicyManager 构造带审计的策略管理器。
   func NewAuditedPolicyManager(inner *KindPolicyManager) *AuditedPolicyManager {
	   return &AuditedPolicyManager{
		   inner: inner,
		  clock: func() time.Time { return time.Now().UTC() },
	   }
   }

// SetPolicy 带审计日志的策略更新。
  func (a *AuditedPolicyManager) SetPolicy(ctx context.Context, p *fxonce.KindPolicy, actor string) error {
	   before := a.inner.GetPolicy(p.Kind)
	   if err := a.inner.SetPolicy(ctx, p, actor); err != nil {
		  return err
	   }
	   audit := &PolicyChangeAudit{
		  ID: newAuditID(),
		   Kind: p.Kind,
		   After: p,
		   Actor: actor,
		  At:    a.clock(),
	   }
	   // 仅在策略确实存在时记录 before（避免误导）
	   if before != nil && before.UpdatedAt.After(time.Time{}) {
		   audit.Before = before
	   }
	  a.mu.Lock()
	   a.audits = append(a.audits, audit)
	  a.mu.Unlock()
	  return nil
   }

// ListAudits 返回所有审计记录（按时间倒序，最多 limit 条）。

   func (a *AuditedPolicyManager) ListAudits(limit int) []*PolicyChangeAudit {
	   a.mu.Lock()
	   defer a.mu.Unlock()
	  out := make([]*PolicyChangeAudit,
		   len(a.audits))
	  copy(out, a.audits)
	  // 倒序
	  for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		   out[i], out[j] = out[j], out[i]
	   }
	   if limit > 0 && len(out) > limit {
		  out = out[:limit]
	   }
	  return out
  }

// newAuditID 生成审计记录 ID（复用 uuid）。
  func newAuditID() string {
	  return fmt.Sprintf("pol-audit-%d", time.Now().UnixNano())
  }
