package cfgdual

import (
	"context"
	"fmt"
	"math"
	"time"
	"github.com/rs/zerolog"
)

// ─────────────────────────────────────────────────────────────────────────────
// DecisionPump 双评决策落仓泵
// ─────────────────────────────────────────────────────────────────────────────
// DecisionPump 从 Engine 缓冲区消费 DualDecision 并批量写入持久化层。

type DecisionPump struct {
	engine *Engine
	store   Store
	metrics *DualMetrics
	log     zerolog.Logger
}

// NewDecisionPump 构造落仓泵。
   func NewDecisionPump(engine *Engine, store Store, metrics *DualMetrics, log zerolog.Logger) *DecisionPump {
	   return &DecisionPump{engine: engine, store: store, metrics: metrics, log: log}
  }

// Tick 单次消费循环（由 workerloop.Run 驱动）。
  func (p *DecisionPump) Tick(ctx context.Context) error {
	   if p.engine == nil {
		   return nil
	  }
	   batch := p.engine.DrainBuf(256)
	  if len(batch) == 0 {
		  return nil
	  }
	  failed := 0
	  for _, d := range batch {
		   if err := p.store.SaveDecision(ctx, d); err != nil {
			   p.log.Error().Err(err).Str("id", d.ID).Msg("cfgdual: decision pump save failed")
			   failed++
		  }
	  }
	  if p.metrics != nil {
		  p.metrics.DecisionsPumped.Add(float64(len(batch) - failed))
		   p.metrics.DecisionDropped.Add(float64(failed))
		   p.metrics.BufLen.Set(float64(p.engine.BufLen()))
	  }
	   if failed > 0 {
		  return fmt.Errorf("decision pump: %d/%d writes failed", failed, len(batch))
	   }
	  return nil
  }

// ─────────────────────────────────────────────────────────────────────────────
// DiffAggregate 差分聚合 Worker
// ─────────────────────────────────────────────────────────────────────────────
// DiffAggregateConfig 聚合窗口参数。

type DiffAggregateConfig struct {
	// WindowSize 滑动窗口大小（默认 5 分钟）。
	WindowSize time.Duration
	// MaxLookback 向前回溯的窗口数量（默认 3）。
	MaxLookback int
}

// DefaultDiffAggregateConfig 默认配置。

  func DefaultDiffAggregateConfig() DiffAggregateConfig {
	   return DiffAggregateConfig{
		   WindowSize: 5 * time.Minute,
		  MaxLookback: 3,
	  }
   }

// DiffAggregate 差分聚合 Worker。
type DiffAggregate struct {
	store Store
	seals *SealRegistry
	policy SealPolicy
	cfg    DiffAggregateConfig
	idgen  func() string
	clock func() time.Time
	metrics *DualMetrics
	log zerolog.Logger
}

// NewDiffAggregate 构造差分聚合器。

  func NewDiffAggregate(
	   store Store,
	   seals *SealRegistry,
	  policy SealPolicy,
	  cfg DiffAggregateConfig,
	  idgen func() string,
	metrics *DualMetrics,
	log zerolog.Logger,
) *DiffAggregate {
	return &DiffAggregate{
		store:  store,
		seals:  seals,
		policy: policy,
		cfg:   cfg,
		idgen: idgen,
		clock: func() time.Time { return time.Now().UTC() },
		metrics: metrics,
		log:     log,
	}
}

// WithClock 注入时钟（用）。
  func (a *DiffAggregate) WithClock(c func() time.Time) *DiffAggregate {
	   if c != nil {
		   a.clock = c
	   }
	  return a
  }

  // Tick 对所有活跃发布执行差分聚合（由 workerloop.Run 驱动）。

  func (a *DiffAggregate) Tick(ctx context.Context) error {
	  releases,
		  err := a.store.ListActiveReleases(ctx)
	   if err != nil {
		   return fmt.Errorf("diff aggregate: list active releases: %w", err)
	  }
	   for _, rel := range releases {
		   if err := a.aggregateRelease(ctx, rel); err != nil {
			   a.log.Error().Err(err).Str("release_id", rel.ID).Msg("cfgdual: diff aggregate failed")
		   }
	  }
	  return nil
   }

