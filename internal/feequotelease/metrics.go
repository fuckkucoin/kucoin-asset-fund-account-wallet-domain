package feequotelease

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// metricLeaseIssued 签发租约计数。
	metricLeaseIssued = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "fee",
		Subsystem: "quote_lease",
		Name:      "issued_total",
		Help: "Total fee quote leases issued.",
	}, []string{"kind", "asset"})
	// metricLeaseApplied 成功消费租约计数。
	metricLeaseApplied = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "fee",
		Subsystem: "quote_lease",
		Name:      "applied_total",
		Help: "Total fee quote leases successfully applied (consumed).",
	}, []string{"kind", "asset"})
	// metricLeaseDrift 漂移拒绝计数。
	metricLeaseDrift = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "fee",
		Subsystem: "quote_lease",
		Name:      "drift_total",
		Help: "Total fee quote apply attempts rejected due to drift.",
	}, []string{"kind", "drift_kind"})
	// metricLeaseExpired 过期租约计数。
	metricLeaseExpired = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "fee",
		Subsystem: "quote_lease",
		Name:      "expired_total",
		Help:      "Total fee quote leases expired by TTL.",
	}, []string{"kind"})
	// metricLeaseRevoked 吊销租约计数。
	metricLeaseRevoked = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "fee",
		Subsystem: "quote_lease",
		Name:      "revoked_total",
		Help:      "Total fee quote leases revoked by schedule change.",
	}, []string{"kind", "reason"})
	// metricSchGen 当前 sch_gen 值（Gauge）。
	metricSchGen = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "fee",
		Subsystem: "quote_lease",
		Name:      "sch_generation_current",
		Help: "Current schedule generation (sch_gen) per kind/asset.",
	}, []string{"kind", "asset"})
)

// recordIssued 记录签发事件。

   func recordIssued(kind, asset string) {
	   metricLeaseIssued.WithLabelValues(kind, asset).Inc()
  }

// recordApplied 记录消费事件。

   func recordApplied(kind, asset string) {
	   metricLeaseApplied.WithLabelValues(kind, asset).Inc()
  }

// recordDrift 记录漂移拒绝事件。

   func recordDrift(kind string, driftKind DriftProofKind) {
	  metricLeaseDrift.WithLabelValues(kind, string(driftKind)).Inc()
   }

// recordExpired 记录过期事件。
   func recordExpired(kind string) {
	  metricLeaseExpired.WithLabelValues(kind).Inc()
   }

// recordRevoked 记录吊销事件。
  func recordRevoked(kind, reason string) {
	   metricLeaseRevoked.WithLabelValues(kind, reason).Inc()
  }

// recordSchGen 更新 sch_gen gauge。
  func recordSchGen(kind, asset string, gen int64) {
	  metricSchGen.WithLabelValues(kind, asset).Set(float64(gen))
   }
