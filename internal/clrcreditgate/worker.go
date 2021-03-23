package clrcreditgate

import (
	"context"
	"errors"
	"time"
)

// ──────────────────────────────────────────────────────────────────────────────
// WorkerConfig
// ──────────────────────────────────────────────────────────────────────────────
// WorkerConfig Worker 配置。

type WorkerConfig struct {
	// ExpiryScanInterval 过期租约扫描间隔（默认 60s）。
	ExpiryScanInterval time.Duration
	// UnredeemedScanInterval 未贷记巡检间隔（默认 5min）。
	UnredeemedScanInterval time.Duration
	// DrainPumpInterval 排空泵检测间隔（默认 30s）。
	DrainPumpInterval time.Duration
	// BatchSize 单次扫描最大处理数（默认 200）。
	BatchSize int
	// WorkerID 标识字符串（供日志/审计）。
	WorkerID string
}

// fill 填充默认值。
   func (c *WorkerConfig) fill() {
	   if c.ExpiryScanInterval <= 0 {
		  c.ExpiryScanInterval = 60 * time.Second
	   }
	   if c.UnredeemedScanInterval <= 0 {
		   c.UnredeemedScanInterval = 5 * time.Minute
	  }
	  if c.DrainPumpInterval <= 0 {
		  c.DrainPumpInterval = 30 * time.Second
	   }
	   if c.BatchSize <= 0 {
		   c.BatchSize = 200
	  }
	   if c.WorkerID == "" {
		   c.WorkerID = "clrcreditgate.worker"
	  }
   }

// ──────────────────────────────────────────────────────────────────────────────
// AlertSink 告警接口
// ──────────────────────────────────────────────────────────────────────────────
// AlertSink 告警接口（由调用方注入）。

type AlertSink interface {
	// AlertUnredeemed 报告存在未贷记 released Item（含 batchID, itemID, age）。
	AlertUnredeemed(batchID, itemID, asset, amount string, age time.Duration)
}

// noopAlertSink 无操作告警。

type noopAlertSink struct{}

  func (noopAlertSink) AlertUnredeemed(_, _, _, _ string, _ time.Duration) {}

// ──────────────────────────────────────────────────────────────────────────────
// Worker
// ──────────────────────────────────────────────────────────────────────────────
// Worker clrcreditgate 后台 Worker（含三类巡检）。
type Worker struct {
	engine  *Engine
	store   Store
	metrics *Metrics
	alert AlertSink
	cfg   WorkerConfig
	clock func() time.Time
}

// NewWorker 构造 Worker。
  func NewWorker(engine *Engine, store Store, metrics *Metrics, alert AlertSink, cfg WorkerConfig) *Worker {
	   cfg.fill()
	  if alert == nil {
		  alert = noopAlertSink{}
	   }
	  if metrics == nil {
		  metrics = NewMetrics(nil)
	  }
	  return &Worker{
		   engine:  engine,
		   store:   store,
		  metrics: metrics,
		   alert:   alert,
		   cfg:   cfg,
		  clock: func() time.Time { return time.Now().UTC() },
	  }
  }

// WithClock 注入时钟。
  func (w *Worker) WithClock(fn func() time.Time) *Worker {
	  if fn != nil {
		  w.clock = fn
	   }
	  return w
   }

  // Run 启动所有 Worker goroutine，阻塞直到 ctx 取消。

  func (w *Worker) Run(ctx context.Context) {
	  expiryDone := make(chan struct{})
	  unredeemedDone := make(chan struct{})
	   drainDone := make(chan struct{})
	  go func() {
		   defer close(expiryDone)
		   w.runLeaseExpiryLoop(ctx)
	   }()
	  go func() {
		  defer close(unredeemedDone)
		  w.runUnredeemedLoop(ctx)
	  }()
	   go func() {
		  defer close(drainDone)
		   w.runDrainPumpLoop(ctx)
	   }()
	  <-ctx.Done()
	  <-expiryDone
	  <-unredeemedDone
	  <-drainDone
  }

