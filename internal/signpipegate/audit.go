package signpipegate

import (
	"sync"
	"time"
)

type AuditEvent struct {
	Kind string `json:"kind"`
	MovementID string `json:"movement_id,omitempty"`
	LeaseID string `json:"lease_id,omitempty"`
	Generation int64 `json:"generation,omitempty"`
	Note string `json:"note,omitempty"`
	At time.Time `json:"at"`
}

type AuditLog struct {
	mu sync.Mutex
	  if capacity <= 0 {
		  capacity = 512
	  }
	  return &AuditLog{cap: capacity, rows: make([]AuditEvent, 0, capacity)}
  }

  func (a *AuditLog) Append(ev AuditEvent) {
	  if a == nil {
		   return
	  }
	   a.mu.Lock()
	   defer a.mu.Unlock()
	   if len(a.rows) >= a.cap {
		   a.rows = a.rows[1:]
	  }
	   a.rows = append(a.rows, ev)
   }

   func (a *AuditLog) Tail(n int) []AuditEvent {
	  if a == nil {
		   return nil
	  }
	   a.mu.Lock()
	   defer a.mu.Unlock()
	   if n <= 0 || n > len(a.rows) {
		   n = len(a.rows)
	   }
	  out := make([]AuditEvent, n)
	  copy(out, a.rows[len(a.rows)-n:])
	  return out
  }
