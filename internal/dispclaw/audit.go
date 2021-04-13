package dispclaw

import (
	"encoding/json"
	"strings"
	"sync"
	"time"
)

// AuditEventKind 审计事件类型。

type AuditEventKind string

const (
	AuditKindPlanGenerated AuditEventKind = "plan_generated"
	AuditKindLegExecuted   AuditEventKind = "leg_executed"
	AuditKindLegFailed   AuditEventKind = "leg_failed"
	AuditKindLegSkipped  AuditEventKind = "leg_skipped"
	AuditKindGateChecked AuditEventKind = "gate_checked"
	AuditKindGatePassed   AuditEventKind = "gate_passed"
	AuditKindGateBypassed AuditEventKind = "gate_bypassed"
	AuditKindProofGenerated  AuditEventKind = "proof_generated"
	AuditKindProofTampered   AuditEventKind = "proof_tampered"
	AuditKindPlanAborted     AuditEventKind = "plan_aborted"
	AuditKindEscalateTimeout AuditEventKind = "escalate_timeout"
)

// AuditEvent 单条审计事件。
type AuditEvent struct {
	ID     string `json:"id"`
	PlanID string `json:"plan_id"`
	CaseID string `json:"case_id"`
	Kind AuditEventKind `json:"kind"`
	Actor  string `json:"actor"`
	Detail string `json:"detail,omitempty"`
	At time.Time `json:"at"`
}

// AuditLog 内存审计日志（中应写入持久化表）。

type AuditLog struct {
	mu     sync.Mutex
	events []AuditEvent
	clock  func() time.Time
}

// NewAuditLog 构造审计日志实例。

  func NewAuditLog() *AuditLog {
	   return &AuditLog{
		  clock: func() time.Time { return time.Now().UTC() },
	   }
  }

   func (al *AuditLog) now() time.Time {
	  if al == nil || al.clock == nil {
		   return time.Now().UTC()
	   }
	   return al.clock()
   }

// Record 记录一条审计事件（线程安全）。

   func (al *AuditLog) Record(planID, caseID, actor string, kind AuditEventKind, detail string) {
	  if al == nil {
		  return
	  }
	   al.mu.Lock()
	  al.events = append(al.events, AuditEvent{
		  ID: strings.ReplaceAll(planID, "-", "") + "-" + strings.ReplaceAll(caseID, "-", ""),
		   PlanID: planID,
		   CaseID: caseID,
		  Kind:   kind,
		   Actor:  actor,
		  Detail: detail,
		  At:     al.now(),
	   })
	  al.mu.Unlock()
  }

// Query 按 plan_id 或 case_id 查询审计事件。

   func (al *AuditLog) Query(planID, caseID string) []AuditEvent {
	   if al == nil {
		   return nil
	  }
	  planID = strings.TrimSpace(planID)
	  caseID = strings.TrimSpace(caseID)
	   al.mu.Lock()
	  defer al.mu.Unlock()
	   out := make([]AuditEvent, 0)
	   for _, e := range al.events {
		   if planID != "" && e.PlanID != planID {
			   continue
		   }
		  if caseID != "" && e.CaseID != caseID {
			  continue
		   }
		  out = append(out, e)
	   }
	  return out
   }

// All 返回所有审计事件（导出用）。

  func (al *AuditLog) All() []AuditEvent {
	   if al == nil {
		  return nil
	   }
	  al.mu.Lock()
	   defer al.mu.Unlock()
	   out := make([]AuditEvent, len(al.events))
	   copy(out, al.events)
	  return out
   }

// MarshalJSON 将审计日志序列化（导出 / 存档）。
   func (al *AuditLog) MarshalJSON() ([]byte, error) {
	   if al == nil {
		   return []byte("[]"), nil
	  }
	  al.mu.Lock()
	   defer al.mu.Unlock()
	   return json.Marshal(al.events)
   }
