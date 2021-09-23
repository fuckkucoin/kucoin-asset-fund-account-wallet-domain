package outeffect

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/fxonce"
	"account-assets.kucoin.net/funding/internal/persist"
)

// ────────────────────────────────────────────────────────────────────────────
// EventEnvelope — 出箱事件解析结构
// ────────────────────────────────────────────────────────────────────────────
// EventEnvelope 是 persist.OutboxEvent.Payload 的 JSON 解析结构。
// outboxpump 入箱时，payload 应为此形态。
type EventEnvelope struct {
	// BusinessKey 业务主键（唯一标识本次资金副作用的业务实体）。
	BusinessKey string `json:"business_key"`
	// Generation 投递世代号（出箱泵每次重新入箱时递增）。
	Generation int64 `json:"generation"`
	// Data 业务数据体（由各业务模块定义形态）。
	Data json.RawMessage `json:"data,omitempty"`
}

// ParseEnvelope 从 OutboxEvent.Payload 解析 EventEnvelope。
// 若 payload 不是 JSON 或缺少必要字段，返回 ParseError。
   func ParseEnvelope(payload string) (*EventEnvelope, error) {
	  var env EventEnvelope
	  if err := json.Unmarshal([]byte(payload), &env); err != nil {
		  return nil, fmt.Errorf("outeffect: parse envelope json: %w", err)
	  }
	  if strings.TrimSpace(env.BusinessKey) == "" {
		  return nil, fmt.Errorf("outeffect: envelope missing business_key")
	   }
	   if env.Generation <= 0 {
		  return nil, fmt.Errorf("outeffect: envelope generation must be > 0, got %d", env.Generation)
	  }
	   return &env, nil
  }

// ────────────────────────────────────────────────────────────────────────────
// FundHandler — 下游资金 handler（账本意图产生者）
// ────────────────────────────────────────────────────────────────────────────
// FundHandler 在 CAS Apply 通过后调用，负责产生账本意图。
// 返回 error 时，Relay 将 result 标记为失败，但不回滚 effect（effect 已 applied）。
// 账本意图产生失败属于下游问题，需独立补偿路径处理。
type FundHandler func(ctx context.Context, env *EventEnvelope, result fxonce.MarkResult) error

// BypassHandler 非资金 kind 旁路时调用的 handler（不走效果闸）。

type BypassHandler func(ctx context.Context, event persist.OutboxEvent) error

// ────────────────────────────────────────────────────────────────────────────
// RelayConfig
// ────────────────────────────────────────────────────────────────────────────
// RelayConfig Relay 配置。
type RelayConfig struct {
	// FundHandlers 按 kind 注册的资金 handler（key: kind string）。
	FundHandlers map[string]FundHandler
	// BypassHandler 非资金 kind 的旁路 handler（nil 则直接 Mark 不调用下游）。
	BypassHandler BypassHandler
	// MaxRetry Relay 层面的最大重试（不同于 CAS 层 gen 重试）。
	MaxRetry int
}

// ────────────────────────────────────────────────────────────────────────────
// Relay
// ────────────────────────────────────────────────────────────────────────────
// Relay 将 outboxpump 的 Handler 调用路由到效果闸 + 下游资金 handler。
type Relay struct {
	adapter *Adapter
	cfg RelayConfig
	clock func() time.Time
}

// NewRelay 构造 Relay。

   func NewRelay(adapter *Adapter, cfg RelayConfig) *Relay {
	  if cfg.MaxRetry <= 0 {
		   cfg.MaxRetry = 5
	  }
	   if cfg.FundHandlers == nil {
		   cfg.FundHandlers = map[string]FundHandler{}
	   }
	  return &Relay{
		  adapter: adapter,
		  cfg:     cfg,
		   clock: func() time.Time { return time.Now().UTC() },
	  }
  }

// WithClock 替换时钟（用）。

   func (r *Relay) WithClock(c func() time.Time) *Relay {
	  if r != nil && c != nil {
		  r.clock = c
	  }
	   return r
  }

   // RegisterFundHandler 注册资金 kind 的下游 handler（热注册，线程不安全，应在 Start 前完成）。
   func (r *Relay) RegisterFundHandler(kind string, h FundHandler) {
	  if r != nil && kind != "" && h != nil {
		   r.cfg.FundHandlers[kind] = h
	   }
  }