// aggregateRelease 对单个发布执行一次窗口内差分聚合。

  func (a *DiffAggregate) aggregateRelease(ctx context.Context, rel *GrayRelease) error {
	  now := a.clock()
	  windowEnd := now
	  windowStart := now.Add(-a.cfg.WindowSize)
	   // 拉取窗口内决策。
	  decisions, err := a.store.ListDecisions(ctx, rel.ID, 10000)
	  if err != nil {
		   return err
	  }
	  // 过滤窗口内记录。
	   var (
		  total     int64
		   diffCount int64
		  allowDeny  int64
		   denyAllow  int64
		   limitDrift int64
		   killSwitch int64
	   )
	  for _, d := range decisions {
		   if d.CreatedAt.Before(windowStart) {
			   continue
		  }
		   total++
		  if !d.HasDiff {
			  continue
		   }
		   diffCount++
		   switch DiffKind(d.DiffKind) {
		  case DiffAllow:
			  allowDeny++
		   case DiffDeny:
			   denyAllow++
		   case DiffLimit:
			   limitDrift++
		   case DiffKillSwitch:
			  killSwitch++
		  }
	   }
	   ratio := 0.0
	   if total > 0 {
		   ratio = math.Round(float64(diffCount)/float64(total)*10000) / 100
	  }
	  exceedsPolicy := a.policy.ShouldEnforce(&SplitDiff{
		  AllowDenyCount: allowDeny,
		  KillSwitchCount: killSwitch,
		   DiffRatio:       ratio,
	   })
	  diff := &SplitDiff{
		  ID:             a.idgen(),
		   GrayReleaseID:  rel.ID,
		   Generation:     rel.Generation,
		  WindowStart:    windowStart,
		  WindowEnd:      windowEnd,
		  TotalDecisions: total,
		  DiffCount:      diffCount,
		   DiffRatio:      ratio,
		  AllowDenyCount: allowDeny,
		  DenyAllowCount:  denyAllow,
		  LimitDriftCount: limitDrift,
		  KillSwitchCount: killSwitch,
		   ExceedsPolicy: exceedsPolicy,
		   CreatedAt:     now,
	   }
	   if err := a.store.SaveSplitDiff(ctx, diff); err != nil {
		  return fmt.Errorf("save split diff: %w", err)
	   }
	   if a.metrics != nil {
		   a.metrics.DiffRatio.WithLabelValues(rel.ID).Set(ratio)
		  a.metrics.AllowDenyTotal.WithLabelValues(rel.ID).Add(float64(allowDeny))
	   }
	  // 超限时触发 SealExecutor（inline 触发，避免延迟）。
	  if exceedsPolicy && a.seals != nil {
		   gate := a.seals.GetOrCreate(rel.ID)
		   act, err := gate.Enforce(diff.ID,
			   fmt.Sprintf("diff_ratio=%.2f allow_deny=%d kill_switch=%d", ratio, allowDeny, killSwitch),
			  "system",
		   )
		   if err != nil {
			   a.log.Warn().Err(err).Str("release_id", rel.ID).Msg("cfgdual: seal enforce failed")
		   }
		   if act != nil {
			   if err := a.store.SaveSealAction(ctx, act); err != nil {
				  a.log.Error().Err(err).Str("release_id", rel.ID).Msg("cfgdual: save seal action failed")
			   }
			   if a.metrics != nil {
				  a.metrics.SealEnforced.WithLabelValues(rel.ID).Inc()
			  }
			   a.log.Warn().
				   Str("release_id", rel.ID).
				  Str("diff_id", diff.ID).
				   Float64("ratio", ratio).
				  Int64("allow_deny", allowDeny).
				   Msg("cfgdual: SEAL ENFORCED - split-brain detected")
		  }
	  }
	   return nil
  }

// ─────────────────────────────────────────────────────────────────────────────
// SealExecutor 封堵执行 Worker
// ─────────────────────────────────────────────────────────────────────────────
// SealExecutor 定期扫描近期 SplitDiff，对超限记录触发 Seal。
// 注：DiffAggregate 已内联触发；SealExecutor 作为补偿扫描，处理滞后落仓场景。
type SealExecutor struct {
	store Store
	seals *SealRegistry
	policy SealPolicy
	idgen   func() string
	metrics *DualMetrics
	log zerolog.Logger
}

