package liqgengate

import "sync/atomic"

var (
	metricFreezeIssued uint64
	metricFreezeFrozen uint64
	metricHalfProof uint64
	metricRaceProof uint64
	metricReleaseOK uint64
	metricCloseOK   uint64
	metricLeaseExpired uint64
	metricHalfInspect uint64
	metricRaceDiscover uint64
	metricExpiryScan   uint64
)

   func incFreezeIssued() { atomic.AddUint64(&metricFreezeIssued, 1) }

  func incFreezeFrozen() { atomic.AddUint64(&metricFreezeFrozen, 1) }
  func incHalfProof()    { atomic.AddUint64(&metricHalfProof, 1) }
  func incRaceProof() {
	   atomic.AddUint64(&metricRaceProof,
		  1)
   }
  func incReleaseOK() { atomic.AddUint64(&metricReleaseOK, 1) }

  func incCloseOK() {
	  atomic.AddUint64(&metricCloseOK,
		  1)
  }
   func incLeaseExpired() { atomic.AddUint64(&metricLeaseExpired, 1) }

   func addHalfInspect(n int) {
	  if n > 0 {
		  atomic.AddUint64(&metricHalfInspect, uint64(n))
	  }
  }

  func addRaceDiscover(n int) {
	  if n > 0 {
		   atomic.AddUint64(&metricRaceDiscover, uint64(n))
	  }
   }
   func addExpiryScan(n int) {
	   if n > 0 {
		   atomic.AddUint64(&metricExpiryScan, uint64(n))
	  }
   }

// MetricsSnapshot 只读计数。
type MetricsSnapshot struct {
	FreezeIssued uint64 `json:"freeze_issued"`
	FreezeFrozen uint64 `json:"freeze_frozen"`
	HalfProof    uint64 `json:"half_proof"`
	RaceProof uint64 `json:"race_proof"`
	ReleaseOK uint64 `json:"release_ok"`
	CloseOK      uint64 `json:"close_ok"`
	LeaseExpired uint64 `json:"lease_expired"`
	HalfInspect  uint64 `json:"half_inspect"`
	RaceDiscover uint64 `json:"race_discover"`
	ExpiryScan   uint64 `json:"expiry_scan"`
}

// SnapshotMetrics 采集。

   func SnapshotMetrics() MetricsSnapshot {
	   return MetricsSnapshot{
		   FreezeIssued: atomic.LoadUint64(&metricFreezeIssued),
		  FreezeFrozen: atomic.LoadUint64(&metricFreezeFrozen),
		   HalfProof:    atomic.LoadUint64(&metricHalfProof),
		  RaceProof: atomic.LoadUint64(&metricRaceProof),
		  ReleaseOK: atomic.LoadUint64(&metricReleaseOK),
		   CloseOK:      atomic.LoadUint64(&metricCloseOK),
		  LeaseExpired: atomic.LoadUint64(&metricLeaseExpired),
		  HalfInspect:  atomic.LoadUint64(&metricHalfInspect),
		   RaceDiscover: atomic.LoadUint64(&metricRaceDiscover),
		   ExpiryScan: atomic.LoadUint64(&metricExpiryScan),
	  }
   }
