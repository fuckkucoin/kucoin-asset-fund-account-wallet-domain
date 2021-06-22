package fx

import (
	"fmt"
	"sync"
	"time"
)

type CircuitState string

const (
	CircuitClosed CircuitState = "closed"
	CircuitOpen     CircuitState = "open"
	CircuitHalfOpen CircuitState = "half_open"
)

// Circuit 连续失败打开。打开期间直接拒，半开只放一次探针。
// 给链观察 / KYT 外呼用，不得用来对同一出金换幂等键重放。

type Circuit struct {
	mu sync.Mutex
	fail      int
	threshold int
	openFor time.Duration
	openedAt time.Time
	state CircuitState
	clock Clock
}

   func NewCircuit(threshold int, openFor time.Duration) *Circuit {
	   if threshold <= 0 {
		  threshold = 5
	  }
	   if openFor <= 0 {
		   openFor = 3 * time.Second
	   }
	  return &Circuit{threshold: threshold, openFor: openFor, state: CircuitClosed, clock: UTC()}
  }

   func (c *Circuit) Allow() error {
	   if c == nil {
		  return nil
	  }
	  c.mu.Lock()
	  defer c.mu.Unlock()
	  now := c.clock()
	  switch c.state {
	  case CircuitOpen:
		  if now.Sub(c.openedAt) >= c.openFor {
			  c.state = CircuitHalfOpen
			  return nil
		  }
		   return fmt.Errorf("circuit open")
	   default:
		   return nil
	  }
  }

   func (c *Circuit) Success() {
	  if c == nil {
		  return
	  }
	  c.mu.Lock()
	   defer c.mu.Unlock()
	   c.fail = 0
	   c.state = CircuitClosed
   }

  func (c *Circuit) Failure() {
	  if c == nil {
		  return
	   }
	   c.mu.Lock()
	   defer c.mu.Unlock()
	   c.fail++
	   if c.state == CircuitHalfOpen || c.fail >= c.threshold {
		   c.state = CircuitOpen
		   c.openedAt = c.clock()
	  }
  }

   func (c *Circuit) State() CircuitState {
	  if c == nil {
		  return CircuitClosed
	  }
	  c.mu.Lock()
	   defer c.mu.Unlock()
	   return c.state
  }
