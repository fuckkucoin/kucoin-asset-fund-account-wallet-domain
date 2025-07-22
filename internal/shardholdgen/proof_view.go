package shardholdgen

import "time"

// GoldenTrace testdata 可复现事故轨迹。
type GoldenTrace struct {
	Scenario string `json:"scenario"`
	AdvanceOK bool `json:"advance_ok"`
	ReleaseOK bool `json:"release_ok"`
	Idempotent bool   `json:"idempotent"`
	GenState   string `json:"gen_state"`
	HalfKind   string `json:"half_kind,omitempty"`
	HalfState  string `json:"half_state,omitempty"`
	ProofKinds   []string `json:"proof_kinds"`
	RejectReason string   `json:"reject_reason,omitempty"`
	ActiveHolds  int      `json:"active_holds"`
	AdvanceCount int64    `json:"advance_count"`
	ReleaseCount int64    `json:"release_count"`
	SingleWinner bool `json:"single_winner"`
	DigestPrefix  string `json:"digest_prefix,omitempty"`
	NoDoubleSpend bool   `json:"no_double_spend"`
}

// ProofView 导出视图。

type ProofView struct {
	ProofID string    `json:"proof_id"`
	Kind    ProofKind `json:"kind"`
	PlanID string `json:"plan_id"`
	LegID    string `json:"leg_id"`
	HoldID   string `json:"hold_id"`
	Digest   string `json:"digest"`
	Evidence string `json:"evidence"`
	DetectedAt time.Time `json:"detected_at"`
}

   func ViewOf(p ShardHoldProof) ProofView {
	   return ProofView{
		   ProofID: p.ProofID, Kind: p.Kind, PlanID: p.PlanID, LegID: p.LegID,
		  HoldID: p.HoldID, Digest: p.Digest, Evidence: p.Evidence, DetectedAt: p.DetectedAt,
	  }
  }

  func kindsOf(proofs []ShardHoldProof) []string {
	   out := make([]string, 0, len(proofs))
	   seen := map[string]bool{}
	   for _, p := range proofs {
		   k := string(p.Kind)
		  if seen[k] {
			   continue
		   }
		   seen[k] = true
		  out = append(out, k)
	   }
	  return out
   }
