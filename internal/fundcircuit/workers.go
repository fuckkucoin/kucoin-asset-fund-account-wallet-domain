package fundcircuit

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/workerloop"
	"github.com/rs/zerolog"
)

// TickReport 一轮作业结果。

type TickReport struct {
	Keys  int          `json:"keys"`
	Trips []Transition `json:"trips,omitempty"`
	Cooled       []Transition `json:"cooled,omitempty"`
	ProbeExpired []Probe      `json:"probe_expired,omitempty"`
	Reopened     []Transition `json:"reopened,omitempty"`
	SignalsPruned int       `json:"signals_pruned"`
	StorePruned   int       `json:"store_pruned"`
	Err           string    `json:"err,omitempty"`
	At            time.Time `json:"at"`
}

// AggregateOnce 聚合作业：把窗口内有信号的作用域重新打分，必要时跳闸开案。
  func (b *Bus) AggregateOnce(ctx context.Context) []Transition {
	   if b == nil {
		  return nil
	  }
	  pol := b.pol.Current()
	  keys := b.sig.ActiveKeys(pol.Window)
	  return b.Evaluate(ctx, keys)
   }

// AdvanceOnce 冷却推进作业：把冷却到期的 open 推进到 half_open。

   func (b *Bus) AdvanceOnce(ctx context.Context) []Transition {
	  if b == nil {
		   return nil
	  }
	   trs := b.cir.Cool(b.pol.Current())
	   for _, tr := range trs {
		   c, err := b.cir.Get(tr.Key)
		  if err != nil {
			   continue
		   }
		   b.persistCircuit(ctx, c)
	   }
	  b.emitGauges()
	  return trs
  }

// ProbeSweepOnce 探针过期作业：未按期回报视为失败，熔断器重开并加长冷却。
// 功能：半开期不留悬挂探针，避免一枚过期探针长期占住配额。
// 重点：过期等同失败，不得默认闭合熔断。
// 风险点：探针有效期设得比试写链路耗时短会反复重开，须与 ProbeTTL 一起调。

   func (b *Bus) ProbeSweepOnce(ctx context.Context) ([]Probe, []Transition) {
	   if b == nil {
		   return nil, nil
	  }
	  pol := b.pol.Current()
	  expired := b.probes.Expire()
	  trs := make([]Transition, 0, len(expired))
	   for _, p := range expired {
		  metrics.FundCircuitProbes.WithLabelValues(ProbeExpired.String()).Inc()
		   b.persistProbe(ctx, p)
		  c, tr, err := b.cir.ProbeFailed(p.CircuitKey, p.Actor, "probe expired without result", pol)
		  if err != nil {
			  continue
		   }
		   b.persistCircuit(ctx, c)
		  trs = append(trs, tr)
	   }
	  if len(expired) > 0 {
		  b.emitGauges()
	  }
	  return expired, trs
   }

// CleanupOnce 清理作业：摘除窗口外信号（内存与落库各一次）。

   func (b *Bus) CleanupOnce(ctx context.Context) (int, int) {
	   if b == nil {
		   return 0, 0
	   }
	  pol := b.pol.Current()
	   b.sig.SetTTL(pol.SignalTTL)
	  mem := b.sig.Prune()
	  store, err := b.PruneStore(ctx)
	  if err != nil {
		   return mem, 0
	  }
	   return mem, store
  }

// Tick 跑一轮全部作业：探针过期 → 冷却推进 → 信号聚合 → 清理。
// 顺序固定：先处理过期探针，再推进冷却，最后聚合，避免同一轮里状态互相抢写。
  func (b *Bus) Tick(ctx context.Context) TickReport {
	  rep := TickReport{At: b.now()}
	  if b == nil {
		  rep.Err = ErrBus.Error()
		   return rep
	  }
	  expired, reopened := b.ProbeSweepOnce(ctx)
	  rep.ProbeExpired = expired
	   rep.Reopened = reopened
	  rep.Cooled = b.AdvanceOnce(ctx)
	   rep.Trips = b.AggregateOnce(ctx)
	   pol := b.pol.Current()
	  rep.Keys = len(b.sig.ActiveKeys(pol.Window))
	   mem, store := b.CleanupOnce(ctx)
	  rep.SignalsPruned = mem
	   rep.StorePruned = store
	  return rep
   }

// WorkerConfig 作业循环参数。
type WorkerConfig struct {
	Name string
	Interval time.Duration
}

// RunWorkers 起熔断总线作业循环。ctx 结束即退出。
// 功能：聚合、冷却、探针过期与清理合成一个 tick，由 workerloop 统一退避与熔断暂停。
// 重点：作业失败只退避重试，不放行入口；入口闸始终读当前熔断状态。
// 风险点：多实例部署须由调用方选主，否则同一轮会重复开案。
   func (b *Bus) RunWorkers(ctx context.Context, cfg WorkerConfig, log zerolog.Logger) error {
	   if b == nil {
		   return ErrBus
	  }
	  name := cfg.Name
	   if name == "" {
		   name = "fund-circuit"
	  }
	   interval := cfg.Interval
	  if interval <= 0 {
		  interval = 15 * time.Second
	  }
	  jobs := workerloop.Compose(
		   func(ctx context.Context) error {
			   b.ProbeSweepOnce(ctx)
			   return nil
		  },
		   func(ctx context.Context) error {
			  b.AdvanceOnce(ctx)
			  return nil
		   },
		   func(ctx context.Context) error {
			   b.AggregateOnce(ctx)
			  return nil
		  },
		  func(ctx context.Context) error {
			   b.CleanupOnce(ctx)
			  return nil
		  },
	  )
	  return workerloop.Run(ctx, workerloop.Config{
		   Name:     name,
		   Interval: interval,
		  MaxBackoff: 2 * time.Minute,
		  FailThresh: 8,
		  CircuitPause: 30 * time.Second,
	  }, log, jobs)
  }

// WorkerTick 供进程级 worker 复用的一轮函数（不自带循环）。

   func (b *Bus) WorkerTick() func(context.Context) {
	   return func(ctx context.Context) {
		  _ = b.Tick(ctx)
	   }
  }
