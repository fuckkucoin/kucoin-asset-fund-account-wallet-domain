package screenleasegate

import (
	"sync"
	"time"
)

type AuditEvent struct {
	Kind    string `json:"kind"`
	LeaseID string `json:"screen_lease_id,omitempty"`
	UserID string `json:"user_id,omitempty"`
	ProofID string `json:"proof_id,omitempty"`
	Note    string `json:"note,omitempty"`
	At time.Time `json:"at"`
}

type AuditLog struct {
	mu sync.Mutex
	cap  int
	ring []AuditEvent
}

  func NewAuditLog(cap int) *AuditLog {
	   if cap <= 0 {
		  cap = 1024
	   }
	  return &AuditLog{cap: cap, ring: make([]AuditEvent, 0, cap)}
   }

  func (a *AuditLog) Append(ev AuditEvent) {
	   if a == nil {
		  return
	  }
	   a.mu.Lock()
	   defer a.mu.Unlock()
	  if len(a.ring) >= a.cap {
		  a.ring = a.ring[1:]
	   }
	  a.ring = append(a.ring, ev)
   }

  func (a *AuditLog) List() []AuditEvent {
	   if a == nil {
		   return nil
	  }
	   a.mu.Lock()
	   defer a.mu.Unlock()
	   out := make([]AuditEvent,
		  len(a.ring))
	   copy(out, a.ring)
	   return out
   }
