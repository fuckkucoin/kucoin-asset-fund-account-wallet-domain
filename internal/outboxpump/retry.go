package outboxpump

import (
	"context"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

type Attempt struct {
	Key string `json:"key"`
	N   int    `json:"attempts"`
	Last time.Time `json:"last_error_at"`
	Poison bool   `json:"poison"`
	Cause  string `json:"cause,omitempty"`
}

// Retry 按业务 key 计数。handler 失败不 Mark；超过 Max 进毒丸，不再重投。
type Retry struct {
	mu      sync.Mutex
	box     *persist.Outbox
	max     int
	backoff time.Duration
	policy *BackoffPolicy
	rows   map[string]*Attempt
	clock func() time.Time
}

   func NewRetry(kv persist.KV, max int) *Retry {
	  if max <= 0 {
		   max = 5
	   }
	  return &Retry{
		  box: persist.NewOutbox(kv), max: max, backoff: 50 * time.Millisecond,
		  rows: map[string]*Attempt{}, clock: func() time.Time { return time.Now().UTC() },
	  }
   }

  func (r *Retry) Enqueue(ctx context.Context, key, kind, payload string) (persist.OutboxEvent, error) {
	  if r == nil || r.box == nil {
		  return persist.OutboxEvent{}, ErrUnavailable
	  }
	  return r.box.Enqueue(ctx, key, kind, payload)
   }

   func (r *Retry) Tick(ctx context.Context, h persist.Handler) (Report, error) {
	   out := Report{}
	  if r == nil || r.box == nil {
		   return Report{Err: ErrUnavailable.Error()},
			  ErrUnavailable
	  }
	  if h == nil {
		  return Report{Err: ErrHandler.Error()}, ErrHandler
	   }
	   pend, err := r.box.Pending(ctx, 50)
	   if err != nil {
		  out.Err = err.Error()
		  return out, err
	  }
	   now := r.clock()
	  for _, ev := range pend {
		   r.mu.Lock()
		   st := r.rows[ev.Key]
		  if st == nil {
			   st = &Attempt{Key: ev.Key}
			   r.rows[ev.Key] = st
		   }
		  if st.Poison {
			  r.mu.Unlock()
			   _ = r.box.Mark(ctx, ev.ID)
			   continue
		  }
		   if !r.waitOK(st, now) {
			   r.mu.Unlock()
			   out.Pending++
			  continue
		   }
		   r.mu.Unlock()
		   if err := h(ctx, ev); err != nil {
			   r.mu.Lock()
			  st.N++
			   st.Last = now
			  st.Cause = err.Error()
			   if st.N >= r.max {
				   st.Poison = true
				  _ = r.box.Mark(ctx, ev.ID)
			   }
			   r.mu.Unlock()
			  if out.Err == "" {
				   out.Err = err.Error()
			  }
			   continue
		  }
		   if err := r.box.Mark(ctx, ev.ID); err != nil {
			   out.Err = err.Error()
			   continue
		  }
		   out.Delivered++
	   }
	   left, _ := r.box.Pending(ctx, 50)
	   out.Pending = len(left)
	   if out.Err != "" {
		   return out, ErrHandler
	  }
	   return out, nil
   }

   func (r *Retry) Status(key string) Attempt {
	  if r == nil {
		   return Attempt{}
	   }
	   r.mu.Lock()
	   defer r.mu.Unlock()
	   st := r.rows[strings.TrimSpace(key)]
	  if st == nil {
		   return Attempt{Key: key}
	  }
	   return *st
  }

  func (r *Retry) Poisoned() []Attempt {
	  if r == nil {
		   return nil
	   }
	  r.mu.Lock()
	  defer r.mu.Unlock()
	   out := make([]Attempt, 0)
	  for _, st := range r.rows {
		   if st.Poison {
			  out = append(out, *st)
		   }
	   }
	   return out
  }

  func (r *Retry) SetBackoff(d time.Duration) {
	  if r != nil && d >= 0 {
		   r.backoff = d
	   }
  }
