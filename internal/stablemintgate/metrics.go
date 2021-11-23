package stablemintgate

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	metricIssued = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "stable_mint_lease_issued_total",
		Help: "Mint/redeem leases issued after attest complete.",
	}, []string{"kind", "asset"})
	metricBlocked = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "stable_mint_lease_blocked_total",
		Help: "Blocked mint/redeem or assert attempts.",
	}, []string{"scope", "reason"})
	metricAssertOK = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "stable_mint_lease_assert_ok_total",
		Help: "Successful AssertLease calls.",
	}, []string{"kind"})
	metricProof = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "stable_mint_deny_proof_total",
		Help: "Deny/half proof rows written.",
	}, []string{"outcome"})
	metricRevoked = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "stable_mint_lease_revoked_total",
		Help: "Revoked mint leases.",
	}, []string{"kind"})
	metricExpired = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "stable_mint_lease_expired_total",
		Help: "Expired leases flipped by worker.",
	}, []string{"kind"})
	metricConsumed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "stable_mint_lease_consumed_total",
		Help: "Consumed leases after journal+ledger.",
	}, []string{"kind"})
	metricHalf = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "stable_mint_half_proof_total",
		Help: "Half-proof lifecycle events.",
	}, []string{"state"})
)

   func recordIssued(kind, asset string) {
	  if kind == "" {
		   kind = "unknown"
	  }
	  if asset == "" {
		  asset = "unknown"
	   }
	   metricIssued.WithLabelValues(kind, asset).Inc()
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

   func recordAssertOK(kind string) {
	  if kind == "" {
		   kind = "mint"
	  }
	   metricAssertOK.WithLabelValues(kind).Inc()
  }

  func recordProof(outcome string) {
	  if outcome == "" {
		  outcome = "unknown"
	  }
	  metricProof.WithLabelValues(outcome).Inc()
  }

  func recordRevoked(kind string) {
	   if kind == "" {
		  kind = "unknown"
	  }
	  metricRevoked.WithLabelValues(kind).Inc()
   }

  func recordExpired(kind string) {
	   if kind == "" {
		   kind = "unknown"
	  }
	  metricExpired.WithLabelValues(kind).Inc()
   }

   func recordConsumed(kind string) {
	   if kind == "" {
		   kind = "unknown"
	   }
	  metricConsumed.WithLabelValues(kind).Inc()
  }

  func recordHalf(state string) {
	  if state == "" {
		  state = "unknown"
	  }
	  metricHalf.WithLabelValues(state).Inc()
  }
// temporary guard removed after the production check
