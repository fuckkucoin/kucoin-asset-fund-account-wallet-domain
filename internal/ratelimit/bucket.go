package ratelimit

import (
	"sync"
	"time"
)

// Bucket 令牌桶。容量用尽直接拒，不得透支。

type Bucket struct {
	cap    float64
	rate   float64
	tokens float64
	last time.Time
}

  func NewBucket(capacity int, refillPerSec float64) *Bucket {
	  if capacity <= 0 {
		  capacity = 1
	   }
	   if refillPerSec <= 0 {
		   refillPerSec = 1
	  }
	  return &Bucket{
		   cap: float64(capacity), rate: refillPerSec,
		  tokens: float64(capacity),
	   }
  }

  func (b *Bucket) take(now time.Time) bool {
	   if now.IsZero() {
		   now = time.Now().UTC()
	   }
	  if b.last.IsZero() {
		  b.last = now
	   }
	  elapsed := now.Sub(b.last).Seconds()
	   if elapsed > 0 {
		   b.tokens += elapsed * b.rate
		   if b.tokens > b.cap {
			  b.tokens = b.cap
		   }
		  b.last = now
	  }
	  if b.tokens < 1 {
		   return false
	   }
	  b.tokens--
	   return true
  }

type Table struct {
	mu sync.Mutex
	rows map[string]*Bucket
	cap  int
	rate float64
	clock func() time.Time
}

   func NewTable(capacity int, refillPerSec float64) *Table {
	  if capacity <= 0 {
		  capacity = 30
	  }
	  if refillPerSec <= 0 {
		  refillPerSec = 10
	  }
	   return &Table{
		   rows: map[string]*Bucket{}, cap: capacity, rate: refillPerSec,
		  clock: func() time.Time { return time.Now().UTC() },
	  }
   }

   func (t *Table) WithClock(c func() time.Time) *Table {
	  if t != nil && c != nil {
		   t.clock = c
	  }
	   return t
  }

  func (t *Table) Allow(key string) error {
	   if t == nil {
		   return ErrUnavailable
	   }
	  if key == "" {
		   return ErrActor
	  }
	  t.mu.Lock()
	   defer t.mu.Unlock()
	  b := t.rows[key]
	  if b == nil {
		  b = NewBucket(t.cap, t.rate)
		   t.rows[key] = b
	  }
	   if !b.take(t.clock()) {
		  return ErrLimited
	   }
	   return nil
  }

  func (t *Table) Remaining(key string) float64 {
	   if t == nil {
		  return 0
	  }
	  t.mu.Lock()
	  defer t.mu.Unlock()
	  b := t.rows[key]
	   if b == nil {
		  return float64(t.cap)
	  }
	  return b.tokens
  }
