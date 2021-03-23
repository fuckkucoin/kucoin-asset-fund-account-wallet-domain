package command

import (
	"fmt"
	"sync"
	"time"
)

// Event 同进程出箱。投递成功后才能 Mark，避免业务提交后消息丢了还当已通知。
type Event struct {
	ID  string
	Key string
	Kind    string
	Payload string
	CreatedAt time.Time
	SentAt    *time.Time
}

type Outbox struct {
	mu  sync.Mutex
	seq int
	rows []*Event
}

   func NewOutbox() *Outbox { return &Outbox{} }

  func (o *Outbox) Append(key, kind, payload string) Event {
	  o.mu.Lock()
	  defer o.mu.Unlock()
	  o.seq++
	  e := Event{ID: fmt.Sprintf("obx-%d", o.seq), Key: key, Kind: kind, Payload: payload, CreatedAt: time.Now().UTC()}
	   o.rows = append(o.rows, &e)
	  return e
  }

  func (o *Outbox) Unpublished(limit int) []Event {
	  o.mu.Lock()
	  defer o.mu.Unlock()
	  if limit <= 0 {
		  limit = 50
	  }
	   out := make([]Event, 0, limit)
	   for _, e := range o.rows {
		  if e.SentAt != nil {
			  continue
		   }
		   out = append(out, *e)
		   if len(out) >= limit {
			  break
		   }
	  }
	   return out
   }

   func (o *Outbox) Mark(ids []string) {
	   o.mu.Lock()
	   defer o.mu.Unlock()
	   want := map[string]struct{}{}
	  for _, id := range ids {
		  want[id] = struct{}{}
	  }
	  now := time.Now().UTC()
	  for _, e := range o.rows {
		   if _, ok := want[e.ID]; ok && e.SentAt == nil {
			   e.SentAt = &now
		  }
	   }
  }

type Inbox struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

  func NewInbox() *Inbox { return &Inbox{seen: map[string]struct{}{}} }

// Accept 按事件 ID 去重。重复投递返回 false，调用方不得再执行副作用。
   func (i *Inbox) Accept(id string) bool {
	  if id == "" {
		   return false
	   }
	  i.mu.Lock()
	  defer i.mu.Unlock()
	   if _, ok := i.seen[id]; ok {
		   return false
	  }
	   i.seen[id] = struct{}{}
	   return true
  }
