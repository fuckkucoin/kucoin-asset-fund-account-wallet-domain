package subfreezecas

import (
	"context"
	"time"
)

// WorkerConfig 后台泵配置。
type WorkerConfig struct {
	PunchInterval time.Duration
	RevokeInterval time.Duration
	ExportInterval time.Duration
	BatchSize int
}

   func (c *WorkerConfig) defaults() {
	   if c.PunchInterval <= 0 {
		   c.PunchInterval = 20 * time.Second
	  }
	  if c.RevokeInterval <= 0 {
		   c.RevokeInterval = 30 * time.Second
	  }
	  if c.ExportInterval <= 0 {
		  c.ExportInterval = 60 * time.Second
	   }
	  if c.BatchSize <= 0 {
		  c.BatchSize = 80
	   }
   }

// PunchWorker 冻后借记击穿巡检。
type PunchWorker struct {
	eng *Engine
	cfg WorkerConfig
}

  func NewPunchWorker(eng *Engine, cfg WorkerConfig) *PunchWorker {
	  cfg.defaults()
	  return &PunchWorker{eng: eng, cfg: cfg}
  }

  func (w *PunchWorker) Run(ctx context.Context) {
	  if w == nil || w.eng == nil {
		  return
	  }
	   t := time.NewTicker(w.cfg.PunchInterval)
	   defer t.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			  return
		  case <-t.C:
			   _,
				   _ = w.eng.InspectPunch(ctx, w.cfg.BatchSize)
		   }
	  }
  }

  func (w *PunchWorker) TickOnce(ctx context.Context) (int, error) {
	  return w.eng.InspectPunch(ctx, w.cfg.BatchSize)
   }

// RevokeWorker 速度窗吊销补偿泵。
type RevokeWorker struct {
	eng *Engine
	cfg WorkerConfig
}

  func NewRevokeWorker(eng *Engine, cfg WorkerConfig) *RevokeWorker {
	  cfg.defaults()
	  return &RevokeWorker{eng: eng,
		   cfg: cfg}
  }

  func (w *RevokeWorker) Run(ctx context.Context) {
	   if w == nil || w.eng == nil {
		  return
	   }
	   t := time.NewTicker(w.cfg.RevokeInterval)
	  defer t.Stop()
	  for {
		   select {
		  case <-ctx.Done():
			  return
		  case <-t.C:
			  _, _ = w.eng.PumpRevocations(ctx, w.cfg.BatchSize)
		   }
	  }
   }

  func (w *RevokeWorker) TickOnce(ctx context.Context) (int, error) {
	   return w.eng.PumpRevocations(ctx, w.cfg.BatchSize)
   }

// ExportWorker 未 ack 证明导出泵。
type ExportWorker struct {
	eng *Engine
	cfg WorkerConfig
	n int
}

   func NewExportWorker(eng *Engine, cfg WorkerConfig) *ExportWorker {
	   cfg.defaults()
	   return &ExportWorker{eng: eng, cfg: cfg}
  }

   func (w *ExportWorker) Run(ctx context.Context) {
	  if w == nil || w.eng == nil {
		  return
	  }
	   t := time.NewTicker(w.cfg.ExportInterval)
	   defer t.Stop()
	   for {
		   select {
		  case <-ctx.Done():
			   return
		   case <-t.C:
			   _, _ = w.TickOnce(ctx)
		   }
	   }
  }

   func (w *ExportWorker) TickOnce(ctx context.Context) (int, error) {
	   if w == nil || w.eng == nil {
		  return 0, ErrUnavailable
	  }
	  rows, err := w.eng.ListOpenProofs(ctx, w.cfg.BatchSize)
	   if err != nil {
		   return 0, err
	   }
	   for i := range rows {
		  _,
			   _ = w.eng.ExportProof(ctx, rows[i].ProofID)
	  }
	  w.n += len(rows)
	  recordExport(len(rows))
	   return len(rows), nil
  }

   func (w *ExportWorker) Exported() int {
	   if w == nil {
		  return 0
	   }
	   return w.n
   }

// StartWorkers 挂后台泵。
  func StartWorkers(ctx context.Context, eng *Engine, cfg WorkerConfig) {
	   if eng == nil {
		  return
	  }
	   if ctx == nil {
		  ctx = context.Background()
	   }
	  cfg.defaults()
	   go NewPunchWorker(eng, cfg).Run(ctx)
	   go NewRevokeWorker(eng, cfg).Run(ctx)
	  go NewExportWorker(eng, cfg).Run(ctx)
   }
