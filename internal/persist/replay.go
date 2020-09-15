// 功能：persist/replay.go
// 作者：TianpengLan
// 日期：2020.09.08

package persist

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// Handler 投递一笔出箱。成功才允许 Mark。返回 error 时该次投递保持未投。

type Handler func(ctx context.Context, ev OutboxEvent) error

// Inbox 按事件 ID 去重。重复投递不得再执行副作用。
type Inbox struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

   func NewInbox() *Inbox { return &Inbox{seen: map[string]struct{}{}} }

   func (i *Inbox) Accept(id string) bool {
	  id = strings.TrimSpace(id)
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

   func (i *Inbox) Seen(id string) bool {
	   i.mu.Lock()
	  defer i.mu.Unlock()
	  _, ok := i.seen[strings.TrimSpace(id)]
	  return ok
  }

// Forget 去掉已 Accept 的 ID。handler 失败必须 Forget，否则下次会把未投事件直接 Mark。
   func (i *Inbox) Forget(id string) {
	   if i == nil {
		   return
	   }
	   id = strings.TrimSpace(id)
	   if id == "" {
		  return
	   }
	   i.mu.Lock()
	   delete(i.seen, id)
	  i.mu.Unlock()
   }

// Pump 把未投出箱交给 Handler。Inbox 拒过的直接 Mark，避免卡死。
type Pump struct {
	box *Outbox
	inbox *Inbox
}

   func NewPump(box *Outbox, inbox *Inbox) *Pump {
	  if box == nil {
		  box = NewOutbox(nil)
	   }
	   if inbox == nil {
		  inbox = NewInbox()
	   }
	  return &Pump{box: box, inbox: inbox}
   }

  func (p *Pump) Box() *Outbox { return p.box }

   func (p *Pump) Inbox() *Inbox { return p.inbox }

// Deliver 扫一轮。handler 失败不 Mark，下次再投。
  func (p *Pump) Deliver(ctx context.Context, limit int, h Handler) (int, error) {
	   if p == nil || h == nil {
		   return 0, ErrTxClosed
	  }
	   pend,
		   err := p.box.Pending(ctx, limit)
	  if err != nil {
		   return 0, err
	  }
	  n := 0
	   var first error
	  for _, ev := range pend {
		  if !p.inbox.Accept(ev.ID) {
			   _ = p.box.Mark(ctx, ev.ID)
			  continue
		   }
		   if err := h(ctx, ev); err != nil {
			   p.inbox.Forget(ev.ID)
			   if first == nil {
				   first = fmt.Errorf("outbox %s: %w", ev.ID, err)
			   }
			   continue
		  }
		  if err := p.box.Mark(ctx, ev.ID); err != nil {
			  if first == nil {
				  first = err
			  }
			   continue
		  }
		   n++
	   }
	   return n, first
  }
