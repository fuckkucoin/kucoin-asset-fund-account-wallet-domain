package notify

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

type Delivery struct {
	ID string `json:"id"`
	Kind string `json:"kind"`
	Key     string    `json:"key"`
	Payload string    `json:"payload"`
	Channel string    `json:"channel"`
	At      time.Time `json:"at"`
	Attempts int `json:"attempts"`
}

type Sink interface {
	Name() string
	Accept(ctx context.Context, d Delivery) error
}

// MemSink 进程内接收。给和同进程 worker 用。
type MemSink struct {
	name string
	mu   sync.Mutex
	rows []Delivery
	fail map[string]int
}

  func NewMemSink(name string) *MemSink {
	  if name == "" {
		  name = "mem"
	   }
	   return &MemSink{name: name, fail: map[string]int{}}
   }

   func (s *MemSink) Name() string { return s.name }

  func (s *MemSink) FailNext(kind string, n int) {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   s.fail[kind] = n
   }

   func (s *MemSink) Accept(_ context.Context, d Delivery) error {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   if n := s.fail[d.Kind]; n > 0 {
		  s.fail[d.Kind] = n - 1
		   return fmt.Errorf("%w: injected", ErrSink)
	  }
	  d.Channel = s.name
	  s.rows = append(s.rows, d)
	  return nil
  }

  func (s *MemSink) List() []Delivery {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  out := make([]Delivery,
		  len(s.rows))
	  copy(out, s.rows)
	  return out
   }

  func (s *MemSink) CountKind(kind string) int {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  n := 0
	  for _, r := range s.rows {
		  if r.Kind == kind {
			  n++
		   }
	   }
	  return n
   }

// MultiSink 按顺序投。一个失败整笔失败，前面成功的由 Inbox 去重兜底。
type MultiSink struct {
	sinks []Sink
}

  func NewMulti(sinks ...Sink) *MultiSink { return &MultiSink{sinks: sinks} }

   func (m *MultiSink) Name() string { return "multi" }

  func (m *MultiSink) Accept(ctx context.Context, d Delivery) error {
	   for _, s := range m.sinks {
		   if s == nil {
			  continue
		   }
		   if err := s.Accept(ctx, d); err != nil {
			  return err
		   }
	   }
	  return nil
   }

   func FilterKind(kind string) string { return strings.TrimSpace(kind) }
