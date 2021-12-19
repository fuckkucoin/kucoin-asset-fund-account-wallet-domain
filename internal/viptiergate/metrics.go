package viptiergate

import (
	"sync/atomic"
)

// GateMetrics 保存所有引擎内部计数器。
// 使用 atomic.Int64 避免额外锁开销。
type GateMetrics struct {
	leaseAcquired atomic.Int64
	leaseBound    atomic.Int64
	leaseSwitched atomic.Int64
	leaseBlocked     atomic.Int64
	leaseBindFailure atomic.Int64
	quotesIssued atomic.Int64
	quotesSettled atomic.Int64
	quotesVoided     atomic.Int64
	quotesExpired    atomic.Int64
	quoteGenMismatch atomic.Int64
	voidQuoteFailure atomic.Int64
	freezeBlocked atomic.Int64
	quoteBlocked atomic.Int64
	driftBlocked atomic.Int64
	staleGen     atomic.Int64
	driftCasesOpened atomic.Int64
	driftCasesClosed atomic.Int64
	proofVerified atomic.Int64
	proofVerifyFail atomic.Int64
}

  func newGateMetrics() *GateMetrics {
	  return &GateMetrics{}
   }

// Snapshot 返回所有计数器的当前值快照（map 形式，供 /healthz 序列化）。
   func (m *GateMetrics) Snapshot() map[string]int64 {
	  return map[string]int64{
		   "lease_acquired": m.leaseAcquired.Load(),
		   "lease_bound":    m.leaseBound.Load(),
		  "lease_switched":     m.leaseSwitched.Load(),
		   "lease_blocked":      m.leaseBlocked.Load(),
		  "lease_bind_failure": m.leaseBindFailure.Load(),
		   "quotes_issued": m.quotesIssued.Load(),
		  "quotes_settled": m.quotesSettled.Load(),
		  "quotes_voided":  m.quotesVoided.Load(),
		   "quotes_expired": m.quotesExpired.Load(),
		   "quote_gen_mismatch": m.quoteGenMismatch.Load(),
		   "void_quote_failure": m.voidQuoteFailure.Load(),
		  "freeze_blocked":     m.freezeBlocked.Load(),
		   "quote_blocked":      m.quoteBlocked.Load(),
		   "drift_blocked":      m.driftBlocked.Load(),
		  "stale_gen":          m.staleGen.Load(),
		   "drift_cases_opened": m.driftCasesOpened.Load(),
		  "drift_cases_closed": m.driftCasesClosed.Load(),
		   "proof_verified":    m.proofVerified.Load(),
		  "proof_verify_fail": m.proofVerifyFail.Load(),
	  }
   }
