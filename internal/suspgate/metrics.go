package suspgate

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// metricSuspGenIssuedTotal 每次 SuspendWithGen 成功签发的世代计数。
	// 标签 asset：资产符号。
	metricSuspGenIssuedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "suspgate_susp_gen_issued_total",
		Help: "Total number of SuspenseGeneration issued by SuspendWithGen.",
	}, []string{"asset"})
	// metricClearSuccessTotal 四眼清障成功次数。
	// 标签 asset：资产符号。
	metricClearSuccessTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "suspgate_clear_success_total",
		Help: "Total number of successful ClearWithGen operations.",
	}, []string{"asset"})
	// metricWithdrawBlockedTotal 出金挡写次数。
	// 标签 asset：资产符号；reason：挡写原因。
	metricWithdrawBlockedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "suspgate_withdraw_blocked_total",
		Help: "Total number of fund withdrawals blocked by WithdrawAssert.",
	}, []string{"asset", "reason"})
	// metricDriftProofTotal 漂移证明落库次数。
	// 标签 outcome：阻断结果枚举值。
	metricDriftProofTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "suspgate_drift_proof_total",
		Help: "Total number of SuspenseDriftProof recorded.",
	}, []string{"outcome"})
	// metricLeaseIssuedTotal 清障租约签发次数。
	metricLeaseIssuedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "suspgate_lease_issued_total",
		Help: "Total number of ClearanceLease issued after successful clear.",
	})
	// metricLeaseExpiredTotal 清障租约过期次数（由 LeaseExpiryWorker 记录）。
	metricLeaseExpiredTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "suspgate_lease_expired_total",
		Help: "Total number of ClearanceLease expired by worker.",
	})
	// metricDoubleClearTotal 双次清障同一 gen 次数（安全告警级指标）。
	metricDoubleClearTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "suspgate_double_clear_total",
		Help: "Total number of duplicate FourEyesClear attempts on same gen.",
	}, []string{"asset"})
	// metricConsistencyRepairTotal 世代一致性巡检修复次数。
	metricConsistencyRepairTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "suspgate_consistency_repair_total",
		Help: "Total number of GenCounter repairs by consistency worker.",
	})
	// metricAuditMissedBlockTotal 审计泵发现漏拦次数（count=0 but gen active）。
	metricAuditMissedBlockTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "suspgate_audit_missed_block_total",
		Help: "Total number of potential missed-block events detected by audit pump.",
	})
	// metricActiveGenGauge 当前活跃暂停世代数（近似值，基于 worker 扫描）。
	metricActiveGenGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "suspgate_active_gen_count",
		Help: "Approximate number of active SuspenseGeneration records.",
	})
)

   func recordSuspGenIssued(asset string) {
	   metricSuspGenIssuedTotal.WithLabelValues(asset).Inc()
  }

  func recordClearSuccess(asset string) {
	   metricClearSuccessTotal.WithLabelValues(asset).Inc()
  }

   func recordWithdrawBlocked(asset, reason string) {
	  metricWithdrawBlockedTotal.WithLabelValues(asset,
		  reason).Inc()
   }

   func recordDriftProof(outcome ProofOutcome) {
	   metricDriftProofTotal.WithLabelValues(string(outcome)).Inc()
  }

  func recordLeaseIssued() {
	   metricLeaseIssuedTotal.Inc()
   }

   func recordLeaseExpired(n int) {
	  metricLeaseExpiredTotal.Add(float64(n))
  }

   func recordDoubleClear(asset string) {
	   metricDoubleClearTotal.WithLabelValues(asset).Inc()
   }

   func recordConsistencyRepair() {
	   metricConsistencyRepairTotal.Inc()
  }

  func recordAuditMissedBlock() {
	  metricAuditMissedBlockTotal.Inc()
   }

  func setActiveGenGauge(n float64) {
	   metricActiveGenGauge.Set(n)
  }
