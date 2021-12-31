package workerbnd

import (
	"context"
	"sync"
	"time"
)

// Clock 对账 Tick 时钟；注入假时钟。
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

type RealClock struct{}

  func (RealClock) Now() time.Time { return time.Now().UTC() }

  func (RealClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// FakeClock 表驱动可控推进。
type FakeClock struct {
	mu sync.Mutex
	now time.Time
	wait []fakeWait
}

type fakeWait struct {
	at time.Time
	ch chan time.Time
}

  func NewFakeClock(t time.Time) *FakeClock {
	  return &FakeClock{now: t.UTC()}
   }

   func (c *FakeClock) Now() time.Time {
	  c.mu.Lock()
	  defer c.mu.Unlock()
	   return c.now
   }

   func (c *FakeClock) After(d time.Duration) <-chan time.Time {
	  c.mu.Lock()
	  defer c.mu.Unlock()
	   ch := make(chan time.Time, 1)
	   c.wait = append(c.wait, fakeWait{at: c.now.Add(d), ch: ch})
	   return ch
   }

   func (c *FakeClock) Advance(d time.Duration) {
	   c.mu.Lock()
	  c.now = c.now.Add(d)
	   now := c.now
	  var keep []fakeWait
	   var fire []fakeWait
	   for _, w := range c.wait {
		   if !w.at.After(now) {
			   fire = append(fire, w)
		   } else {
			  keep = append(keep, w)
		  }
	  }
	  c.wait = keep
	   c.mu.Unlock()
	   for _, w := range fire {
		  select {
		  case w.ch <- now:
		  default:
		  }
	  }
   }

// ReconLoop 按间隔触发 FundsRecon / Tick。

type ReconLoop struct {
	interval time.Duration
	clock    Clock
	tick func(context.Context)
	mu sync.Mutex
	last time.Time
	n int
}

  func NewReconLoop(interval time.Duration, clk Clock, tick func(context.Context)) *ReconLoop {
	   if clk == nil {
		   clk = RealClock{}
	   }
	  if interval <= 0 {
		   interval = time.Second
	  }
	  return &ReconLoop{interval: interval, clock: clk, tick: tick}
  }

  func (l *ReconLoop) Tick(ctx context.Context) bool {
	  if l == nil || l.tick == nil {
		  return false
	  }
	  now := l.clock.Now()
	  l.mu.Lock()
	   if !l.last.IsZero() && now.Sub(l.last) < l.interval {
		  l.mu.Unlock()
		  return false
	  }
	   l.last = now
	  l.n++
	  l.mu.Unlock()
	   l.tick(ctx)
	   return true
   }

  func (l *ReconLoop) Count() int {
	   if l == nil {
		  return 0
	   }
	   l.mu.Lock()
	  defer l.mu.Unlock()
	  return l.n
   }

   func (l *ReconLoop) Run(ctx context.Context) error {
	  if l == nil {
		  return ErrBindRecon
	   }
	   l.Tick(ctx)
	  for {
		  select {
		  case <-ctx.Done():
			  return ctx.Err()
		   case <-l.clock.After(l.interval):
			  l.Tick(ctx)
		  }
	  }
  }
