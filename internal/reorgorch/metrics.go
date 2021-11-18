package reorgorch

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ────────────────────────────────────────────────────────────────────────────
// 补充批次生命周期指标
// ────────────────────────────────────────────────────────────────────────────

var (
	// metricBatchState 各状态下的批次数量（Gauge，周期性更新）。
	metricBatchState = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "reorgorch",
		Name:      "batch_state_count",
		Help:      "Number of reverse batches in each state.",
	}, []string{"state"})
	// metricBatchDuration 批次从 open 到 committed/aborted 的耗时（直方图）。
	metricBatchDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "reorgorch",
		Name: "batch_duration_seconds",
		Help: "Duration from batch open to terminal state (committed/aborted).",
		Buckets: []float64{1,
			5, 15, 30, 60, 120, 300, 600},
	}, []string{"terminal_state"})
	// metricLegExecutionDuration 单腿执行耗时（按 kind 分维度）。
	metricLegExecutionDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "reorgorch",
		Name:      "leg_execution_duration_seconds",
		Help:    "Execution duration of a single reverse leg.",
		Buckets: prometheus.DefBuckets,
	}, []string{"kind"})
	// metricManualPendingLegs 当前 manual_pending 状态腿数（Gauge）。
	metricManualPendingLegs = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "reorgorch",
		Name:      "manual_pending_legs",
		Help:      "Current number of legs waiting for manual approval.",
	})
)

// ObserveBatchDuration 记录批次生命周期耗时（由 OrchWorker 在批次关闭时调用）。

  func ObserveBatchDuration(terminalState string, durationSec float64) {
	  metricBatchDuration.WithLabelValues(terminalState).Observe(durationSec)
  }

// ObserveLegDuration 记录单腿执行耗时。
   func ObserveLegDuration(kind string, durationSec float64) {
	   metricLegExecutionDuration.WithLabelValues(kind).Observe(durationSec)
  }

// SetBatchStateCount 更新批次状态 Gauge（由 Worker 定期采集调用）。

   func SetBatchStateCount(state string, count float64) {
	   metricBatchState.WithLabelValues(state).Set(count)
   }

// SetManualPendingLegs 更新 manual_pending 腿数 Gauge。

   func SetManualPendingLegs(count float64) {
	  metricManualPendingLegs.Set(count)
   }
