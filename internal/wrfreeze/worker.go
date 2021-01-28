package wrfreeze

import (
	"context"
	"time"
)

// WorkerConfig 工作循环配置。
type WorkerConfig struct {
	// TickInterval 工作循环间隔（默认 30s）。
	TickInterval time.Duration
	// PhaseTimeout 阶段执行超时（默认 10min）。
	PhaseTimeout time.Duration
	// WaveRetryMax 波次最大重试次数（默认 3）。
	WaveRetryMax int
}

   func defaultWorkerConfig() WorkerConfig {
	  return WorkerConfig{
		  TickInterval: 30 * time.Second,
		  PhaseTimeout: 10 * time.Minute,
		  WaveRetryMax: 3,
	   }
  }

// Worker 后台工作循环。
type Worker struct {
	plan     *PlanService
	wave     *WaveService
	unfreeze *UnfreezeService
	proof    *ProofService
	rollback *RollbackService
	metrics *Metrics
	cfg     WorkerConfig
	clock   func() time.Time
}

// NewWorker 构造工作循环。

   func NewWorker(
	   plan *PlanService,
	  wave *WaveService,
	  unfreeze *UnfreezeService,
	  proof *ProofService,
	  rollback *RollbackService,
	  metrics *Metrics,
	  cfg WorkerConfig,
  ) *Worker {
	  if cfg.TickInterval <= 0 {
		  cfg = defaultWorkerConfig()
	  }
	   return &Worker{
		   plan: plan, wave: wave, unfreeze: unfreeze,
		  proof: proof,
		  rollback: rollback, metrics: metrics,
		  cfg: cfg,
		  clock: func() time.Time { return time.Now().UTC() },
	   }
  }

// Run 启动工作循环（阻塞直到 ctx 取消）。

   func (w *Worker) Run(ctx context.Context) {
	  ticker := time.NewTicker(w.cfg.TickInterval)
	  defer ticker.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			   return
		  case <-ticker.C:
			  w.tick(ctx)
		  }
	   }
  }

  func (w *Worker) tick(ctx context.Context) {
	  // 1. 过期计划中止
	   n, err := w.plan.ExpireOverduePlans(ctx)
	  if err == nil && n > 0 && w.metrics != nil {
		   w.metrics.PlanExpiredTotal.Add(float64(n))
	   }
	  // 2. 波次推进（对所有 executing 计划）
	  executing, err := w.plan.ListPlans(ctx, string(PlanExecuting), 50)
	   if err == nil {
		   for _, p := range executing {
			  advanced, aerr := w.wave.AdvancePlan(ctx, p.ID)
			   if aerr == nil && advanced && w.metrics != nil {
				  w.metrics.WaveAdvancedTotal.Add(1)
			  }
		   }
	  }
	  // 3. 覆盖率巡检 + 自动封存
	   if w.proof != nil && w.plan != nil && w.wave != nil {
		   _ = w.proof.PatrolAndSeal(ctx, w.plan, w.wave)
	   }
	   // 4. 阶段超时升级
	   w.tickPhaseTimeout(ctx)
	  // 5. 过期事故回滚腿生成
	  if w.rollback != nil {
		   created, rerr := w.rollback.AutoRollbackExpired(ctx)
		   if rerr == nil && created > 0 && w.metrics != nil {
			  w.metrics.RollbackLegCreatedTotal.Add(float64(created))
		  }
	  }
  }

// tickPhaseTimeout 将运行时间超过 PhaseTimeout 的阶段标记为 failed。

   func (w *Worker) tickPhaseTimeout(ctx context.Context) {
	  if w.plan == nil || w.plan.store == nil {
		   return
	   }
	   plans, err := w.plan.ListPlans(ctx, string(PlanExecuting), 50)
	   if err != nil {
		  return
	   }
	  now := w.clock()
	   for _, p := range plans {
		  phases, err := w.unfreeze.ListPhases(ctx, p.ID)
		  if err != nil {
			   continue
		  }
		   for _, ph := range phases {
			   if ph.Status != PhaseRunning {
				   continue
			  }
			  if ph.StartedAt == nil {
				  continue
			  }
			   if now.Sub(*ph.StartedAt) > w.cfg.PhaseTimeout {
				   ph.Status = PhaseFailed
				  ph.IntegrityNote = "phase timeout after " + w.cfg.PhaseTimeout.String()
				  fin := now
				  ph.FinishedAt = &fin
				  ph.UpdatedAt = now
				  _ = w.unfreeze.store.SavePhase(ctx, ph)
				   if w.metrics != nil {
					   w.metrics.PhaseTimeoutTotal.Add(1)
				  }
			   }
		  }
	  }
  }

// SetClock 注入时钟（用）。
   func (w *Worker) SetClock(fn func() time.Time) {
	  if w != nil && fn != nil {
		   w.clock = fn
		   if w.plan != nil {
			  w.plan.SetClock(fn)
		  }
		  if w.wave != nil {
			  w.wave.SetClock(fn)
		  }
		  if w.unfreeze != nil {
			   w.unfreeze.SetClock(fn)
		  }
		  if w.proof != nil {
			   w.proof.SetClock(fn)
		   }
		  if w.rollback != nil {
			   w.rollback.SetClock(fn)
		   }
	  }
  }
