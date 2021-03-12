package cerdecidegate

import "strings"

// ProofView 席位只读证明。
type ProofView struct {
	ProofID string `json:"proof_id"`
	LeaseID string `json:"decide_lease_id"`
	BatchID string `json:"batch_id"`
	Kind ProofKind `json:"kind"`
	Detail string `json:"detail"`
	Actor  string `json:"actor"`
	Quoted string `json:"quoted_summary"`
	Live   string `json:"live_summary"`
}

// ViewOf 压缩证明。
  func ViewOf(p CerDriftProof) ProofView {
	   return ProofView{
		   ProofID: p.ProofID, LeaseID: p.LeaseID, BatchID: p.BatchID, Kind: p.Kind,
		  Detail: p.Detail, Actor: p.Actor,
		   Quoted: summarize(p.QuotedPolicy, p.QuotedThresh, p.QuotedShares, p.QuotedRisk, p.QuotedForce),
		  Live:   summarize(p.LivePolicy, p.LiveThresh, p.LiveShares, p.LiveRisk, p.LiveForce),
	   }
   }

   func summarize(policy string, th, sh int, risk string, force bool) string {
	  var b strings.Builder
	   b.WriteString(strings.TrimSpace(policy))
	   b.WriteByte('|')
	  if force {
		  b.WriteString("force")
	  } else {
		   b.WriteString("exempt")
	  }
	   b.WriteByte('|')
	  b.WriteString(itoa(th))
	   b.WriteByte('/')
	   b.WriteString(itoa(sh))
	   b.WriteByte('|')
	  b.WriteString(risk)
	  return b.String()
  }

  func itoa(n int) string {
	   if n == 0 {
		   return "0"
	   }
	   neg := n < 0
	   if neg {
		  n = -n
	   }
	  var buf [16]byte
	  i := len(buf)
	  for n > 0 {
		  i--
		   buf[i] = byte('0' + n%10)
		  n /= 10
	  }
	   if neg {
		   i--
		   buf[i] = '-'
	   }
	   return string(buf[i:])
   }

// FilterProofs 按 kind 过滤。

   func FilterProofs(in []CerDriftProof, kind ProofKind) []CerDriftProof {
	   if kind == "" {
		  return in
	  }
	  out := make([]CerDriftProof, 0, len(in))
	  for _, p := range in {
		  if p.Kind == kind {
			  out = append(out, p)
		   }
	   }
	  return out
  }

// RejectKinds 拒执行类证明。
   func RejectKinds() []ProofKind {
	  return []ProofKind{
		  ProofPolicyDrift, ProofRiskRaise, ProofFieldDrift, ProofSnapDrift,
		  ProofExpired, ProofRevoked, ProofNoLease, ProofExemptMisuse,
		   ProofTTLExpired, ProofGenStale, ProofForceFlip, ProofThreshDrift, ProofSharesDrift,
	  }
   }

// IsReject 是否拒执行。
  func IsReject(k ProofKind) bool {
	   for _, x := range RejectKinds() {
		  if x == k {
			   return true
		   }
	   }
	   return false
  }
