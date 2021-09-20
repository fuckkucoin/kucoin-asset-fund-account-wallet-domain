package p2pabandon

import (
	"context"
	"time"
)

// FundsBridge 资金操作桥（由调用方注入，本包不直接写 ledger 或 holdcat）。
// Worker 通过此接口完成保证金释放贷记，隔离资金层依赖。
type FundsBridge interface {
	// ReleaseSellHold 释放 P2P 卖单保证金（幂等）。
	// 内部贷记到用户可用余额并释放 holdcat hold。
	// 返回账本分录 ID 与 holdcat hold ID（均为只读引用）。
	ReleaseSellHold(ctx context.Context, planID, orderID, userID, asset, amount, holdID string) (ledgerRef, holdRef string, err error)
}

// ConflictNotifier conflict 事件通知接口（由调用方实现）。
// Worker 将 conflict 计划推送给此接口，触发 desk 告警或工单创建。

type ConflictNotifier interface {
	// NotifyConflict 通知 conflict 事件。
	NotifyConflict(ctx context.Context, plan AbandonPlan, tokens []MutexToken) error
}

// WorkerConfig Worker 公共配置。

type WorkerConfig struct {
	// TimeoutScanInterval 超时信号扫描间隔。
	TimeoutScanInterval time.Duration
	// AbandonAdvInterval 弃单推进扫描间隔。
	AbandonAdvInterval time.Duration
	// AgingPatrolInterval 超龄巡检间隔。
	AgingPatrolInterval time.Duration
	// ProofPatrolInterval 证明巡检间隔。
	ProofPatrolInterval time.Duration
	// ConflictScanInterval 冲突扫描间隔。
	ConflictScanInterval time.Duration
	// BatchSize 每次扫描最大处理数。
	BatchSize int
}

// DefaultWorkerConfig 默认 Worker 配置。
var DefaultWorkerConfig = WorkerConfig{
	TimeoutScanInterval: 30 * time.Second,
	AbandonAdvInterval:  60 * time.Second,
	AgingPatrolInterval: 5 * time.Minute,
	ProofPatrolInterval:  10 * time.Minute,
	ConflictScanInterval: 2 * time.Minute,
	BatchSize:            100,
}

// ─────────────────────────── TimeoutScanWorker ───────────────────────────
// TimeoutScanWorker 超时扫描 Worker。
// 从 OrderTimeoutSource 拉取超时信号，调用 Service.ArmPlan 创建弃单计划。
// 成功创建后 Ack 信号；失败则不 Ack，等待下次重试。

type TimeoutScanWorker struct {
	svc    *Service
	source OrderTimeoutSource
	cfg WorkerConfig
	m   *Metrics
}

  func NewTimeoutScanWorker(svc *Service, source OrderTimeoutSource, m *Metrics) *TimeoutScanWorker {
	   return NewTimeoutScanWorkerWithConfig(svc, source, m, DefaultWorkerConfig)
  }

   func NewTimeoutScanWorkerWithConfig(svc *Service, source OrderTimeoutSource, m *Metrics, cfg WorkerConfig) *TimeoutScanWorker {
	  if cfg.TimeoutScanInterval <= 0 {
		  cfg.TimeoutScanInterval = DefaultWorkerConfig.TimeoutScanInterval
	  }
	   if cfg.BatchSize <= 0 {
		   cfg.BatchSize = DefaultWorkerConfig.BatchSize
	  }
	  return &TimeoutScanWorker{svc: svc,
		   source: source, cfg: cfg, m: m}
   }

