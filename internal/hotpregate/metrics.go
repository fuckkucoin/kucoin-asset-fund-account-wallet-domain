package hotpregate

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// hpgLeaseIssuedTotal 预检租约签发总次数（label: asset）。
	hpgLeaseIssuedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hpg_lease_issued_total",
		Help: "热库存预检租约签发总次数",
	}, []string{"asset"})
	// hpgLeaseReusedTotal 租约幂等复用总次数（label: asset）。
	hpgLeaseReusedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hpg_lease_reused_total",
		Help: "热库存预检租约幂等复用总次数",
	}, []string{"asset"})
	// hpgLeaseConsumedTotal 租约成功消费总次数（label: asset）。
	hpgLeaseConsumedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hpg_lease_consumed_total",
		Help: "热库存预检租约成功消费总次数",
	}, []string{"asset"})
	// hpgLeaseExpiredTotal 租约过期总次数（label: asset）。
	hpgLeaseExpiredTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hpg_lease_expired_total",
		Help: "热库存预检租约过期总次数",
	}, []string{"asset"})
	// hpgDriftDetectedTotal 漂移检测触发总次数（label: asset, severity）。
	hpgDriftDetectedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hpg_drift_detected_total",
		Help: "热库存漂移证明签发总次数",
	}, []string{"asset", "severity"})
	// hpgRecheckTotal 提交复核调用总次数（label: asset, result: accepted|rejected）。
	hpgRecheckTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hpg_recheck_total",
		Help: "热库存提交复核调用总次数",
	}, []string{"asset", "result"})
	// hpgPatrolRoundTotal Worker 巡检轮次总数。
	hpgPatrolRoundTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "hpg_patrol_round_total",
		Help: "热库存漂移巡检 Worker 巡检轮次总数",
	})
	// hpgExpiryRoundTotal Worker 过期扫描轮次总数。
	hpgExpiryRoundTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "hpg_expiry_round_total",
		Help: "热库存预检租约过期 Worker 扫描轮次总数",
	})
	// hpgActiveLeasesGauge 活跃租约数量（Gauge；label: asset）。
	hpgActiveLeasesGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hpg_active_leases",
		Help: "热库存当前活跃（issued/bound）预检租约数量",
	}, []string{"asset"})
)

// ObserveLeaseIssued 记录租约签发。

   func ObserveLeaseIssued(asset string, reused bool) {
	  if reused {
		  hpgLeaseReusedTotal.WithLabelValues(asset).Inc()
	   } else {
		  hpgLeaseIssuedTotal.WithLabelValues(asset).Inc()
	   }
  }

// ObserveLeaseConsumed 记录租约消费。
   func ObserveLeaseConsumed(asset string) {
	  hpgLeaseConsumedTotal.WithLabelValues(asset).Inc()
  }

// ObserveLeaseExpired 记录租约过期。

   func ObserveLeaseExpired(asset string) {
	   hpgLeaseExpiredTotal.WithLabelValues(asset).Inc()
  }

// ObserveDriftDetected 记录漂移检测。
  func ObserveDriftDetected(asset string, severity DriftSeverity) {
	   hpgDriftDetectedTotal.WithLabelValues(asset, string(severity)).Inc()
  }

// ObserveRecheck 记录复核结果。

  func ObserveRecheck(asset string, accepted bool) {
	  result := "rejected"
	  if accepted {
		  result = "accepted"
	  }
	  hpgRecheckTotal.WithLabelValues(asset, result).Inc()
   }

// ObservePatrolRound 记录巡检轮次。
   func ObservePatrolRound() { hpgPatrolRoundTotal.Inc() }

// ObserveExpiryRound 记录过期扫描轮次。

  func ObserveExpiryRound() { hpgExpiryRoundTotal.Inc() }

// SetActiveLeasesGauge 更新活跃租约 Gauge。

   func SetActiveLeasesGauge(asset string, count float64) {
	   hpgActiveLeasesGauge.WithLabelValues(asset).Set(count)
   }
