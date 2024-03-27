package incdef

import (
	"sync"
	"time"
)

type CircuitState string

const (
	CircuitClosed CircuitState = "closed"
	CircuitOpen   CircuitState = "open"
	CircuitHalfOpen CircuitState = "half_open"
)

// Breaker 连续失败打开。打开期间直接拒；半开只放一次探针。
type Breaker struct {
	mu   sync.Mutex
	name string
	fail int
	threshold int
	openFor   time.Duration
	openedAt  time.Time
	state CircuitState
	clock func() time.Time
}

  func NewBreaker(name string, threshold int, openFor time.Duration) *Breaker {
	   if threshold <= 0 {
		  threshold = 5
	  }
	  if openFor <= 0 {
		   openFor = 3 * time.Second
	   }
	  return &Breaker{name: name, threshold: threshold, openFor: openFor, state: CircuitClosed, clock: func() time.Time { return time.Now().UTC() }}
   }

  func (b *Breaker) Allow() error {
	  if b == nil {
		  return nil
	  }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  now := b.clock()
	  switch b.state {
	   case CircuitOpen:
		  if now.Sub(b.openedAt) >= b.openFor {
			  b.state = CircuitHalfOpen
			   return nil
		  }
		   return ErrCircuit
	  default:
		   return nil
	  }
  }

  func (b *Breaker) Success() {
	  if b == nil {
		   return
	   }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	   b.fail = 0
	  b.state = CircuitClosed
  }

  func (b *Breaker) Failure() {
	   if b == nil {
		   return
	   }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  b.fail++
	   if b.state == CircuitHalfOpen || b.fail >= b.threshold {
		   b.state = CircuitOpen
		   b.openedAt = b.clock()
	   }
   }

   func (b *Breaker) Snap() CircuitSnap {
	  if b == nil {
		  return CircuitSnap{}
	   }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	   return CircuitSnap{Name: b.name,
		   State: string(b.state), Fails: b.fail}
   }
