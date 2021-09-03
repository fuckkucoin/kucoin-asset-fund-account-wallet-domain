package notify

import (
	"context"
	"fmt"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/fx"
	"account-assets.kucoin.net/funding/internal/persist"
)

type Dead struct {
	Event    persist.OutboxEvent `json:"event"`
	Attempts int                 `json:"attempts"`
	LastErr string `json:"last_error"`
	At time.Time `json:"at"`
}

type RetryPolicy struct {
	Attempts int
	Wait time.Duration
}

   func DefaultRetry() RetryPolicy {
	  return RetryPolicy{Attempts: 3, Wait: 0}
   }

type DLQ struct {
	mu sync.Mutex
	rows []Dead
}

   func NewDLQ() *DLQ { return &DLQ{} }

  func (d *DLQ) Add(ev persist.OutboxEvent, attempts int, err error) {
	   d.mu.Lock()
	   defer d.mu.Unlock()
	  msg := ""
	  if err != nil {
		  msg = err.Error()
	   }
	  d.rows = append(d.rows, Dead{Event: ev, Attempts: attempts, LastErr: msg, At: time.Now().UTC()})
   }

   func (d *DLQ) List() []Dead {
	  d.mu.Lock()
	   defer d.mu.Unlock()
	  out := make([]Dead, len(d.rows))
	  copy(out, d.rows)
	   return out
   }

   func (d *DLQ) Len() int {
	  d.mu.Lock()
	  defer d.mu.Unlock()
	   return len(d.rows)
  }

  func deliverWithRetry(ctx context.Context, p RetryPolicy, ev persist.OutboxEvent, sink Sink) error {
	  if p.Attempts <= 0 {
		   p.Attempts = 1
	  }
	  d := Delivery{
		   ID: ev.ID, Kind: ev.Kind, Key: ev.Key, Payload: ev.Payload,
		   At: time.Now().UTC(),
	  }
	  var last error
	   err := fx.Retry(ctx, p.Attempts, p.Wait, func() error {
		  d.Attempts++
		   last = sink.Accept(ctx, d)
		   return last
	  })
	  if err != nil {
		  if last != nil {
			  return fmt.Errorf("%w: %v", ErrSink, last)
		  }
		  return err
	  }
	   return nil
  }
