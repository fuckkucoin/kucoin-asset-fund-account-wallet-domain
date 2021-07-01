// 功能：KindGate 是外部调用的入口判断层：
// KindGate 是外部调用的入口判断层：
// - 资金 kind（IsFundKind==true）→ 必须通过效果闸，不可旁路；
// - 非资金 kind → 允许旁路，但可被 KindPolicy 强制纳入效果闸；
// - 被显式封禁的 kind → 任何投递均拒绝，返回 ErrKindBlocked；
// - HTTP 限速（RejectHTTPRateLimit）→ 不计入资金 exactly-once，不产生 DupProof。
// PolicyRegistry 管理运营可配置的 KindPolicy；支持运行时热更新（写锁替换）。

// 作者：Bronson
// 日期：2021.06.28

package fxonce

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ────────────────────────────────────────────────────────────────────────────
// PolicyRegistry — 策略注册表
// ────────────────────────────────────────────────────────────────────────────
// PolicyRegistry 管理所有 KindPolicy，支持运行时热更新。
// 默认策略：资金 kind 强制走闸；非资金 kind 允许旁路。
type PolicyRegistry struct {
	mu       sync.RWMutex
	policies map[string]*KindPolicy
	// defaultMaxRetry 全局默认最大重试次数（单个 kind 未配置时使用）。
	defaultMaxRetry int
}

// NewPolicyRegistry 构造策略注册表，预填充所有默认资金 kind 的策略。

  func NewPolicyRegistry() *PolicyRegistry {
	   r := &PolicyRegistry{
		   policies:        map[string]*KindPolicy{},
		   defaultMaxRetry: 5,
	  }
	  // 预注册所有资金 kind 的默认策略
	  now := time.Now().UTC()
	  for _, fk := range AllFundKinds() {
		   r.policies[string(fk)] = &KindPolicy{
			  Kind: string(fk),
			  GateRequired: true,
			  Blocked:  false,
			  MaxRetry: 5,
			   Memo: "default fund kind policy",
			  UpdatedAt: now,
		   }
	   }
	  return r
   }

// Get 读取指定 kind 的策略；若未显式配置，返回基于 IsFundKind 推导的默认值。

   func (r *PolicyRegistry) Get(kind string) *KindPolicy {
	   kind = strings.TrimSpace(kind)
	  r.mu.RLock()
	   p, ok := r.policies[kind]
	   r.mu.RUnlock()
	   if ok {
		   c := *p
		   return &c
	   }
	  // 推导默认策略
	  return &KindPolicy{
		  Kind:         kind,
		  GateRequired: IsFundKind(kind),
		  Blocked: false,
		   MaxRetry: r.defaultMaxRetry,
		   UpdatedAt: time.Now().UTC(),
	   }
  }

// Set 新增或更新策略（运营 API 调用；热更新不需要重启）。

   func (r *PolicyRegistry) Set(p *KindPolicy) error {
	  if p == nil || strings.TrimSpace(p.Kind) == "" {
		  return fmt.Errorf("fxonce: policy kind must not be empty")
	  }
	  r.mu.Lock()
	   defer r.mu.Unlock()
	  clone := *p
	   clone.UpdatedAt = time.Now().UTC()
	  r.policies[clone.Kind] = &clone
	  return nil
  }

// Check 检查 kind 是否允许投递：
// - Blocked → ErrKindBlocked；
// - 其他 → nil（允许通过，具体是否走闸由 Gate 判断）。

   func (r *PolicyRegistry) Check(kind string) error {
	  p := r.Get(kind)
	  if p.Blocked {
		   return ErrKindBlocked
	   }
	  return nil
   }

// NeedsGate 判断 kind 是否需要走效果闸（强制幂等）。
  func (r *PolicyRegistry) NeedsGate(kind string) bool {
	  return r.Get(kind).GateRequired
  }

// MaxRetry 返回 kind 的最大重试次数（0 则使用 defaultMaxRetry）。
   func (r *PolicyRegistry) MaxRetry(kind string) int {
	   p := r.Get(kind)
	   if p.MaxRetry > 0 {
		  return p.MaxRetry
	   }
	   return r.defaultMaxRetry
  }

// List 返回所有显式配置的策略（快照，按 kind 字典序排列）。
  func (r *PolicyRegistry) List() []*KindPolicy {
	  r.mu.RLock()
	  defer r.mu.RUnlock()
	  out := make([]*KindPolicy, 0, len(r.policies))
	   for _, p := range r.policies {
		  c := *p
		   out = append(out, &c)
	   }
	   return out
   }

// SetDefaultMaxRetry 修改全局默认最大重试次数。

   func (r *PolicyRegistry) SetDefaultMaxRetry(n int) {
	   if n > 0 {
		  r.mu.Lock()
		   r.defaultMaxRetry = n
		  r.mu.Unlock()
	  }
   }

// ────────────────────────────────────────────────────────────────────────────
// KindGate — Kind 闸
// ────────────────────────────────────────────────────────────────────────────
// GateDecision 闸裁决结果。

type GateDecision struct {
	// Kind 被裁决的 kind。
	Kind string `json:"kind"`
	// MustGate true = 必须经效果闸；false = 允许旁路。
	MustGate bool `json:"must_gate"`
	// Blocked true = kind 被显式封禁，投递必须拒绝。
	Blocked bool `json:"blocked"`
	// IsFund 是否为资金类 kind。
	IsFund bool `json:"is_fund"`
	// PolicySource "default"（推导）/ "explicit"（运营配置）。
	PolicySource string `json:"policy_source"`
}

// KindGate 对每次出箱投递做 kind 维度的策略裁决。
// 不依赖 Store（无 I/O），纯内存策略计算；可在热路径调用。

