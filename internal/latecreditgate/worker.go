package latecreditgate

import (
	"context"
	"time"
)

// WorkerConfig 后台扫描配置。

type WorkerConfig struct {
	RacePumpInterval time.Duration
	FenceScanInterval time.Duration
	ZombieInterval    time.Duration
	OccupyTTL time.Duration
	BatchSize int
}

// DefaultWorkerConfig 默认间隔。
var DefaultWorkerConfig = WorkerConfig{
	RacePumpInterval: 15 * time.Second,
	FenceScanInterval: 30 * time.Second,
	ZombieInterval: time.Minute,
	OccupyTTL: 30 * time.Minute,
	BatchSize: 100,
}

// WorkerMetrics 可选指标回调。

type WorkerMetrics struct {
	IncRaceDrained func(int)
	IncFenceOmit func()
	IncZombie func()
}

   func (m *WorkerMetrics) raceDrained(n int) {
	  if m != nil && m.IncRaceDrained != nil {
		  m.IncRaceDrained(n)
	   }
  }
  func (m *WorkerMetrics) fenceOmit() {
	  if m != nil && m.IncFenceOmit != nil {
		  m.IncFenceOmit()
	   }
   }

   func (m *WorkerMetrics) zombie() {
	  if m != nil && m.IncZombie != nil {
		  m.IncZombie()
	   }
  }

// Worker 巡检进程。
type Worker struct {
	svc *Service
	cfg     WorkerConfig
	metrics *WorkerMetrics
}

// NewWorker 创建巡检进程。

   func NewWorker(svc *Service, cfg WorkerConfig, metrics *WorkerMetrics) *Worker {
	  if cfg.BatchSize <= 0 {
		   cfg.BatchSize = DefaultWorkerConfig.BatchSize
	  }
	  if cfg.RacePumpInterval <= 0 {
		   cfg.RacePumpInterval = DefaultWorkerConfig.RacePumpInterval
	  }
	  if cfg.FenceScanInterval <= 0 {
		   cfg.FenceScanInterval = DefaultWorkerConfig.FenceScanInterval
	   }
	  if cfg.ZombieInterval <= 0 {
		   cfg.ZombieInterval = DefaultWorkerConfig.ZombieInterval
	  }
	   if cfg.OccupyTTL <= 0 {
		   cfg.OccupyTTL = DefaultWorkerConfig.OccupyTTL
	   }
	   return &Worker{svc: svc, cfg: cfg, metrics: metrics}
  }

// Run 启动泵直到 ctx 取消。
  func (w *Worker) Run(ctx context.Context) {
	  go w.loop(ctx,
		   w.cfg.RacePumpInterval, w.PumpRacesOnce)
	  go w.loop(ctx, w.cfg.FenceScanInterval, w.InspectFenceOnce)
	  go w.loop(ctx, w.cfg.ZombieInterval, w.SweepZombiesOnce)
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

  // PumpRacesOnce 抽取未 Ack 事件（可复放）。
  func (w *Worker) PumpRacesOnce(ctx context.Context) int {
	   evs, err := w.svc.DrainOpenRaces(ctx, w.cfg.BatchSize)
	  if err != nil {
		  return 0
	  }
	   w.metrics.raceDrained(len(evs))
	   return len(evs)
   }

// InspectFenceOnce 巡检 occupied 记录是否缺失围栏（历史脏数据）。

  func (w *Worker) InspectFenceOnce(ctx context.Context) int {
	   gens, err := w.svc.store.ListGensByState(ctx, GenOccupied, w.cfg.BatchSize)
	   if err != nil {
		  return 0
	   }
	  n := 0
	   for _, g := range gens {
		   if IsOmittedFence(FenceToken{Epoch: g.FenceEpoch, Token: g.FenceToken}) {
			  n++
			  w.metrics.fenceOmit()
		   }
	  }
	  return n
   }

// SweepZombiesOnce 收口超时占用（不回拨世代，仅记指标；状态由 ops Resolve）。
   func (w *Worker) SweepZombiesOnce(ctx context.Context) int {
	  now := w.svc.now()
	  gens,
		  err := w.svc.store.ListOccupiedExpired(ctx, now, w.cfg.OccupyTTL, w.cfg.BatchSize)
	   if err != nil {
		   return 0
	   }
	  for range gens {
		  w.metrics.zombie()
	  }
	  return len(gens)
   }
