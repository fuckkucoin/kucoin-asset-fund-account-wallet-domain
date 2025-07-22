package reorgorch

import (
	"context"
	"time"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rs/zerolog"
	"account-assets.kucoin.net/funding/internal/reorgacct"
)

// ────────────────────────────────────────────────────────────────────────────
// Prometheus 指标
// ────────────────────────────────────────────────────────────────────────────

var (
	metricPlanTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "reorgorch",
		Name:      "plan_total",
		Help: "Total number of reverse batches planned.",
	}, []string{"result"}) // result: success / error / idem
	metricExecTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "reorgorch",
		Name:      "exec_total",
		Help:      "Total number of batch execute rounds.",
	}, []string{"result"})
	metricLegDoneTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "reorgorch",
		Name: "leg_done_total",
		Help: "Total legs completed per kind.",
	}, []string{"kind", "result"}) // result: done / skipped / failed
	metricIncidentTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "reorgorch",
		Name: "incident_total",
		Help: "Total incident tickets opened.",
	})
)

// ────────────────────────────────────────────────────────────────────────────
// WorkerConfig Worker 配置
// ────────────────────────────────────────────────────────────────────────────
// WorkerConfig 配置所有轮询间隔（零值使用默认值）。

type WorkerConfig struct {
	PlanInterval    time.Duration // 默认 20s
	ExecInterval    time.Duration // 默认 10s
	TimeoutInterval time.Duration // 默认 2min
	GuardInterval   time.Duration // 默认 5min
	BatchSize       int           // 默认 10
	LegTimeout      time.Duration // 默认 30s
}

  func (c WorkerConfig) withDefaults() WorkerConfig {
	   if c.PlanInterval <= 0 {
		  c.PlanInterval = 20 * time.Second
	   }
	  if c.ExecInterval <= 0 {
		   c.ExecInterval = 10 * time.Second
	  }
	  if c.TimeoutInterval <= 0 {
		   c.TimeoutInterval = 2 * time.Minute
	  }
	  if c.GuardInterval <= 0 {
		   c.GuardInterval = 5 * time.Minute
	   }
	   if c.BatchSize <= 0 {
		   c.BatchSize = 10
	  }
	  if c.LegTimeout <= 0 {
		  c.LegTimeout = 30 * time.Second
	  }
	   return c
  }

// ────────────────────────────────────────────────────────────────────────────
// OrchWorker
// ────────────────────────────────────────────────────────────────────────────
// OrchWorker 编排引擎后台 Worker。
type OrchWorker struct {
	orch      *Orchestrator
	guardScan *GuardScanner
	acctStore *reorgacct.Store
	orchStore *Store
	cfg WorkerConfig
	log zerolog.Logger
}

// NewOrchWorker 构造 OrchWorker。
   func NewOrchWorker(
	  orch *Orchestrator,
	   guardScan *GuardScanner,
	   acctStore *reorgacct.Store,
	  orchStore *Store,
	  cfg WorkerConfig,
	  log zerolog.Logger,
   ) *OrchWorker {
	   return &OrchWorker{
		   orch: orch,
		  guardScan: guardScan,
		  acctStore: acctStore,
		   orchStore: orchStore,
		   cfg: cfg.withDefaults(),
		   log: log,
	  }
   }

// Run 启动所有子循环，阻塞至 ctx.Done。
   func (w *OrchWorker) Run(ctx context.Context) {
	  w.log.Info().Interface("config", w.cfg).Msg("reorgorch worker started")
	  planTick := time.NewTicker(w.cfg.PlanInterval)
	   execTick := time.NewTicker(w.cfg.ExecInterval)
	  timeoutTick := time.NewTicker(w.cfg.TimeoutInterval)
	  guardTick := time.NewTicker(w.cfg.GuardInterval)
	  defer planTick.Stop()
	   defer execTick.Stop()
	   defer timeoutTick.Stop()
	  defer guardTick.Stop()
	   // 启动时立即触发一轮。
	   w.runPlanLoop(ctx)
	  w.runExecLoop(ctx)
	   for {
		   select {
		  case <-ctx.Done():
			   w.log.Info().Msg("reorgorch worker stopping")
			   return
		   case <-planTick.C:
			  w.runPlanLoop(ctx)
		   case <-execTick.C:
			  w.runExecLoop(ctx)
		   case <-timeoutTick.C:
			  w.runTimeoutCheck(ctx)
		  case <-guardTick.C:
			   w.guardScan.scan(ctx)
		   }
	  }
   }