// ──────────────────────────────────────────────────────────────────────────────
// LeaseExpiryWorker
// ──────────────────────────────────────────────────────────────────────────────

  func (w *Worker) runLeaseExpiryLoop(ctx context.Context) {
	  ticker := time.NewTicker(w.cfg.ExpiryScanInterval)
	  defer ticker.Stop()
	   for {
		   select {
		   case <-ctx.Done():
			   return
		   case <-ticker.C:
			   res := w.ScanExpiredLeases(ctx)
			   w.metrics.recordExpiryScan(res)
		  }
	  }
   }

// ScanExpiredLeases 扫描并标记过期 Lease（单次）。
  func (w *Worker) ScanExpiredLeases(ctx context.Context) ScanResult {
	   now := w.clock()
	  res := ScanResult{At: now}
	  leases, err := w.store.ListLeases(ctx, LeaseListFilter{
		  States: []LeaseState{LeaseIssued},
		   ExpiredBefore: now,
		   Limit:         w.cfg.BatchSize,
	  })
	   if err != nil {
		  res.Errors++
		   return res
	  }
	   res.Scanned = len(leases)
	   for _, l := range leases {
		  if err := w.store.MarkLeaseExpired(ctx, l.LeaseID, now); err != nil {
			   if !errors.Is(err, ErrInvalidTransition) {
				   res.Errors++
			  }
			  continue
		  }
		  res.Expired++
		   w.metrics.incLeaseExpired()
	   }
	  return res
   }

// ──────────────────────────────────────────────────────────────────────────────
// UnredeemedWorker
// ──────────────────────────────────────────────────────────────────────────────

  func (w *Worker) runUnredeemedLoop(ctx context.Context) {
	   ticker := time.NewTicker(w.cfg.UnredeemedScanInterval)
	  defer ticker.Stop()
	  for {
		   select {
		  case <-ctx.Done():
			   return
		  case <-ticker.C:
			   res := w.ScanUnredeemed(ctx)
			  w.metrics.recordUnredeemedScan(res)
		   }
	   }
   }

// ScanUnredeemed 扫描已 released 但无 credit_ok 证明的 issued Lease（单次）。
// 只告警，不自动补贷记——需人工确认后重新 ApplyWithLease。

   func (w *Worker) ScanUnredeemed(ctx context.Context) ScanResult {
	   now := w.clock()
	   res := ScanResult{At: now}
	   // 查询所有 issued Lease（含未过期部分）
	   leases, err := w.store.ListLeases(ctx, LeaseListFilter{
		  States: []LeaseState{LeaseIssued},
		  Limit: w.cfg.BatchSize,
	  })
	  if err != nil {
		   res.Errors++
		   return res
	   }
	   res.Scanned = len(leases)
	   for _, l := range leases {
		   // 已过期的 issued Lease 由 LeaseExpiryWorker 处理
		   if l.Expired(now) {
			  continue
		   }
		   // 检查是否已有 credit_ok 证明
		  if _, err2 := w.store.GetOKProofByItem(ctx, l.ItemID); err2 == nil {
			   // 已贷记，正常
			  continue
		   }
		   // 无证明且 Lease 仍 issued：属于待 Apply 项，若超过告警阈值则通知
		  age := now.Sub(l.IssuedAt)
		  if age > w.cfg.ExpiryScanInterval*2 {
			   w.alert.AlertUnredeemed(l.BatchID,
				   l.ItemID, l.Asset, l.Amount, age)
			   w.metrics.incUnredeemedAlert()
		  }
	   }
	  return res
  }

// ──────────────────────────────────────────────────────────────────────────────
// DrainPumpWorker
// ──────────────────────────────────────────────────────────────────────────────

  func (w *Worker) runDrainPumpLoop(ctx context.Context) {
	  ticker := time.NewTicker(w.cfg.DrainPumpInterval)
	  defer ticker.Stop()
	  for {
		   select {
		  case <-ctx.Done():
			   return
		   case <-ticker.C:
			   res := w.PumpDrain(ctx)
			   w.metrics.recordDrainPump(res)
		  }
	  }
   }

