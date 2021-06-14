package fundmesh

import (
	"sync"
	"time"
)

type AuditEvent struct {
	ID string `json:"id"`
	Kind       string `json:"kind"`
	WithdrawID string `json:"withdraw_id"`
	Entry EntryKind `json:"entry"`
	Code   string `json:"code"`
	Detail string `json:"detail"`
	At time.Time `json:"at"`
}

type AuditLog struct {
	mu   sync.Mutex
	rows []AuditEvent
}

  func (a *AuditLog) Append(ev AuditEvent) {
	  if a == nil {
		  return
	  }
	  if ev.At.IsZero() {
		  ev.At = time.Now().UTC()
	   }
	  a.mu.Lock()
	   defer a.mu.Unlock()
	   a.rows = append(a.rows, ev)
   }

   func (a *AuditLog) List() []AuditEvent {
	  if a == nil {
		   return nil
	  }
	   a.mu.Lock()
	   defer a.mu.Unlock()
	  out := make([]AuditEvent, len(a.rows))
	   copy(out, a.rows)
	  return out
   }
