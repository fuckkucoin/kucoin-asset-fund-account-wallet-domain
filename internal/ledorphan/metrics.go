package ledorphan

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// ── 孤儿扫描 ─────────────────────────────────────────────────────────────
	OrphanScanRuns = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_ledorphan_orphan_scan_runs_total",
		Help: "OrphanScanner 完整执行轮次数（成功）。",
	})
	OrphanScanErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_ledorphan_orphan_scan_errors_total",
		Help: "OrphanScanner 执行失败轮次数（需告警）。",
	})
	OrphanScanBatches = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_ledorphan_orphan_scan_batches_total",
		Help: "OrphanScanner 累计扫描批次数。",
	})
	OrphanScanFound = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_ledorphan_orphan_scan_found_total",
		Help: "OrphanScanner 累计发现孤儿分录数（> 0 需运营处置）。",
	})
	// ── 世代锁回收 ────────────────────────────────────────────────────────────
	GenReclaimRuns = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_ledorphan_gen_reclaim_runs_total",
		Help: "GenerationReclaimer 执行轮次数。",
	})
	GenReclaimErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_ledorphan_gen_reclaim_errors_total",
		Help: "GenerationReclaimer 执行失败轮次数。",
	})
	GenExpiredTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_ledorphan_gen_expired_total",
		Help: "累计回收的过期世代锁数量。",
	})
	// ── 封堵动作 ──────────────────────────────────────────────────────────────
	SealAppliedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_ledorphan_seal_applied_total",
		Help: "封堵/冲正/忽略动作成功执行数，按 kind 分类。",
	}, []string{"kind"})
	SealFailedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_ledorphan_seal_failed_total",
		Help: "封堵动作失败数，按 kind 分类（需告警）。",
	}, []string{"kind"})
	SealOrphanCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_ledorphan_seal_orphan_count_total",
		Help: "累计处置孤儿分录数（含 sealed/reversed/ignored）。",
	})
	// ── 完整性校验 ────────────────────────────────────────────────────────────
	IntegrityCheckRuns = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_ledorphan_integrity_check_runs_total",
		Help: "IntegrityChecker 执行轮次数。",
	})
	IntegrityCheckErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_ledorphan_integrity_check_errors_total",
		Help: "IntegrityChecker 执行异常数。",
	})
	IntegrityCheckInvalid = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_ledorphan_integrity_check_invalid_total",
		Help: "完整性证明哈希校验失败数（P1 告警：存储异常或软件 bug）。",
	})
	// ── 孤儿老化 ──────────────────────────────────────────────────────────────
	AgingRuns = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_ledorphan_aging_runs_total",
		Help: "AgingUpgrader 执行轮次数。",
	})
	AgingErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_ledorphan_aging_errors_total",
		Help: "AgingUpgrader 执行异常数。",
	})
	AgingOrphans = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_ledorphan_aging_orphans_total",
		Help: "累计超 SLA 未处置孤儿告警次数（持续增长需运营干预）。",
	})
	// ── API 请求 ──────────────────────────────────────────────────────────────
	OrphanAPIRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_ledorphan_api_requests_total",
		Help: "孤儿封堵引擎 HTTP API 请求数，按 path 和 status 分类。",
	}, []string{"path", "status"})
	OrphanAPIDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "aa_ledorphan_api_duration_seconds",
		Help:    "孤儿封堵引擎 HTTP API 响应时延。",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2},
	}, []string{"path"})
)
