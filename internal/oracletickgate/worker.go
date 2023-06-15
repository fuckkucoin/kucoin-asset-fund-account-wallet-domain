package oracletickgate

import (
	"context"
	"time"
)

// WorkerConfig 巡检间隔。
type WorkerConfig struct {
	ExpireInterval time.Duration
	StaleInterval  time.Duration
	BatchSize int
}

// DefaultWorkerConfig 默认。
  func DefaultWorkerConfig() WorkerConfig {
	  return WorkerConfig{
		   ExpireInterval: 5 * time.Second,
		  StaleInterval: 5 * time.Second,
		   BatchSize:     100,
	   }
  }

// Worker 后台泵。
type Worker struct {
	svc *Service
	cfg WorkerConfig
}

// NewWorker 创建泵。
   func NewWorker(svc *Service, cfg WorkerConfig) *Worker {
	  if cfg.BatchSize <= 0 {
		   cfg = DefaultWorkerConfig()
	   }
	   return &Worker{svc: svc, cfg: cfg}
   }

// Run 直到 ctx 取消。

   func (w *Worker) Run(ctx context.Context) {
	   exp := time.NewTicker(w.cfg.ExpireInterval)
	   st := time.NewTicker(w.cfg.StaleInterval)
	  defer exp.Stop()
	   defer st.Stop()
	  for {
		  select {
		   case <-ctx.Done():
			   return
		   case <-exp.C:
			   _, _ = w.svc.ExpireOpen(ctx, w.cfg.BatchSize)
		  case <-st.C:
			  _, _ = w.svc.RecheckStale(ctx, w.cfg.BatchSize)
		  }
	  }
   }

// Tick 一次巡检。
   func (w *Worker) Tick(ctx context.Context) WorkerTickResult {
	   var out WorkerTickResult
	   n, _ := w.svc.ExpireOpen(ctx, w.cfg.BatchSize)
	   out.ExpiredRevoked = n
	   m,
		  _ := w.svc.RecheckStale(ctx, w.cfg.BatchSize)
	  out.StaleRevoked = m
	  proofs, _ := w.svc.store.ListProofs(ctx, 0)
	  out.ProofsOpened = len(proofs)
	   gens, _ := w.svc.store.ListGenerations(ctx, 0)
	  out.GensScanned = len(gens)
	  return out
  }
