package scoreleasegate

import "sync/atomic"

type GateMetrics struct {
	issued atomic.Int64
	bound  atomic.Int64
	consumed atomic.Int64
	denied atomic.Int64
	expired   atomic.Int64
	revoked   atomic.Int64
	proofs    atomic.Int64
	haltBumps atomic.Int64
	packBumps atomic.Int64
	modeBumps  atomic.Int64
	windowBump atomic.Int64
}

   func newGateMetrics() *GateMetrics { return &GateMetrics{} }

  func (m *GateMetrics) Snapshot() GateStats {
	  if m == nil {
		  return GateStats{}
	   }
	  return GateStats{
		   Issued: m.issued.Load(),
		   Bound: m.bound.Load(),
		   Consumed: m.consumed.Load(),
		  Denied:   m.denied.Load(),
		   Expired:  m.expired.Load(),
		   Revoked:  m.revoked.Load(),
		   Proofs:    m.proofs.Load(),
		  HaltBumps: m.haltBumps.Load(),
		  PackBumps: m.packBumps.Load(),
		  ModeBumps: m.modeBumps.Load(),
		  WindowBump: m.windowBump.Load(),
	   }
   }
