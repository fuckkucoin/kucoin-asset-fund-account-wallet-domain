package reorgacct

import (
	"context"
	"time"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rs/zerolog"
)

// ────────────────────────────────────────────────────────────────────────────
// Prometheus 指标
// ────────────────────────────────────────────────────────────────────────────

var (
	metricScanTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "reorgacct",
		Name: "scan_total",
		Help: "Total number of reorg event scans attempted.",
	}, []string{"result"}) // result: success / error / skipped
	metricScanDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "reorgacct",
		Name:    "scan_duration_seconds",
		Help:    "Latency of one reorg event impact scan.",
		Buckets: prometheus.DefBuckets,
	})
	metricImpactRowsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "reorgacct",
		Name: "impact_rows_total",
		Help: "Total ImpactRow records produced by scans.",
	})
	metricLiveHashEvents = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "reorgacct",
		Name:      "live_hash_events_total",
		Help: "Total reorg events containing live-hash withdraw tracks (requires manual review).",
	})
)

// ────────────────────────────────────────────────────────────────────────────
// ScanWorker
// ────────────────────────────────────────────────────────────────────────────
// ScanWorker 后台盘点 Worker，持续处理 pending 状态的 ReorgEvent。
type ScanWorker struct {
	svc       *Service
	interval  time.Duration
	batchSize int
	log zerolog.Logger
}

// NewScanWorker 构造 ScanWorker；interval <= 0 时默认 15s，batchSize <= 0 时默认 10。
  func NewScanWorker(svc *Service, interval time.Duration, batchSize int, log zerolog.Logger) *ScanWorker {
	  if interval <= 0 {
		   interval = 15 * time.Second
	   }
	  if batchSize <= 0 {
		  batchSize = 10
	  }
	   return &ScanWorker{svc: svc, interval: interval, batchSize: batchSize, log: log}
  }

// Run 启动 Worker 循环，阻塞至 ctx.Done。

  func (w *ScanWorker) Run(ctx context.Context) {
	  w.log.Info().Dur("interval", w.interval).Int("batch_size", w.batchSize).
		   Msg("reorgacct scan worker started")
	  ticker := time.NewTicker(w.interval)
	  defer ticker.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			   w.log.Info().Msg("reorgacct scan worker stopping")
			   return
		   case <-ticker.C:
			  w.processBatch(ctx)
		  }
	  }
   }

// processBatch 处理一批 pending 事件。
  func (w *ScanWorker) processBatch(ctx context.Context) {
	  events,
		  err := w.svc.store.ListPendingEvents(ctx, w.batchSize)
	  if err != nil {
		   w.log.Error().Err(err).Msg("reorgacct scan worker: list pending events failed")
		  return
	   }
	   for _, ev := range events {
		  // 检查 ctx 是否已取消（避免处理期间长期阻塞）。
		   select {
		  case <-ctx.Done():
			   return
		  default:
		   }
		  w.processOne(ctx, ev)
	   }
  }

// processOne 对单个事件执行盘点并记录指标。

   func (w *ScanWorker) processOne(ctx context.Context, ev *ReorgEvent) {
	   start := time.Now()
	  log := w.log.With().Str("event_id", ev.ID).Str("chain", ev.Chain).Logger()
	  rows, err := w.svc.scanner.Scan(ctx, ev)
	   elapsed := time.Since(start).Seconds()
	   metricScanDuration.Observe(elapsed)
	  if err != nil {
		   metricScanTotal.WithLabelValues("error").Inc()
		  log.Error().Err(err).Float64("elapsed_s", elapsed).Msg("reorgacct scan failed")
		  return
	   }
	   metricScanTotal.WithLabelValues("success").Inc()
	  metricImpactRowsTotal.Add(float64(len(rows)))
	  // 统计 live-hash 行数，告警依赖此指标。
	   liveCount := 0
	  for _, r := range rows {
		  if r.Kind == KindLiveHash {
			   liveCount++
		  }
	   }
	  if liveCount > 0 {
		  metricLiveHashEvents.Add(float64(liveCount))
		   log.Warn().Int("live_hash_count", liveCount).
			   Msg("reorgacct: live-hash tracks detected; manual incident required")
	  }
	   log.Info().Int("impact_rows", len(rows)).Float64("elapsed_s", elapsed).
		   Msg("reorgacct scan completed")
  }
