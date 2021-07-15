package hmacwritelease

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// metricLeaseIssuedTotal VerifyAndLease 成功签发租约次数。
	// 标签 key_id：验签密钥 ID。
	metricLeaseIssuedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hmac_write_lease_issued_total",
		Help: "Total number of HmacWriteLease issued by VerifyAndLease.",
	}, []string{"key_id"})
	// metricWriteBlockedTotal WriteCheck 拒绝次数。
	// 标签 scope：写操作作用域；reason：拒绝原因。
	metricWriteBlockedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hmac_write_lease_write_blocked_total",
		Help: "Total number of fund writes blocked by WriteCheck.",
	}, []string{"scope", "reason"})
	// metricWriteAllowedTotal WriteCheck 放行次数。
	// 标签 scope：写操作作用域。
	metricWriteAllowedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hmac_write_lease_write_allowed_total",
		Help: "Total number of fund writes allowed by WriteCheck.",
	}, []string{"scope"})
	// metricDriftProofTotal HmacWriteDriftProof 落库次数。
	// 标签 reason：漂移原因。
	metricDriftProofTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hmac_write_lease_drift_proof_total",
		Help: "Total number of HmacWriteDriftProof recorded.",
	}, []string{"reason"})
	// metricLeaseRevokedTotal 租约吊销次数。
	// 标签 key_id：验签密钥 ID。
	metricLeaseRevokedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hmac_write_lease_revoked_total",
		Help: "Total number of HmacWriteLease revoked by halt flip or key revocation.",
	}, []string{"key_id"})
	// metricLeaseExpiredTotal 租约过期次数（由 worker 扫描流转）。
	metricLeaseExpiredTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hmac_write_lease_expired_total",
		Help: "Total number of HmacWriteLease expired by expiry worker.",
	}, []string{"key_id"})
	// metricHaltFlipTotal Halt 翻转次数（递增世代）。
	metricHaltFlipTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "hmac_write_lease_halt_flip_total",
		Help: "Total number of halt generation flips (SetHalt true).",
	})
	// metricNonceCleanedTotal nonce 清理条数（由 worker 周期执行）。
	metricNonceCleanedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "hmac_write_lease_nonce_cleaned_total",
		Help: "Total number of expired nonce records cleaned by worker.",
	})
)

   func recordLeaseIssuedMetric(keyID string) {
	   metricLeaseIssuedTotal.WithLabelValues(keyID).Inc()
   }

  func recordWriteBlockedMetric(scope, reason string) {
	   metricWriteBlockedTotal.WithLabelValues(scope, reason).Inc()
  }

   func recordWriteAllowedMetric(scope string) {
	   metricWriteAllowedTotal.WithLabelValues(scope).Inc()
   }

  func recordDriftProofMetric(reason string) {
	  metricDriftProofTotal.WithLabelValues(reason).Inc()
   }

   func recordLeaseRevokedMetric(keyID string) {
	  metricLeaseRevokedTotal.WithLabelValues(keyID).Inc()
  }

   func recordLeaseExpiredMetric(keyID string) {
	   metricLeaseExpiredTotal.WithLabelValues(keyID).Inc()
  }

  func recordHaltFlipMetric(gen int64) {
	  _ = gen
	  metricHaltFlipTotal.Inc()
   }

   func recordNonceCleanedMetric(n int) {
	  metricNonceCleanedTotal.Add(float64(n))
  }
