package reconflow

import (
	"context"
	"time"
	"github.com/rs/zerolog"
)

// ──────────────────────────────────────────────
// Worker 配置
// ──────────────────────────────────────────────
// WorkerConfig Worker 启动配置。
type WorkerConfig struct {
	// ClassifyInterval classify_pump 扫描间隔。
	ClassifyInterval time.Duration
	// ActionInterval action_exec_pump 扫描间隔。
	ActionInterval time.Duration
	// TimeoutInterval timeout_escalate_pump 扫描间隔。
	TimeoutInterval time.Duration
	// ReplayInterval replay_eval_pump 扫描间隔。
	ReplayInterval time.Duration
	// BatchSize 每轮最大处理 Run 数。
	BatchSize int
	// Actor Worker 自报身份（不得为 system）。
	Actor string
	// LeaderFunc 可选：返回 true 表示本实例持有领导权（nil 时直通）。
	LeaderFunc func(ctx context.Context) bool
	// DiffChecker 对账重评接口：返回 true 表示差异仍存在。
	DiffChecker DiffChecker
	Log         zerolog.Logger
}

   func (c *WorkerConfig) normalize() {
	   if c.ClassifyInterval <= 0 {
		  c.ClassifyInterval = 30 * time.Second
	  }
	  if c.ActionInterval <= 0 {
		   c.ActionInterval = 15 * time.Second
	   }
	   if c.TimeoutInterval <= 0 {
		  c.TimeoutInterval = 2 * time.Minute
	  }
	   if c.ReplayInterval <= 0 {
		  c.ReplayInterval = 5 * time.Minute
	  }
	   if c.BatchSize <= 0 || c.BatchSize > 200 {
		  c.BatchSize = 50
	  }
	  if c.Actor == "" {
		   c.Actor = "reconflow-worker"
	   }
  }

// DiffChecker 对账重评接口：RunID 对应的差异是否仍存在。
// 实现方只读消费三方差异事实；false 表示差异已消失。

type DiffChecker interface {
	StillDiff(ctx context.Context, runID string) (bool, error)
}

// ──────────────────────────────────────────────
// Workers 管理器
// ──────────────────────────────────────────────
// Workers 管理四个 Worker 的生命周期。
type Workers struct {
	engine *Engine
	cfg    WorkerConfig
}

// NewWorkers 构造 Workers。

  func NewWorkers(engine *Engine, cfg WorkerConfig) *Workers {
	  cfg.normalize()
	  return &Workers{engine: engine, cfg: cfg}
   }

// RunAll 启动全部 Worker（在独立 goroutine 中运行）；ctx 取消后全部退出。
  func (w *Workers) RunAll(ctx context.Context) {
	  go w.classifyPump(ctx)
	   go w.actionExecPump(ctx)
	   go w.timeoutEscalatePump(ctx)
	  go w.replayEvalPump(ctx)
  }

// ──────────────────────────────────────────────
// 1. classifyPump：分类推进泵
// ──────────────────────────────────────────────
// classifyPump 扫描处于 open/classifying 状态的 Run，推进到 acting 或 waiting_human。
   func (w *Workers) classifyPump(ctx context.Context) {
	  ticker := time.NewTicker(w.cfg.ClassifyInterval)
	  defer ticker.Stop()
	  log := w.cfg.Log.With().Str("worker", "classify_pump").Logger()
	   for {
		  select {
		   case <-ctx.Done():
			   log.Info().Msg("classify_pump 退出")
			  return
		   case <-ticker.C:
			  if !w.isLeader(ctx) {
				   continue
			  }
			  ReconFlowWorkerTick.WithLabelValues("classify_pump").Inc()
			   if err := w.runClassifyPump(ctx); err != nil {
				   log.Error().Err(err).Msg("classify_pump 扫描失败")
				  ReconFlowWorkerErrors.WithLabelValues("classify_pump").Inc()
			   }
		   }
	  }
  }

   func (w *Workers) runClassifyPump(ctx context.Context) error {
	  if w.engine.store == nil {
		   return nil
	   }
	  runs, err := w.engine.store.ListActiveRuns(ctx, w.cfg.BatchSize)
	   if err != nil {
		  return err
	   }
	   for _, r := range runs {
		  if r.State != RunStateOpen && r.State != RunStateClassifying {
			  continue
		   }
		   if err := w.engine.AdvanceRun(ctx, r.ID, w.cfg.Actor); err != nil {
			  // 单 Run 失败不阻塞其他。
			  w.cfg.Log.Warn().Str("run_id", r.ID).Err(err).Msg("classify_pump: AdvanceRun 失败")
		  }
	   }
	  return nil
  }

// ──────────────────────────────────────────────
// 2. actionExecPump：动作执行泵
// ──────────────────────────────────────────────
// actionExecPump 扫描处于 acting 状态的 Run，推进可执行步骤。

  func (w *Workers) actionExecPump(ctx context.Context) {
	  ticker := time.NewTicker(w.cfg.ActionInterval)
	  defer ticker.Stop()
	   log := w.cfg.Log.With().Str("worker", "action_exec_pump").Logger()
	  for {
		   select {
		   case <-ctx.Done():
			   log.Info().Msg("action_exec_pump 退出")
			  return
		   case <-ticker.C:
			  if !w.isLeader(ctx) {
				  continue
			  }
			   ReconFlowWorkerTick.WithLabelValues("action_exec_pump").Inc()
			   if err := w.runActionExecPump(ctx); err != nil {
				   log.Error().Err(err).Msg("action_exec_pump 扫描失败")
				   ReconFlowWorkerErrors.WithLabelValues("action_exec_pump").Inc()
			   }
		  }
	   }
  }

   func (w *Workers) runActionExecPump(ctx context.Context) error {
	   if w.engine.store == nil {
		   return nil
	   }
	   runs, err := w.engine.store.ListActiveRuns(ctx, w.cfg.BatchSize)
	  if err != nil {
		   return err
	   }
	  count := 0
	   for _, r := range runs {
		   if r.State != RunStateActing {
			   continue
		  }
		  if err := w.engine.AdvanceRun(ctx, r.ID, w.cfg.Actor); err != nil {
			  w.cfg.Log.Warn().Str("run_id", r.ID).Err(err).Msg("action_exec_pump: AdvanceRun 失败")
		  }
		  count++
	   }
	  if count > 0 {
		   ReconFlowActiveRuns.Set(float64(len(runs) - count))
	   }
	  return nil
  }