// PumpDrain 检查 draining 批次进度，完成排空或超时强制废弃（单次）。
  func (w *Worker) PumpDrain(ctx context.Context) ScanResult {
	   now := w.clock()
	  res := ScanResult{At: now}
	   if w.engine == nil {
		   res.Errors++
		   return res
	   }
	   // 由于 MemStore 不提供"按状态列出 BatchDrain"接口，此处通过 engine.ExportLeases
	   // 的 BatchID 集合进行覆盖（ SQLStore 可为直接 SELECT status=draining）。
	  drainBatches, err := w.listDrainingBatches(ctx)
	   if err != nil {
		  res.Errors++
		   return res
	   }
	   res.Scanned = len(drainBatches)
	   for _, batchID := range drainBatches {
		  drain, err2 := w.store.GetBatchDrain(ctx, batchID)
		  if err2 != nil {
			   res.Errors++
			  continue
		   }
		  if drain.Status != DrainDraining {
			  continue
		  }
		   unconsumed, err3 := w.store.CountUnconsumedLeases(ctx, batchID, now)
		   if err3 != nil {
			   res.Errors++
			  continue
		   }
		   // 排空完成
		  if unconsumed == 0 {
			   patch := &BatchDrainState{SnapshotAt: now, UpdatedAt: now}
			   if err4 := w.store.UpdateBatchDrainStatus(ctx, batchID, DrainDraining, DrainClosed, patch); err4 == nil {
				  res.DrainClosed++
				  w.metrics.incDrainClosed()
			  }
			   continue
		   }
		   // 超时：强制废弃剩余 issued Lease
		  if !drain.DrainTimeoutAt.IsZero() && now.After(drain.DrainTimeoutAt) {
			   voided := w.voidRemaining(ctx,
				   batchID, now)
			  res.Voided += voided
			  // 废弃完成后尝试关批
			  if _, err5 := w.store.CountUnconsumedLeases(ctx, batchID, now); err5 == nil {
				  patch := &BatchDrainState{SnapshotAt: now,
					   UpdatedAt: now}
				  if err6 := w.store.UpdateBatchDrainStatus(ctx, batchID, DrainDraining, DrainClosed, patch); err6 == nil {
					  res.DrainClosed++
					   w.metrics.incDrainClosed()
				   }
			  }
		  }
	   }
	  return res
  }

// voidRemaining 废弃指定批次中所有 issued Lease（drain 超时路径）。
  func (w *Worker) voidRemaining(ctx context.Context, batchID string, now time.Time) int {
	  leases, err := w.store.ListLeases(ctx, LeaseListFilter{
		  BatchID: batchID,
		  States:  []LeaseState{LeaseIssued},
		   Limit: w.cfg.BatchSize,
	   })
	   if err != nil {
		  return 0
	  }
	  count := 0
	   for _, l := range leases {
		   if err2 := w.store.MarkLeaseVoid(ctx, l.LeaseID, "drain_timeout", now); err2 == nil {
			   count++
			   w.metrics.incLeaseVoidedDrainTimeout()
		  }
	  }
	  return count
  }

// listDrainingBatches 返回当前处于 draining 状态的 BatchID 列表（内存实现遍历所有 drain 记录）。
  func (w *Worker) listDrainingBatches(ctx context.Context) ([]string, error) {
	   // 通过 ExportLeases 遍历现有 BatchID 集合（内存实现 workaround）
	  // SQLStore 应实现 SELECT DISTINCT batch_id FROM clr_batch_drain WHERE status='draining'
	  leases, err := w.store.ListLeases(ctx, LeaseListFilter{
		  States: []LeaseState{LeaseIssued},
		   Limit:  w.cfg.BatchSize,
	   })
	  if err != nil {
		   return nil, err
	  }
	   seen := make(map[string]struct{})
	   out := make([]string, 0)
	   for _, l := range leases {
		  if _, ok := seen[l.BatchID]; ok {
			   continue
		   }
		  seen[l.BatchID] = struct{}{}
		  // 仅返回已进入 draining 状态的批次
		   if drain, err2 := w.store.GetBatchDrain(ctx, l.BatchID); err2 == nil && drain.Status == DrainDraining {
			  out = append(out, l.BatchID)
		   }
	   }
	   return out, nil
  }
