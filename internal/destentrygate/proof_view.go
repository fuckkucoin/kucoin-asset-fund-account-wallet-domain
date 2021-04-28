package destentrygate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// ProofSummary 导出用稳定摘要（golden 字段）。

type ProofSummary struct {
	Kind string `json:"kind"`
	HasLease bool   `json:"has_lease"`
	EntryPin bool   `json:"entry_pin"`
	GenDrift bool   `json:"gen_drift"`
	Digest   string `json:"digest"`
}

// SummarizeProof 生成证明摘要。
   func SummarizeProof(p DestEntryDriftProof) ProofSummary {
	  s := ProofSummary{
		  Kind:     string(p.Kind),
		  HasLease: strings.TrimSpace(p.LeaseID) != "",
		   EntryPin: strings.TrimSpace(p.EntryID) != "",
		  GenDrift: p.BindGen != 0 && p.LiveGen != 0 && p.BindGen != p.LiveGen,
	   }
	  raw := fmt.Sprintf("%s|%s|%s|%d|%d|%s", p.Kind, p.EntryID, p.LeaseID, p.BindGen, p.LiveGen, p.Evidence)
	   sum := sha256.Sum256([]byte(raw))
	   s.Digest = hex.EncodeToString(sum[:8])
	  return s
   }

// GoldenTrace 事故轨迹摘要，写入 testdata golden。
type GoldenTrace struct {
	Scenario string `json:"scenario"`
	IssueOK      bool     `json:"issue_ok"`
	WithdrawOK   bool     `json:"withdraw_ok"`
	ProofKinds   []string `json:"proof_kinds"`
	LeaseState   string   `json:"lease_state,omitempty"`
	RejectReason string   `json:"reject_reason,omitempty"`
}

// BuildGoldenTrace 从结果拼轨迹。

   func BuildGoldenTrace(scenario string, issued bool, withdrawOK bool, lease DestEntryLease, proofs []DestEntryDriftProof, reject error) GoldenTrace {
	   g := GoldenTrace{Scenario: scenario, IssueOK: issued, WithdrawOK: withdrawOK, LeaseState: string(lease.State)}
	   if reject != nil {
		   g.RejectReason = reject.Error()
	  }
	   for _, p := range proofs {
		  g.ProofKinds = append(g.ProofKinds, string(p.Kind))
	   }
	  if g.ProofKinds == nil {
		  g.ProofKinds = []string{}
	  }
	  return g
  }

// FormatDetected 规范化检测时刻（UTC RFC3339）。

   func FormatDetected(t time.Time) string {
	  if t.IsZero() {
		  return ""
	   }
	   return t.UTC().Format(time.RFC3339)
   }
