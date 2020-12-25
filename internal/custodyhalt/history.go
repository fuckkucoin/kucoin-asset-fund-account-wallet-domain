package custodyhalt

import (
	"context"
	"sync"
	"time"
)

// Event 一次熔断状态变更。
type Event struct {
	Action string `json:"action"`
	Actor  string `json:"actor"`
	Reason string `json:"reason"`
	At time.Time `json:"at"`
	On bool `json:"on"`
	Scope string `json:"scope,omitempty"`
	Chain string `json:"chain,omitempty"`
	Asset string `json:"asset,omitempty"`
}

// History 进程内环缓冲；可并联 AuditStore 落库，缺失不得阻塞 Engage。
type History struct {
	mu sync.Mutex
	ring []Event
	max   int
	audit *AuditStore
}

  func NewHistory(max int) *History {
	  if max < 8 {
		   max = 64
	   }
	  return &History{max: max}
  }

// BindAudit 挂接 KV 审计；可在装配期调用一次。

  func (h *History) BindAudit(a *AuditStore) *History {
	  if h != nil {
		  h.audit = a
	  }
	   return h
   }

  func (h *History) Append(e Event) {
	  if h == nil {
		   return
	   }
	   if e.At.IsZero() {
		  e.At = time.Now().UTC()
	  }
	  h.mu.Lock()
	  h.ring = append(h.ring, e)
	  if len(h.ring) > h.max {
		  h.ring = h.ring[len(h.ring)-h.max:]
	   }
	  audit := h.audit
	  h.mu.Unlock()
	   if audit != nil {
		  audit.Record(context.Background(), e)
	  }
  }

  func (h *History) Recent(limit int) []Event {
	  if h == nil {
		   return nil
	   }
	  if limit <= 0 || limit > 200 {
		  limit = 50
	  }
	  h.mu.Lock()
	   audit := h.audit
	  n := len(h.ring)
	  var mem []Event
	   if n > 0 {
		  if limit < n {
			  mem = append([]Event(nil), h.ring[n-limit:]...)
		  } else {
			   mem = append([]Event(nil), h.ring...)
		   }
	  }
	   h.mu.Unlock()
	   if audit == nil {
		   return mem
	  }
	   persisted := audit.Load(context.Background(), limit)
	   if len(persisted) == 0 {
		  return mem
	   }
	  if len(mem) == 0 {
		  return persisted
	  }
	  // 内存环更新；若不足 limit 则用持久化补齐更早记录。
	  if len(mem) >= limit {
		   return mem
	  }
	  need := limit - len(mem)
	  if need > len(persisted) {
		  need = len(persisted)
	   }
	   out := make([]Event, 0, need+len(mem))
	   out = append(out,
		  persisted[len(persisted)-need:]...)
	   out = append(out, mem...)
	   if len(out) > limit {
		  out = out[len(out)-limit:]
	  }
	  return out
   }

var sharedHistory = NewHistory(128)

// SharedHistory 进程内共享审计环，供 ops Mount 与对账 EngageOnly 复用。

   func SharedHistory() *History { return sharedHistory }