// HandleEvent 处理单个 OutboxEvent（供 outboxpump.Handler 包装调用）。
// 返回值语义与 outboxpump.Handler 一致：
// - nil → 投递成功（可 Mark）；
// - error → 投递失败（不 Mark，退回重试）。
// 注意：CAS 拒绝（stale_gen / dup_gen / poisoned）视为「已处理」，返回 nil
// 防止 outboxpump 持续重投造成更多 DupProof。
  func (r *Relay) HandleEvent(ctx context.Context, event persist.OutboxEvent) error {
	   kind := strings.TrimSpace(event.Kind)
	   if kind == "" {
		  return fmt.Errorf("outeffect relay: event kind must not be empty")
	   }
	   // 解析 envelope（提取 business_key 和 generation）
	  env, err := ParseEnvelope(event.Payload)
	   if err != nil {
		   // payload 形态非法：非资金 kind 可以 bypass；资金 kind 必须报错
		  if fxonce.IsFundKind(kind) {
			   return fmt.Errorf("outeffect relay: fund kind %s requires valid envelope: %w", kind, err)
		   }
		  // 非资金 kind：调用旁路 handler 或直接成功
		  return r.callBypass(ctx, event)
	   }
	   hr := HandlerResult{
		  BusinessKey: env.BusinessKey,
		  Kind: kind,
		  Generation: env.Generation,
		  Payload: event.Payload,
		  OutboxEventID: event.ID,
		  Success: true,
	   }
	  result,
		   err2 := r.adapter.Process(ctx, hr)
	   if err2 != nil && !result.Rejected {
		   return fmt.Errorf("outeffect relay: adapter process: %w", err2)
	   }
	  // 旁路路径：调用 BypassHandler
	   if result.Bypassed {
		  return r.callBypass(ctx, event)
	  }
	  // CAS 拒绝路径：返回 nil（视为已处理，防止持续重投）
	   if result.Rejected {
		   // poisoned 特殊处理：需要阻止下游消费，但也不重投
		  return nil
	   }
	  // 幂等回放路径：返回 nil（历史 effect 已 applied，无需再次产生账本意图）
	   if result.Idempotent {
		  return nil
	  }
	   // 正常 Applied 路径：调用下游资金 handler 产生账本意图
	   if result.Applied {
		  h, ok := r.cfg.FundHandlers[kind]
		  if !ok || h == nil {
			  // 没有注册下游 handler：记录但不失败（intent 须由调用方保证）
			   return nil
		  }
		   if handlerErr := h(ctx, env, result.MarkResult); handlerErr != nil {
			   return fmt.Errorf("outeffect relay: fund handler %s: %w", kind, handlerErr)
		  }
	  }
	   return nil
   }

// callBypass 调用旁路 handler（非资金 kind）。
   func (r *Relay) callBypass(ctx context.Context, event persist.OutboxEvent) error {
	  if r.cfg.BypassHandler == nil {
		  return nil
	   }
	   return r.cfg.BypassHandler(ctx, event)
   }

// ────────────────────────────────────────────────────────────────────────────
// RelayStats — Relay 运行期统计
// ────────────────────────────────────────────────────────────────────────────
// RelayStats Relay 累计统计（供 desk 查询）。

type RelayStats struct {
	// Processed 累计处理事件数。
	Processed int64 `json:"processed"`
	// Applied 累计 CAS 通过数量。
	Applied int64 `json:"applied"`
	// Idempotent 累计幂等回放数量。
	Idempotent int64 `json:"idempotent"`
	// Rejected 累计 CAS 拒绝数量。
	Rejected int64 `json:"rejected"`
	// Bypassed 累计旁路数量（非资金 kind）。
	Bypassed int64 `json:"bypassed"`
	// Errors 累计错误数量（不含 CAS 拒绝）。
	Errors int64 `json:"errors"`
	// LastProcessedAt 最近一次处理时刻。
	LastProcessedAt *time.Time `json:"last_processed_at,omitempty"`
}
