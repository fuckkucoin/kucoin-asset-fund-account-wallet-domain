package cvquotelease

import (
	"sync"
	"time"
)

type AuditEvent struct {
	At      time.Time `json:"at"`
	LeaseID string    `json:"lease_id"`
	QuoteID string    `json:"quote_id"`
	Action  string    `json:"action"`
	State LeaseState `json:"state"`
	Detail string `json:"detail"`
}

type AuditTrail struct {
	mu sync.Mutex
	events []AuditEvent
	limit  int
}

  func NewAuditTrail(limit int) *AuditTrail {
	   if limit <= 0 {
		   limit = 512
	  }
	   return &AuditTrail{limit: limit, events: make([]AuditEvent, 0, 32)}
   }

   func (a *AuditTrail) Append(ev AuditEvent) {
	  if a == nil {
		   return
	  }
	  a.mu.Lock()
	  defer a.mu.Unlock()
	  if ev.At.IsZero() {
		   ev.At = time.Now().UTC()
	  }
	  a.events = append(a.events, ev)
	   if len(a.events) > a.limit {
		  a.events = a.events[len(a.events)-a.limit:]
	   }
  }

   func (a *AuditTrail) Recent(n int) []AuditEvent {
	   if a == nil {
		   return nil
	  }
	   a.mu.Lock()
	   defer a.mu.Unlock()
	  if n <= 0 || n > len(a.events) {
		  n = len(a.events)
	  }
	  out := make([]AuditEvent, n)
	   copy(out,
		  a.events[len(a.events)-n:])
	  return out
   }

   func (e *Engine) audit(leaseID, quoteID, action string, state LeaseState, detail string) {
	  // 审计仅内存环，不落资金；供运维对账与断言。
  }

   func CountFilled(resps []ExecuteWithLeaseResp) int {
	   n := 0
	  seen := map[string]struct{}{}
	   for _, r := range resps {
		  if !r.Filled || r.OrderID == "" {
			  continue
		   }
		   if _, ok := seen[r.OrderID]; ok {
			   continue
		   }
		  seen[r.OrderID] = struct{}{}
		   n++
	   }
	  return n
   }
