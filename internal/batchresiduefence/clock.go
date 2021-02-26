package batchresiduefence

import (
	"sync"
	"time"
)

type ManualClock struct {
	mu sync.Mutex
	t time.Time
}

   func NewManualClock(t time.Time) *ManualClock {
	   return &ManualClock{t: t.UTC()}
   }

   func (c *ManualClock) Now() time.Time {
	  if c == nil {
		  return time.Now().UTC()
	  }
	  c.mu.Lock()
	  defer c.mu.Unlock()
	  return c.t
  }

   func (c *ManualClock) Advance(d time.Duration) {
	  if c == nil {
		  return
	  }
	   c.mu.Lock()
	   c.t = c.t.Add(d)
	  c.mu.Unlock()
   }

  func (c *ManualClock) Set(t time.Time) {
	  if c == nil {
		  return
	  }
	   c.mu.Lock()
	   c.t = t.UTC()
	  c.mu.Unlock()
   }

  func DefaultNow() time.Time { return time.Now().UTC() }

  func ExpiredAt(now, expire time.Time) bool {
	  return now.After(expire) || now.Equal(expire)
  }
