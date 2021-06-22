package forbidgate

import (
	"context"
	"encoding/json"
	"time"
)

// AuditEvent 审计事件结构。
type AuditEvent struct {
	EventType  string    `json:"event_type"`
	OccurredAt time.Time `json:"occurred_at"`
	UserID     string    `json:"user_id"`
	Actor string `json:"actor"`
	Reason  string         `json:"reason"`
	Details map[string]any `json:"details,omitempty"`
}

// AuditWriter 审计写出接口（回调，由外部注入）。
type AuditWriter interface {
	Write(ctx context.Context, ev AuditEvent) error
}

// AuditWriterFunc 函数类型适配器。
type AuditWriterFunc func(ctx context.Context, ev AuditEvent) error

// Write 实现 AuditWriter。

   func (f AuditWriterFunc) Write(ctx context.Context, ev AuditEvent) error {
	   if f == nil {
		   return nil
	   }
	  return f(ctx, ev)
  }

// noopAuditWriter 空审计写出（默认）。

type noopAuditWriter struct{}

  func (noopAuditWriter) Write(_ context.Context, _ AuditEvent) error { return nil }

// ---- Gate 审计集成 ----
// auditGenBump 记录禁提世代抬升审计事件。

   func auditGenBump(ctx context.Context, w AuditWriter, lease ForbidLease) {
	   if w == nil {
		   return
	   }
	   _ = w.Write(ctx, AuditEvent{
		  EventType:  "forbid_gen_bump",
		  OccurredAt: lease.IssuedAt,
		   UserID:     lease.UserID,
		   Actor:      lease.Actor,
		  Reason:     lease.Reason,
		   Details: map[string]any{
			  "generation": lease.Generation,
			   "state":      string(lease.State),
			   "lease_id": lease.ID,
		  },
	  })
  }

// auditBindIssued 记录预检绑定发放审计事件。
  func auditBindIssued(ctx context.Context, w AuditWriter, bind WdPrefightBind) {
	   if w == nil {
		  return
	  }
	   _ = w.Write(ctx, AuditEvent{
		   EventType:  "forbid_bind_issued",
		  OccurredAt: bind.IssuedAt,
		   UserID: bind.UserID,
		   Actor:  "system",
		   Reason: "prefight_bind",
		   Details: map[string]any{
			  "bind_id":      bind.ID,
			  "withdraw_ref": bind.WithdrawRef,
			   "generation":   bind.Generation,
			   "expires_at":   bind.ExpiresAt.Format(time.RFC3339),
		   },
	  })
   }

// auditBindRevoked 记录预检绑定吊销审计事件。

  func auditBindRevoked(ctx context.Context, w AuditWriter, bindID, userID, reason string) {
	   if w == nil {
		   return
	  }
	  _ = w.Write(ctx, AuditEvent{
		  EventType:  "forbid_bind_revoked",
		  OccurredAt: time.Now().UTC(),
		  UserID:     userID,
		  Actor: "system",
		  Reason: reason,
		  Details: map[string]any{
			   "bind_id": bindID,
		   },
	  })
  }

// auditDriftCaseOpen 记录漂移案例开启审计事件。
  func auditDriftCaseOpen(ctx context.Context, w AuditWriter, dc ForbidDriftCase) {
	   if w == nil {
		  return
	   }
	  _ = w.Write(ctx, AuditEvent{
		  EventType: "forbid_drift_case_open",
		   OccurredAt: dc.DetectedAt,
		  UserID: dc.UserID,
		  Actor:  "system",
		  Reason: string(dc.Kind),
		   Details: map[string]any{
			  "case_id": dc.ID,
			   "withdraw_ref": dc.WithdrawRef,
			   "gen_at_precheck": dc.GenAtPrecheck,
			   "gen_at_detect":   dc.GenAtDetect,
		  },
	  })
   }

// auditDriftCaseClosed 记录漂移案例关闭审计事件。
   func auditDriftCaseClosed(ctx context.Context, w AuditWriter, dc ForbidDriftCase) {
	  if w == nil {
		  return
	   }
	  _ = w.Write(ctx, AuditEvent{
		   EventType:  "forbid_drift_case_closed",
		   OccurredAt: time.Now().UTC(),
		  UserID:     dc.UserID,
		  Actor:      dc.ClosedBy,
		   Reason: "proof_confirmed",
		   Details: map[string]any{
			   "case_id":  dc.ID,
			   "proof_id": dc.ProofID,
		  },
	  })
  }

// MarshalAuditEvent 序列化审计事件为 JSON 字节（供日志写出）。
   func MarshalAuditEvent(ev AuditEvent) ([]byte, error) {
	  return json.Marshal(ev)
   }