// ──────────────────────────────────────────────
// 3. timeoutEscalatePump：SLA 超时升级泵
// ──────────────────────────────────────────────
// timeoutEscalatePump 扫描 SLA 超时的非终态 Run，触发 EscalateRun。

   func (w *Workers) timeoutEscalatePump(ctx context.Context) {
	  ticker := time.NewTicker(w.cfg.TimeoutInterval)
	   defer ticker.Stop()
	  log := w.cfg.Log.With().Str("worker", "timeout_escalate_pump").Logger()
	  for {
		  select {
		   case <-ctx.Done():
			   log.Info().Msg("timeout_escalate_pump 退出")
			   return
		   case <-ticker.C:
			  if !w.isLeader(ctx) {
				  continue
			   }
			   ReconFlowWorkerTick.WithLabelValues("timeout_escalate_pump").Inc()
			   now := time.Now().UTC()
			   if err := w.runTimeoutEscalatePump(ctx, now); err != nil {
				   log.Error().Err(err).Msg("timeout_escalate_pump 扫描失败")
				  ReconFlowWorkerErrors.WithLabelValues("timeout_escalate_pump").Inc()
			  }
		  }
	  }
   }

  func (w *Workers) runTimeoutEscalatePump(ctx context.Context, now time.Time) error {
	   if w.engine.store == nil {
		  return nil
	   }
	  runs, err := w.engine.store.ListBreachedRuns(ctx, now, w.cfg.BatchSize)
	   if err != nil {
		   return err
	  }
	  for _, r := range runs {
		  if !r.SLABreached(now) {
			  continue
		  }
		  ReconFlowSLABreached.WithLabelValues(r.DiffKind).Inc()
		  if err := w.engine.EscalateRun(ctx, r.ID, w.cfg.Actor); err != nil {
			  w.cfg.Log.Error().Str("run_id", r.ID).Err(err).Msg("timeout_escalate_pump: EscalateRun 失败")
			   ReconFlowWorkerErrors.WithLabelValues("timeout_escalate_pump").Inc()
		  } else {
			  w.cfg.Log.Warn().Str("run_id", r.ID).Str("diff_kind", r.DiffKind).Msg("SLA 超时升级")
		  }
	   }
	   return nil
   }

// ──────────────────────────────────────────────
// 4. replayEvalPump：对账重评泵
// ──────────────────────────────────────────────
// replayEvalPump 对非终态 Run 重新评估差异是否仍存在；差异消失则关闭 Run。
   func (w *Workers) replayEvalPump(ctx context.Context) {
	  if w.cfg.DiffChecker == nil {
		  // 未注入 DiffChecker 时此泵不运行（避免误关单）。
		  return
	  }
	  ticker := time.NewTicker(w.cfg.ReplayInterval)
	  defer ticker.Stop()
	  log := w.cfg.Log.With().Str("worker", "replay_eval_pump").Logger()
	  for {
		   select {
		   case <-ctx.Done():
			  log.Info().Msg("replay_eval_pump 退出")
			   return
		  case <-ticker.C:
			  if !w.isLeader(ctx) {
				   continue
			   }
			  ReconFlowWorkerTick.WithLabelValues("replay_eval_pump").Inc()
			  if err := w.runReplayEvalPump(ctx); err != nil {
				   log.Error().Err(err).Msg("replay_eval_pump 扫描失败")
				   ReconFlowWorkerErrors.WithLabelValues("replay_eval_pump").Inc()
			   }
		   }
	   }
  }

   func (w *Workers) runReplayEvalPump(ctx context.Context) error {
	  if w.engine.store == nil {
		   return nil
	   }
	   runs, err := w.engine.store.ListActiveRuns(ctx, w.cfg.BatchSize)
	  if err != nil {
		   return err
	   }
	  for _, r := range runs {
		   if !r.Active() {
			   continue
		   }
		  stillDiff, err := w.cfg.DiffChecker.StillDiff(ctx, r.ID)
		   if err != nil {
			  w.cfg.Log.Warn().Str("run_id", r.ID).Err(err).Msg("replay_eval_pump: StillDiff 失败")
			  continue
		  }
		   if err := w.engine.ReplayEval(ctx, r.ID, stillDiff, w.cfg.Actor); err != nil {
			  w.cfg.Log.Warn().Str("run_id", r.ID).Err(err).Msg("replay_eval_pump: ReplayEval 失败")
		  }
	  }
	   return nil
  }

// ──────────────────────────────────────────────
// 领导权检查
// ──────────────────────────────────────────────

   func (w *Workers) isLeader(ctx context.Context) bool {
	   if w.cfg.LeaderFunc == nil {
		   return true
	  }
	   return w.cfg.LeaderFunc(ctx)
   }
