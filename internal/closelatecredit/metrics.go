package closelatecredit

import "sync/atomic"

var (
	metricClose uint64
	metricReopen uint64
	metricAdmit  uint64
	metricReject uint64
	metricDispute uint64
	metricMissing  uint64
	metricMismatch uint64
	metricStale    uint64
	metricProof    uint64
	metricHold uint64
	metricResolve uint64
	metricScan     uint64
	metricHoldPump uint64
	metricExport   uint64
	metricForceDeny uint64
)

   func incClose()  { atomic.AddUint64(&metricClose, 1) }
  func incReopen() { atomic.AddUint64(&metricReopen, 1) }

  func incAdmit() {
	   atomic.AddUint64(&metricAdmit,
		   1)
  }
  func incReject()  { atomic.AddUint64(&metricReject, 1) }
   func incDispute() { atomic.AddUint64(&metricDispute, 1) }

  func incMissing()  { atomic.AddUint64(&metricMissing, 1) }
  func incMismatch() { atomic.AddUint64(&metricMismatch, 1) }
  func incStale()    { atomic.AddUint64(&metricStale, 1) }
   func incProof()    { atomic.AddUint64(&metricProof, 1) }
  func incHold()     { atomic.AddUint64(&metricHold, 1) }

  func incResolve() {
	   atomic.AddUint64(&metricResolve,
		  1)
   }

  func incForceDeny() { atomic.AddUint64(&metricForceDeny, 1) }

  func addScan(n int) {
	   if n > 0 {
		   atomic.AddUint64(&metricScan, uint64(n))
	   }
   }

   func addHoldPump(n int) {
	  if n > 0 {
		   atomic.AddUint64(&metricHoldPump, uint64(n))
	   }
  }

   func addExport(n int) {
	  if n > 0 {
		  atomic.AddUint64(&metricExport, uint64(n))
	   }
  }

// MetricsSnapshot 只读计数。
type MetricsSnapshot struct {
	Close uint64 `json:"close"`
	Reopen uint64 `json:"reopen"`
	Admit uint64 `json:"admit"`
	Reject   uint64 `json:"reject"`
	Dispute  uint64 `json:"dispute"`
	Missing  uint64 `json:"missing_epoch"`
	Mismatch uint64 `json:"epoch_mismatch"`
	Stale uint64 `json:"stale_after_reopen"`
	Proof   uint64 `json:"proof"`
	Hold    uint64 `json:"hold"`
	Resolve uint64 `json:"resolve"`
	Scan uint64 `json:"late_scan"`
	HoldPump uint64 `json:"hold_pump"`
	Export    uint64 `json:"export"`
	ForceDeny uint64 `json:"force_live_denied"`
}

// SnapshotMetrics 采集。
   func SnapshotMetrics() MetricsSnapshot {
	   return MetricsSnapshot{
		   Close:  atomic.LoadUint64(&metricClose),
		   Reopen: atomic.LoadUint64(&metricReopen),
		   Admit:  atomic.LoadUint64(&metricAdmit),
		   Reject: atomic.LoadUint64(&metricReject),
		   Dispute: atomic.LoadUint64(&metricDispute),
		  Missing:  atomic.LoadUint64(&metricMissing),
		   Mismatch: atomic.LoadUint64(&metricMismatch),
		   Stale:    atomic.LoadUint64(&metricStale),
		   Proof:    atomic.LoadUint64(&metricProof),
		  Hold:     atomic.LoadUint64(&metricHold),
		  Resolve: atomic.LoadUint64(&metricResolve),
		   Scan: atomic.LoadUint64(&metricScan),
		  HoldPump: atomic.LoadUint64(&metricHoldPump),
		   Export:   atomic.LoadUint64(&metricExport),
		   ForceDeny: atomic.LoadUint64(&metricForceDeny),
	   }
   }
