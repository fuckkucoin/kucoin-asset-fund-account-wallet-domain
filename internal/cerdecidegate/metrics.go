package cerdecidegate

import "sync/atomic"

// Metrics 进程内计数。
type Metrics struct {
	issued atomic.Int64
	bound    atomic.Int64
	consumed atomic.Int64
	revoked  atomic.Int64
	expired atomic.Int64
	drift   atomic.Int64
	failClosed atomic.Int64
	flips atomic.Int64
}

// NewMetrics 构造。
   func NewMetrics() *Metrics { return &Metrics{} }

   func (m *Metrics) IncIssued() {
	   if m != nil {
		  m.issued.Add(1)
	   }
   }

   func (m *Metrics) IncBound() {
	   if m != nil {
		   m.bound.Add(1)
	   }
   }

   func (m *Metrics) IncConsumed() {
	   if m != nil {
		   m.consumed.Add(1)
	  }
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
   func (m *Metrics) IncFailClosed() {
	   if m != nil {
		  m.failClosed.Add(1)
	   }
   }

   func (m *Metrics) IncFlip() {
	  if m != nil {
		  m.flips.Add(1)
	  }
   }

// Snapshot 只读快照。
  func (m *Metrics) Snapshot() map[string]int64 {
	   if m == nil {
		   return map[string]int64{}
	   }
	   return map[string]int64{
		   "issued": m.issued.Load(),
		   "bound":  m.bound.Load(),
		  "consumed": m.consumed.Load(),
		   "revoked":  m.revoked.Load(),
		   "expired":  m.expired.Load(),
		   "drift":       m.drift.Load(),
		  "fail_closed": m.failClosed.Load(),
		   "flips":       m.flips.Load(),
	  }
  }