// NewSealExecutor 构造封堵执行器。
   func NewSealExecutor(
	  store Store,
	  seals *SealRegistry,
	  policy SealPolicy,
	  idgen func() string,
	metrics *DualMetrics,
	log zerolog.Logger,
) *SealExecutor {
	return &SealExecutor{
		store: store,
		seals:  seals,
		policy: policy,
		idgen:   idgen,
		metrics: metrics,
		log: log,
	}
}

// Tick 单次封堵扫描（由 workerloop.Run 驱动）。
   func (e *SealExecutor) Tick(ctx context.Context) error {
	  releases, err := e.store.ListActiveReleases(ctx)
	   if err != nil {
		  return fmt.Errorf("seal executor: list releases: %w", err)
	  }
	  for _, rel := range releases {
		  if err := e.checkRelease(ctx, rel); err != nil {
			  e.log.Error().Err(err).Str("release_id", rel.ID).Msg("cfgdual: seal executor check failed")
		  }
	   }
	  return nil
  }

// checkRelease 检查单个发布的最新 SplitDiff，若超限则触发封堵。
  func (e *SealExecutor) checkRelease(ctx context.Context, rel *GrayRelease) error {
	   diffs,
		   err := e.store.ListSplitDiffs(ctx, rel.ID, 1)
	  if err != nil || len(diffs) == 0 {
		   return err
	   }
	  latest := diffs[0]
	   if !e.policy.ShouldEnforce(latest) {
		   return nil
	   }
	   gate := e.seals.GetOrCreate(rel.ID)
	  if gate.IsEnforced() {
		   return nil // 已封堵，幂等
	  }
	  act, err := gate.Enforce(latest.ID,
		  fmt.Sprintf("seal executor: diff_ratio=%.2f allow_deny=%d", latest.DiffRatio, latest.AllowDenyCount),
		  "system",
	  )
	   if err != nil {
		   return err
	  }
	  if act != nil {
		   if err := e.store.SaveSealAction(ctx, act); err != nil {
			  e.log.Error().Err(err).Str("release_id", rel.ID).Msg("cfgdual: save seal action failed")
		  }
		   if e.metrics != nil {
			  e.metrics.SealEnforced.WithLabelValues(rel.ID).Inc()
		  }
		   e.log.Error().
			  Str("release_id", rel.ID).
			  Float64("diff_ratio", latest.DiffRatio).
			   Int64("allow_deny", latest.AllowDenyCount).
			   Msg("cfgdual: SEAL ENFORCED by executor")
	   }
	  return nil
  }

// ─────────────────────────────────────────────────────────────────────────────
// RollbackPatrol 回滚巡检 Worker
// ─────────────────────────────────────────────────────────────────────────────
// RollbackPatrolConfig 回滚巡检参数。
type RollbackPatrolConfig struct {
	// AutoRollbackThreshold allow_deny 累计超过此值时自动回滚（默认 10）。
	AutoRollbackThreshold int64
	// SealedDurationBeforeRollback 封堵持续超过此时长后自动回滚（默认 30 分钟）。
	SealedDurationBeforeRollback time.Duration
}

// DefaultRollbackPatrolConfig 默认巡检配置。

   func DefaultRollbackPatrolConfig() RollbackPatrolConfig {
	   return RollbackPatrolConfig{
		   AutoRollbackThreshold:        10,
		   SealedDurationBeforeRollback: 30 * time.Minute,
	   }
  }

// RollbackPatrol 监测已封堵发布，持续差分超限时自动触发回滚。
type RollbackPatrol struct {
	store Store
	seals *SealRegistry
	rollback *RollbackManager
	cfg     RollbackPatrolConfig
	clock   func() time.Time
	metrics *DualMetrics
	log zerolog.Logger
}

// NewRollbackPatrol 构造回滚巡检器。

  func NewRollbackPatrol(
	   store Store,
	   seals *SealRegistry,
	  rollback *RollbackManager,
	  cfg RollbackPatrolConfig,
	   metrics *DualMetrics,
	   log zerolog.Logger,
  ) *RollbackPatrol {
	  return &RollbackPatrol{
		   store: store,
		  seals:    seals,
		   rollback: rollback,
		   cfg:      cfg,
		   clock:   func() time.Time { return time.Now().UTC() },
		  metrics: metrics,
		  log:     log,
	  }
   }