// runPlanLoop 对所有 scanned 状态的 ReorgEvent 触发 Plan。
   func (w *OrchWorker) runPlanLoop(ctx context.Context) {
	  events, err := w.acctStore.ListEvents(ctx, "", reorgacct.EventStateScanned, w.cfg.BatchSize, 0)
	  if err != nil {
		   w.log.Error().Err(err).Msg("reorgorch plan loop: list scanned events failed")
		  return
	  }
	  for _, ev := range events {
		   select {
		  case <-ctx.Done():
			   return
		   default:
		   }
		  _, _, err := w.orch.Plan(ctx, ev.ID)
		   if err == ErrGuardConflict {
			  // 已规划，幂等跳过。
			  metricPlanTotal.WithLabelValues("idem").Inc()
			  continue
		   }
		  if err != nil {
			  metricPlanTotal.WithLabelValues("error").Inc()
			   w.log.Error().Err(err).Str("event_id", ev.ID).Msg("reorgorch plan failed")
			   continue
		  }
		  metricPlanTotal.WithLabelValues("success").Inc()
		   // Plan 完成后立即推进 ReorgEvent → orch_started。
		   _ = w.acctStore.UpdateEventState(ctx, ev.ID, reorgacct.EventStateOrchStarted)
	   }
  }

// runExecLoop 对所有 executing（或 planning）状态批次接着执行腿。
  func (w *OrchWorker) runExecLoop(ctx context.Context) {
	  // planning → executing → 接着。
	  planningBatches, _ := w.orchStore.ListBatches(ctx, "", BatchStatePlanning, w.cfg.BatchSize, 0)
	  for _, b := range planningBatches {
		   select {
		  case <-ctx.Done():
			   return
		   default:
		  }
		  res,
			   err := w.orch.Execute(ctx, b.ID)
		  w.logExecResult(b.ID, res, err)
	  }
	  // 接着执行 executing 批次（可能上一轮未完成）。
	  execBatches, _ := w.orchStore.ListExecutingBatches(ctx, w.cfg.BatchSize)
	   for _, b := range execBatches {
		  select {
		   case <-ctx.Done():
			  return
		  default:
		   }
		   res, err := w.orch.Execute(ctx, b.ID)
		   w.logExecResult(b.ID, res, err)
	  }
   }

// runTimeoutCheck 检查 executing 批次中超时的腿，标记 failed 并开事故单。

  func (w *OrchWorker) runTimeoutCheck(ctx context.Context) {
	   batches, _ := w.orchStore.ListExecutingBatches(ctx, w.cfg.BatchSize)
	  for _, batch := range batches {
		  legs, err := w.orchStore.ListLegs(ctx, batch.ID)
		   if err != nil {
			   continue
		  }
		   for _, leg := range legs {
			  if leg.State != LegStatePending || leg.StartedAt == nil {
				  continue
			   }
			   if time.Since(*leg.StartedAt) > w.cfg.LegTimeout {
				  _ = w.orchStore.UpdateLegState(ctx, leg.ID, LegStateFailed, "timeout")
				   metricLegDoneTotal.WithLabelValues(leg.Kind, "failed").Inc()
				  metricIncidentTotal.Inc()
				  w.log.Warn().Str("leg_id", leg.ID).Str("batch_id", batch.ID).
					  Msg("reorgorch: leg timed out; marked failed")
			  }
		   }
	   }
  }

  func (w *OrchWorker) logExecResult(batchID string, res ExecuteResult, err error) {
	  if err != nil {
		   metricExecTotal.WithLabelValues("error").Inc()
		  w.log.Error().Err(err).Str("batch_id", batchID).Msg("reorgorch exec failed")
		   return
	  }
	   metricExecTotal.WithLabelValues("success").Inc()
	  metricLegDoneTotal.WithLabelValues("all", "done").Add(float64(res.Done))
	  metricLegDoneTotal.WithLabelValues("all", "skipped").Add(float64(res.Skipped))
	   metricLegDoneTotal.WithLabelValues("all", "failed").Add(float64(res.Failed))
	  metricIncidentTotal.Add(float64(res.Incidents))
	   w.log.Info().
		  Str("batch_id", batchID).Str("state", res.BatchState).
		  Int("done", res.Done).Int("failed", res.Failed).Int("skipped", res.Skipped).
		  Msg("reorgorch exec round completed")
   }