// Run 启动超时扫描 Worker（阻塞；在 ctx 取消时返回）。
   func (w *TimeoutScanWorker) Run(ctx context.Context) {
	  ticker := time.NewTicker(w.cfg.TimeoutScanInterval)
	   defer ticker.Stop()
	  for {
		  select {
		  case <-ctx.Done():
			  return
		   case <-ticker.C:
			   w.once(ctx)
		  }
	  }
  }

   func (w *TimeoutScanWorker) once(ctx context.Context) {
	   sigs, err := w.source.PollTimeoutSignals(ctx, w.cfg.BatchSize)
	  if err != nil {
		  metricsInc(w.m, MetricWorkerError, "timeout_scan")
		  return
	  }
	  for _, sig := range sigs {
		  idem := "arm-plan:" + sig.OrderID
		  _, armErr := w.svc.ArmPlan(ArmPlanReq{
			   OrderID: sig.OrderID,
			   UserID:  sig.UserID,
			   Asset:   sig.Asset,
			   Amount:  sig.Amount,
			   HoldID: sig.HoldID,
			   IdempotencyKey: idem,
		  })
		   if armErr != nil && armErr != ErrPlanExists {
			  metricsInc(w.m, MetricWorkerError, "arm_plan")
			  continue
		  }
		   // 成功或幂等重复，均 Ack。
		  _ = w.source.AckTimeoutSignal(ctx,
			   sig.OrderID)
		   metricsInc(w.m, MetricPlanArmed, "")
	  }
   }

// ─────────────────────────── AbandonAdvWorker ───────────────────────────
// AbandonAdvWorker 弃单推进 Worker。
// 扫描所有 armed 计划，对宽限期已满的计划调用 AdvanceAbandon（armed→abandoning），
// 再调用 FundsBridge.ReleaseSellHold 完成资金释放，最后调用 CommitRelease。

type AbandonAdvWorker struct {
	svc    *Service
	bridge FundsBridge
	cfg    WorkerConfig
	m      *Metrics
}

  func NewAbandonAdvWorker(svc *Service, bridge FundsBridge, m *Metrics) *AbandonAdvWorker {
	  return NewAbandonAdvWorkerWithConfig(svc, bridge, m, DefaultWorkerConfig)
  }

   func NewAbandonAdvWorkerWithConfig(svc *Service, bridge FundsBridge, m *Metrics, cfg WorkerConfig) *AbandonAdvWorker {
	  if cfg.AbandonAdvInterval <= 0 {
		   cfg.AbandonAdvInterval = DefaultWorkerConfig.AbandonAdvInterval
	   }
	  if cfg.BatchSize <= 0 {
		   cfg.BatchSize = DefaultWorkerConfig.BatchSize
	   }
	   return &AbandonAdvWorker{svc: svc, bridge: bridge, cfg: cfg, m: m}
  }

