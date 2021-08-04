package holdopsgate

import "sync/atomic"

type GateMetrics struct {
	issued   atomic.Int64
	bound    atomic.Int64
	consumed atomic.Int64
	denied atomic.Int64
	proofs  atomic.Int64
	expired atomic.Int64
	revoked     atomic.Int64
	dualRelease atomic.Int64
	gapBlocked atomic.Int64
	ackSafe     atomic.Int64
	genBumps    atomic.Int64
	workerTicks atomic.Int64
}

   func newGateMetrics() *GateMetrics { return &GateMetrics{} }

   func (m *GateMetrics) Snapshot(openLeases, openProofs int, gen int64) Stats {
	   if m == nil {
		   return Stats{}
	   }
	  return Stats{
		  Issued:   m.issued.Load(),
		   Bound:    m.bound.Load(),
		  Consumed: m.consumed.Load(),
		  Denied: m.denied.Load(),
		  Proofs: m.proofs.Load(),
		  Expired: m.expired.Load(),
		  Revoked:     m.revoked.Load(),
		   DualRelease: m.dualRelease.Load(),
		   GapBlocked:  m.gapBlocked.Load(),
		   AckSafe:     m.ackSafe.Load(),
		  GenBumps: m.genBumps.Load(),
		   WorkerTicks: m.workerTicks.Load(),
		   OpenLeases:  openLeases,
		  OpenProofs:  openProofs,
		  HoldOpsGen:  gen,
	  }
  }
