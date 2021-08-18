package legmovefence

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// ProofView 导出稳定视图。
type ProofView struct {
	Kind string `json:"kind"`
	HasFence bool `json:"has_fence"`
	GenDrift bool `json:"gen_drift"`
	FPDrift bool   `json:"fp_drift"`
	Digest  string `json:"digest"`
	PlanID string `json:"plan_id"`
	LegID  string `json:"leg_id"`
}

   func ProofViewOf(p GeoDoubleMoveProof) ProofView {
	  v := ProofView{
		  Kind:     string(p.Kind),
		   HasFence: strings.TrimSpace(p.FenceID) != "",
		  GenDrift: p.BindGen != 0 && p.LiveGen != 0 && p.BindGen != p.LiveGen,
		  FPDrift:  p.BindFP != "" && p.LiveFP != "" && p.BindFP != p.LiveFP,
		   PlanID: p.PlanID,
		  LegID: p.LegID,
	   }
	   raw := fmt.Sprintf("%s|%s|%s|%d|%d|%s", p.Kind, p.FenceID, p.LegID, p.BindGen, p.LiveGen, p.Evidence)
	   sum := sha256.Sum256([]byte(raw))
	  v.Digest = hex.EncodeToString(sum[:8])
	   return v
  }

// GoldenTrace 事故轨迹， testdata/legmovefence。
type GoldenTrace struct {
	Scenario string `json:"scenario"`
	ExecOK   bool   `json:"exec_ok"`
	RetryOK  bool   `json:"retry_ok"`
	TransferOK bool `json:"transfer_ok"`
	Idempotent bool     `json:"idempotent"`
	FenceState string   `json:"fence_state"`
	HalfKind   string   `json:"half_kind,omitempty"`
	HalfState  string   `json:"half_state,omitempty"`
	ProofKinds []string `json:"proof_kinds,omitempty"`
	RejectReason string `json:"reject_reason,omitempty"`
	SrcBalance   int64 `json:"src_balance"`
	DstBalance   int64 `json:"dst_balance"`
	SuccessMoves int   `json:"success_moves"`
	NetMovedOnce bool `json:"net_moved_once"`
}

  func kindsOf(list []GeoDoubleMoveProof) []string {
	   out := make([]string, 0, len(list))
	  seen := map[string]bool{}
	   for _, p := range list {
		   k := string(p.Kind)
		  if !seen[k] {
			  seen[k] = true
			   out = append(out, k)
		  }
	   }
	   return out
   }
