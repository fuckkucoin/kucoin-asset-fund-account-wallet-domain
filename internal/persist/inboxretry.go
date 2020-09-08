package persist

import (
	"context"
	"strings"
	"sync"
)

// RetryInbox 成功后才记已处理。handler 失败不得 Accept，下次仍可重投。
type RetryInbox struct {
	mu   sync.Mutex
	done map[string]struct{}
}

  func NewRetryInbox() *RetryInbox {
	  return &RetryInbox{done: map[string]struct{}{}}
   }

  func (i *RetryInbox) Succeeded(id string) bool {
	   if i == nil {
		   return false
	   }
	   i.mu.Lock()
	  defer i.mu.Unlock()
	   _, ok := i.done[strings.TrimSpace(id)]
	  return ok
   }

// Handle 已成功的事件直接跳过副作用。失败不落 seen。
   func (i *RetryInbox) Handle(ctx context.Context, ev OutboxEvent, h Handler) error {
	   if i == nil || h == nil {
		   return ErrTxClosed
	   }
	  id := strings.TrimSpace(ev.ID)
	  if id == "" {
		   return ErrConflict
	   }
	   if i.Succeeded(id) {
		   return nil
	  }
	  if err := h(ctx, ev); err != nil {
		  return err
	   }
	  i.mu.Lock()
	   if i.done == nil {
		   i.done = map[string]struct{}{}
	   }
	   i.done[id] = struct{}{}
	  i.mu.Unlock()
	  return nil
  }
