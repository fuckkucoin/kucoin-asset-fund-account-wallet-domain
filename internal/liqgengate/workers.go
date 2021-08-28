package liqgengate

import (
	"context"
	"sync"
	"time"
)

// WorkerConfig 巡检间隔。
type WorkerConfig struct {
	Interval  time.Duration
	BatchSize int
}

   func (c WorkerConfig) withDefaults() WorkerConfig {
	   if c.Interval <= 0 {
		   c.Interval = 30 * time.Second
	   }
	   if c.BatchSize <= 0 {
		   c.BatchSize = 200
	  }
	  return c
  }

type loopWorker struct {
	name string
	eng  *Engine
	cfg WorkerConfig
	fn func(context.Context, int) (int, error)
	stop chan struct{}
	done   chan struct{}
	once   sync.Once
	mu     sync.Mutex
	lastN  int
	lastAt time.Time
	lastErr error
}

  func newLoop(name string, eng *Engine, cfg WorkerConfig, fn func(context.Context, int) (int, error)) *loopWorker {
	   return &loopWorker{name: name, eng: eng, cfg: cfg.withDefaults(), fn: fn, stop: make(chan struct{}), done: make(chan struct{})}
}

   func (w *loopWorker) Start(ctx context.Context) {
	   go func() {
		   defer close(w.done)
		  t := time.NewTicker(w.cfg.Interval)
		   defer t.Stop()
		   w.tick(ctx)
		   for {
			  select {
			  case <-ctx.Done():
				  return
			   case <-w.stop:
				   return
			  case <-t.C:
				   w.tick(ctx)
			   }
		  }
	  }()
  }

  func (w *loopWorker) tick(ctx context.Context) {
	  n, err := w.fn(ctx, w.cfg.BatchSize)
	   w.mu.Lock()
	  w.lastN = n
	   w.lastErr = err
	  w.lastAt = time.Now().UTC()
	   w.mu.Unlock()
	   switch w.name {
	   case "half":
		  addHalfInspect(n)
	  case "race":
		   addRaceDiscover(n)
	  case "expiry":
		   addExpiryScan(n)
	  }
   }

  func (w *loopWorker) Stop() {
	   w.once.Do(func() { close(w.stop) })
	  <-w.done
   }

  func (w *loopWorker) Snapshot() (int, time.Time, error) {
	   w.mu.Lock()
	  defer w.mu.Unlock()
	   return w.lastN, w.lastAt, w.lastErr
  }

// HalfInspectWorker 半截冻结巡检。
type HalfInspectWorker struct{ *loopWorker }

// NewHalfInspectWorker 构造。

   func NewHalfInspectWorker(eng *Engine, cfg WorkerConfig) *HalfInspectWorker {
	  return &HalfInspectWorker{newLoop("half", eng, cfg, eng.InspectHalf)}
   }

// RaceDiscoverWorker 世代冲突发现泵。
type RaceDiscoverWorker struct{ *loopWorker }

// NewRaceDiscoverWorker 构造。
   func NewRaceDiscoverWorker(eng *Engine, cfg WorkerConfig) *RaceDiscoverWorker {
	  return &RaceDiscoverWorker{newLoop("race", eng, cfg, eng.DiscoverRace)}
   }

// ExpiryScanWorker 过期租约扫描。

type ExpiryScanWorker struct{ *loopWorker }

// NewExpiryScanWorker 构造。

  func NewExpiryScanWorker(eng *Engine, cfg WorkerConfig) *ExpiryScanWorker {
	  return &ExpiryScanWorker{newLoop("expiry", eng, cfg, eng.ScanExpired)}
  }

// ProofProveWorker 证明固化泵。
type ProofProveWorker struct{ *loopWorker }

// NewProofProveWorker 构造。
  func NewProofProveWorker(eng *Engine, cfg WorkerConfig) *ProofProveWorker {
	  return &ProofProveWorker{newLoop("prove", eng, cfg, eng.ProveOpen)}
  }

// RunOnce 同步跑一轮（与运维）。

   func (w *HalfInspectWorker) RunOnce(ctx context.Context) (int, error) {
	   return w.eng.InspectHalf(ctx, w.cfg.BatchSize)
   }

// RunOnce 同步一轮。
   func (w *RaceDiscoverWorker) RunOnce(ctx context.Context) (int, error) {
	   return w.eng.DiscoverRace(ctx,
		   w.cfg.BatchSize)
  }

// RunOnce 同步一轮。

  func (w *ExpiryScanWorker) RunOnce(ctx context.Context) (int, error) {
	   return w.eng.ScanExpired(ctx, w.cfg.BatchSize)
  }

// RunOnce 同步一轮。
  func (w *ProofProveWorker) RunOnce(ctx context.Context) (int, error) {
	  return w.eng.ProveOpen(ctx, w.cfg.BatchSize)
  }
