package cerdecidegate

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
	  risk := time.NewTicker(w.cfg.RiskPumpInterval)
	  defer exp.Stop()
	   defer flip.Stop()
	  defer risk.Stop()
	   for {
		  select {
		  case <-ctx.Done():
			  return
		   case <-exp.C:
			  _, _ = w.ScanExpiry(ctx)
		  case <-flip.C:
			  _, _ = w.PumpFlips(ctx)
		  case <-risk.C:
			   _, _ = w.PumpRisks(ctx)
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

// PumpFlips 消费待处理翻转事件。
  func (w *Worker) PumpFlips(ctx context.Context) (int, error) {
	   if w == nil || w.eng == nil || w.store == nil {
		  return 0, ErrUnavailable
	   }
	   pending,
		   err := w.store.ListPendingFlips(ctx, w.cfg.BatchSize)
	   if err != nil {
		   return 0, err
	  }
	  n := 0
	  for _, ev := range pending {
		   if ev.Kind == FlipDestRisk {
			  continue
		   }
		  got, err := w.eng.OnFlip(ctx, ev)
		   if err != nil {
			  continue
		   }
		   n += got
	  }
	  return n, nil
  }

// PumpRisks 只处理风险档抬升事件。
   func (w *Worker) PumpRisks(ctx context.Context) (int, error) {
	   if w == nil || w.eng == nil || w.store == nil {
		   return 0, ErrUnavailable
	  }
	  pending, err := w.store.ListPendingFlips(ctx, w.cfg.BatchSize)
	  if err != nil {
		  return 0, err
	  }
	  n := 0
	   for _, ev := range pending {
		  if ev.Kind != FlipDestRisk {
			  continue
		  }
		  got, err := w.eng.OnFlip(ctx, ev)
		  if err != nil {
			  continue
		   }
		  n += got
	   }
	   return n, nil
   }

// Tick 一次巡检。

  func (w *Worker) Tick(ctx context.Context) (TickReport, error) {
	  var rep TickReport
	  n, err := w.ScanExpiry(ctx)
	   if err != nil {
		   return rep, err
	  }
	  rep.Expired = n
	   f, err := w.PumpFlips(ctx)
	   if err != nil {
		  return rep, err
	   }
	   rep.Flips = f
	   rep.Revoked += f
	  r, err := w.PumpRisks(ctx)
	  if err != nil {
		  return rep, err
	   }
	   rep.Risks = r
	  rep.Revoked += r
	   return rep, nil
  }
