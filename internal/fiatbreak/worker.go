package fiatbreak

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/metrics"
)

// WorkerConfig worker 周期配置。

type WorkerConfig struct {
	// ReconInterval 收据对账扫描周期（默认 30s）。
	ReconInterval time.Duration
	// AgingInterval 老化催办扫描周期（默认 5min）。
	AgingInterval time.Duration
	// LeakInterval 挂账泄漏巡逻周期（默认 1min）。
	LeakInterval time.Duration
}

// DefaultWorkerConfig 默认 worker 配置。
  func DefaultWorkerConfig() WorkerConfig {
	   return WorkerConfig{
		  ReconInterval: 30 * time.Second,
		   AgingInterval: 5 * time.Minute,
		   LeakInterval:  1 * time.Minute,
	  }
  }

// Worker 聚合三条后台协程。

type Worker struct {
	breaks   *BreakService
	suspense *SuspenseService
	aging *AgingService
	cfg   WorkerConfig
	clock func() time.Time
	// reconQueue 外部可推送待对账预约 ID 的队列。
	reconQueue chan reconJob
}

type reconJob struct {
	BookingID    string
	PSPReceiptID string
	PSPAmount    string
	PSPStatus string
	Currency  string
	Evidence  string
}

// NewWorker 创建 worker 实例。

  func NewWorker(breaks *BreakService, suspense *SuspenseService, aging *AgingService, cfg WorkerConfig) *Worker {
	  return &Worker{
		  breaks: breaks,
		   suspense: suspense,
		  aging: aging,
		   cfg:   cfg,
		   clock: func() time.Time { return time.Now().UTC() },
		  reconQueue: make(chan reconJob, 256),
	   }
   }

// EnqueueRecon 将 PSP 收据加入对账队列（线程安全，满则丢弃，不阻塞热路径）。
   func (w *Worker) EnqueueRecon(bookingID, pspReceiptID, pspAmount, pspStatus, currency, evidence string) {
	  if w == nil {
		   return
	  }
	   select {
	  case w.reconQueue <- reconJob{
		  BookingID: bookingID,
		   PSPReceiptID: pspReceiptID,
		  PSPAmount:    pspAmount,
		   PSPStatus: pspStatus,
		   Currency: currency,
		  Evidence: evidence,
	   }:
	   default:
		   metrics.FiatWorker.WithLabelValues("recon_drop").Inc()
	   }
  }

// Run 启动三条后台 worker 协程，阻塞直到 ctx 取消。

   func (w *Worker) Run(ctx context.Context) {
	   if w == nil {
		   return
	   }
	  go w.runReconPump(ctx)
	   go w.runAgingEscalate(ctx)
	   go w.runLeakPatrol(ctx)
	  <-ctx.Done()
   }

// ─────────────────────────────────────────────────────────
// 收据对账泵
// ─────────────────────────────────────────────────────────
// runReconPump 消费对账队列，对每条 PSP 收据执行 Detect。
  func (w *Worker) runReconPump(ctx context.Context) {
	  interval := w.cfg.ReconInterval
	  if interval <= 0 {
		   interval = 30 * time.Second
	   }
	   ticker := time.NewTicker(interval)
	   defer ticker.Stop()
	   for {
		  select {
		  case <-ctx.Done():
			   return
		   case job := <-w.reconQueue:
			   w.processRecon(ctx, job)
		  case <-ticker.C:
			   // 定时扫描：处理积压队列
			   w.drainReconQueue(ctx)
		   }
	   }
   }

// processRecon 执行单条收据对账。

   func (w *Worker) processRecon(ctx context.Context, job reconJob) {
	  if w.breaks == nil {
		  return
	  }
	   _, err := w.breaks.Detect(ctx, DetectReq{
		   BookingID: job.BookingID,
		  PSPReceiptID: job.PSPReceiptID,
		  PSPAmount:    job.PSPAmount,
		   PSPStatus:    job.PSPStatus,
		  Currency: job.Currency,
		   Evidence: job.Evidence,
	  })
	  if err != nil {
		  metrics.FiatWorker.WithLabelValues("recon_error").Inc()
		   return
	   }
	  metrics.FiatWorker.WithLabelValues("recon_ok").Inc()
  }

// drainReconQueue 清空对账队列积压（非阻塞）。

  func (w *Worker) drainReconQueue(ctx context.Context) {
	   for {
		   select {
		  case <-ctx.Done():
			   return
		   case job := <-w.reconQueue:
			  w.processRecon(ctx, job)
		   default:
			  return
		  }
	   }
  }

// ─────────────────────────────────────────────────────────
// 老化催办
// ─────────────────────────────────────────────────────────
// runAgingEscalate 定期扫描 open 断裂/挂账，超期升级到桌面。
   func (w *Worker) runAgingEscalate(ctx context.Context) {
	   interval := w.cfg.AgingInterval
	  if interval <= 0 {
		   interval = 5 * time.Minute
	   }
	  ticker := time.NewTicker(interval)
	  defer ticker.Stop()
	  for {
		  select {
		   case <-ctx.Done():
			  return
		  case <-ticker.C:
			   events := w.aging.Scan()
			   if len(events) > 0 {
				   metrics.FiatWorker.WithLabelValues("aging_escalate").Add(float64(len(events)))
			  }
		   }
	  }
   }

// ─────────────────────────────────────────────────────────
// 挂账泄漏巡逻
// ─────────────────────────────────────────────────────────
// runLeakPatrol 定期检查 open 挂账超期，记录泄漏指标并告警。

   func (w *Worker) runLeakPatrol(ctx context.Context) {
	   interval := w.cfg.LeakInterval
	  if interval <= 0 {
		  interval = 1 * time.Minute
	  }
	   ticker := time.NewTicker(interval)
	  defer ticker.Stop()
	   for {
		   select {
		  case <-ctx.Done():
			  return
		  case <-ticker.C:
			   now := w.clock()
			   leaked := w.suspense.LeakedEntries(now)
			  if len(leaked) > 0 {
				   metrics.FiatWorker.WithLabelValues("suspense_leak").Add(float64(len(leaked)))
				  // 泄漏总余额报警
				   totals := w.suspense.TotalOpen()
				   for asset, amt := range totals {
					   metrics.FiatSuspenseBalance.WithLabelValues(asset).Set(func() float64 {
						   f, _ := amt.Float64()
						  return f
					  }())
				  }
			  }
		   }
	  }
   }
