package feepathgate

import "sync/atomic"

// GateMetrics 持有所有事件的原子计数器。
// Prometheus 层通过 RegisterPrometheus 可选挂载；不注册时退化为进程内计数。

type GateMetrics struct {
	leaseAcquired atomic.Int64
	leaseBound    atomic.Int64
	leaseSettled  atomic.Int64
	leaseAborted  atomic.Int64
	pathConflict atomic.Int64
	staleGen       atomic.Int64
	replayConflict atomic.Int64
	idempotentReplay atomic.Int64
	dualHitDetected  atomic.Int64
	hedgeImbalance   atomic.Int64
	proofVerified    atomic.Int64
	proofVerifyFail atomic.Int64
}

   func newGateMetrics() *GateMetrics { return &GateMetrics{} }

// Snapshot 返回各计数器快照，用于 /healthz 与断言。

   func (m *GateMetrics) Snapshot() map[string]int64 {
	   return map[string]int64{
		  "lease_acquired": m.leaseAcquired.Load(),
		  "lease_bound":    m.leaseBound.Load(),
		  "lease_settled":  m.leaseSettled.Load(),
		  "lease_aborted":  m.leaseAborted.Load(),
		   "path_conflict":  m.pathConflict.Load(),
		  "stale_gen":       m.staleGen.Load(),
		   "replay_conflict": m.replayConflict.Load(),
		  "idempotent_replay": m.idempotentReplay.Load(),
		  "dual_hit_detected": m.dualHitDetected.Load(),
		  "hedge_imbalance":   m.hedgeImbalance.Load(),
		   "proof_verified":    m.proofVerified.Load(),
		  "proof_verify_fail": m.proofVerifyFail.Load(),
	   }
  }
