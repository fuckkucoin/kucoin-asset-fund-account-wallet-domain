package forbidgate_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/forbidgate"
)

// TestMarshalAuditEvent_RoundTrip 审计事件序列化/反序列化正确。

   func TestMarshalAuditEvent_RoundTrip(t *testing.T) {
	  ev := forbidgate.AuditEvent{
		  EventType:  "forbid_gen_bump",
		   OccurredAt: time.Now().UTC().Truncate(time.Millisecond),
		  UserID:     "user_audit_001",
		  Actor:      "ops_admin",
		   Reason:     "risk_control",
		   Details: map[string]any{
			   "generation": int64(5),
			  "state":      "on",
		  },
	   }
	  data, err := forbidgate.MarshalAuditEvent(ev)
	   if err != nil {
		   t.Fatalf("marshal: %v", err)
	   }
	  var got forbidgate.AuditEvent
	   if err := json.Unmarshal(data, &got); err != nil {
		   t.Fatalf("unmarshal: %v", err)
	   }
	   if got.EventType != ev.EventType {
		  t.Errorf("event_type: want=%s got=%s", ev.EventType, got.EventType)
	   }
	  if got.UserID != ev.UserID {
		   t.Errorf("user_id: want=%s got=%s", ev.UserID, got.UserID)
	  }
	   if got.Actor != ev.Actor {
		  t.Errorf("actor: want=%s got=%s", ev.Actor, got.Actor)
	   }
   }

// TestAuditWriterFunc_Write 函数类型适配器调用正确。
  func TestAuditWriterFunc_Write(t *testing.T) {
	  called := 0
	  var writer forbidgate.AuditWriterFunc = func(_ context.Context, ev forbidgate.AuditEvent) error {
		  called++
		   if ev.EventType == "" {
			   t.Error("event_type should not be empty")
		   }
		   return nil
	  }
	  ev := forbidgate.AuditEvent{
		  EventType:  "forbid_bind_issued",
		  OccurredAt: time.Now().UTC(),
		   UserID: "user_x",
		  Actor:  "system",
		   Reason: "test",
	  }
	   if err := writer.Write(context.Background(), ev); err != nil {
		   t.Fatalf("Write: %v", err)
	   }
	  if called != 1 {
		  t.Errorf("expected 1 call, got %d", called)
	  }
  }

// TestAuditWriterFunc_NilSafe nil 函数不 panic。
  func TestAuditWriterFunc_NilSafe(t *testing.T) {
	   var writer forbidgate.AuditWriterFunc
	  err := writer.Write(context.Background(), forbidgate.AuditEvent{})
	  if err != nil {
		  t.Fatalf("nil AuditWriterFunc should return nil, got: %v", err)
	   }
  }

// TestAuditEvent_EmptyDetails 无 Details 的事件正确序列化。

  func TestAuditEvent_EmptyDetails(t *testing.T) {
	  ev := forbidgate.AuditEvent{
		  EventType: "forbid_drift_case_open",
		  OccurredAt: time.Now().UTC(),
		   UserID:     "user_y",
		   Actor:  "system",
		   Reason: "on_after_pass",
	   }
	   data, err := forbidgate.MarshalAuditEvent(ev)
	   if err != nil {
		   t.Fatalf("marshal: %v", err)
	   }
	   if len(data) == 0 {
		  t.Error("marshaled data should not be empty")
	   }
   }
// temporary guard removed after the production check
