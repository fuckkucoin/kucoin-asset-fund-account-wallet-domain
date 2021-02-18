package availcas

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	metricLeasesIssued = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_avail_leases_issued_total",
		Help: "可用快照租约颁发总数",
	})
	metricLeasesExpired = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_avail_leases_expired_total",
		Help: "自然过期回收的租约数",
	})
	metricLeasesRevoked = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_avail_leases_revoked_total",
		Help: "因外部余额变更被撤销的租约数",
	})
	metricLeaseExpireErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_avail_lease_expire_errors_total",
		Help: "租约过期回收失败次数（worker 告警）",
	})
	metricIntentsAccepted = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_debit_intents_accepted_total",
		Help: "CAS 屏障接受的意图数",
	})
	metricIntentsRejected = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_debit_intents_rejected_total",
		Help: "CAS 屏障拒绝的意图数，按拒绝码分组",
	}, []string{"reject_code"})
	metricRaceProofsCreated = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_race_proofs_created_total",
		Help: "竞态证明创建总数；速率上升表示同窗并发加剧",
	})
	metricOverdrawAttempts = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_overdraw_attempts_total",
		Help: "超提企图次数（Amount > ExpectedAvail）；速率 > 0.1/min 触发 P1",
	})
	metricBarrierDecisionMs = promauto.NewHistogram(prometheus.HistogramOpts{
		Name: "aa_barrier_decision_ms",
		Help:    "屏障裁决延迟（毫秒）；p99 目标 < 20ms",
		Buckets: []float64{1, 2, 5, 10, 20, 50, 100, 200},
	})
	metricWorkerSweepDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name: "aa_avail_worker_sweep_seconds",
		Help: "Worker 单轮回收耗时（秒）",
		Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 5},
	})
	metricWorkerConflictScanDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name: "aa_avail_worker_conflict_scan_seconds",
		Help:    "Worker 冲突扫描轮耗时（秒）",
		Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 5},
	})
	metricProofIntegrityErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_proof_integrity_errors_total",
		Help: "证明完整性巡检发现的错误数",
	})
)

// recordRejectMetric 按 RejectCode 递增 intentsRejected 计数器。
  func recordRejectMetric(code RejectCode) {
	   metricIntentsRejected.WithLabelValues(string(code)).Inc()
  }
