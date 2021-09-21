package occupbucketfence

import (
	"context"
	"time"
)

const (
	defaultPatrolInterval = 45 * time.Second
	defaultHalfInterval   = 30 * time.Second
	defaultExpireInterval = 40 * time.Second
	defaultMetricsInterval = 30 * time.Second
	defaultBatch = 200
)

// WorkerConfig 周期配置。

type WorkerConfig struct {
	PatrolInterval time.Duration
	HalfInterval   time.Duration
	ExpireInterval time.Duration
	MetricsInterval time.Duration
	BatchSize int
	NodeID    string
}

  func (c *WorkerConfig) fill() {
	  if c.PatrolInterval <= 0 {
		   c.PatrolInterval = defaultPatrolInterval
	   }
	  if c.HalfInterval <= 0 {
		   c.HalfInterval = defaultHalfInterval
	  }
	   if c.ExpireInterval <= 0 {
		   c.ExpireInterval = defaultExpireInterval
	  }
	   if c.MetricsInterval <= 0 {
		   c.MetricsInterval = defaultMetricsInterval
	  }
	   if c.BatchSize <= 0 {
		  c.BatchSize = defaultBatch
	   }
	   if c.NodeID == "" {
		  c.NodeID = "occupbucketfence.worker"
	   }
  }

// Worker 巡检集合。

type Worker struct {
	engine *Engine
	store  Store
	metrics *Metrics
	config WorkerConfig
}

// NewWorker 构造。
   func NewWorker(engine *Engine, store Store, metrics *Metrics, cfg WorkerConfig) *Worker {
	   cfg.fill()
	  return &Worker{engine: engine, store: store, metrics: metrics, config: cfg}
  }

// RunAll 启动全部泵。
  func (w *Worker) RunAll(ctx context.Context) {
	  if w == nil || ctx == nil {
		  return
	   }
	   go w.loop(ctx, w.config.PatrolInterval, w.tickPatrol)
	   go w.loop(ctx, w.config.HalfInterval, w.tickHalf)
	  go w.loop(ctx, w.config.ExpireInterval, w.tickExpire)
	  go w.loop(ctx, w.config.MetricsInterval, w.tickMetrics)
  }

   func (w *Worker) loop(ctx context.Context, d time.Duration, fn func(context.Context)) {
	   t := time.NewTicker(d)
	   defer t.Stop()
	  fn(ctx)
	   for {
		  select {
		  case <-ctx.Done():
			  return
		  case <-t.C:
			  fn(ctx)
		   }
	   }
  }

  func (w *Worker) tickPatrol(ctx context.Context) {
	   if w.engine == nil {
		   return
	   }
	   list, err := w.engine.PatrolConserve(ctx, w.config.BatchSize)
	  if w.metrics != nil {
		   if err != nil {
			   w.metrics.RecordScan("patrol", false)
			  return
		   }
		   w.metrics.RecordScan("patrol", true)
		   w.metrics.SetBreakOpen(len(list))
	  }
  }

   func (w *Worker) tickHalf(ctx context.Context) {
	  if w.engine == nil || w.store == nil {
		  return
	   }
	  list, err := w.store.ListOpenHalves(ctx, w.config.BatchSize)
	   if err != nil {
		  if w.metrics != nil {
			   w.metrics.RecordScan("half", false)
		   }
		  return
	  }
	  ok := 0
	   for _, h := range list {
		   if _, err := w.engine.ResumeHalf(ctx, h.LotID, h.CaseID); err == nil {
			  ok++
		   }
	  }
	   if w.metrics != nil {
		   w.metrics.RecordScan("half", true)
		   w.metrics.AddHalfResumed(ok)
	   }
  }

   func (w *Worker) tickExpire(ctx context.Context) {
	  if w.engine == nil {
		  return
	  }
	  n, err := w.engine.ExpireStale(ctx, w.config.BatchSize)
	   if w.metrics != nil {
		   w.metrics.RecordScan("expire", err == nil)
		  if err == nil {
			   w.metrics.AddExpired(n)
		   }
	  }
   }

  func (w *Worker) tickMetrics(ctx context.Context) {
	   if w.metrics == nil || w.store == nil {
		  return
	   }
	  halves, err := w.store.ListOpenHalves(ctx, w.config.BatchSize)
	   if err == nil {
		  w.metrics.SetHalfOpen(len(halves))
	   }
   }

// RunOnce 触发一轮。
   func (w *Worker) RunOnce(ctx context.Context) {
	  w.tickPatrol(ctx)
	   w.tickHalf(ctx)
	  w.tickExpire(ctx)
	  w.tickMetrics(ctx)
   }
