package haltepochgate

import (
	"sync/atomic"
)

var (
	metricEngage atomic.Int64
	metricClear  atomic.Int64
	metricSlipOpen   atomic.Int64
	metricRevoked    atomic.Int64
	metricScanRevoke atomic.Int64
	metricProofs     atomic.Int64
	metricExport     atomic.Int64
	metricLaneReject atomic.Int64
)

   func ObserveEngage() { metricEngage.Add(1) }

  func ObserveClear() { metricClear.Add(1) }

  func ObserveSlip() { metricSlipOpen.Add(1) }

  func ObserveRevoke(n int) {
	  if n > 0 {
		   metricRevoked.Add(int64(n))
	  }
  }
  func ObserveScan(rev, proofs int) {
	   metricScanRevoke.Add(int64(rev))
	  _ = proofs
  }

  func ObserveProofs(n int) {
	  if n > 0 {
		   metricProofs.Add(int64(n))
	  }
   }
  func ObserveExport(n int) {
	  if n > 0 {
		  metricExport.Add(int64(n))
	  }
   }
   func ObserveLaneReject() { metricLaneReject.Add(1) }

// SnapshotMetrics 进程计数快照。
type SnapshotMetrics struct {
	Engage int64 `json:"engage"`
	Clear    int64 `json:"clear"`
	SlipOpen int64 `json:"slip_open"`
	Revoked    int64 `json:"revoked"`
	ScanRevoke int64 `json:"scan_revoke"`
	Proofs     int64 `json:"proofs"`
	Export     int64 `json:"export"`
	LaneReject int64 `json:"lane_reject"`
}

   func Metrics() SnapshotMetrics {
	   return SnapshotMetrics{
		   Engage: metricEngage.Load(),
		  Clear:  metricClear.Load(),
		   SlipOpen: metricSlipOpen.Load(),
		   Revoked:  metricRevoked.Load(),
		  ScanRevoke: metricScanRevoke.Load(),
		   Proofs:     metricProofs.Load(),
		  Export:     metricExport.Load(),
		  LaneReject: metricLaneReject.Load(),
	  }
  }
