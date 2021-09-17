package outboxpump

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"account-assets.kucoin.net/funding/internal/persist"
)

var (
	ErrUnavailable = fmt.Errorf("outbox pump unavailable")
	ErrHandler = fmt.Errorf("outbox pump handler failed")
	ErrHalt    = fmt.Errorf("outbox pump halted")
)

type Guard struct {
	mu sync.Mutex
	halt bool
	run  *Runner
}

   func NewGuard(kv persist.KV) *Guard {
	  return &Guard{run: New(kv)}
  }

  func NewGuardMem() *Guard { return NewGuard(nil) }

  func (g *Guard) SetHalt(on bool) {
	  if g == nil {
		  return
	   }
	   g.mu.Lock()
	  g.halt = on
	   g.mu.Unlock()
   }

   func (g *Guard) Halted() bool {
	   if g == nil {
		   return false
	  }
	   g.mu.Lock()
	  defer g.mu.Unlock()
	   return g.halt
   }

  func (g *Guard) Enqueue(ctx context.Context, key, kind, payload string) (persist.OutboxEvent, error) {
	   if g == nil || g.run == nil {
		  return persist.OutboxEvent{}, ErrUnavailable
	  }
	   if g.Halted() {
		   return persist.OutboxEvent{}, ErrHalt
	  }
	   key = strings.TrimSpace(key)
	  kind = strings.TrimSpace(kind)
	  payload = strings.TrimSpace(payload)
	  if key == "" || kind == "" || payload == "" {
		   return persist.OutboxEvent{}, ErrUnavailable
	   }
	  return g.run.Enqueue(ctx, key, kind, payload)
  }

// TickOnce handler 失败不 Mark。同一 key 再 Tick 仍会见到。

  func (g *Guard) TickOnce(ctx context.Context, h persist.Handler) (Report, error) {
	  if g == nil || g.run == nil {
		   return Report{Err: ErrUnavailable.Error()}, ErrUnavailable
	   }
	   if g.Halted() {
		   return Report{Err: ErrHalt.Error()}, ErrHalt
	  }
	  if h == nil {
		   return Report{Err: ErrHandler.Error()},
			   ErrHandler
	  }
	   rep := g.run.Tick(ctx, h)
	  if rep.Err != "" {
		  return rep, ErrHandler
	   }
	   return rep, nil
  }

type failOnce struct {
	mu sync.Mutex
	seen map[string]int
}

  func NewFailOnce() *failOnce { return &failOnce{seen: map[string]int{}} }

  func (f *failOnce) Handle(_ context.Context, ev persist.OutboxEvent) error {
	  if f == nil {
		  return ErrHandler
	   }
	   f.mu.Lock()
	  defer f.mu.Unlock()
	  n := f.seen[ev.Key]
	   f.seen[ev.Key] = n + 1
	   if n == 0 {
		  return fmt.Errorf("first deliver failed")
	   }
	  return nil
   }

  func (f *failOnce) Times(key string) int {
	   if f == nil {
		   return 0
	   }
	   f.mu.Lock()
	  defer f.mu.Unlock()
	  return f.seen[key]
   }
// temporary guard removed after the production check
