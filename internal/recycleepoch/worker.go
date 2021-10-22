package recycleepoch

import (
	"context"
	"time"
)

type WorkerConfig struct {
	CoolPumpInterval   time.Duration
	ProofPumpInterval  time.Duration
	InspectInterval    time.Duration
	BatchSize          int
	AutoReadyOnElapsed bool
}

var DefaultWorkerConfig = WorkerConfig{
	CoolPumpInterval:  15 * time.Second,
	ProofPumpInterval: 20 * time.Second,
	InspectInterval: 30 * time.Second,
	BatchSize:          100,
	AutoReadyOnElapsed: true,
}

type WorkerMetrics struct {
	IncCoolPumped func(int)
	IncProofDrained func(int)
	IncInspectFail  func()
	IncAutoReady func()
}

   func (m *WorkerMetrics) coolPumped(n int) {
	   if m != nil && m.IncCoolPumped != nil {
		  m.IncCoolPumped(n)
	   }
   }

  func (m *WorkerMetrics) proofDrained(n int) {
	   if m != nil && m.IncProofDrained != nil {
		  m.IncProofDrained(n)
	  }
  }
  func (m *WorkerMetrics) inspectFail() {
	   if m != nil && m.IncInspectFail != nil {
		  m.IncInspectFail()
	   }
   }
   func (m *WorkerMetrics) autoReady() {
	  if m != nil && m.IncAutoReady != nil {
		   m.IncAutoReady()
	   }
  }

type Worker struct {
	svc *Service
	cfg     WorkerConfig
	metrics *WorkerMetrics
}

   func NewWorker(svc *Service, cfg WorkerConfig, metrics *WorkerMetrics) *Worker {
	  if cfg.BatchSize <= 0 {
		  cfg.BatchSize = DefaultWorkerConfig.BatchSize
	   }
	  if cfg.CoolPumpInterval <= 0 {
		   cfg.CoolPumpInterval = DefaultWorkerConfig.CoolPumpInterval
	  }
	   if cfg.ProofPumpInterval <= 0 {
		   cfg.ProofPumpInterval = DefaultWorkerConfig.ProofPumpInterval
	   }
	  if cfg.InspectInterval <= 0 {
		  cfg.InspectInterval = DefaultWorkerConfig.InspectInterval
	  }
	  return &Worker{svc: svc, cfg: cfg, metrics: metrics}
   }

   func (w *Worker) Run(ctx context.Context) {
	   go w.loop(ctx, w.cfg.CoolPumpInterval, w.PumpCoolOnce)
	  go w.loop(ctx, w.cfg.ProofPumpInterval, w.PumpProofsOnce)
	  go w.loop(ctx, w.cfg.InspectInterval, w.InspectOnce)
	  <-ctx.Done()
   }

   func (w *Worker) loop(ctx context.Context, d time.Duration, fn func(context.Context) int) {
	   tick := time.NewTicker(d)
	   defer tick.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			  return
		  case <-tick.C:
			  _ = fn(ctx)
		  }
	   }
  }

   // PumpCoolOnce 扫描冷却已满的 cooling 槽；可选自动 ReadyAssert。
   func (w *Worker) PumpCoolOnce(ctx context.Context) int {
	   now := w.svc.now()
	   rows, err := w.svc.store.ListCoolingElapsed(ctx, now, w.cfg.BatchSize)
	  if err != nil {
		   return 0
	   }
	  n := 0
	   for _, e := range rows {
		   n++
		  if w.cfg.AutoReadyOnElapsed {
			  _, err := w.svc.ReadyAssert(ctx, ReadyRequest{
				   Chain: e.Slot.Chain, Address: e.Slot.Address, ActorID: "recycle-epoch-worker",
			   })
			   if err == nil {
				  w.metrics.autoReady()
			   }
		   }
	   }
	  w.metrics.coolPumped(n)
	   return n
  }

   func (w *Worker) PumpProofsOnce(ctx context.Context) int {
	  ps, err := w.svc.DrainOpenProofs(ctx, w.cfg.BatchSize)
	  if err != nil {
		   return 0
	   }
	  w.metrics.proofDrained(len(ps))
	  return len(ps)
  }

// InspectOnce 巡检：cooling 未满却存在 assigned 绑定、或缺失 epoch 的观察。
  func (w *Worker) InspectOnce(ctx context.Context) int {
	   rows, err := w.svc.store.ListEpochsByState(ctx, EpochCooling, w.cfg.BatchSize)
	  if err != nil {
		   return 0
	   }
	   n := 0
	   now := w.svc.now()
	  for _, e := range rows {
		   if e.State == EpochCooling && !e.CoolElapsed(now) {
			  b, berr := w.svc.store.GetBind(ctx, e.Slot)
			   if berr == nil && b != nil && b.State == BindActive && b.Generation != e.Generation {
				  n++
				   w.metrics.inspectFail()
			  }
		   }
	  }
	  return n
   }
