package lendoccupygate

import (
	"context"
	"time"
)

const (
	defaultHalfPlaceInterval  = 60 * time.Second
	defaultDupReleaseInterval = 90 * time.Second
	defaultMetricsInterval    = 30 * time.Second
	workerBatch               = 200
)

// WorkerConfig Worker 配置。
type WorkerConfig struct {
	// HalfPlaceInterval 半截扫描周期（默认 60s）。
	HalfPlaceInterval time.Duration
	// DupReleaseInterval 重复释放巡检周期（默认 90s）。
	DupReleaseInterval time.Duration
	// MetricsInterval 指标采集周期（默认 30s）。
	MetricsInterval time.Duration
	// StaleTTL placed 租约悬挂容忍时间（默认 5min）。
	StaleTTL time.Duration
	// BatchSize 单批最大处理数（默认 200）。
	BatchSize int
	// NodeID Worker 节点 ID（供日志追踪）。
	NodeID string
}

  func (c *WorkerConfig) fill() {
	   if c.HalfPlaceInterval <= 0 {
		   c.HalfPlaceInterval = defaultHalfPlaceInterval
	  }
	   if c.DupReleaseInterval <= 0 {
		  c.DupReleaseInterval = defaultDupReleaseInterval
	  }
	  if c.MetricsInterval <= 0 {
		  c.MetricsInterval = defaultMetricsInterval
	   }
	  if c.StaleTTL <= 0 {
		   c.StaleTTL = defaultStaleTTL
	  }
	  if c.BatchSize <= 0 {
		  c.BatchSize = workerBatch
	  }
	  if c.NodeID == "" {
		  c.NodeID = "lendoccupygate.worker"
	   }
  }

// Worker lendoccupygate 后台 Worker 集合。

type Worker struct {
	engine  *Engine
	scanner *Scanner
	store   Store
	metrics *Metrics
	config WorkerConfig
}

// NewWorker 构造 Worker。
  func NewWorker(engine *Engine, scanner *Scanner, store Store, metrics *Metrics, cfg WorkerConfig) *Worker {
	  cfg.fill()
	   return &Worker{
		  engine: engine,
		  scanner: scanner,
		  store: store,
		  metrics: metrics,
		  config: cfg,
	  }
   }

// RunAll 启动所有 Worker goroutine（非阻塞）。
// 通过 context 取消触发退出。

   func (w *Worker) RunAll(ctx context.Context) {
	  go w.runHalfPlaceWorker(ctx)
	   go w.runDupReleaseWorker(ctx)
	   go w.runMetricsWorker(ctx)
  }

// runHalfPlaceWorker 周期执行半截扫描补偿。
   func (w *Worker) runHalfPlaceWorker(ctx context.Context) {
	  ticker := time.NewTicker(w.config.HalfPlaceInterval)
	  defer ticker.Stop()
	   for {
		   select {
		   case <-ctx.Done():
			   return
		   case <-ticker.C:
			  result, err := w.scanner.ScanHalfPlace(ctx)
			   if err != nil {
				  if w.metrics != nil {
					  w.metrics.RecordScanError("half_place")
				  }
				  continue
			   }
			   if w.metrics != nil {
				  w.metrics.RecordHalfPlaceScan(result)
			  }
		   }
	   }
   }

// runDupReleaseWorker 周期执行重复释放巡检。
// 此实现拉取近期 placed 订单列表并检测 gen 已消费但 lease 未关闭的记录。
  func (w *Worker) runDupReleaseWorker(ctx context.Context) {
	   ticker := time.NewTicker(w.config.DupReleaseInterval)
	   defer ticker.Stop()
	  for {
		  select {
		  case <-ctx.Done():
			   return
		   case <-ticker.C:
			   w.runDupReleaseScan(ctx)
		  }
	  }
   }

   func (w *Worker) runDupReleaseScan(ctx context.Context) {
	   // 查询近期可能孤立的 placed 租约
	   threshold := time.Now().UTC().Add(-w.config.StaleTTL)
	   leases, err := w.store.ListStalePlacedLeases(ctx, threshold, w.config.BatchSize)
	   if err != nil {
		  if w.metrics != nil {
			  w.metrics.RecordScanError("dup_release")
		   }
		  return
	  }
	  orderIDs := make([]string, 0, len(leases))
	   for _, l := range leases {
		  orderIDs = append(orderIDs, l.OrderID)
	   }
	  if len(orderIDs) == 0 {
		   return
	  }
	  result,
		   err := w.scanner.ScanDupRelease(ctx, orderIDs)
	   if err != nil {
		  if w.metrics != nil {
			  w.metrics.RecordScanError("dup_release")
		   }
		   return
	   }
	   if w.metrics != nil {
		   w.metrics.RecordDupReleaseScan(result)
	  }
  }

// runMetricsWorker 周期采集并上报 placed 租约计数等指标。
   func (w *Worker) runMetricsWorker(ctx context.Context) {
	  if w.metrics == nil {
		  return
	   }
	   ticker := time.NewTicker(w.config.MetricsInterval)
	   defer ticker.Stop()
	   for {
		  select {
		  case <-ctx.Done():
			   return
		  case <-ticker.C:
			   threshold := time.Now().UTC().Add(-w.config.StaleTTL)
			  leases, err := w.store.ListStalePlacedLeases(ctx, threshold, w.config.BatchSize)
			  if err != nil {
				  continue
			  }
			   w.metrics.SetStalePlacedCount(len(leases))
		   }
	   }
  }
