package occseggate

import "sync/atomic"

var (
	metricIssued uint64
	metricCommit uint64
	metricAbort uint64
	metricGhost uint64
	metricRace  uint64
	metricStale uint64
	metricMissing uint64
	metricRevoke uint64
	metricLive uint64
	metricAck  uint64
	metricInspect uint64
	metricRevokePump uint64
	metricExport uint64
)

  func incIssued() { atomic.AddUint64(&metricIssued, 1) }
  func incCommit() {
	   atomic.AddUint64(&metricCommit,
		  1)
   }

  func incAbort() { atomic.AddUint64(&metricAbort, 1) }
  func incGhost() { atomic.AddUint64(&metricGhost, 1) }

  func incRace() { atomic.AddUint64(&metricRace, 1) }

  func incStale()   { atomic.AddUint64(&metricStale, 1) }
   func incMissing() { atomic.AddUint64(&metricMissing, 1) }
   func incLive()    { atomic.AddUint64(&metricLive, 1) }

  func incAck() { atomic.AddUint64(&metricAck, 1) }

   func incRevoke(n int) {
	  if n > 0 {
		  atomic.AddUint64(&metricRevoke, uint64(n))
	   }
  }

   func addInspect(n int) {
	   if n > 0 {
		  atomic.AddUint64(&metricInspect, uint64(n))
	   }
  }

  func addRevokePump(n int) {
	  if n > 0 {
		  atomic.AddUint64(&metricRevokePump, uint64(n))
	   }
   }

  func addExport(n int) {
	  if n > 0 {
		   atomic.AddUint64(&metricExport,
			   uint64(n))
	  }
   }

// MetricsSnapshot 只读计数。
type MetricsSnapshot struct {
	Issued uint64 `json:"issued"`
	Commit uint64 `json:"commit"`
	Abort  uint64 `json:"abort"`
	Ghost  uint64 `json:"ghost"`
	Race   uint64 `json:"race"`
	Stale  uint64 `json:"stale"`
	Missing uint64 `json:"missing"`
	Revoke uint64 `json:"revoke"`
	Live   uint64 `json:"live_unfreeze"`
	Ack        uint64 `json:"ack"`
	Inspect    uint64 `json:"inspect"`
	RevokePump uint64 `json:"revoke_pump"`
	Export uint64 `json:"export"`
}

// SnapshotMetrics 采集。

   func SnapshotMetrics() MetricsSnapshot {
	   return MetricsSnapshot{
		   Issued: atomic.LoadUint64(&metricIssued),
		  Commit: atomic.LoadUint64(&metricCommit),
		   Abort: atomic.LoadUint64(&metricAbort),
		  Ghost: atomic.LoadUint64(&metricGhost),
		   Race:  atomic.LoadUint64(&metricRace),
		   Stale: atomic.LoadUint64(&metricStale),
		   Missing: atomic.LoadUint64(&metricMissing),
		   Revoke: atomic.LoadUint64(&metricRevoke),
		  Live:   atomic.LoadUint64(&metricLive),
		   Ack: atomic.LoadUint64(&metricAck),
		   Inspect: atomic.LoadUint64(&metricInspect),
		   RevokePump: atomic.LoadUint64(&metricRevokePump),
		   Export: atomic.LoadUint64(&metricExport),
	  }
   }
