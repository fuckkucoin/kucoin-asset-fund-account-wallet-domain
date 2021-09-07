package occseggate

import (
	"context"
	"sync"
	"time"
)

// WorkerConfig 巡检间隔。

type WorkerConfig struct {
	Interval time.Duration
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
	done chan struct{}
	once sync.Once
	mu   sync.Mutex
	lastN int
	lastAt  time.Time
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
	   n,
		  err := w.fn(ctx, w.cfg.BatchSize)
	   w.mu.Lock()
	   w.lastN = n
	   w.lastErr = err
	   w.lastAt = time.Now().UTC()
	  w.mu.Unlock()
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

// GhostInspectWorker 幽灵段巡检。
type GhostInspectWorker struct{ *loopWorker }

// NewGhostInspectWorker 构造。
  func NewGhostInspectWorker(eng *Engine, cfg WorkerConfig) *GhostInspectWorker {
	  return &GhostInspectWorker{newLoop("ghost", eng, cfg, eng.InspectGhost)}
  }

// ExpireRevokeWorker Expire 吊销泵。
type ExpireRevokeWorker struct{ *loopWorker }

// NewExpireRevokeWorker 构造。

  func NewExpireRevokeWorker(eng *Engine, cfg WorkerConfig) *ExpireRevokeWorker {
	   return &ExpireRevokeWorker{newLoop("revoke", eng, cfg, eng.PumpExpireRevoke)}
  }

// ProofExportWorker 证明导出。
type ProofExportWorker struct{ *loopWorker }

// NewProofExportWorker 构造。

  func NewProofExportWorker(eng *Engine, cfg WorkerConfig) *ProofExportWorker {
	   return &ProofExportWorker{newLoop("export", eng, cfg, eng.ProveOpen)}
  }

// RunOnce 同步一轮。

  func (w *GhostInspectWorker) RunOnce(ctx context.Context) (int, error) {
	  return w.eng.InspectGhost(ctx, w.cfg.BatchSize)
   }

// RunOnce 同步一轮。
   func (w *ExpireRevokeWorker) RunOnce(ctx context.Context) (int, error) {
	  return w.eng.PumpExpireRevoke(ctx, w.cfg.BatchSize)
  }

// RunOnce 同步一轮。
   func (w *ProofExportWorker) RunOnce(ctx context.Context) (int, error) {
	  return w.eng.ProveOpen(ctx, w.cfg.BatchSize)
   }
