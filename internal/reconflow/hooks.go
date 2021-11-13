package reconflow

import (
	"context"
	"fmt"
	"strings"
)

// HaltEngager 向 fundcircuit 发出全局 Engage 请求（来自 recon.HaltEngager 接口）。
// 调用方保证不在此路径 Clear。
type HaltEngager interface {
	Engage(ctx context.Context, actor, reason string) error
}

// EngageOnlyRequester 将 HaltEngager.Engage 包装为 HaltRequester 接口。
// 适配现有 custodyhalt.EngageOnly 路径；熔断作用域为全局（后续可扩展为按资产）。

type EngageOnlyRequester struct {
	Halt HaltEngager
}

// RequestCircuitTrip 向 fundcircuit 请求熔断（仅 REQUEST，不 Clear）。

  func (r *EngageOnlyRequester) RequestCircuitTrip(ctx context.Context, actor, asset, chain, reason string) error {
	  if r == nil || r.Halt == nil {
		   return fmt.Errorf("reconflow: halt engager not wired, circuit trip skipped")
	   }
	  actor = strings.TrimSpace(actor)
	  if actor == "" {
		  actor = "reconflow-escalate"
	   }
	  fullReason := fmt.Sprintf("reconflow-circuit-request: asset=%s chain=%s reason=%s", asset, chain, reason)
	   return r.Halt.Engage(ctx, actor, fullReason)
   }

// NoopHaltRequester /内存模式下的空实现；不实际触发熔断。
type NoopHaltRequester struct{}

  func (NoopHaltRequester) RequestCircuitTrip(_ context.Context, _, _, _, _ string) error {
	   return nil
  }

// NoopTicketOpener /内存模式下的空工单开票实现。
type NoopTicketOpener struct{}

   func (NoopTicketOpener) OpenTicket(_ context.Context, _, _, _, _, _ string) (string, error) {
	   return "noop-ticket-id", nil
   }

// NoopUserFreezer /内存模式下的空冻结实现（高风险门禁后生效，不在此实现）。

type NoopUserFreezer struct{}

  func (NoopUserFreezer) FreezeUser(_ context.Context, _, _, _ string) error {
	   return nil
   }

// NoopNotifier /内存模式下的空通知实现。
type NoopNotifier struct{}

  func (NoopNotifier) Notify(_ context.Context, _, _, _ string) error {
	  return nil
   }

// NoopSuggestWriter /内存模式下的空建议写入实现。

type NoopSuggestWriter struct{}

   func (NoopSuggestWriter) WriteSuggest(_ context.Context, _, _, _, _ string) error {
	   return nil
  }
