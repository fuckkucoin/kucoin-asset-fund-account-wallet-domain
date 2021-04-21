package dustquotegate

import (
	"context"
	"time"
)

// Worker 后台泵。

type Worker struct {
	eng *Engine
	store Store
	metric *Metrics
	cfg    WorkerConfig
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
	  exp := time.NewTicker(w.cfg.ExpiryScanInterval)
	   flip := time.NewTicker(w.cfg.FlipPumpInterval)
	   watch := time.NewTicker(w.cfg.ScanWatchInterval)
	  defer exp.Stop()
	  defer flip.Stop()
	  defer watch.Stop()
	  for {
		  select {
		   case <-ctx.Done():
			   return
		  case <-exp.C:
			   _, _ = w.ScanExpiry(ctx)
		  case <-flip.C:
			  _, _ = w.PumpFlips(ctx)
		  case <-watch.C:
			   _, _ = w.WatchHaltedScans(ctx)
		   }
	   }
  }

// ScanExpiry 过期扫描入口。
   func (w *Worker) ScanExpiry(ctx context.Context) (int, error) {
	   if w == nil || w.eng == nil {
		   return 0, ErrUnavailable
	  }
	  return w.eng.ExpireOpen(ctx, w.cfg.BatchSize)
  }

// PumpFlips 消费待处理翻转事件并吊销在途租约。

  func (w *Worker) PumpFlips(ctx context.Context) (int, error) {
	  if w == nil || w.eng == nil || w.store == nil {
		  return 0, ErrUnavailable
	   }
	  pending, err := w.store.ListPendingFlips(ctx, w.cfg.BatchSize)
	  if err != nil {
		   return 0, err
	  }
	   n := 0
	  for _, ev := range pending {
		  got, err := w.eng.OnFlip(ctx, ev)
		   if err != nil {
			   continue
		   }
		  n += got
	   }
	   return n, nil
  }

// WatchHaltedScans 巡检已中断 Scan，确认后续笔不会被偷偷续兑。
  func (w *Worker) WatchHaltedScans(ctx context.Context) (int, error) {
	   if w == nil || w.store == nil {
		  return 0, ErrUnavailable
	  }
	  rows, err := w.store.ListHaltedCursors(ctx)
	  if err != nil {
		   return 0, err
	   }
	  if w.metric != nil {
		  w.metric.SetHaltedScans(len(rows))
	  }
	  return len(rows), nil
   }
