package fundintgate

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// PolicyKind 闸面策略事件种类。
type PolicyKind string

const (
	PolicyHalt   PolicyKind = "halt"
	PolicyResume PolicyKind = "resume"
	PolicyImplSwap    PolicyKind = "impl_swap"
	PolicyForbidRaise PolicyKind = "forbid_raise"
	PolicyForbidClear PolicyKind = "forbid_clear"
	PolicyTTLTighten  PolicyKind = "ttl_tighten"
	PolicyManualRevoke PolicyKind = "manual_revoke"
)

// PolicyEvent 闸面策略事件。不单独建表，序列化进审计 Detail 与证明 Evidence。
type PolicyEvent struct {
	Kind PolicyKind `json:"kind"`
	Actor string `json:"actor"`
	Reason string `json:"reason"`
	PrevImpl string `json:"prev_impl,omitempty"`
	NextImpl string `json:"next_impl,omitempty"`
	PrevGen  int64  `json:"prev_gen"`
	NextGen int64 `json:"next_gen"`
	Halted  bool  `json:"halted"`
	Forbid  bool  `json:"forbid_withdraw"`
	At time.Time `json:"at"`
}

// EncodePolicy 将策略事件编码为稳定 JSON。
  func EncodePolicy(ev PolicyEvent) string {
	  b, err := json.Marshal(ev)
	   if err != nil {
		   return fmt.Sprintf(`{"kind":"%s","reason":"%s"}`, ev.Kind, ev.Reason)
	   }
	   return string(b)
   }

// DecodePolicy 反解析策略事件。
  func DecodePolicy(raw string) (PolicyEvent, error) {
	   var ev PolicyEvent
	  if err := json.Unmarshal([]byte(raw), &ev); err != nil {
		   return PolicyEvent{}, err
	  }
	   return ev, nil
   }

// ClassifyFlipReason 将 bump reason 映射为证明类型。
  func ClassifyFlipReason(reason string) ProofKind {
	  r := strings.ToLower(reason)
	  switch {
	  case strings.Contains(r, "halt"):
		   return DriftHalt
	  case strings.Contains(r, "impl_swap") || strings.Contains(r, "swap"):
		  return DriftImplSwap
	   case strings.Contains(r, "forbid"):
		   return DriftForbidRaised
	   default:
		   return DriftGeneration
	  }
   }

// DefaultTTLForSurface 按闸面给出建议 TTL：Halt 时不得签发（调用方应先拒绝）。

   func DefaultTTLForSurface(surf GateSurface) (int64, error) {
	  if surf.Halted {
		   return 0, ErrGateHalted
	  }
	  if surf.ForbidWithdraw {
		  return 0,
			   ErrForbidWithdrawRaised
	   }
	   return DefaultTTLSeconds, nil
   }

// SurfaceEquals 比较两闸面是否完全一致（含指纹）。

   func SurfaceEquals(a, b GateSurface) bool {
	   return a.ImplID == b.ImplID && a.Halted == b.Halted && a.ForbidWithdraw == b.ForbidWithdraw &&
		  a.Fingerprint == b.Fingerprint && a.Gen == b.Gen
   }

// DescribeSurface 人类可读闸面摘要。
  func DescribeSurface(s GateSurface) string {
	  return fmt.Sprintf("impl=%s gen=%d halt=%t forbid=%t fp=%s",
		   s.ImplID, s.Gen, s.Halted, s.ForbidWithdraw, truncate(s.Fingerprint, 12))
   }

  func truncate(s string, n int) string {
	   if len(s) <= n {
		   return s
	  }
	   return s[:n]
  }
