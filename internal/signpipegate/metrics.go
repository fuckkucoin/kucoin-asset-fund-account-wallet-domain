package signpipegate

import "sync/atomic"

type GateMetrics struct {
	armTotal atomic.Int64
	armOK atomic.Int64
	armDenied atomic.Int64
	enterOK   atomic.Int64
	sealTotal atomic.Int64
	sealOK atomic.Int64
	sealDenied    atomic.Int64
	faultTotal    atomic.Int64
	faultObserved atomic.Int64
}

  func newGateMetrics() *GateMetrics {
	   return &GateMetrics{}
   }

type GateStats struct {
	ArmTotal int64 `json:"arm_total"`
	ArmOK    int64 `json:"arm_ok"`
	ArmDenied int64 `json:"arm_denied"`
	EnterOK   int64 `json:"enter_ok"`
	SealTotal int64 `json:"seal_total"`
	SealOK int64 `json:"seal_ok"`
	SealDenied int64 `json:"seal_denied"`
	FaultTotal int64 `json:"fault_total"`
	FaultObserved int64 `json:"fault_observed"`
}

  func (m *GateMetrics) Snapshot() GateStats {
	  if m == nil {
		   return GateStats{}
	  }
	   return GateStats{
		   ArmTotal: m.armTotal.Load(),
		   ArmOK:      m.armOK.Load(),
		  ArmDenied:  m.armDenied.Load(),
		   EnterOK:    m.enterOK.Load(),
		  SealTotal:  m.sealTotal.Load(),
		   SealOK:     m.sealOK.Load(),
		   SealDenied: m.sealDenied.Load(),
		   FaultTotal: m.faultTotal.Load(),
		   FaultObserved: m.faultObserved.Load(),
	  }
   }
