package sagaresidue

import (
	"fmt"
	"time"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ── Worker 指标 ──────────────────────────────────────────────────────────────

var (
	ResidueWorkerScanRuns = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_sagaresidue_worker_scan_runs_total",
		Help: "ResidueScanner 完整执行轮次（成功）。",
	})
	ResidueWorkerScanErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_sagaresidue_worker_scan_errors_total",
		Help: "ResidueScanner 执行失败轮次（需告警）。",
	})
	ResidueWorkerResidueFound = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_sagaresidue_worker_residue_found_total",
		Help: "ResidueScanner 累计发现残留科目数（> 0 需运营确认）。",
	})
	ResidueWorkerExecuteRuns = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_sagaresidue_worker_execute_runs_total",
		Help: "CloseoutExecutor 完整执行轮次（成功）。",
	})
	ResidueWorkerExecuteErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_sagaresidue_worker_execute_errors_total",
		Help: "CloseoutExecutor 执行失败轮次（需告警）。",
	})
	ResidueWorkerMutexReclaimed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_sagaresidue_worker_mutex_reclaimed_total",
		Help: "MutexReclaimer 累计回收过期世代锁数。",
	})
	ResidueWorkerAgingRuns = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_sagaresidue_worker_aging_runs_total",
		Help: "AgingUpgrader 完整执行轮次（成功）。",
	})
	ResidueWorkerAgingUpgraded = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_sagaresidue_worker_aging_upgraded_total",
		Help: "AgingUpgrader 累计升级残留 Run 数（Severity 提升）。",
	})
)

// ── API 指标 ─────────────────────────────────────────────────────────────────

var (
	ResidueAPIRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_sagaresidue_api_requests_total",
		Help: "sagaresidue HTTP API 请求数（按路径和状态码分组）。",
	}, []string{"path", "code"})
	ResidueAPILatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "aa_sagaresidue_api_latency_seconds",
		Help:    "sagaresidue HTTP API 请求延迟分布（秒）。",
		Buckets: prometheus.DefBuckets,
	}, []string{"path"})
)

// ── 证明指标 ─────────────────────────────────────────────────────────────────

var (
	ResidueProofIssued = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_sagaresidue_proof_issued_total",
		Help: "CloseProof 累计签发数。",
	})
	ResidueProofVerifyTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_sagaresidue_proof_verify_total",
		Help: "CloseProof 累计校验次数。",
	})
	ResidueProofVerifyInvalid = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_sagaresidue_proof_verify_invalid_total",
		Help: "CloseProof 校验失败次数（> 0 需 P1 告警）。",
	})
)

// ── 收口腿指标 ────────────────────────────────────────────────────────────────

var (
	ResidueLegApplied = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_sagaresidue_leg_applied_total",
		Help: "收口腿成功执行数（按 kind 分组）。",
	}, []string{"kind"})
	ResidueLegFailed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_sagaresidue_leg_failed_total",
		Help: "收口腿执行失败数（按 kind 分组）。",
	}, []string{"kind"})
	ResidueLegSkipped = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_sagaresidue_leg_skipped_total",
		Help: "收口腿被跳过数（按 kind 分组）。",
	}, []string{"kind"})
)

// ── Run 状态仪表盘 ────────────────────────────────────────────────────────────

var (
	ResidueRunStateGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_sagaresidue_run_state",
		Help: "当前各状态 ResidueRun 的数量快照（供告警查询）。",
	}, []string{"state"})
)

// ── 辅助函数 ─────────────────────────────────────────────────────────────────
// recordAPIMetrics 记录 HTTP API 请求指标。
   func recordAPIMetrics(path string, code int, start time.Time) {
	   ResidueAPIRequests.WithLabelValues(path, fmt.Sprintf("%d", code)).Inc()
	   ResidueAPILatency.WithLabelValues(path).Observe(time.Since(start).Seconds())
   }
