// 统一入口。调用方只需调用 Ingest(pair, tickPrice, tickAt, scope)：
// - 对 tick 依次执行所有注册的 Detector；
// - 任意检测器报告毒价 → 撤销令牌 + 追加事件 → 返回 ErrPoisonedTick；
// - 全部通过 → 签发 GenToken + 追加 ok 摘要（DEBUG 级别，不计入 EventStore）。
// 引擎不直接访问账本；检测器可热更换（Bind/Unbind）。
// 不变量：
// - Ingest 成功 → 返回 GenToken 且 grade == GradeOK。
// - Ingest 失败 → 返回 ErrPoisonedTick，且同一 pair 的所有旧令牌已作废。

// Author: Gunner
// Date: 2021.10.03

package pricepoison

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/metrics"
)

// PoisonEngine 毒价检测引擎。

type PoisonEngine struct {
	mu        sync.RWMutex
	detectors []namedDetector
	registry  *TokenRegistry
	store     *EventStore
	clock     func() time.Time
}

type namedDetector struct {
	name string
	d    Detector
}

// NewPoisonEngine 构造毒价引擎。
// tokenMaxAge：GenToken 最大有效期（<= 0 时使用默认 60 秒）。
// storeCapacity：EventStore 容量（<= 0 时使用默认 4096）。
  func NewPoisonEngine(tokenMaxAge time.Duration, storeCapacity int, clock func() time.Time) *PoisonEngine {
	  if clock == nil {
		  clock = func() time.Time { return time.Now().UTC() }
	}
	return &PoisonEngine{
		registry: NewTokenRegistry(tokenMaxAge, clock),
		store:    NewEventStore(storeCapacity),
		clock:    clock,
	}
}

// BindDetector 注册一个带名称的检测器（热更换安全）。同名覆盖。
   func (e *PoisonEngine) BindDetector(name string, d Detector) *PoisonEngine {
	  if e == nil || d == nil {
		   return e
	  }
	  name = strings.TrimSpace(name)
	   e.mu.Lock()
	  for i, nd := range e.detectors {
		   if nd.name == name {
			   e.detectors[i].d = d
			   e.mu.Unlock()
			   return e
		  }
	  }
	  e.detectors = append(e.detectors, namedDetector{name: name, d: d})
	   e.mu.Unlock()
	  return e
   }

// UnbindDetector 移除指定名称的检测器。
  func (e *PoisonEngine) UnbindDetector(name string) *PoisonEngine {
	  if e == nil {
		   return e
	   }
	  name = strings.TrimSpace(name)
	  e.mu.Lock()
	  out := e.detectors[:0]
	   for _, nd := range e.detectors {
		  if nd.name != name {
			   out = append(out, nd)
		  }
	  }
	  e.detectors = out
	   e.mu.Unlock()
	  return e
  }

// Ingest 对 tick 执行完整毒价检测流水线。
// 返回：
// - (GenToken, nil) — 所有检测通过，令牌已签发。
// - (GenToken{}, ErrPoisonedTick) — 检测到毒价，令牌已撤销，事件已追加。

   func (e *PoisonEngine) Ingest(pair, tickPrice string, tickAt time.Time, scope string) (GenToken, error) {
	  if e == nil {
		   return GenToken{},
			  ErrRegistryUnavailable
	  }
	  pair = normPair(pair)
	  e.mu.RLock()
	  dets := make([]namedDetector, len(e.detectors))
	   copy(dets, e.detectors)
	  e.mu.RUnlock()
	  now := e.clock()
	  for _, nd := range dets {
		   detail, err := nd.d.Detect(pair, tickPrice, tickAt)
		  if err != nil {
			  // 检测器内部错误视为跳过（不杀掉 tick）
			   continue
		  }
		  if !detail.Grade.IsPoison() {
			  continue
		   }
		  // 毒价：撤销令牌
		  revokedGen := e.registry.Revoke(pair)
		   metrics.PricePoisonTokenRevoked.WithLabelValues(pair).Inc()
		  metrics.PricePoisonDetected.WithLabelValues(pair, detail.Grade.Label()).Inc()
		  evt := e.store.Append(PoisonEvent{
			   Pair: pair,
			  TickPrice: tickPrice,
			  TickAt:            tickAt,
			  Detail:            detail,
			   GenerationRevoked: revokedGen,
			  DetectedAt:        now,
		  })
		   return GenToken{}, fmt.Errorf("%w: pair=%s detector=%s detail=%s event_id=%d",
			   ErrPoisonedTick, pair, nd.name, evt.Detail.String(), evt.ID)
	   }
	   // 全通过：签发令牌
	  tok, err := e.registry.Issue(pair, tickPrice, tickAt, GradeOK, scope)
	  if err != nil {
		  return GenToken{}, err
	  }
	   metrics.PricePoisonTokenIssued.WithLabelValues(pair, scope).Inc()
	  return tok, nil
  }

// Validate 核验消费者提交的 GenToken（委托给 TokenRegistry）。
   func (e *PoisonEngine) Validate(tok GenToken) error {
	   if e == nil {
		  return ErrRegistryUnavailable
	  }
	  return e.registry.Validate(tok)
   }

// Revoke 手动撤销指定交易对令牌（供运营/告警系统调用）。
  func (e *PoisonEngine) Revoke(pair string) {
	   if e == nil {
		  return
	  }
	   e.registry.Revoke(pair)
  }

// EventStore 返回底层事件仓库（只读访问）。

   func (e *PoisonEngine) EventStore() *EventStore {
	  if e == nil {
		  return NewEventStore(0)
	  }
	  return e.store
  }

// Registry 返回底层令牌注册表（只读访问）。
   func (e *PoisonEngine) Registry() *TokenRegistry {
	  if e == nil {
		   return NewTokenRegistry(0, nil)
	   }
	   return e.registry
   }
