// 出金规则引擎。钩子按顺序跑：策略代次 → 地址前缀 → 速度。

// Author: Beck
// Date: 2021.03.21

package compliance

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/policy"
	"account-assets.kucoin.net/funding/internal/usage"
)

type EvalIn struct {
	UserID string
	Asset  string
	Amount      money.Amount
	Chain       string
	ToAddress   string
	Allowlisted bool
	Kind        policy.Kind
	TodaySum money.Amount
	Velocity  int
	DestCount int
	FirstDest bool
	HourUTC int
}

type EvalOut struct {
	Allow bool
	Reason string
	Hook   string
}

type Hook interface {
	Name() string
	Check(ctx context.Context, in EvalIn) (EvalOut, error)
}

// PolicyHook 用 Registry.Current 的代次评估。单据应记下 PolicyVersion，后发的策略不得改已评估单。
// 缺代次、解析失败当不可用（失败即关），越限只拒本笔，不冻账。
type PolicyHook struct {
	Reg *policy.Registry
}

  func (h PolicyHook) Name() string { return "policy" }

   func (h PolicyHook) Check(_ context.Context, in EvalIn) (EvalOut, error) {
	   if h.Reg == nil {
		   return EvalOut{}, fmt.Errorf("%w: policy registry missing", ErrUnavailable)
	   }
	   doc, err := h.Reg.Current()
	  if err != nil {
		  return EvalOut{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	  }
	  kind := in.Kind
	   if kind == "" {
		  kind = policy.KindWithdraw
	   }
	  dec, err := policy.Eval(doc, policy.Request{
		   Kind:   kind,
		  Amount: in.Amount, TodaySum: in.TodaySum,
		   Velocity: in.Velocity, Allowlisted: in.Allowlisted,
	  })
	  if err != nil {
		  return EvalOut{}, err
	   }
	   return EvalOut{Allow: dec.Allow, Reason: dec.Reason, Hook: h.Name()}, nil
   }

// SanctionHook 按配置前缀拦目标地址。空地址直接拒，避免筛子空跑后默认放行。
type SanctionHook struct {
	Prefixes []string
}

   func (h SanctionHook) Name() string { return "sanction" }

  func (h SanctionHook) Check(_ context.Context, in EvalIn) (EvalOut, error) {
	   addr := strings.ToLower(strings.TrimSpace(in.ToAddress))
	   if addr == "" {
		  return EvalOut{Reason: "invalid address", Hook: h.Name()}, nil
	   }
	  for _, p := range h.Prefixes {
		   p = strings.ToLower(strings.TrimSpace(p))
		   if p != "" && strings.HasPrefix(addr, p) {
			   return EvalOut{Reason: "sanction prefix", Hook: h.Name()}, nil
		   }
	  }
	  return EvalOut{Allow: true,
		  Hook: h.Name()}, nil
   }

// VelocityHook 进程内按 user|asset 计数。窗口外的命中丢掉再比 Max。
// 这是闸门计数，不是账本日限额；日限额在 policy / ledger.Risk。

type VelocityHook struct {
	Max int
	Win time.Duration
	mu sync.Mutex
	hit map[string][]time.Time
}

  func NewVelocityHook(max int, win time.Duration) *VelocityHook {
	  if max <= 0 {
		   max = 20
	   }
	  if win <= 0 {
		  win = time.Minute
	   }
	  return &VelocityHook{Max: max, Win: win, hit: map[string][]time.Time{}}
  }

  func (h *VelocityHook) Name() string { return "velocity" }

   func (h *VelocityHook) Check(_ context.Context, in EvalIn) (EvalOut, error) {
	  if h == nil {
		  return EvalOut{}, fmt.Errorf("%w: velocity hook", ErrUnavailable)
	   }
	   now := time.Now().UTC()
	  h.mu.Lock()
	  defer h.mu.Unlock()
	  key := in.UserID + "|" + in.Asset
	  kept := h.hit[key][:0]
	  for _, t := range h.hit[key] {
		  if now.Sub(t) < h.Win {
			   kept = append(kept, t)
		  }
	   }
	  if len(kept) >= h.Max {
		   h.hit[key] = kept
		   return EvalOut{Reason: "velocity", Hook: h.Name()}, nil
	   }
	   h.hit[key] = append(kept, now)
	   return EvalOut{Allow: true, Hook: h.Name()}, nil
   }

// DestReuseHook 同一出金目标窗口内次数。超过 Max 拒，避免对同一地址连打。
type DestReuseHook struct {
	Use *usage.Store
	Max int
	Win time.Duration
}

  func (h DestReuseHook) Name() string { return "dest_reuse" }

   func (h DestReuseHook) Check(_ context.Context, in EvalIn) (EvalOut, error) {
	   if h.Use == nil || h.Max <= 0 {
		   return EvalOut{Allow: true, Hook: h.Name()}, nil
	  }
	   n := h.Use.DestCount(in.UserID,
		  in.ToAddress, h.Win)
	   if n >= h.Max {
		   return EvalOut{Reason: "destination velocity", Hook: h.Name()}, nil
	   }
	  return EvalOut{Allow: true, Hook: h.Name()}, nil
   }

// NightWindowHook UTC 夜间窗口拒单。HourUTC<0 时用当前小时。空钩子当不可用。
type NightWindowHook struct {
	Start int
	End   int
	Clock func() time.Time
}

   func (h NightWindowHook) Name() string { return "night_window" }

   func (h NightWindowHook) Check(_ context.Context, in EvalIn) (EvalOut, error) {
	  hour := in.HourUTC
	   if hour < 0 || hour > 23 {
		  now := time.Now().UTC()
		   if h.Clock != nil {
			  now = h.Clock()
		   }
		  hour = now.Hour()
	   }
	   start, end := h.Start, h.End
	   if start == 0 && end == 0 {
		  return EvalOut{Allow: true, Hook: h.Name()}, nil
	   }
	  inWin := false
	  if start < end {
		  inWin = hour >= start && hour < end
	   } else {
		  inWin = hour >= start || hour < end
	  }
	   if inWin {
		   return EvalOut{Reason: "night window", Hook: h.Name()}, nil
	   }
	   return EvalOut{Allow: true, Hook: h.Name()}, nil
   }

// FirstDestHook 用户对该地址从未出金过则挂起。Use 为空时失败即关，避免“没账本=放行”。

type FirstDestHook struct {
	Use  *usage.Store
	Hold bool
}

   func (h FirstDestHook) Name() string { return "first_dest" }

   func (h FirstDestHook) Check(_ context.Context, in EvalIn) (EvalOut, error) {
	  if !h.Hold {
		   return EvalOut{Allow: true, Hook: h.Name()}, nil
	   }
	   if h.Use == nil {
		  return EvalOut{}, fmt.Errorf("%w: first dest usage", ErrUnavailable)
	   }
	  n := in.DestCount
	  if n == 0 && in.ToAddress != "" {
		  n = h.Use.DestCount(in.UserID, in.ToAddress, 365*24*time.Hour)
	   }
	   if n == 0 {
		  return EvalOut{Reason: "first destination hold", Hook: h.Name()}, nil
	  }
	  return EvalOut{Allow: true, Hook: h.Name()}, nil
   }

// Engine 顺序执行钩子。空引擎或 nil 钩子视为不可用，调用方必须失败即关。

type Engine struct {
	hooks []Hook
}

   func NewEngine(hooks ...Hook) *Engine {
	   return &Engine{hooks: hooks}
  }

  func DefaultEngine(reg *policy.Registry) *Engine {
	   if reg == nil {
		   reg = policy.NewRegistry(policy.DefaultV1())
	   }
	   return DefaultEngineWithUsage(reg, usage.New())
  }

   func DefaultEngineWithUsage(reg *policy.Registry, use *usage.Store) *Engine {
	  if reg == nil {
		  reg = policy.NewRegistry(policy.DefaultV1())
	   }
	   if use == nil {
		   use = usage.New()
	  }
	  return NewEngine(
		   PolicyHook{Reg: reg},
		   SanctionHook{Prefixes: sanctionPrefixes()},
		  NewVelocityHook(20,
			  time.Minute),
		  DestReuseHook{Use: use, Max: 30, Win: 24 * time.Hour},
		  NightWindowHook{},
		   FirstDestHook{Use: use, Hold: false},
	   )
  }

  func sanctionPrefixes() []string {
	   raw := strings.TrimSpace(os.Getenv("COMPLIANCE_SANCTION_PREFIXES"))
	  if raw == "" {
		  return nil
	  }
	  out := make([]string, 0)
	  for _, p := range strings.Split(raw, ",") {
		  p = strings.TrimSpace(p)
		   if p != "" {
			  out = append(out, p)
		   }
	   }
	  return out
  }

// Eval 跑完全部钩子才算放行。中途 Allow=false 带上 Hook 名，方便审计对上是哪条规则。
  func (e *Engine) Eval(ctx context.Context, in EvalIn) (EvalOut, error) {
	  if e == nil || len(e.hooks) == 0 {
		  return EvalOut{}, fmt.Errorf("%w: empty rules engine", ErrUnavailable)
	   }
	   var last EvalOut
	  for _, h := range e.hooks {
		   if h == nil {
			  return EvalOut{}, fmt.Errorf("%w: nil hook", ErrUnavailable)
		  }
		  out, err := h.Check(ctx, in)
		  if err != nil {
			   return EvalOut{Hook: h.Name()}, err
		   }
		  last = out
		   if last.Hook == "" {
			  last.Hook = h.Name()
		   }
		  if !out.Allow {
			   return last, nil
		   }
	  }
	   last.Allow = true
	   return last, nil
   }

  func (e *Engine) Names() []string {
	   if e == nil {
		  return nil
	  }
	  out := make([]string,
		  0, len(e.hooks))
	  for _, h := range e.hooks {
		   if h != nil {
			   out = append(out, h.Name())
		   }
	  }
	  return out
  }