// Tick 单次巡检循环（由 workerloop.Run 驱动）。
  func (p *RollbackPatrol) Tick(ctx context.Context) error {
	  releases, err := p.store.ListActiveReleases(ctx)
	  if err != nil {
		   return fmt.Errorf("rollback patrol: list releases: %w", err)
	   }
	   for _, rel := range releases {
		   if err := p.patrolRelease(ctx, rel); err != nil {
			   p.log.Error().Err(err).Str("release_id", rel.ID).Msg("cfgdual: rollback patrol failed")
		  }
	   }
	   return nil
   }

// patrolRelease 检查单个发布：封堵持续超时或累计 allow_deny 超阈值则触发自动回滚。
   func (p *RollbackPatrol) patrolRelease(ctx context.Context, rel *GrayRelease) error {
	   gate := p.seals.Get(rel.ID)
	  if gate == nil || !gate.IsEnforced() {
		   return nil // 未封堵，无需巡检
	   }
	  // 查询最新差分。
	  diffs,
		   err := p.store.ListSplitDiffs(ctx, rel.ID, 1)
	  if err != nil || len(diffs) == 0 {
		  return err
	  }
	   latest := diffs[0]
	   // 自动回滚条件：allow_deny 超阈或封堵持续超时。
	   shouldRollback := false
	   reason := ""
	  if latest.AllowDenyCount >= p.cfg.AutoRollbackThreshold {
		  shouldRollback = true
		   reason = fmt.Sprintf("auto rollback: allow_deny_count=%d >= threshold=%d",
			  latest.AllowDenyCount, p.cfg.AutoRollbackThreshold)
	  } else {
		   // 检查封堵动作时间。
		  actions, err := p.store.ListSealActions(ctx, rel.ID)
		  if err == nil {
			  for _, a := range actions {
				   if a.ToState == SealEnforced {
					  if p.clock().Sub(a.CreatedAt) >= p.cfg.SealedDurationBeforeRollback {
						   shouldRollback = true
						   reason = fmt.Sprintf("auto rollback: sealed for %v",
							   p.clock().Sub(a.CreatedAt).Round(time.Second))
						  break
					   }
				  }
			   }
		  }
	  }
	   if !shouldRollback {
		   return nil
	   }
	  proof, err := p.rollback.Rollback(ctx, rel, latest.ID, "system", reason)
	   if err != nil {
		   p.log.Error().Err(err).Str("release_id", rel.ID).Msg("cfgdual: auto rollback failed")
		  return err
	  }
	   if p.metrics != nil {
		   p.metrics.RollbackTotal.WithLabelValues(rel.ID).Inc()
	   }
	  p.log.Error().
		   Str("release_id", rel.ID).
		   Str("proof_id", proof.ID).
		   Str("reason", reason).
		   Msg("cfgdual: AUTO ROLLBACK EXECUTED")
	  return nil
   }

// ─────────────────────────────────────────────────────────────────────────────
// WorkerSet 统一管理四类 Worker 的启动参数
// ─────────────────────────────────────────────────────────────────────────────
// WorkerTickFn 返回各 worker 的 Tick 函数（供 workerloop.Run 使用）。
type WorkerTickFn = func(ctx context.Context) error

// WorkerSet 打包四类 Worker 的 Tick 函数，供启动程序注册到 workerloop。
type WorkerSet struct {
	DecisionPump  WorkerTickFn
	DiffAggregate WorkerTickFn
	SealExecutor WorkerTickFn
	RollbackPatrol WorkerTickFn
}

// BuildWorkerSet 构造口径 WorkerSet。
   func BuildWorkerSet(mgr *Manager, log zerolog.Logger) *WorkerSet {
	   metrics := NewDualMetrics()
	   pump := NewDecisionPump(mgr.Engine, mgr.Store, metrics, log)
	   agg := NewDiffAggregate(mgr.Store, mgr.Seals, mgr.Policy,
		  DefaultDiffAggregateConfig(), mgr.idgen, metrics, log)
	  exec := NewSealExecutor(mgr.Store, mgr.Seals, mgr.Policy, mgr.idgen, metrics, log)
	   patrol := NewRollbackPatrol(mgr.Store, mgr.Seals, mgr.Rollback,
		   DefaultRollbackPatrolConfig(), metrics, log)
	   return &WorkerSet{
		   DecisionPump: pump.Tick,
		  DiffAggregate: agg.Tick,
		  SealExecutor: exec.Tick,
		   RollbackPatrol: patrol.Tick,
	   }
  }
