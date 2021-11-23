package stucksettle

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// MetricLeaseAcquired 成功申请回收租约次数。
	MetricLeaseAcquired = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "stucksettle_lease_acquired_total",
		Help: "Number of recover leases successfully acquired",
	}, []string{"transfer_id_prefix"})
	// MetricDupBlocked 重复回收被封堵次数（触发 DupRecoverCase）。
	MetricDupBlocked = promauto.NewCounter(prometheus.CounterOpts{
		Name: "stucksettle_dup_blocked_total",
		Help: "Number of duplicate recover attempts blocked",
	})
	// MetricAdvanceLocked Advance 互斥锁拦截 Recover 次数。
	MetricAdvanceLocked = promauto.NewCounter(prometheus.CounterOpts{
		Name: "stucksettle_advance_locked_total",
		Help: "Number of recover attempts rejected by advance mutex",
	})
	// MetricHoldBypass Hold 态旁路拒绝次数。
	MetricHoldBypass = promauto.NewCounter(prometheus.CounterOpts{
		Name: "stucksettle_hold_bypass_total",
		Help: "Number of bypass recover attempts rejected due to hold state",
	})
	// MetricProofIssued 效果证明固化次数。
	MetricProofIssued = promauto.NewCounter(prometheus.CounterOpts{
		Name: "stucksettle_proof_issued_total",
		Help: "Number of effect proofs issued and stored",
	})
	// MetricProofMissing 因证明缺失阻止 Recovered 标记次数。
	MetricProofMissing = promauto.NewCounter(prometheus.CounterOpts{
		Name: "stucksettle_proof_missing_total",
		Help: "Number of times recovered mark was blocked due to missing proof",
	})
	// MetricLeaseExpired 租约超时失效次数。
	MetricLeaseExpired = promauto.NewCounter(prometheus.CounterOpts{
		Name: "stucksettle_lease_expired_total",
		Help: "Number of recover leases expired without completion",
	})
	// MetricDupCaseNew 双结案件新增数。
	MetricDupCaseNew = promauto.NewCounter(prometheus.CounterOpts{
		Name: "stucksettle_dup_case_new_total",
		Help: "Number of new dup recover cases detected",
	})
	// MetricDupCaseClosed 双结案件关闭数（双人确认）。
	MetricDupCaseClosed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "stucksettle_dup_case_closed_total",
		Help: "Number of dup recover cases closed by dual confirmation",
	})
	// MetricWorkerScanDuration worker 扫描耗时分布。
	MetricWorkerScanDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name: "stucksettle_worker_scan_duration_seconds",
		Help: "Duration of stucksettle worker scan cycle",
		Buckets: prometheus.DefBuckets,
	})
	// MetricGenBump 世代号递增次数。
	MetricGenBump = promauto.NewCounter(prometheus.CounterOpts{
		Name: "stucksettle_generation_bump_total",
		Help: "Number of times a stuck generation was bumped for a new recover attempt",
	})
)
