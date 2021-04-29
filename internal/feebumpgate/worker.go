package feebumpgate

import (
	"context"
	"time"
)

// Worker 后台泵。
type Worker struct {
	eng *Engine
	store Store
	metric *Metrics
	cfg WorkerConfig
}

// NewWorker 构造。
  func NewWorker(eng *Engine, store Store, metric *Metrics, cfg WorkerConfig) *Worker {
	  cfg.Fill()
	   return &Worker{eng: eng, store: store, metric: metric, cfg: cfg}
   }

// Run 直到 ctx 取消。

  func (w *Worker) Run(ctx context.Context) {
	   if w == nil || w.eng == nil {
		  return
	  }
	  orph := time.NewTicker(w.cfg.OrphanScanInterval)
	   ret := time.NewTicker(w.cfg.RetireScanInterval)
	  defer orph.Stop()
	   defer ret.Stop()
	  for {
		  select {
		  case <-ctx.Done():
			   return
		  case <-orph.C:
			  _, _ = w.ScanOrphans(ctx)
		  case <-ret.C:
			   _, _ = w.ScanExpired(ctx)
		  }
	  }
   }

// ScanOrphans 孤儿巡检入口。

  func (w *Worker) ScanOrphans(ctx context.Context) (int, error) {
	   if w == nil || w.eng == nil {
		  return 0, ErrUnavailable
	   }
	  return w.eng.DetectOrphans(ctx, w.cfg.BatchSize)
   }

// ScanExpired 超时退役入口。

   func (w *Worker) ScanExpired(ctx context.Context) (int, error) {
	   if w == nil || w.eng == nil {
		   return 0, ErrUnavailable
	   }
	   return w.eng.RetireExpired(ctx, time.Now().UTC(), w.cfg.BatchSize)
   }

// ExportOpen 导出 open 证明。
  func (w *Worker) ExportOpen(ctx context.Context) ([]RbfOrphanProof, error) {
	  if w == nil || w.eng == nil {
		   return nil, ErrUnavailable
	   }
	  return w.eng.ListOpenOrphans(ctx, w.cfg.BatchSize)
   }
