package feebumpgate

import "sync/atomic"

// Metrics 进程内计数。
type Metrics struct {
	signed atomic.Int64
	broadcast  atomic.Int64
	retired    atomic.Int64
	orphans    atomic.Int64
	failClosed atomic.Int64
	asserted   atomic.Int64
	finals atomic.Int64
}

// NewMetrics 构造。
   func NewMetrics() *Metrics { return &Metrics{} }

  func (m *Metrics) IncSigned() {
	  if m != nil {
		  m.signed.Add(1)
	   }
  }
   func (m *Metrics) IncBroadcast() {
	   if m != nil {
		   m.broadcast.Add(1)
	   }
   }
  func (m *Metrics) IncRetire() {
	   if m != nil {
		  m.retired.Add(1)
	  }
   }
   func (m *Metrics) IncOrphan() {
	   if m != nil {
		  m.orphans.Add(1)
	   }
   }

   func (m *Metrics) IncFailClosed() {
	   if m != nil {
		  m.failClosed.Add(1)
	   }
   }
   func (m *Metrics) IncAssert() {
	  if m != nil {
		   m.asserted.Add(1)
	   }
   }

   func (m *Metrics) IncFinal() {
	   if m != nil {
		   m.finals.Add(1)
	   }
   }

// Snapshot 只读快照。
   func (m *Metrics) Snapshot() map[string]int64 {
	   if m == nil {
		   return map[string]int64{}
	  }
	   return map[string]int64{
		  "signed":    m.signed.Load(),
		   "broadcast": m.broadcast.Load(),
		  "retired":     m.retired.Load(),
		  "orphans":     m.orphans.Load(),
		  "fail_closed": m.failClosed.Load(),
		   "asserted": m.asserted.Load(),
		  "finals": m.finals.Load(),
	  }
   }
