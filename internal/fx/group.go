package fx

import (
	"context"
	"sync"
)

// Group 有界并发。一个子任务失败仍等其它结束，返回第一个 error。

type Group struct {
	ctx    context.Context
	cancel context.CancelFunc
	sem    chan struct{}
	wg sync.WaitGroup
	mu  sync.Mutex
	err error
}

   func NewGroup(ctx context.Context, limit int) *Group {
	   if limit <= 0 {
		   limit = 8
	  }
	  c, cancel := context.WithCancel(ctx)
	  return &Group{ctx: c, cancel: cancel, sem: make(chan struct{}, limit)}
   }

   func (g *Group) Go(fn func(context.Context) error) {
	   g.wg.Add(1)
	  go func() {
		   defer g.wg.Done()
		   select {
		  case <-g.ctx.Done():
			  g.fail(g.ctx.Err())
			   return
		  case g.sem <- struct{}{}:
		   }
		  defer func() { <-g.sem }()
		   if err := fn(g.ctx); err != nil {
			   g.fail(err)
		   }
	   }()
}

   func (g *Group) fail(err error) {
	   g.mu.Lock()
	  defer g.mu.Unlock()
	  if g.err == nil {
		  g.err = err
		  g.cancel()
	   }
  }

  func (g *Group) Wait() error {
	   g.wg.Wait()
	  g.cancel()
	   g.mu.Lock()
	   defer g.mu.Unlock()
	   return g.err
  }
