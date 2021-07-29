package invleasegate

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// PrecheckLeaseTotal 预检租约状态迁移计数（issued/committed/expired/blocked）。
	PrecheckLeaseTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_invleasegate_precheck_lease_total",
		Help: "InvPrecheckLease state transitions by target state",
	}, []string{"state"})
	// DriftProofTotal 漂移证明落地计数（按类型与落地结果）。
	DriftProofTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_invleasegate_drift_proof_total",
		Help: "InvGateDriftProof saves by kind and result (ok/fail)",
	}, []string{"kind", "result"})
	// AdviceFlipTotal 建议翻转检测计数（按资产）。
	AdviceFlipTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_invleasegate_advice_flip_total",
		Help: "Advice fingerprint flip events detected by asset",
	}, []string{"asset"})
	// CheckerDownTotal checker 不可用事件计数（按拒绝路径）。
	CheckerDownTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_invleasegate_checker_down_total",
		Help: "DriftChecker unavailable events by path (precheck_bind/commit_recheck/flip_guard)",
	}, []string{"path"})
	// CommitRecheckTotal 提交复验计数（按结果）。
	CommitRecheckTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_invleasegate_commit_recheck_total",
		Help: "CommitRecheck calls by result (ok/expired/blocked/flip/checker_down/no_lease)",
	}, []string{"result"})
	// WorkerExpiryTotal Worker 过期扫描计数（每批）。
	WorkerExpiryTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_invleasegate_worker_expiry_total",
		Help: "Lease expiry worker batch results by result (expired/error)",
	}, []string{"result"})
	// WorkerFlipTotal Worker 翻转巡检计数（每批）。
	WorkerFlipTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_invleasegate_worker_flip_total",
		Help: "Advice flip guard worker batch results by result (flipped/ok/error)",
	}, []string{"result"})
	// ActiveLeaseGauge 当前 issued+bound 状态租约数量（近似，仅供监控）。
	ActiveLeaseGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_invleasegate_active_lease_count",
		Help: "Approximate count of issued+bound precheck leases (worker-updated)",
	})
)
