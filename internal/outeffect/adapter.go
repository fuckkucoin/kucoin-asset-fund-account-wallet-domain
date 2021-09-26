package outeffect

import (
	"context"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/fxonce"
)

// ────────────────────────────────────────────────────────────────────────────
// HandlerResult — handler 执行结果（由调用方填入）
// ────────────────────────────────────────────────────────────────────────────
// HandlerResult handler 执行完成后，由调用方（outboxpump / webhook handler）
// 填入此结构，交给 Adapter.Process 做效果闸裁决。
type HandlerResult struct {
	// BusinessKey 业务主键（如 deposit_id、withdrawal_id）。
	BusinessKey string `json:"business_key"`
	// Kind 资金副作用 kind（如 "credit_intent"）。
	Kind string `json:"kind"`
	// Generation 本次投递世代号（outboxpump 或 webhook 分配）。
	Generation int64 `json:"generation"`
	// Payload handler 处理结果载荷（JSON 序列化）。
	Payload string `json:"payload,omitempty"`
	// OutboxEventID 对应 persist.OutboxEvent.ID（审计关联用）。
	OutboxEventID string `json:"outbox_event_id,omitempty"`
	// Success handler 是否成功执行。
	// false 时，Adapter 调用 CASEngine.Nack，不写 DupProof。
	Success bool `json:"success"`
	// ErrCode handler 失败时的错误码（fxonce.RejectCode）。
	// 必须区分 RejectHTTPRateLimit 与 RejectHandlerError。
	ErrCode fxonce.RejectCode `json:"err_code,omitempty"`
}

// ProcessResult Adapter.Process 的返回结果。
type ProcessResult struct {
	// MarkResult CAS 裁决结果（仅 Success=true 时有效）。
	MarkResult fxonce.MarkResult `json:"mark_result"`
	// Bypassed kind 已旁路，未走效果闸。
	Bypassed bool `json:"bypassed"`
	// Rejected CAS 拒绝（stale_gen / dup_gen / poisoned / blocked）。
	Rejected bool `json:"rejected"`
	// Idempotent 幂等回放路径。
	Idempotent bool `json:"idempotent"`
	// Applied 正常 apply 通过，调用方可产生账本意图。
	Applied bool `json:"applied"`
	// NackRecorded handler 失败，Nack 已记录。
	NackRecorded bool `json:"nack_recorded"`
}

// ────────────────────────────────────────────────────────────────────────────
// Adapter
// ────────────────────────────────────────────────────────────────────────────
// Adapter 将 outboxpump / webhook 的 handler 结果路由至 fxonce 效果闸。
type Adapter struct {
	gw      *fxonce.FundEffectGateway
	metrics *fxonce.Metrics
	clock   func() time.Time
}

// NewAdapter 构造 Adapter。
   func NewAdapter(gw *fxonce.FundEffectGateway, m *fxonce.Metrics) *Adapter {
	  if m == nil {
		  m = fxonce.NewMetrics("outeffect")
	   }
	  return &Adapter{
		  gw:      gw,
		   metrics: m,
		  clock: func() time.Time { return time.Now().UTC() },
	   }
   }

// WithClock 替换时钟（用）。
  func (a *Adapter) WithClock(c func() time.Time) *Adapter {
	  if a != nil && c != nil {
		   a.clock = c
	   }
	  return a
   }

   // Process 处理单次 handler 执行结果，返回效果闸裁决。
   // 调用时序：
   // 1. outboxpump/webhook 取得出箱事件；
   // 2. 业务 handler 执行；
  // 3. 调用方填入 HandlerResult；
   // 4. 调用 Adapter.Process；
   // 5. 根据 ProcessResult.Applied 决定是否产生账本意图。
  func (a *Adapter) Process(ctx context.Context, hr HandlerResult) (ProcessResult, error) {
	  if err := validateHandlerResult(hr); err != nil {
		   return ProcessResult{}, err
	  }
	  req := fxonce.MarkRequest{
		   BusinessKey: hr.BusinessKey,
		   Kind:        fxonce.FundKind(hr.Kind),
		   ExpectedGen: hr.Generation,
		  Payload:       hr.Payload,
		   OutboxEventID: hr.OutboxEventID,
	   }
	   // handler 失败路径
	  if !hr.Success {
		  if hr.ErrCode == fxonce.RejectHTTPRateLimit {
			   // HTTP 限速：不计入资金 exactly-once，直接返回，不写 Nack 记录
			  return ProcessResult{Rejected: true, MarkResult: fxonce.MarkResult{Rejected: true, RejectCode: fxonce.RejectHTTPRateLimit}}, nil
		  }
		  // 其他 handler 失败：记录 Nack
		  if nackErr := a.gw.Engine().Nack(ctx, req, hr.ErrCode); nackErr != nil {
			  return ProcessResult{NackRecorded: false}, fmt.Errorf("outeffect adapter nack: %w", nackErr)
		   }
		   return ProcessResult{NackRecorded: true, Rejected: true}, nil
	   }
	   // handler 成功路径：调用 RouteApply
	  start := a.clock()
	  result, bypassed, err := a.gw.RouteApply(ctx, req)
	  latencyMs := float64(a.clock().Sub(start).Milliseconds())
	  if a.metrics != nil {
		  a.metrics.RecordApply(result, latencyMs)
		   // 记录 kind 裁决
		   if bypassed {
			   a.metrics.RecordGateDecision(hr.Kind, "bypass")
		  } else if result.Rejected {
			  a.metrics.RecordGateDecision(hr.Kind, "rejected")
		   } else {
			   a.metrics.RecordGateDecision(hr.Kind, "must_gate")
		   }
	  }
	   if err != nil {
		  return ProcessResult{Rejected: true, MarkResult: result, Bypassed: bypassed}, err
	  }
	   return ProcessResult{
		  MarkResult: result,
		  Bypassed: bypassed,
		   Rejected: result.Rejected,
		   Idempotent: result.Idempotent,
		   Applied:    !bypassed && !result.Rejected && !result.Idempotent,
	  }, nil
  }

// ────────────────────────────────────────────────────────────────────────────
// 验证工具
// ────────────────────────────────────────────────────────────────────────────

  func validateHandlerResult(hr HandlerResult) error {
	   if strings.TrimSpace(hr.BusinessKey) == "" {
		   return fmt.Errorf("outeffect: business_key must not be empty")
	  }
	   if strings.TrimSpace(hr.Kind) == "" {
		   return fmt.Errorf("outeffect: kind must not be empty")
	  }
	   if hr.Generation <= 0 {
		  return fmt.Errorf("outeffect: generation must be > 0, got %d", hr.Generation)
	   }
	   if !hr.Success && hr.ErrCode == "" {
		   hr.ErrCode = fxonce.RejectHandlerError
	  }
	  return nil
  }
