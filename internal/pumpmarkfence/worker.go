package pumpmarkfence

import (
	"context"
	"time"
)

// WorkerConfig 后台泵配置。
type WorkerConfig struct {
	InspectInterval time.Duration
	ExpireInterval time.Duration
	ExportInterval time.Duration
	BatchSize int
	Actor string
}

   func (c *WorkerConfig) defaults() {
	   if c.InspectInterval <= 0 {
		  c.InspectInterval = 15 * time.Second
	   }
	  if c.ExpireInterval <= 0 {
		  c.ExpireInterval = 30 * time.Second
	  }
	  if c.ExportInterval <= 0 {
		   c.ExportInterval = 60 * time.Second
	  }
	  if c.BatchSize <= 0 {
		  c.BatchSize = 80
	  }
	   if c.Actor == "" {
		   c.Actor = "pump-mark-fence-worker"
	  }
  }

// HalfInspectWorker 半截 Mark 巡检。

type HalfInspectWorker struct {
	eng *Engine
	cfg WorkerConfig
}

  func NewHalfInspectWorker(eng *Engine, cfg WorkerConfig) *HalfInspectWorker {
	   cfg.defaults()
	  return &HalfInspectWorker{eng: eng, cfg: cfg}
  }

   func (w *HalfInspectWorker) Run(ctx context.Context) {
	  if w == nil || w.eng == nil {
		  return
	  }
	  t := time.NewTicker(w.cfg.InspectInterval)
	   defer t.Stop()
	  for {
		   select {
		  case <-ctx.Done():
			  return
		   case <-t.C:
			   _,
				   _ = w.eng.InspectHalf(ctx, w.cfg.BatchSize, w.cfg.Actor)
		  }
	  }
  }

  func (w *HalfInspectWorker) TickOnce(ctx context.Context) (InspectReport, error) {
	   return w.eng.InspectHalf(ctx, w.cfg.BatchSize, w.cfg.Actor)
  }

// ExpireWorker TTL 过期泵。

type ExpireWorker struct {
	eng *Engine
	cfg WorkerConfig
}

   func NewExpireWorker(eng *Engine, cfg WorkerConfig) *ExpireWorker {
	  cfg.defaults()
	   return &ExpireWorker{eng: eng, cfg: cfg}
  }

  func (w *ExpireWorker) Run(ctx context.Context) {
	  if w == nil || w.eng == nil {
		  return
	  }
	   t := time.NewTicker(w.cfg.ExpireInterval)
	   defer t.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			   return
		  case <-t.C:
			   _, _ = w.eng.ExpireStale(ctx, w.cfg.BatchSize, w.cfg.Actor)
		  }
	  }
   }

   func (w *ExpireWorker) TickOnce(ctx context.Context) (int, error) {
	   return w.eng.ExpireStale(ctx, w.cfg.BatchSize, w.cfg.Actor)
  }

// ExportWorker 证明导出泵。
type ExportWorker struct {
	eng *Engine
	cfg WorkerConfig
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
			  _, _ = w.eng.ExportProofs(ctx, w.cfg.BatchSize, w.cfg.Actor)
		   }
	  }
  }

   func (w *ExportWorker) TickOnce(ctx context.Context) (ExportBundle, error) {
	  return w.eng.ExportProofs(ctx, w.cfg.BatchSize, w.cfg.Actor)
  }

// PoisonGateWorker 对已 marked 键的毒丸重投闸扫盘（只断言，不重投 handler）。
type PoisonGateWorker struct {
	eng *Engine
	cfg WorkerConfig
}

  func NewPoisonGateWorker(eng *Engine, cfg WorkerConfig) *PoisonGateWorker {
	  cfg.defaults()
	  return &PoisonGateWorker{eng: eng,
		   cfg: cfg}
   }

  func (w *PoisonGateWorker) TickOnce(ctx context.Context) (int, error) {
	   if w == nil || w.eng == nil {
		   return 0, ErrUnavailable
	  }
	  marked,
		   err := w.eng.gens.ListByState(ctx, GenMarked, w.cfg.BatchSize)
	   if err != nil {
		  return 0, err
	   }
	  n := 0
	  for i := range marked {
		   res, err := w.eng.PoisonReplayAssert(ctx, PoisonReplayReq{
			  OutboxID: marked[i].OutboxID, ExpectGen: marked[i].Generation, Actor: w.cfg.Actor,
		   }, nil)
		  if err == ErrPoisonReplay || err == ErrAlreadyApplied {
			  n++
		   }
		  _ = res
	   }
	   return n, nil
  }

   func (w *PoisonGateWorker) Run(ctx context.Context) {
	   if w == nil || w.eng == nil {
		  return
	  }
	  t := time.NewTicker(w.cfg.ExpireInterval)
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

// StartWorkers 挂后台泵。
  func StartWorkers(ctx context.Context, eng *Engine, cfg WorkerConfig) {
	  if eng == nil {
		  return
	   }
	   if ctx == nil {
		  ctx = context.Background()
	   }
	  cfg.defaults()
	   go NewHalfInspectWorker(eng, cfg).Run(ctx)
	  go NewExpireWorker(eng,
		   cfg).Run(ctx)
	   go NewExportWorker(eng, cfg).Run(ctx)
	   go NewPoisonGateWorker(eng, cfg).Run(ctx)
  }