type KindGate struct {
	registry *PolicyRegistry
}

// NewKindGate 构造 KindGate。

  func NewKindGate(registry *PolicyRegistry) *KindGate {
	  if registry == nil {
		   registry = NewPolicyRegistry()
	   }
	  return &KindGate{registry: registry}
  }

// Decide 对指定 kind 做裁决。不修改任何状态。

   func (g *KindGate) Decide(kind string) GateDecision {
	   kind = strings.TrimSpace(kind)
	   p := g.registry.Get(kind)
	   source := "default"
	   g.registry.mu.RLock()
	   if _, ok := g.registry.policies[kind]; ok {
		  source = "explicit"
	   }
	   g.registry.mu.RUnlock()
	   return GateDecision{
		  Kind:     kind,
		   MustGate: p.GateRequired,
		   Blocked:  p.Blocked,
		   IsFund: IsFundKind(kind),
		   PolicySource: source,
	  }
   }

// MustRoute 判断 kind 是否必须路由到效果闸（封禁直接拒绝）。
// 返回 (needsGate bool, err error)：
// - err != nil → 投递必须拒绝（ErrKindBlocked）；
// - needsGate == true → 必须通过 CASEngine.Apply；
// - needsGate == false → 允许旁路（非资金 kind 且策略未强制）。

   func (g *KindGate) MustRoute(kind string) (needsGate bool, err error) {
	   d := g.Decide(kind)
	   if d.Blocked {
		  return false, ErrKindBlocked
	   }
	  return d.MustGate, nil
  }

// ────────────────────────────────────────────────────────────────────────────
// KindPolicyStore — 策略持久化接口（用于运营 API 持久化策略）
// ────────────────────────────────────────────────────────────────────────────
// KindPolicyStore 持久化 KindPolicy（可选；不持久化时重启后策略丢失）。
type KindPolicyStore interface {
	SavePolicy(ctx context.Context, p *KindPolicy) error
	LoadPolicies(ctx context.Context) ([]*KindPolicy, error)
}

// MemKindPolicyStore KindPolicyStore 的内存实现（用）。
type MemKindPolicyStore struct {
	mu       sync.RWMutex
	policies map[string]*KindPolicy
}

// NewMemKindPolicyStore 构造内存策略仓。

  func NewMemKindPolicyStore() *MemKindPolicyStore {
	   return &MemKindPolicyStore{policies: map[string]*KindPolicy{}}
   }

// SavePolicy 保存策略（覆盖已有同 kind 记录）。

   func (s *MemKindPolicyStore) SavePolicy(_ context.Context, p *KindPolicy) error {
	  if p == nil || p.Kind == "" {
		  return fmt.Errorf("fxonce: policy kind must not be empty")
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  c := *p
	  s.policies[c.Kind] = &c
	   return nil
  }

// LoadPolicies 加载所有持久化策略（注册表启动时调用）。
   func (s *MemKindPolicyStore) LoadPolicies(_ context.Context) ([]*KindPolicy, error) {
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	  out := make([]*KindPolicy, 0, len(s.policies))
	  for _, p := range s.policies {
		  c := *p
		  out = append(out, &c)
	  }
	  return out, nil
   }

// ────────────────────────────────────────────────────────────────────────────
// Gate + Engine 组合入口（供适配器层使用）
// ────────────────────────────────────────────────────────────────────────────
// FundEffectGateway 是对外暴露的统一入口：
// 判断是否需要走闸 → 调用 CASEngine.Apply → 返回 MarkResult。
// 非资金 kind 直接 bypass，不写任何效果记录。

type FundEffectGateway struct {
	gate   *KindGate
	engine *CASEngine
}

// NewFundEffectGateway 构造网关。

   func NewFundEffectGateway(store Store, registry *PolicyRegistry) *FundEffectGateway {
	   if registry == nil {
		   registry = NewPolicyRegistry()
	   }
	  return &FundEffectGateway{
		   gate: NewKindGate(registry),
		   engine: NewCASEngine(store,
			  registry),
	   }
   }

// Gate 返回内部 KindGate（供 desk API 查询策略使用）。
  func (gw *FundEffectGateway) Gate() *KindGate { return gw.gate }

// Engine 返回内部 CASEngine（供 Worker 使用）。
  func (gw *FundEffectGateway) Engine() *CASEngine { return gw.engine }

// RouteApply 是热路径：判断 kind 是否需走闸，需走则调用 CASEngine.Apply。
// 返回值：
// - result.Rejected=false, result.Idempotent=false → 正常 apply，可产生账本意图；
// - result.Idempotent=true → 幂等回放，直接返回历史结果；
// - result.Rejected=true → 拒绝，不可产生账本意图；
// - bypassed=true → kind 旁路，不走效果闸（调用方自行决定是否产生意图）。

  func (gw *FundEffectGateway) RouteApply(ctx context.Context, req MarkRequest) (result MarkResult, bypassed bool, err error) {
	   needsGate, gateErr := gw.gate.MustRoute(string(req.Kind))
	  if gateErr != nil {
		   // 封禁 kind，直接拒绝
		  return MarkResult{Rejected: true, RejectCode: RejectKindBlocked}, false, gateErr
	  }
	  if !needsGate {
		  // 非资金 kind 旁路：不写效果仓，不产生 DupProof
		  return MarkResult{}, true, nil
	  }
	  result,
		   err = gw.engine.Apply(ctx, req)
	  return result, false, err
   }

// WithClock 替换时钟（用）。
   func (gw *FundEffectGateway) WithClock(c func() time.Time) *FundEffectGateway {
	  if gw != nil && c != nil {
		   gw.engine.WithClock(c)
	  }
	   return gw
  }