// Run 启动弃单推进 Worker（阻塞；在 ctx 取消时返回）。
  func (w *AbandonAdvWorker) Run(ctx context.Context) {
	  ticker := time.NewTicker(w.cfg.AbandonAdvInterval)
	  defer ticker.Stop()
	  for {
		  select {
		   case <-ctx.Done():
			  return
		  case <-ticker.C:
			   w.once(ctx)
		   }
	  }
   }

   func (w *AbandonAdvWorker) once(ctx context.Context) {
	  now := w.svc.now()
	  armed := w.svc.ListArmedPlans()
	   count := 0
	   for _, plan := range armed {
		   if count >= w.cfg.BatchSize {
			  break
		  }
		  // 宽限期：ArmedAt + GraceSecs 之后才实际发起弃单释放。
		  graceDur := time.Duration(plan.GraceSecs) * time.Second
		   if now.Before(plan.ArmedAt.Add(graceDur)) {
			  continue
		  }
		  w.advanceOne(ctx, plan)
		  count++
	   }
	   // 同时处理 abandoning 状态（可能因重启中断，需恢复推进）。
	  abandoning := w.svc.ListAbandoningPlans()
	  for _, plan := range abandoning {
		  if count >= w.cfg.BatchSize {
			  break
		   }
		  // 找到 reserved 状态的 release 令牌，接着推进。
		   tokens := w.svc.ListTokensByPlan(plan.ID)
		  for _, t := range tokens {
			   if t.Kind == TokenKindRelease && t.Status == TokenStatusReserved {
				  w.releaseOne(ctx, plan, t)
				  count++
				  break
			   }
		   }
	   }
  }

   func (w *AbandonAdvWorker) advanceOne(ctx context.Context, plan AbandonPlan) {
	   updatedPlan, token, err := w.svc.AdvanceAbandon(AdvanceAbandonReq{
		  PlanID: plan.ID,
		   Actor: "abandon_adv_worker",
	   })
	  if err != nil {
		  metricsInc(w.m, MetricWorkerError, "advance_abandon")
		  return
	  }
	   if updatedPlan.Status != PlanStatusAbandoning {
		  return
	   }
	  w.releaseOne(ctx, updatedPlan, token)
  }

   func (w *AbandonAdvWorker) releaseOne(ctx context.Context, plan AbandonPlan, token MutexToken) {
	  if w.bridge == nil {
		  metricsInc(w.m, MetricWorkerError, "no_bridge")
		   return
	  }
	  ledgerRef, holdRef, err := w.bridge.ReleaseSellHold(
		  ctx, plan.ID, plan.OrderID, plan.UserID, plan.Asset, plan.Amount, plan.HoldID,
	   )
	  if err != nil {
		   metricsInc(w.m, MetricWorkerError, "release_sell_hold")
		   return
	  }
	   _, _, commitErr := w.svc.CommitRelease(CommitReleaseReq{
		  PlanID: plan.ID,
		   TokenID:   token.ID,
		   LedgerRef: ledgerRef,
		   HoldRef: holdRef,
		   Actor: "abandon_adv_worker",
	  })
	  if commitErr != nil && commitErr != ErrTokenConflict && commitErr != ErrPlanTerminal {
		  metricsInc(w.m, MetricWorkerError, "commit_release")
		   return
	  }
	  if commitErr == ErrTokenConflict {
		   metricsInc(w.m, MetricConflict, "release_vs_fill")
		  return
	   }
	  metricsInc(w.m, MetricPlanReleased, "")
   }

// ─────────────────────────── AgingPatrolWorker ───────────────────────────
// AgingPatrolWorker 超龄巡检 Worker。
// 定期调用 PatrolAging，更新 HoldAge 阶段并自动升级超 SLA 的 hold。
type AgingPatrolWorker struct {
	svc      *Service
	observer HoldObserver
	cfg      WorkerConfig
	m *Metrics
}

  func NewAgingPatrolWorker(svc *Service, observer HoldObserver, m *Metrics) *AgingPatrolWorker {
	  return NewAgingPatrolWorkerWithConfig(svc, observer, m, DefaultWorkerConfig)
   }

  func NewAgingPatrolWorkerWithConfig(svc *Service, observer HoldObserver, m *Metrics, cfg WorkerConfig) *AgingPatrolWorker {
	   if cfg.AgingPatrolInterval <= 0 {
		   cfg.AgingPatrolInterval = DefaultWorkerConfig.AgingPatrolInterval
	   }
	   return &AgingPatrolWorker{svc: svc, observer: observer, cfg: cfg, m: m}
  }

// Run 启动超龄巡检 Worker。
   func (w *AgingPatrolWorker) Run(ctx context.Context) {
	  ticker := time.NewTicker(w.cfg.AgingPatrolInterval)
	   defer ticker.Stop()
	  for {
		  select {
		  case <-ctx.Done():
			  return
		   case <-ticker.C:
			  w.once(ctx)
		   }
	   }
  }

   func (w *AgingPatrolWorker) once(ctx context.Context) {
	   holdBirthFn := func(planID, holdID string) (time.Time, error) {
		   return w.observer.HoldBirth(ctx, planID, holdID)
	   }
	   result := w.svc.PatrolAging(holdBirthFn)
	   if result.EscCnt > 0 {
		   metricsAdd(w.m,
			  MetricHoldEscalated, float64(result.EscCnt))
	   }
	  for range result.Errors {
		  metricsInc(w.m, MetricWorkerError, "aging_patrol")
	   }
  }

