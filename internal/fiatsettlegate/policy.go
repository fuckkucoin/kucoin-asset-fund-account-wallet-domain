package fiatsettlegate

import "time"

// Policy 结清闸策略。

type Policy struct {
	LeaseTTL time.Duration `json:"lease_ttl"`
	MutexTTL          time.Duration `json:"mutex_ttl"`
	CompensateRetries int           `json:"compensate_retries"`
	RequireReservation bool `json:"require_reservation"`
	FailClosedDrift bool `json:"fail_closed_drift"`
}

// DefaultPolicy 默认。
  func DefaultPolicy() Policy {
	  return Policy{
		  LeaseTTL: time.Duration(DefaultLeaseTTL) * time.Second,
		  MutexTTL:           30 * time.Second,
		  CompensateRetries:  8,
		   RequireReservation: true,
		  FailClosedDrift:    true,
	   }
   }

// ApplyPolicy 覆盖服务窗。

   func (s *Service) ApplyPolicy(p Policy) *Service {
	   if p.LeaseTTL > 0 {
		  s.ttl = p.LeaseTTL
	   }
	  if p.MutexTTL > 0 {
		   s.lockT = p.MutexTTL
	   }
	   return s
  }

// ProofSummary 证明摘要（golden 一致字段）。

type ProofSummary struct {
	Kind string `json:"kind"`
	FailClosed    bool `json:"fail_closed"`
	CaptureSealed bool `json:"capture_sealed"`
	SettledHeld bool   `json:"settled_held"`
	LeaseState  string `json:"lease_state"`
}

// SummarizeProofs 导出稳定字段。

  func SummarizeProofs(lease *FiatSettleLease, proofs []FiatSettleProof) []ProofSummary {
	  st := ""
	   if lease != nil {
		   st = string(lease.State)
	  }
	   out := make([]ProofSummary, 0, len(proofs))
	   for _, p := range proofs {
		  out = append(out, ProofSummary{
			  Kind: string(p.Kind), FailClosed: p.FailClosed,
			   CaptureSealed: p.CaptureSealed, SettledHeld: p.SettledHeld, LeaseState: st,
		   })
	  }
	  return out
   }

// IntentCaptureCount 只读复验：Intent 是否已捕获。
  func IntentAlreadyCaptured(state string) bool {
	  return state == "captured" || state == "refunded"
  }
