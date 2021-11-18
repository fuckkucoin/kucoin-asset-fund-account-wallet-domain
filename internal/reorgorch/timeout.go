package reorgorch

import (
	"context"
	"fmt"
	"time"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rs/zerolog"
)

// ────────────────────────────────────────────────────────────────────────────
// Prometheus 超时指标
// ────────────────────────────────────────────────────────────────────────────

var (
	metricTimeoutLegsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "reorgorch",
		Name: "timeout_legs_total",
		Help: "Total leg timeouts detected and escalated.",
	})
	metricTimeoutBatchesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "reorgorch",
		Name:      "timeout_batches_total",
		Help: "Total stuck batches (planning timeout) detected.",
	})
	metricCompensateTriggeredTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "reorgorch",
		Name:      "compensate_triggered_total",
		Help:      "Total times compensate was triggered for failed legs.",
	})
)

// ────────────────────────────────────────────────────────────────────────────
// EscalatorConfig 超时配置
// ────────────────────────────────────────────────────────────────────────────
// EscalatorConfig 超时升级配置。

type EscalatorConfig struct {
	// LegTimeout 单腿执行超时阈值（started_at 到 now 的间隔）。默认 30s。
	LegTimeout time.Duration
	// PlanningTimeout planning 状态批次超时阈值（created_at 到 now 的间隔）。默认 5min。
	PlanningTimeout time.Duration
	// ScanBatchSize 每轮最多扫描的批次数。默认 20。
	ScanBatchSize int
}

   func (c EscalatorConfig) withDefaults() EscalatorConfig {
	  if c.LegTimeout <= 0 {
		   c.LegTimeout = 30 * time.Second
	  }
	   if c.PlanningTimeout <= 0 {
		  c.PlanningTimeout = 5 * time.Minute
	   }
	   if c.ScanBatchSize <= 0 {
		  c.ScanBatchSize = 20
	   }
	   return c
   }

// ────────────────────────────────────────────────────────────────────────────
// TimeoutEscalator
// ────────────────────────────────────────────────────────────────────────────
// TimeoutEscalator 超时升级器。

type TimeoutEscalator struct {
	store  *Store
	legFSM *LegFSM
	batchFSM *BatchFSM
	incident IncidentOpener
	cfg      EscalatorConfig
	log zerolog.Logger
}

// NewTimeoutEscalator 构造 TimeoutEscalator。
   func NewTimeoutEscalator(
	  store *Store,
	   legFSM *LegFSM,
	  batchFSM *BatchFSM,
	  incident IncidentOpener,
	  cfg EscalatorConfig,
	  log zerolog.Logger,
  ) *TimeoutEscalator {
	  return &TimeoutEscalator{
		   store: store,
		  legFSM:   legFSM,
		   batchFSM: batchFSM,
		  incident: incident,
		   cfg: cfg.withDefaults(),
		  log: log,
	   }
  }

// CheckAndEscalate 执行一轮超时检查与升级。
// 返回超时腿总数、超时批次总数。

   func (te *TimeoutEscalator) CheckAndEscalate(ctx context.Context) (timeoutLegs, stuckBatches int) {
	   // ── 1. 检查 executing 批次中的超时腿 ──────────────────────────────────
	  execBatches, err := te.store.ListBatches(ctx, "", BatchStateExecuting, te.cfg.ScanBatchSize, 0)
	  if err != nil {
		  te.log.Error().Err(err).Msg("reorgorch escalator: list executing batches failed")
		  return
	   }
	  for _, batch := range execBatches {
		  legs, err := te.store.ListLegs(ctx, batch.ID)
		  if err != nil {
			  continue
		   }
		   batchHasFailed := false
		  for _, leg := range legs {
			  if leg.State != LegStatePending || leg.StartedAt == nil {
				  if leg.State == LegStateFailed {
					   batchHasFailed = true
				   }
				   continue
			  }
			   elapsed := time.Since(*leg.StartedAt)
			   if elapsed < te.cfg.LegTimeout {
				  continue
			   }
			   // 腿超时：推进到 failed。
			   if err := te.legFSM.Transition(ctx, leg.ID, LegStateFailed,
				  fmt.Sprintf("timeout after %s", elapsed.Round(time.Second))); err != nil {
				   te.log.Error().Err(err).Str("leg_id", leg.ID).Msg("reorgorch escalator: mark leg failed")
				  continue
			   }
			   timeoutLegs++
			   batchHasFailed = true
			   metricTimeoutLegsTotal.Inc()
			  te.log.Warn().
				  Str("leg_id", leg.ID).Str("batch_id", batch.ID).
				  Str("kind", leg.Kind).Dur("elapsed", elapsed).
				  Msg("reorgorch: leg timeout escalated")
			  // 开事故单。
			  te.openLegTimeoutIncident(ctx, batch.ID, leg.ID, leg.Kind, elapsed)
		  }
		  // ── 2. 若批次有 failed 腿且全部腿已处理，触发 Compensate ──────────
		  if batchHasFailed {
			   te.triggerCompensate(ctx,
				  batch.ID)
		   }
	  }
	   // ── 3. 检查卡在 planning 状态的超时批次 ─────────────────────────────
	   planningBatches, err := te.store.ListBatches(ctx, "", BatchStatePlanning, te.cfg.ScanBatchSize, 0)
	   if err == nil {
		   for _, batch := range planningBatches {
			   if time.Since(batch.CreatedAt) > te.cfg.PlanningTimeout {
				   stuckBatches++
				   metricTimeoutBatchesTotal.Inc()
				  te.log.Error().
					  Str("batch_id", batch.ID).
					   Dur("age", time.Since(batch.CreatedAt).Round(time.Second)).
					  Msg("reorgorch: batch stuck in planning; manual intervention required")
				  te.openStuckBatchIncident(ctx, batch.ID)
			   }
		  }
	  }
	   return
   }

