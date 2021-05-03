package dustquotegate

import "sync/atomic"

// Metrics 进程内计数。

type Metrics struct {
	quoted atomic.Int64
	bound  atomic.Int64
	settled atomic.Int64
	revoked atomic.Int64
	expired      atomic.Int64
	drift        atomic.Int64
	conserveFail atomic.Int64
	scanHalt atomic.Int64
	haltedScans atomic.Int64
}

// NewMetrics 构造。
  func NewMetrics() *Metrics { return &Metrics{} }

   func (m *Metrics) IncQuoted() {
	   if m != nil {
		   m.quoted.Add(1)
	  }
  }

  func (m *Metrics) IncBound() {
	  if m != nil {
		   m.bound.Add(1)
  }

   func (m *Metrics) IncRevoked() {
	  if m != nil {
		   m.revoked.Add(1)
	   }
  }

  func (m *Metrics) AddRevoked(n int) {
	  if m != nil && n > 0 {
		   m.revoked.Add(int64(n))
	   }
  }
  func (m *Metrics) AddExpired(n int) {
	  if m != nil && n > 0 {
		   m.expired.Add(int64(n))
	   }
  }
  func (m *Metrics) IncDrift() {
	  if m != nil {
		  m.drift.Add(1)
	   }
  }
   func (m *Metrics) IncConserveFail() {
	   if m != nil {
		  m.conserveFail.Add(1)
	   }
  }
  func (m *Metrics) IncScanHalt() {
	  if m != nil {
		  m.scanHalt.Add(1)
	  }
   }
   func (m *Metrics) SetHaltedScans(n int) {
	   if m != nil {
		  m.haltedScans.Store(int64(n))
	  }
   }

// Snapshot 只读快照。
   func (m *Metrics) Snapshot() map[string]int64 {
	   if m == nil {
		  return map[string]int64{}
	  }
	   return map[string]int64{
		   "quoted":  m.quoted.Load(),
		  "bound":   m.bound.Load(),
		  "settled": m.settled.Load(),
		   "revoked": m.revoked.Load(),
		   "expired": m.expired.Load(),
		   "drift":   m.drift.Load(),
		   "conserve_fail": m.conserveFail.Load(),
		   "scan_halt":     m.scanHalt.Load(),
		   "halted_scans":  m.haltedScans.Load(),
	  }
   }
