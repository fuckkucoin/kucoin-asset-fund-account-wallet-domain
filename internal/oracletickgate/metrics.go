package oracletickgate

import "sync/atomic"

// Metrics 进程内计数。
type Metrics struct {
	Issued atomic.Int64
	Bound    atomic.Int64
	Consumed atomic.Int64
	Revoked  atomic.Int64
	Expired        atomic.Int64
	Proofs         atomic.Int64
	FundWritesHeld atomic.Int64
	ShadowBlocked atomic.Int64
}

  func (m *Metrics) Snapshot() map[string]int64 {
	   if m == nil {
		   return map[string]int64{}
	   }
	   return map[string]int64{
		   "issued": m.Issued.Load(),
		   "bound": m.Bound.Load(),
		  "consumed": m.Consumed.Load(),
		   "revoked":  m.Revoked.Load(),
		   "expired":          m.Expired.Load(),
		  "proofs":           m.Proofs.Load(),
		   "fund_writes_held": m.FundWritesHeld.Load(),
		  "shadow_blocked": m.ShadowBlocked.Load(),
	   }
  }

  func ObserveLease(m *Metrics, l *TickLease) {
	  if m == nil || l == nil {
		   return
	   }
	   switch l.State {
	  case LeaseIssued:
		  m.Issued.Add(1)
	  case LeaseBound:
		  m.Bound.Add(1)
	  case LeaseConsumed:
		   m.Consumed.Add(1)
	   case LeaseRevoked:
		   m.Revoked.Add(1)
	  case LeaseExpired:
		  m.Expired.Add(1)
	  }
  }

   func ObserveProof(m *Metrics, p TickDriftProof) {
	  if m == nil {
		  return
	  }
	   m.Proofs.Add(1)
	   if p.FundWriteHeld {
		   m.FundWritesHeld.Add(1)
	  }
	  if p.Kind == ProofShadowImpersonate {
		  m.ShadowBlocked.Add(1)
	   }
   }
