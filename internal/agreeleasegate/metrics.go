package agreeleasegate

import "sync/atomic"

var (
	metricLeaseIssued uint64
	metricLeaseRevoked  uint64
	metricLeaseExpired  uint64
	metricLeaseConsumed uint64
	metricPlanBound   uint64
	metricExecBound   uint64
	metricProofOpened uint64
	metricHalfPlan    uint64
)

  func incLeaseIssued()  { atomic.AddUint64(&metricLeaseIssued, 1) }
  func incLeaseRevoked() { atomic.AddUint64(&metricLeaseRevoked, 1) }

   func incLeaseExpired() { atomic.AddUint64(&metricLeaseExpired, 1) }

   func incLeaseConsumed() { atomic.AddUint64(&metricLeaseConsumed, 1) }

  func incPlanBound() { atomic.AddUint64(&metricPlanBound, 1) }

   func incExecBound() {
	   atomic.AddUint64(&metricExecBound,
		  1)
   }

   func incProofOpened() { atomic.AddUint64(&metricProofOpened, 1) }

  func incHalfPlan() {
	  atomic.AddUint64(&metricHalfPlan,
		  1)
  }

// MetricsSnapshot 计数快照。
type MetricsSnapshot struct {
	LeaseIssued  uint64 `json:"alg_lease_issued_total"`
	LeaseRevoked uint64 `json:"alg_lease_revoked_total"`
	LeaseExpired uint64 `json:"alg_lease_expired_total"`
	LeaseConsumed uint64 `json:"alg_lease_consumed_total"`
	PlanBound uint64 `json:"alg_plan_bound_total"`
	ExecBound uint64 `json:"alg_exec_bound_total"`
	ProofOpened uint64 `json:"alg_proof_opened_total"`
	HalfPlan uint64 `json:"alg_half_plan_total"`
}

// SnapshotMetrics 读取。
  func SnapshotMetrics() MetricsSnapshot {
	  return MetricsSnapshot{
		  LeaseIssued: atomic.LoadUint64(&metricLeaseIssued),
		   LeaseRevoked: atomic.LoadUint64(&metricLeaseRevoked),
		   LeaseExpired:  atomic.LoadUint64(&metricLeaseExpired),
		   LeaseConsumed: atomic.LoadUint64(&metricLeaseConsumed),
		  PlanBound:     atomic.LoadUint64(&metricPlanBound),
		   ExecBound:   atomic.LoadUint64(&metricExecBound),
		  ProofOpened: atomic.LoadUint64(&metricProofOpened),
		  HalfPlan:    atomic.LoadUint64(&metricHalfPlan),
	   }
   }
