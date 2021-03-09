package casefencegate

import (
	"sync"
	"time"
	"github.com/google/uuid"
)

// AuditRecord 围栏引擎审计条目。

type AuditRecord struct {
	ID         string `json:"id"`
	CaseID     string `json:"case_id,omitempty"`
	TicketID   string `json:"ticket_id,omitempty"`
	MovementID string `json:"movement_id,omitempty"`
	UserID     string `json:"user_id"`
	Actor string `json:"actor"`
	Action string `json:"action"`
	FromState string `json:"from_state,omitempty"`
	ToState string `json:"to_state,omitempty"`
	Reason  string `json:"reason,omitempty"`
	PolicyVer  int       `json:"policy_version,omitempty"`
	FenceEpoch int64     `json:"fence_epoch,omitempty"`
	At         time.Time `json:"at"`
}

// AuditTrail 审计追踪存储。
type AuditTrail struct {
	mu sync.Mutex
	rows []AuditRecord
}

// NewAuditTrail 创建审计追踪。
  func NewAuditTrail() *AuditTrail { return &AuditTrail{} }

  func (t *AuditTrail) Append(r AuditRecord) AuditRecord {
	  if t == nil {
		   return r
	   }
	   t.mu.Lock()
	  defer t.mu.Unlock()
	  if r.ID == "" {
		  r.ID = uuid.NewString()
	  }
	  if r.At.IsZero() {
		  r.At = time.Now().UTC()
	   }
	  t.rows = append(t.rows, r)
	   return r
  }

   func (t *AuditTrail) List(caseID string, limit int) []AuditRecord {
	   if t == nil {
		   return nil
	  }
	  t.mu.Lock()
	   defer t.mu.Unlock()
	  if limit <= 0 {
		   limit = 200
	   }
	  out := make([]AuditRecord, 0, limit)
	  for i := len(t.rows) - 1; i >= 0 && len(out) < limit; i-- {
		   if caseID != "" && t.rows[i].CaseID != caseID {
			  continue
		   }
		  out = append(out, t.rows[i])
	   }
	  return out
   }

   func (t *AuditTrail) ListByMovement(movementID string, limit int) []AuditRecord {
	  if t == nil {
		  return nil
	  }
	   t.mu.Lock()
	  defer t.mu.Unlock()
	   if limit <= 0 {
		  limit = 200
	   }
	   out := make([]AuditRecord, 0, limit)
	  for i := len(t.rows) - 1; i >= 0 && len(out) < limit; i-- {
		   if movementID != "" && t.rows[i].MovementID != movementID {
			  continue
		  }
		   out = append(out, t.rows[i])
	  }
	   return out
   }

  func (t *AuditTrail) Count() int {
	  if t == nil {
		   return 0
	   }
	  t.mu.Lock()
	  defer t.mu.Unlock()
	  return len(t.rows)
   }
