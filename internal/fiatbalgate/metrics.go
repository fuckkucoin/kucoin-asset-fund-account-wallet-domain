package fiatbalgate

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// MetricCasesOpened 开案总计数（label: asset, imbalance_kind）。
	MetricCasesOpened = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fiatbalgate_cases_opened_total",
		Help: "Total number of fiat imbalance cases opened.",
	}, []string{"asset", "imbalance_kind"})
	// MetricCasesBlocking 当前处于 blocking 状态的案件数（label: asset）。
	MetricCasesBlocking = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "fiatbalgate_cases_blocking",
		Help: "Number of fiat imbalance cases currently in blocking state.",
	}, []string{"asset"})
	// MetricCasesCleared 已清障案件总计数（label: asset）。
	MetricCasesCleared = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fiatbalgate_cases_cleared_total",
		Help: "Total number of fiat imbalance cases cleared.",
	}, []string{"asset"})
	// MetricPostsBlocked Post 因不平衡案被挡写的总计数（label: asset）。
	MetricPostsBlocked = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fiatbalgate_posts_blocked_total",
		Help: "Total number of fiatjnl Post calls blocked due to imbalance case.",
	}, []string{"asset"})
	// MetricScanRuns 扫描器运行次数（label: asset, result: new_case/already_open/clean）。
	MetricScanRuns = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fiatbalgate_scan_runs_total",
		Help: "Total scan runs by the imbalance scanner.",
	}, []string{"asset", "result"})
	// MetricProofsExported 证明导出总计数（label: asset）。
	MetricProofsExported = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fiatbalgate_proofs_exported_total",
		Help: "Total number of imbalance proofs exported.",
	}, []string{"asset"})
	// MetricGenSynced 已同步的世代快照总数。
	MetricGenSynced = promauto.NewCounter(prometheus.CounterOpts{
		Name: "fiatbalgate_gen_synced_total",
		Help: "Total number of journal generation snapshots synced to fiat_jnl_gens.",
	})
	// MetricScanLatency 扫描延迟直方图（秒）。
	MetricScanLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "fiatbalgate_scan_latency_seconds",
		Help:    "Latency of imbalance scan runs.",
		Buckets: prometheus.DefBuckets,
	})
)
