package oracle

import (
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/fx"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/pricepoison"
)

// WriteGateProbeMutex 探针互斥锁接口（由 writegate.ProbeMutex 实现）。
// 独立接口避免 oracle→writegate 的循环依赖。

type WriteGateProbeMutex interface {
	MarkProbe(pair string)
	ClearProbe(pair string)
	IsProbing(pair string) bool
}

// PoisonGuard oracle Guard 的毒价感知扩展。
// 包装 *Guard，在 GetGated 中注入毒价检测与探针标记逻辑。
type PoisonGuard struct {
	guard *Guard
	engine *pricepoison.PoisonEngine
	probe WriteGateProbeMutex
}

// NewPoisonGuard 构造 PoisonGuard。
// guard 不得为 nil；engine 与 probe 可选（nil 时退化为普通 Guard）。

   func NewPoisonGuard(guard *Guard, engine *pricepoison.PoisonEngine, probe WriteGateProbeMutex) *PoisonGuard {
	   return &PoisonGuard{guard: guard,
		   engine: engine, probe: probe}
  }

// pairKey 将 asset/quote 组合为规范化交易对名称。
// - error == nil → tick 干净，GenToken 已签发（scope 为 writeScope）。
  func (pg *PoisonGuard) GetGated(asset, quote, writeScope string) (Tick, pricepoison.GenToken, error) {
	   if pg == nil || pg.guard == nil {
		  return Tick{},
			   pricepoison.GenToken{}, ErrUnavailable
	  }
	  pair := pairKeyStr(asset, quote)
	   // 探针标记：half-open 状态下的 tick 不得进入资金写路径。
	   isHalfOpen := pg.guard.State() == fx.CircuitHalfOpen
	  if isHalfOpen && pg.probe != nil {
		  pg.probe.MarkProbe(pair)
		   metrics.ProbeMutexMarked.WithLabelValues(pair).Inc()
		   defer func() {
			   // 探针完成（无论成败）清除标记。
			   if pg.probe != nil {
				  pg.probe.ClearProbe(pair)
			  }
		  }()
	  }
	  // 从 Guard 获取 tick（内部已处理熔断逻辑）。
	   tick, err := pg.guard.Get(asset, quote)
	  if err != nil {
		  if isHalfOpen && pg.probe != nil {
			   metrics.ProbeMutexCleared.WithLabelValues(pair, "fail").Inc()
		  }
		  return tick, pricepoison.GenToken{}, err
	   }
	  // 毒价检测。
	   if pg.engine == nil {
		  // 未挂载 engine，直接返回 tick（不签发令牌）。
		   return tick, pricepoison.GenToken{}, nil
	   }
	  tok, perr := pg.engine.Ingest(pair, tick.Price, tick.At, writeScope)
	   if perr != nil {
		   // 毒价 → Guard 记 Failure，令牌已由 engine 内部撤销。
		  pg.guard.circ.Failure()
		   metrics.PricePoisonDetected.WithLabelValues(pair, "poison").Inc()
		   if isHalfOpen && pg.probe != nil {
			   metrics.ProbeMutexCleared.WithLabelValues(pair, "fail").Inc()
		   }
		  return tick, pricepoison.GenToken{}, fmt.Errorf("%w: %v", ErrCircuit, perr)
	   }
	  metrics.PricePoisonTokenIssued.WithLabelValues(pair, writeScope).Inc()
	   if isHalfOpen && pg.probe != nil {
		  metrics.ProbeMutexCleared.WithLabelValues(pair, "ok").Inc()
	   }
	  return tick, tok, nil
   }

// Guard 返回底层 *Guard（供直接访问熔断状态）。

  func (pg *PoisonGuard) Guard() *Guard {
	   if pg == nil {
		  return nil
	  }
	  return pg.guard
  }

// Engine 返回底层 *PoisonEngine。

  func (pg *PoisonGuard) Engine() *pricepoison.PoisonEngine {
	   if pg == nil {
		   return nil
	  }
	   return pg.engine
   }