// ─────────────────────────── ProofPatrolWorker ───────────────────────────
// ProofPatrolWorker 证明巡检 Worker。
// 定期巡检所有 CloseProof 的哈希完整性；发现篡改时记录 metrics 并告警。

type ProofPatrolWorker struct {
	svc *Service
	cfg WorkerConfig
	m   *Metrics
}

  func NewProofPatrolWorker(svc *Service, m *Metrics) *ProofPatrolWorker {
	  return NewProofPatrolWorkerWithConfig(svc, m, DefaultWorkerConfig)
  }

  func NewProofPatrolWorkerWithConfig(svc *Service, m *Metrics, cfg WorkerConfig) *ProofPatrolWorker {
	  if cfg.ProofPatrolInterval <= 0 {
		   cfg.ProofPatrolInterval = DefaultWorkerConfig.ProofPatrolInterval
	  }
	   return &ProofPatrolWorker{svc: svc, cfg: cfg, m: m}
  }

// Run 启动证明巡检 Worker。

  func (w *ProofPatrolWorker) Run(ctx context.Context) {
	  ticker := time.NewTicker(w.cfg.ProofPatrolInterval)
	  defer ticker.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			   return
		  case <-ticker.C:
			  w.once()
		  }
	   }
  }

   func (w *ProofPatrolWorker) once() {
	   result := w.svc.PatrolAllProofs()
	   if result.TamperFound > 0 {
		   metricsAdd(w.m, MetricProofTamper, float64(result.TamperFound))
	   }
	  for range result.Errors {
		  metricsInc(w.m, MetricWorkerError, "proof_patrol")
	   }
   }

// ─────────────────────────── ConflictDeskWorker ───────────────────────────
// ConflictDeskWorker 冲突仲裁 Worker。
// 扫描 conflict 状态的计划，调用 ConflictNotifier 推送到 desk 告警或工单。
// 本 Worker 不自动仲裁（需人工 desk 操作），仅确保 conflict 被及时通知。
type ConflictDeskWorker struct {
	svc *Service
	notifier ConflictNotifier
	cfg      WorkerConfig
	m        *Metrics
	notified map[string]bool // 已通知的 plan_id（防重复推送）
}

   func NewConflictDeskWorker(svc *Service, notifier ConflictNotifier, m *Metrics) *ConflictDeskWorker {
	  return NewConflictDeskWorkerWithConfig(svc,
		  notifier, m, DefaultWorkerConfig)
  }

   func NewConflictDeskWorkerWithConfig(svc *Service, notifier ConflictNotifier, m *Metrics, cfg WorkerConfig) *ConflictDeskWorker {
	   if cfg.ConflictScanInterval <= 0 {
		   cfg.ConflictScanInterval = DefaultWorkerConfig.ConflictScanInterval
	   }
	  return &ConflictDeskWorker{
		  svc: svc,
		   notifier: notifier,
		   cfg: cfg,
		   m:   m,
		  notified: make(map[string]bool),
	  }
  }

// Run 启动冲突仲裁通知 Worker。
   func (w *ConflictDeskWorker) Run(ctx context.Context) {
	  ticker := time.NewTicker(w.cfg.ConflictScanInterval)
	   defer ticker.Stop()
	  for {
		   select {
		   case <-ctx.Done():
			  return
		  case <-ticker.C:
			   w.once(ctx)
		  }
	  }
   }

   func (w *ConflictDeskWorker) once(ctx context.Context) {
	   plans := w.svc.ListAllPlans()
	   for _, plan := range plans {
		  if plan.Status != PlanStatusConflict {
			  continue
		  }
		  if w.notified[plan.ID] {
			  continue
		  }
		  tokens := w.svc.ListTokensByPlan(plan.ID)
		   if w.notifier != nil {
			   err := w.notifier.NotifyConflict(ctx,
				  plan, tokens)
			   if err != nil {
				  metricsInc(w.m, MetricWorkerError, "conflict_notify")
				  continue
			   }
		  }
		  w.notified[plan.ID] = true
		  metricsInc(w.m, MetricConflict, "notified")
	   }
   }
