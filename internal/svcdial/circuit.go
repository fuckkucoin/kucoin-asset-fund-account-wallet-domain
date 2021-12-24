package svcdial

import (
	"sync"
	"time"
)

// Circuit 单服务熔断状态。
type Circuit struct {
	mu        sync.Mutex
	failures  int
	threshold int
	openUntil time.Time
	halfOpen   bool
	resetAfter time.Duration
	clock func() time.Time
}

  func NewCircuit(threshold int, resetAfter time.Duration) *Circuit {
	  if threshold <= 0 {
		  threshold = 5
	  }
	  if resetAfter <= 0 {
		  resetAfter = 2 * time.Second
	  }
	   return &Circuit{
		   threshold: threshold,
		   resetAfter: resetAfter,
		  clock:      func() time.Time { return time.Now().UTC() },
	   }
  }

// Allow 是否允许发起拨号。
   func (c *Circuit) Allow() bool {
	  if c == nil {
		  return true
	  }
	   c.mu.Lock()
	  defer c.mu.Unlock()
	  now := c.clock()
	  if c.openUntil.IsZero() || now.After(c.openUntil) {
		   if !c.openUntil.IsZero() {
			  c.halfOpen = true
			  c.openUntil = time.Time{}
		   }
		  return true
	  }
	   return false
  }

  func (c *Circuit) Success() {
	  if c == nil {
		  return
	   }
	   c.mu.Lock()
	   c.failures = 0
	   c.halfOpen = false
	  c.openUntil = time.Time{}
	  c.mu.Unlock()
   }

  func (c *Circuit) Failure() {
	  if c == nil {
		   return
	   }
	  c.mu.Lock()
	   defer c.mu.Unlock()
	   c.failures++
	   if c.halfOpen || c.failures >= c.threshold {
		  c.openUntil = c.clock().Add(c.resetAfter)
		  c.halfOpen = false
		  c.failures = 0
	  }
  }

  func (c *Circuit) Open() bool {
	  if c == nil {
		  return false
	  }
	   c.mu.Lock()
	   defer c.mu.Unlock()
	   return !c.openUntil.IsZero() && c.clock().Before(c.openUntil)
  }