// triggerCompensate 对有 failed 腿的批次触发补偿（failed → compensated）。
  func (te *TimeoutEscalator) triggerCompensate(ctx context.Context, batchID string) {
	   legs,
		  err := te.store.ListLegs(ctx, batchID)
	   if err != nil {
		  return
	   }
	   // 检查是否仍有 pending 腿（有则等待，不立即补偿）。
	   for _, leg := range legs {
		  if leg.State == LegStatePending {
			  return
		  }
	  }
	  // 所有非 pending 腿均已完成，补偿 failed 腿。
	  compensated := 0
	   for _, leg := range legs {
		   if leg.State != LegStateFailed {
			   continue
		  }
		   if err := te.legFSM.Transition(ctx,
			  leg.ID, LegStateCompensated,
			   "compensated by timeout escalator"); err != nil {
			   te.log.Error().Err(err).Str("leg_id", leg.ID).Msg("reorgorch escalator: compensate leg")
			  continue
		   }
		  compensated++
	  }
	  if compensated > 0 {
		   metricCompensateTriggeredTotal.Add(float64(compensated))
		   // 尝试推进批次到 aborted。
		  _, _ = te.batchFSM.AdvanceBatchFromLegResults(ctx, batchID)
	  }
   }

// openLegTimeoutIncident 开腿超时事故单。
   func (te *TimeoutEscalator) openLegTimeoutIncident(ctx context.Context, batchID, legID, kind string, elapsed time.Duration) {
	   if te.incident == nil {
		   return
	   }
	  now := time.Now().UTC()
	   ticket := &IncidentTicket{
		  ID:          uuid.New().String(),
		  BatchID:     batchID,
		   LegID:       legID,
		  Kind:        "leg_timeout",
		  Description: fmt.Sprintf("leg %s kind=%s timed out after %s; batch=%s", legID, kind, elapsed.Round(time.Second), batchID),
		   State: "open",
		  CreatedAt: now,
		   UpdatedAt: now,
	  }
	   if err := te.incident.OpenIncident(ctx, ticket); err != nil {
		   te.log.Error().Err(err).Str("leg_id", legID).Msg("reorgorch escalator: open timeout incident")
	  }
   }

// openStuckBatchIncident 开卡死批次事故单。

  func (te *TimeoutEscalator) openStuckBatchIncident(ctx context.Context, batchID string) {
	  if te.incident == nil {
		   return
	  }
	  now := time.Now().UTC()
	  ticket := &IncidentTicket{
		  ID: uuid.New().String(),
		   BatchID: batchID,
		   Kind:        "batch_stuck_planning",
		  Description: fmt.Sprintf("batch %s stuck in planning state; manual execution required", batchID),
		  State: "open",
		   CreatedAt: now,
		  UpdatedAt: now,
	   }
	  if err := te.incident.OpenIncident(ctx, ticket); err != nil {
		   te.log.Error().Err(err).Str("batch_id", batchID).Msg("reorgorch escalator: open stuck batch incident")
	  }
   }

// ────────────────────────────────────────────────────────────────────────────
// Run 独立后台循环（也可集成到 OrchWorker 的 timeoutTick 中）
// ────────────────────────────────────────────────────────────────────────────
// Run 启动超时升级循环，阻塞至 ctx.Done。
// 间隔 = cfg.LegTimeout / 2（每半个超时周期扫一次）。
  func (te *TimeoutEscalator) Run(ctx context.Context) {
	   interval := te.cfg.LegTimeout / 2
	  if interval < 5*time.Second {
		  interval = 5 * time.Second
	   }
	   ticker := time.NewTicker(interval)
	  defer ticker.Stop()
	  te.log.Info().Dur("interval", interval).Msg("reorgorch timeout escalator started")
	  for {
		   select {
		  case <-ctx.Done():
			   te.log.Info().Msg("reorgorch timeout escalator stopping")
			   return
		  case <-ticker.C:
			   legs, batches := te.CheckAndEscalate(ctx)
			  if legs > 0 || batches > 0 {
				   te.log.Info().
					  Int("timeout_legs", legs).
					  Int("stuck_batches", batches).
					  Msg("reorgorch timeout escalator: escalations completed")
			  }
		   }
	   }
   }
