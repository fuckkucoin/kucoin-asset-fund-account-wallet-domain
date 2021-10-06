package pumpmarkfence

import (
	"context"
	"sync"
	"account-assets.kucoin.net/funding/internal/persist"
)

// CountSink 全额效果计数。
type CountSink struct {
	mu   sync.Mutex
	byID map[string]int
}

  func NewCountSink() *CountSink { return &CountSink{byID: map[string]int{}} }

   func (s *CountSink) Handler() persist.Handler {
	   return func(_ context.Context, ev persist.OutboxEvent) error {
		   if s == nil {
			  return ErrHandler
		   }
		  s.mu.Lock()
		  if s.byID == nil {
			  s.byID = map[string]int{}
		   }
		   s.byID[ev.ID]++
		  s.mu.Unlock()
		  return nil
	   }
   }

   func (s *CountSink) Times(outboxID string) int {
	   if s == nil {
		  return 0
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   return s.byID[outboxID]
  }

  func (s *CountSink) Total() int {
	   if s == nil {
		   return 0
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   n := 0
	   for _, v := range s.byID {
		  n += v
	  }
	  return n
  }
