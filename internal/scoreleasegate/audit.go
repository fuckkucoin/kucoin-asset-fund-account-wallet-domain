package scoreleasegate

import "sync"

type AuditLog struct {
	mu  sync.Mutex
	cap int
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

   func (a *AuditLog) Snapshot() []AuditEvent {
	   if a == nil {
		   return nil
	  }
	  a.mu.Lock()
	  defer a.mu.Unlock()
	  out := make([]AuditEvent, len(a.ring))
	  copy(out, a.ring)
	   return out
   }

   func (a *AuditLog) Filter(kind string) []AuditEvent {
	  all := a.Snapshot()
	   out := make([]AuditEvent, 0)
	   for _, ev := range all {
		  if kind == "" || ev.Kind == kind {
			   out = append(out, ev)
		   }
	   }
	  return out
   }
