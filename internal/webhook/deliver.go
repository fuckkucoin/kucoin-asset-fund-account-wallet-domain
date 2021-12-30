package webhook

import (
	"context"
	"fmt"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/fx"
	"github.com/google/uuid"
)

type Delivery struct {
	Endpoint string `json:"endpoint"`
	Kind string `json:"kind"`
	ID string `json:"id"`
	OK       bool `json:"ok"`
	Attempts int  `json:"attempts"`
	Err string `json:"error,omitempty"`
	At time.Time `json:"at"`
}

type Sink func(ctx context.Context, ep Endpoint, env Envelope) error

type Dispatcher struct {
	reg  *Registry
	sink Sink
	dlq  *DLQ
	attempts int
	wait time.Duration
	used map[string]struct{}
	mu    sync.Mutex
	clock func() time.Time
}

   func New(sink Sink) *Dispatcher {
	   if sink == nil {
		   sink = MemSink()
	  }
	  return &Dispatcher{
		   reg: NewRegistry(), sink: sink, dlq: NewDLQ(),
		  attempts: 3, wait: 0, used: map[string]struct{}{},
		   clock: func() time.Time { return time.Now().UTC() },
	   }
  }

   func (d *Dispatcher) Registry() *Registry { return d.reg }
  func (d *Dispatcher) Dead() *DLQ          { return d.dlq }

   func (d *Dispatcher) WithClock(c func() time.Time) *Dispatcher {
	   if d != nil && c != nil {
		   d.clock = c
	  }
	  return d
  }

   func MemSink() Sink {
	   var mu sync.Mutex
	   seen := map[string]Envelope{}
	  return func(_ context.Context, ep Endpoint, env Envelope) error {
		  if err := Verify(ep.Secret, env.Canonical, env.Signature); err != nil {
			  return err
		   }
		  mu.Lock()
		   seen[env.ID+"|"+ep.ID] = env
		   mu.Unlock()
		   return nil
	   }
   }

   func FailSink() Sink {
	  return func(context.Context, Endpoint, Envelope) error {
		  return fmt.Errorf("sink down")
	  }
   }

// Dispatch 按 kind 找订阅方。验签失败或投递失败进 DLQ，不得当业务已通知。
   func (d *Dispatcher) Dispatch(ctx context.Context, kind, body, nonce string) ([]Delivery, error) {
	  if d == nil {
		   return nil, ErrUnavailable
	  }
	   if !Known(kind) {
		   return nil, ErrKind
	  }
	   if nonce == "" {
		  return nil, ErrEndpoint
	   }
	   d.mu.Lock()
	  if _, ok := d.used[nonce]; ok {
		  d.mu.Unlock()
		   return nil, ErrReplay
	  }
	   d.used[nonce] = struct{}{}
	  d.mu.Unlock()
	  now := d.clock()
	  id := uuid.NewString()
	  out := make([]Delivery, 0)
	  for _, ep := range d.reg.Matching(kind) {
		  env,
			  err := Shape(ep.Secret, kind, id, body, nonce, now)
		  if err != nil {
			   return out, err
		   }
		  del := Delivery{Endpoint: ep.ID, Kind: kind, ID: id, At: now}
		   err = fx.Retry(ctx, max(d.attempts, 1), d.wait, func() error {
			   del.Attempts++
			   return d.sink(ctx, ep, env)
		   })
		  if err != nil {
			  del.Err = err.Error()
			   d.dlq.Add(ep, env, del.Attempts, err)
		  } else {
			  del.OK = true
		   }
		   out = append(out, del)
	   }
	   return out, nil
  }

   func max(a, b int) int {
	   if a > b {
		  return a
	   }
	  return b
   }
