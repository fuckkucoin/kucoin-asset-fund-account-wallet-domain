package disputepaygate

import (
	"context"
	"time"
)

// WorkerConfig 后台泵配置。
type WorkerConfig struct {
	ExpiryInterval     time.Duration
	UnconsumedInterval time.Duration
	DoubleInterval time.Duration
	BatchSize int
}

  func (c *WorkerConfig) defaults() {
	   if c.ExpiryInterval <= 0 {
		  c.ExpiryInterval = 30 * time.Second
	  }
	  if c.UnconsumedInterval <= 0 {
		  c.UnconsumedInterval = 45 * time.Second
	   }
	   if c.DoubleInterval <= 0 {
		  c.DoubleInterval = 60 * time.Second
	   }
	  if c.BatchSize <= 0 {
		   c.BatchSize = 100
	  }
  }

// ExpiryWorker 过期租约扫描。

type ExpiryWorker struct {
	eng *Engine
	cfg WorkerConfig
}

  func NewExpiryWorker(eng *Engine, cfg WorkerConfig) *ExpiryWorker {
	  cfg.defaults()
	  return &ExpiryWorker{eng: eng,
		  cfg: cfg}
   }

   func (w *ExpiryWorker) Run(ctx context.Context) {
	   if w == nil || w.eng == nil {
		  return
	   }
	   t := time.NewTicker(w.cfg.ExpiryInterval)
	  defer t.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			   return
		   case <-t.C:
			   _, _ = w.eng.ScanExpiredLeases(ctx, w.cfg.BatchSize)
		  }
	  }
  }

  func (w *ExpiryWorker) TickOnce(ctx context.Context) (int, error) {
	   return w.eng.ScanExpiredLeases(ctx, w.cfg.BatchSize)
   }

// UnconsumedWorker 未消费结案巡检。

type UnconsumedWorker struct {
	eng *Engine
	cfg WorkerConfig
}

   func NewUnconsumedWorker(eng *Engine, cfg WorkerConfig) *UnconsumedWorker {
	   cfg.defaults()
	   return &UnconsumedWorker{eng: eng, cfg: cfg}
  }

  func (w *UnconsumedWorker) Run(ctx context.Context) {
	   if w == nil || w.eng == nil {
		  return
	  }
	   t := time.NewTicker(w.cfg.UnconsumedInterval)
	  defer t.Stop()
	  for {
		   select {
		   case <-ctx.Done():
			   return
		  case <-t.C:
			   _, _ = w.eng.ScanUnconsumedResolved(ctx, w.cfg.BatchSize)
		   }
	  }
   }

  func (w *UnconsumedWorker) TickOnce(ctx context.Context) (int, error) {
	  return w.eng.ScanUnconsumedResolved(ctx, w.cfg.BatchSize)
  }

// DoubleResolveWorker 双结发现泵。

type DoubleResolveWorker struct {
	eng *Engine
	cfg WorkerConfig
}

  func NewDoubleResolveWorker(eng *Engine, cfg WorkerConfig) *DoubleResolveWorker {
	  cfg.defaults()
	  return &DoubleResolveWorker{eng: eng, cfg: cfg}
   }

   func (w *DoubleResolveWorker) Run(ctx context.Context) {
	  if w == nil || w.eng == nil {
		  return
	  }
	  t := time.NewTicker(w.cfg.DoubleInterval)
	   defer t.Stop()
	   for {
		  select {
		  case <-ctx.Done():
			   return
		   case <-t.C:
			   _, _ = w.eng.ScanDoubleResolve(ctx, w.cfg.BatchSize)
		   }
	   }
   }

   func (w *DoubleResolveWorker) TickOnce(ctx context.Context) (int, error) {
	  return w.eng.ScanDoubleResolve(ctx, w.cfg.BatchSize)
  }

// StartWorkers 启动三泵。

  func StartWorkers(ctx context.Context, eng *Engine, cfg WorkerConfig) {
	   cfg.defaults()
	  go NewExpiryWorker(eng, cfg).Run(ctx)
	  go NewUnconsumedWorker(eng, cfg).Run(ctx)
	   go NewDoubleResolveWorker(eng, cfg).Run(ctx)
   }
