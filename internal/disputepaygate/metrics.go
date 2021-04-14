package disputepaygate

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	metricPayIssued = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dispute_pay_lease_issued_total",
		Help: "Dispute pay leases issued after resolve.",
	}, []string{"asset"})
	metricClawIssued = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dispute_claw_lease_issued_total",
		Help: "Dispute claw leases issued after reject.",
	}, []string{"asset"})
	metricBlocked = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dispute_pay_lease_blocked_total",
		Help: "Blocked ledger or resolve attempts.",
	}, []string{"scope", "reason"})
	metricAssertOK = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dispute_pay_lease_assert_ok_total",
		Help: "Successful AssertPayLease calls.",
	}, []string{"scope"})
	metricProof = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dispute_pay_proof_total",
		Help: "DisputePayProof rows written.",
	}, []string{"outcome"})
	metricRevoked = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dispute_pay_lease_revoked_total",
		Help: "Revoked pay or claw leases.",
	}, []string{"kind"})
	metricExpired = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dispute_pay_lease_expired_total",
		Help: "Expired leases flipped by worker.",
	}, []string{"kind"})
	metricUnconsumed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dispute_pay_unconsumed_total",
		Help: "Unconsumed resolved leases observed by inspector.",
	}, []string{"kind"})
)

  func recordPayIssued(asset string) {
	  if asset == "" {
		   asset = "unknown"
	  }
	   metricPayIssued.WithLabelValues(asset).Inc()
  }

  func recordClawIssued(asset string) {
	  if asset == "" {
		   asset = "unknown"
	  }
	   metricClawIssued.WithLabelValues(asset).Inc()
  }

   func recordBlocked(scope, reason string) {
	  if scope == "" {
		   scope = "unknown"
	   }
	  if reason == "" {
		  reason = "unknown"
	  }
	   metricBlocked.WithLabelValues(scope, reason).Inc()
  }

   func recordAssertOK(scope string) {
	  if scope == "" {
		   scope = "credit"
	  }
	  metricAssertOK.WithLabelValues(scope).Inc()
  }

   func recordProof(outcome string) {
	  if outcome == "" {
		   outcome = "unknown"
	  }
	  metricProof.WithLabelValues(outcome).Inc()
  }

  func recordRevoked(kind string) {
	   metricRevoked.WithLabelValues(kind).Inc()
  }

   func recordExpired(kind string) {
	   metricExpired.WithLabelValues(kind).Inc()
  }

   func recordUnconsumed(kind string) {
	  metricUnconsumed.WithLabelValues(kind).Inc()
   }
