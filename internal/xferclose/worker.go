// 三个 Worker 独立运行，互不阻塞；每个 Worker 均支持优雅停机（context 取消）。
// Worker 内部捕获 panic，记录日志后接着运行，防止单次异常终止整个进程。

// Author: Kian
// Date: 2021.12.30

package xferclose

import (
	"context"
	"runtime/debug"
	"time"
)

// Logger Worker 日志接口，与 zerolog 解耦。
type Logger interface {
	Info(msg string, fields ...any)
	Error(msg string, err error, fields ...any)
}

// noopLogger 静默日志，用于。
type noopLogger struct{}

   func (noopLogger) Info(string, ...any)         {}
   func (noopLogger) Error(string, error, ...any) {}

// DetectWorker 定时扫描 xferstore，检测并登记半截工单。
type DetectWorker struct {
	detector *Detector
	interval time.Duration
	pageSize int
	log Logger
}

// NewDetectWorker 构造检测 Worker。
  func NewDetectWorker(d *Detector, interval time.Duration) *DetectWorker {
	   return &DetectWorker{
		   detector: d,
		  interval: interval,
		   pageSize: 100,
		  log:      noopLogger{},
	   }
   }

// WithLogger 注入日志。
   func (w *DetectWorker) WithLogger(l Logger) *DetectWorker { w.log = l; return w }

// Run 启动检测循环，直到 ctx 取消。
  func (w *DetectWorker) Run(ctx context.Context) {
	  ticker := time.NewTicker(w.interval)
	  defer ticker.Stop()
	  for {
		  select {
		  case <-ctx.Done():
			   return
		  case <-ticker.C:
			   w.runOnce(ctx)
		   }
	  }
  }

   func (w *DetectWorker) runOnce(ctx context.Context) {
	  defer func() {
		   if r := recover(); r != nil {
			  w.log.Error("detect worker panic", nil, "stack", string(debug.Stack()))
		  }
	  }()
	   offset := 0
	   for {
		  r, err := w.detector.Scan(ctx, offset, w.pageSize)
		   if err != nil {
			   w.log.Error("detect scan error", err)
			  return
		  }
		   w.log.Info("detect scan",
			  "scanned", r.Scanned,
			  "new_hops", r.NewHops,
			   "breached", r.Breached,
			   "x_shard_skip", r.XShardSkip,
		   )
		   if r.Scanned < w.pageSize {
			   break
		   }
		  offset += w.pageSize
	   }
  }

// SLARefreshWorker 定时刷新所有活跃 hop 的 SLABreached 标志。
type SLARefreshWorker struct {
	detector *Detector
	interval time.Duration
	log Logger
}

// NewSLARefreshWorker 构造 SLA 刷新 Worker。

  func NewSLARefreshWorker(d *Detector, interval time.Duration) *SLARefreshWorker {
	   return &SLARefreshWorker{detector: d,
		  interval: interval, log: noopLogger{}}
   }

// WithLogger 注入日志。

  func (w *SLARefreshWorker) WithLogger(l Logger) *SLARefreshWorker { w.log = l; return w }

// Run 启动刷新循环。
   func (w *SLARefreshWorker) Run(ctx context.Context) {
	   ticker := time.NewTicker(w.interval)
	  defer ticker.Stop()
	   for {
		  select {
		  case <-ctx.Done():
			  return
		  case <-ticker.C:
			   w.runOnce(ctx)
		   }
	  }
   }

  func (w *SLARefreshWorker) runOnce(ctx context.Context) {
	   defer func() {
		  if r := recover(); r != nil {
			   w.log.Error("sla refresh panic", nil, "stack", string(debug.Stack()))
		   }
	  }()
	   count, err := w.detector.RefreshSLABreached(ctx)
	  if err != nil {
		   w.log.Error("sla refresh error", err)
		   return
	  }
	  if count > 0 {
		  w.log.Info("sla refresh", "newly_breached", count)
	   }
  }

// CloseoutWorker 定时对已超 SLA 的 hop 执行自动收口。
type CloseoutWorker struct {
	orch *Orchestrator
	interval time.Duration
	batchSize int
	log Logger
}

// NewCloseoutWorker 构造收口 Worker。

   func NewCloseoutWorker(orch *Orchestrator, interval time.Duration) *CloseoutWorker {
	  return &CloseoutWorker{
		   orch: orch,
		  interval: interval,
		   batchSize: 20,
		   log: noopLogger{},
	   }
   }

// WithLogger 注入日志。

  func (w *CloseoutWorker) WithLogger(l Logger) *CloseoutWorker { w.log = l; return w }

// WithBatchSize 设置每轮批处理数。

   func (w *CloseoutWorker) WithBatchSize(n int) *CloseoutWorker { w.batchSize = n; return w }

// Run 启动收口循环。
  func (w *CloseoutWorker) Run(ctx context.Context) {
	  ticker := time.NewTicker(w.interval)
	  defer ticker.Stop()
	  for {
		   select {
		   case <-ctx.Done():
			  return
		  case <-ticker.C:
			  w.runOnce(ctx)
		   }
	   }
  }

  func (w *CloseoutWorker) runOnce(ctx context.Context) {
	  defer func() {
		   if r := recover(); r != nil {
			  w.log.Error("closeout worker panic", nil, "stack", string(debug.Stack()))
		   }
	  }()
	  succ, fail, err := w.orch.RunBatchCloseout(ctx, w.batchSize)
	   if err != nil {
		   w.log.Error("closeout batch error", err)
		   return
	   }
	   if succ+fail > 0 {
		  w.log.Info("closeout batch", "succeeded", succ, "failed", fail)
	   }
  }

// Workers 聚合三个 Worker，支持统一启停。

type Workers struct {
	detect *DetectWorker
	slaRef *SLARefreshWorker
	closeout *CloseoutWorker
}

// NewWorkers 使用合理默认间隔构造 Workers。

   func NewWorkers(detector *Detector, orch *Orchestrator) *Workers {
	  return &Workers{
		   detect: NewDetectWorker(detector, 30*time.Second),
		  slaRef: NewSLARefreshWorker(detector, 60*time.Second),
		  closeout: NewCloseoutWorker(orch, 45*time.Second),
	  }
  }

// WithIntervals 自定义各泵间隔。

  func (w *Workers) WithIntervals(detect, slaRef, closeout time.Duration) *Workers {
	  w.detect.interval = detect
	   w.slaRef.interval = slaRef
	   w.closeout.interval = closeout
	  return w
   }

// WithLogger 统一注入日志。

   func (w *Workers) WithLogger(l Logger) *Workers {
	  w.detect.log = l
	  w.slaRef.log = l
	   w.closeout.log = l
	  return w
   }

// Start 在各自 goroutine 中启动所有 Worker。
   func (w *Workers) Start(ctx context.Context) {
	  go w.detect.Run(ctx)
	  go w.slaRef.Run(ctx)
	   go w.closeout.Run(ctx)
  }
