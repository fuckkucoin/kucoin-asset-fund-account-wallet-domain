package balproofgate

import "sync/atomic"

// GateMetrics 原子计数。

type GateMetrics struct {
	issueTotal atomic.Int64
	issueOk    atomic.Int64
	issueDenied atomic.Int64
	issueIdempotent atomic.Int64
	recheckTotal atomic.Int64
	recheckOk    atomic.Int64
	recheckDenied atomic.Int64
	consumeTotal atomic.Int64
	consumeOk    atomic.Int64
	consumeDenied atomic.Int64
	leaseBound atomic.Int64
	leaseConsumed atomic.Int64
	leaseExpired atomic.Int64
	leaseRevoked atomic.Int64
	genBumped atomic.Int64
	genStale  atomic.Int64
	staleLease atomic.Int64
	driftOpened atomic.Int64
	driftProven atomic.Int64
	driftClosed  atomic.Int64
	driftBlocked atomic.Int64
	driftPatrol atomic.Int64
	revokeTotal atomic.Int64
	revokeOk        atomic.Int64
	revokeHit       atomic.Int64
	revokePropagate atomic.Int64
	expireHit       atomic.Int64
	expireScan atomic.Int64
}

   func newGateMetrics() *GateMetrics { return &GateMetrics{} }

// Snapshot 导出。
  func (m *GateMetrics) Snapshot() map[string]int64 {
	   if m == nil {
		   return map[string]int64{}
	   }
	  return map[string]int64{
		  "issue_total": m.issueTotal.Load(),
		  "issue_ok":    m.issueOk.Load(),
		   "issue_denied":     m.issueDenied.Load(),
		   "issue_idempotent": m.issueIdempotent.Load(),
		   "recheck_total":    m.recheckTotal.Load(),
		  "recheck_ok":       m.recheckOk.Load(),
		  "recheck_denied": m.recheckDenied.Load(),
		  "consume_total":  m.consumeTotal.Load(),
		  "consume_ok":     m.consumeOk.Load(),
		   "consume_denied": m.consumeDenied.Load(),
		  "lease_bound": m.leaseBound.Load(),
		  "lease_consumed": m.leaseConsumed.Load(),
		  "lease_expired": m.leaseExpired.Load(),
		  "lease_revoked": m.leaseRevoked.Load(),
		  "gen_bumped": m.genBumped.Load(),
		  "gen_stale":  m.genStale.Load(),
		  "stale_lease":  m.staleLease.Load(),
		   "drift_opened": m.driftOpened.Load(),
		   "drift_proven": m.driftProven.Load(),
		   "drift_closed":  m.driftClosed.Load(),
		  "drift_blocked": m.driftBlocked.Load(),
		  "drift_patrol": m.driftPatrol.Load(),
		   "revoke_total": m.revokeTotal.Load(),
		  "revoke_ok": m.revokeOk.Load(),
		  "revoke_hit":       m.revokeHit.Load(),
		   "revoke_propagate": m.revokePropagate.Load(),
		   "expire_hit":       m.expireHit.Load(),
		  "expire_scan": m.expireScan.Load(),
	   }
   }
