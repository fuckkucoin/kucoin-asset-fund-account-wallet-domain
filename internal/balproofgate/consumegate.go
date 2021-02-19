package balproofgate

import "time"

// GateVerdict 闸判定结果。
type GateVerdict struct {
	Allow bool `json:"allow"`
	Reason string `json:"reason"`
	DriftKind DriftKind `json:"drift_kind,omitempty"`
	Err       error     `json:"-"`
}

// EvaluateConsumeGate 纯判定，供与 HTTP 预检复用。

   func EvaluateConsumeGate(lease ProofLease, now time.Time, revoked bool, currentGen int64, live LiveView, blockingDrift bool) GateVerdict {
	  if revoked {
		  return GateVerdict{Reason: "revoked", Err: ErrRevoked}
	  }
	  if lease.Status == LeaseExpired {
		   return GateVerdict{Reason: "expired", Err: ErrLeaseExpired}
	   }
	  if !lease.ExpiresAt.IsZero() && !now.Before(lease.ExpiresAt) {
		  return GateVerdict{Reason: "ttl", Err: ErrLeaseExpired}
	  }
	  if lease.Status == LeaseRevoked {
		  return GateVerdict{Reason: "revoked-status", Err: ErrRevoked}
	   }
	   if lease.Status == LeaseConsumed {
		  return GateVerdict{Reason: "terminal", Err: ErrLeaseTerminal}
	  }
	   if lease.Status != LeaseBound && lease.Status != LeaseIssued {
		   return GateVerdict{Reason: "not-bound", Err: ErrLeaseNotBound}
	  }
	   if lease.Generation != currentGen {
		  return GateVerdict{Reason: "stale-gen", Err: ErrStaleGen}
	  }
	   if blockingDrift {
		   return GateVerdict{Reason: "drift-blocked", Err: ErrDriftBlocked}
	   }
	   kind := ClassifyDrift(lease.Snapshot, live)
	   if kind != "" {
		  return GateVerdict{Reason: "live-drift", DriftKind: kind, Err: ErrLiveDrift}
	  }
	  return GateVerdict{Allow: true, Reason: "ok"}
  }

// MustDenyConsume 断言夹具：签发后余额变化必须 Deny。

  func MustDenyConsume(issued LiveSnapshotRoot, live LiveView) bool {
	   return ClassifyDrift(issued, live) != ""
  }
