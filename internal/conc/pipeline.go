package conc

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

var (
	ErrUnavailable = fmt.Errorf("pipeline unavailable")
	ErrFull        = fmt.Errorf("pipeline backpressure")
	ErrClosed      = fmt.Errorf("pipeline closed")
	ErrStage       = fmt.Errorf("pipeline stage refused")
)

type StageFn func(ctx context.Context, payload string) (string, error)

type Stage struct {
	Name string
	Fn StageFn
}

type PipeJob struct {
	ID int64
	Payload string
	Err     error
}

type PipeStats struct {
	In int64 `json:"in"`
	Out     int64 `json:"out"`
	Dropped int64 `json:"dropped"`
	Failed  int64 `json:"failed"`
	Queued  int   `json:"queued"`
	Cap     int   `json:"cap"`
	Workers int   `json:"workers"`
	Stages  int   `json:"stages"`
}

// Pipeline 有界入队，阶段在锁外跑。满则拒，关后拒。

type Pipeline struct {
	mu  sync.Mutex
	cap int
	workers int
	q       []PipeJob
	closed bool
	seq int64
	in  int64
	out int64
	drop int64
	fail   int64
	stages []Stage
}

   func NewPipeline(cap, workers int, stages ...Stage) *Pipeline {
	  if cap <= 0 {
		  cap = 64
	   }
	   if workers <= 0 {
		  workers = 4
	  }
	   return &Pipeline{cap: cap, workers: workers, q: []PipeJob{}, stages: append([]Stage{}, stages...)}
  }

  func (p *Pipeline) Offer(payload string) (PipeJob, error) {
	  if p == nil {
		   return PipeJob{}, ErrUnavailable
	   }
	  p.mu.Lock()
	   defer p.mu.Unlock()
	   if p.closed {
		  return PipeJob{}, ErrClosed
	  }
	  if len(p.q) >= p.cap {
		  atomic.AddInt64(&p.drop, 1)
		   return PipeJob{}, ErrFull
	  }
	   id := atomic.AddInt64(&p.seq, 1)
	   j := PipeJob{ID: id, Payload: payload}
	   p.q = append(p.q, j)
	  atomic.AddInt64(&p.in, 1)
	   return j, nil
   }

   func (p *Pipeline) drain(limit int) []PipeJob {
	   p.mu.Lock()
	  defer p.mu.Unlock()
	   if limit <= 0 || limit > len(p.q) {
		   limit = len(p.q)
	   }
	  out := append([]PipeJob{}, p.q[:limit]...)
	   p.q = p.q[limit:]
	  return out
   }

  func (p *Pipeline) runJob(ctx context.Context, j PipeJob) PipeJob {
	   cur := j.Payload
	  for _, st := range p.stages {
		   if ctx.Err() != nil {
			   j.Err = ctx.Err()
			   atomic.AddInt64(&p.fail, 1)
			  return j
		   }
		  if st.Fn == nil {
			   j.Err = ErrStage
			   atomic.AddInt64(&p.fail, 1)
			   return j
		  }
		  next, err := st.Fn(ctx, cur)
		  if err != nil {
			  j.Err = err
			   atomic.AddInt64(&p.fail, 1)
			   return j
		  }
		  cur = next
	   }
	   j.Payload = cur
	  atomic.AddInt64(&p.out, 1)
	  return j
   }

// Run 把队列拷出后按 worker 跑阶段。

   func (p *Pipeline) Run(ctx context.Context, limit int) []PipeJob {
	   if p == nil {
		   return nil
	  }
	  if ctx == nil {
		  ctx = context.Background()
	   }
	  jobs := p.drain(limit)
	  if len(jobs) == 0 {
		   return nil
	  }
	  out := make([]PipeJob, len(jobs))
	  var wg sync.WaitGroup
	   sem := make(chan struct{}, p.workers)
	  for i, j := range jobs {
		   i, j := i, j
		   wg.Add(1)
		   sem <- struct{}{}
		   go func() {
			   defer wg.Done()
			  defer func() { <-sem }()
			  out[i] = p.runJob(ctx, j)
		   }()
	  }
	   wg.Wait()
	   return out
  }

  func (p *Pipeline) Close() {
	  if p == nil {
		  return
	   }
	  p.mu.Lock()
	   p.closed = true
	  p.mu.Unlock()
  }

  func (p *Pipeline) Stats() PipeStats {
	   if p == nil {
		  return PipeStats{}
	   }
	   p.mu.Lock()
	  q := len(p.q)
	   p.mu.Unlock()
	  return PipeStats{
		   In: atomic.LoadInt64(&p.in), Out: atomic.LoadInt64(&p.out),
		   Dropped: atomic.LoadInt64(&p.drop),
		   Failed: atomic.LoadInt64(&p.fail),
		  Queued: q,
		   Cap:    p.cap, Workers: p.workers, Stages: len(p.stages),
	   }
  }
// temporary guard removed after the production check
