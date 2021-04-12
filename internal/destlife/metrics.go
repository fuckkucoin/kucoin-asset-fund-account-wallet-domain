package destlife

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// StateTransitions 状态转换计数（label: from_state, to_state）。
	StateTransitions = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_destlife_state_transitions_total",
		Help: "Lifecycle state transitions; alert on revoking/revoked spikes",
	}, []string{"from_state", "to_state"})
	// CooldownChecks 冷却检查结果（label: result=pass|block|error）。
	CooldownChecks = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_destlife_cooldown_checks_total",
		Help: "Cooldown gate checks; block means entry still cooling",
	}, []string{"result"})
	// InflightAcquire 在途引用获取计数（label: result=ok|duplicate|error）。
	InflightAcquire = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_destlife_inflight_acquire_total",
		Help: "Inflight ref acquisitions; duplicate means idempotent reuse",
	}, []string{"result"})
	// InflightRelease 在途引用释放计数（label: auto_finalized=true|false）。
	InflightRelease = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_destlife_inflight_release_total",
		Help: "Inflight ref releases; auto_finalized=true means revoking→revoked triggered",
	}, []string{"auto_finalized"})
	// BypassDetected 冷却绕过检测计数（label: reason）。
	BypassDetected = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_destlife_bypass_detected_total",
		Help: "Bypass attempts detected; alert on any non-zero clock_drift",
	}, []string{"reason"})
	// WorkerActivations Worker 冷却激活处理数。
	WorkerActivations = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_destlife_worker_activations_total",
		Help: "Entries activated from cooling to active by the worker",
	})
	// WorkerRevocations Worker 撤销完成数。
	WorkerRevocations = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_destlife_worker_revocations_total",
		Help: "Entries finalized from revoking to revoked by the worker",
	})
	// WorkerBypassPatrol Worker 绕过巡检处理数。
	WorkerBypassPatrol = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_destlife_worker_bypass_patrol_total",
		Help: "Unblocked bypass proofs patrolled and blocked by worker",
	})
	// GenMismatches 世代不一致计数（label: source=strict|api）。
	GenMismatches = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_destlife_gen_mismatches_total",
		Help: "Generation mismatches between lifecycle and book; alert on any",
	}, []string{"source"})
	// ActiveInflightGauge 当前活跃在途引用数（瞬时）。
	ActiveInflightGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_destlife_active_inflight_refs",
		Help: "Current count of active (unreleased) inflight refs",
	})
	// CoolingEntries 当前处于 cooling 状态的条目数（Worker 扫描后更新）。
	CoolingEntries = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_destlife_cooling_entries",
		Help: "Current count of entries in cooling state",
	})
	// RevokingEntries 当前处于 revoking 状态的条目数（Worker 扫描后更新）。
	RevokingEntries = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_destlife_revoking_entries",
		Help: "Current count of entries in revoking state awaiting inflight drain",
	})
	// AuditAppends 生命周期审计写入数。
	AuditAppends = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_destlife_audit_appends_total",
		Help: "Lifecycle audit rows written",
	})
)
