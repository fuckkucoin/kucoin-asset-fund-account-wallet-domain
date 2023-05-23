package oracletickgate

import (
	"sort"
	"strings"
)

// ProofSummary golden 一致字段。
type ProofSummary struct {
	Kind string `json:"kind"`
	FailClosed bool `json:"fail_closed"`
	FundWriteHeld bool   `json:"fund_write_held"`
	LeaseState    string `json:"lease_state,omitempty"`
	Pair          string `json:"pair,omitempty"`
}

// SummarizeProofs 稳定排序。
   func SummarizeProofs(proofs []TickDriftProof, lease *TickLease) []ProofSummary {
	  out := make([]ProofSummary,
		   0, len(proofs))
	   st := ""
	   if lease != nil {
		  st = string(lease.State)
	  }
	  for _, p := range proofs {
		  out = append(out, ProofSummary{
			   Kind: string(p.Kind), FailClosed: p.FailClosed, FundWriteHeld: p.FundWriteHeld,
			   LeaseState: st, Pair: p.Pair,
		   })
	   }
	   sort.Slice(out, func(i, j int) bool {
		  if out[i].Kind == out[j].Kind {
			  return out[i].Pair < out[j].Pair
		   }
		   return out[i].Kind < out[j].Kind
	   })
	   return out
  }

// FirstKind 首个证明类型。
   func FirstKind(proofs []TickDriftProof) string {
	  if len(proofs) == 0 {
		   return ""
	   }
	  return string(proofs[0].Kind)
  }

// HasKind 是否含类型。
   func HasKind(proofs []TickDriftProof, kind ProofKind) bool {
	   for _, p := range proofs {
		   if p.Kind == kind {
			   return true
		  }
	  }
	  return false
  }

// CompactDetail 截断。
   func CompactDetail(s string, n int) string {
	  s = strings.TrimSpace(s)
	  if n <= 0 || len(s) <= n {
		  return s
	  }
	  return s[:n]
  }
