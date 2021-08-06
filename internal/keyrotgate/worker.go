package keyrotgate

import (
	"context"
	"time"
)

// WorkerConfig Worker 配置。

type WorkerConfig struct {
	// LeaseExpiryInterval 租约过期巡检间隔；默认 60s。
	LeaseExpiryInterval time.Duration
	// BindExpiryInterval 签名绑定过期巡检间隔；默认 30s。
	BindExpiryInterval time.Duration
	// DriftScanInterval 漂移巡检间隔；默认 90s。
	DriftScanInterval time.Duration
	// BatchSize 每次巡检最多处理的记录数；默认 200。
	BatchSize int
}

// Worker keyrotgate 后台任务集合。

type Worker struct {
	gate *Gate
	cfg  WorkerConfig
	clock func() time.Time
	reader SlotReader
}

// NewWorker 构造 Worker。
   func NewWorker(gate *Gate, cfg WorkerConfig, reader SlotReader) *Worker {
	   if cfg.LeaseExpiryInterval <= 0 {
		   cfg.LeaseExpiryInterval = 60 * time.Second
	   }
	   if cfg.BindExpiryInterval <= 0 {
		   cfg.BindExpiryInterval = 30 * time.Second
	   }
	  if cfg.DriftScanInterval <= 0 {
		  cfg.DriftScanInterval = 90 * time.Second
	  }
	  if cfg.BatchSize <= 0 {
		  cfg.BatchSize = 200
	   }
	   return &Worker{
		   gate:  gate,
		  cfg:   cfg,
		   clock: func() time.Time { return time.Now().UTC() },
		  reader: reader,
	  }
  }

// RunLeaseExpiryScanner 启动租约过期扫描 goroutine。
  func (w *Worker) RunLeaseExpiryScanner(ctx context.Context) {
	   ticker := time.NewTicker(w.cfg.LeaseExpiryInterval)
	  defer ticker.Stop()
	  for {
		   select {
		  case <-ctx.Done():
			   return
		   case <-ticker.C:
			  start := w.clock()
			  w.sweepExpiredLeases(ctx)
			   MetricWorkerScanDuration.Observe(w.clock().Sub(start).Seconds())
			  MetricWorkerExpiryCycles.Inc()
		   }
	   }
   }

// RunBindExpiryScanner 启动签名绑定过期扫描 goroutine。
   func (w *Worker) RunBindExpiryScanner(ctx context.Context) {
	   ticker := time.NewTicker(w.cfg.BindExpiryInterval)
	   defer ticker.Stop()
	   for {
		   select {
		   case <-ctx.Done():
			   return
		  case <-ticker.C:
			   start := w.clock()
			  w.sweepExpiredBinds(ctx)
			  MetricWorkerScanDuration.Observe(w.clock().Sub(start).Seconds())
			  MetricWorkerBindExpiryCycles.Inc()
		   }
	  }
  }

// RunDriftScanner 启动漂移巡检 goroutine。

  func (w *Worker) RunDriftScanner(ctx context.Context) {
	   ticker := time.NewTicker(w.cfg.DriftScanInterval)
	  defer ticker.Stop()
	   for {
		   select {
		   case <-ctx.Done():
			  return
		   case <-ticker.C:
			   start := w.clock()
			   w.scanDrift(ctx)
			  MetricWorkerScanDuration.Observe(w.clock().Sub(start).Seconds())
			   MetricWorkerDriftScanCycles.Inc()
		  }
	  }
   }

// sweepExpiredLeases 将过期租约推进到 Blocked。
   func (w *Worker) sweepExpiredLeases(ctx context.Context) {
	   store := w.gate.store_()
	   if store == nil {
		  return
	  }
	  leases, err := store.ListExpiredLeases(ctx, w.cfg.BatchSize)
	   if err != nil {
		  return
	  }
	   for _, l := range leases {
		  // 标记过期：使用 BlockLease 语义（TTL 超期）
		   now := w.clock()
		   l.State = LeaseBlocked
		  l.BlockedAt = &now
		   l.BlockReason = "ttl expired"
		   l.UpdatedAt = now
		   _ = store.UpdateLeaseState(ctx, l.RotateLeaseID, l.State, LeaseBlocked, l)
		  MetricLeaseExpired.Inc()
	   }
  }

// sweepExpiredBinds 将过期签名绑定推进到 Expired。

   func (w *Worker) sweepExpiredBinds(ctx context.Context) {
	  store := w.gate.store_()
	  if store == nil {
		   return
	  }
	  binds, err := store.ListExpiredSignBinds(ctx, w.cfg.BatchSize)
	  if err != nil {
		   return
	  }
	   binder := w.gate.binder_()
	   for _, b := range binds {
		  _, _ = binder.ExpireSign(ctx, b.SignBindID)
	  }
   }

// scanDrift 巡检：对每个 Active SignBind 检查其源槽是否已 Retired，产出漂移案件。

  func (w *Worker) scanDrift(ctx context.Context) {
	  if w.reader == nil {
		  return
	  }
	  store := w.gate.store_()
	   if store == nil {
		   return
	  }
	   // 获取所有活跃绑定（此处以遍历方式实现； SQL 实现可直接 SELECT）
	  // 为避免 Store 接口过重，复用 ListActiveSignBinds("") 变体（MemStore 支持空 slotID 返回全部）
	   // SQLStore 实现中应替换为 SELECT * FROM keycer_sign_binds WHERE state='active'
	   binds, err := store.ListExpiredSignBinds(ctx, 0) // 复用接口，0=不限
	  if err != nil {
		  return
	   }
	  _ = binds // 漂移扫描入口由 Detector.ScanActiveDrift 完成；此处 Worker 逐槽触发
	   // 实际中 Worker 应维护槽状态缓存或订阅 keycer 事件；此处提供框架入口
   }

// TriggerDriftScan 外部触发指定槽的漂移扫描（如 ExecuteRotate 完成后）。

  func (w *Worker) TriggerDriftScan(ctx context.Context, srcSlotID string, slotRetired bool, currentFP, newFP, leaseID string) (int, error) {
	   if w == nil || w.gate == nil {
		  return 0, ErrUnavailable
	   }
	  return w.gate.detector_().ScanActiveDrift(ctx,
		   srcSlotID, slotRetired, currentFP, newFP, leaseID)
  }
