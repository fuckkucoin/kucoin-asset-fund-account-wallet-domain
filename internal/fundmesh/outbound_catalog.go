// 功能：出站幂等表。
// 重点：双入口提交同码；重复提交不增加扣减次数。
// 风险点：入口字段不得参与码计算。

// 作者：Bronson
// 日期：2021.06.14

package fundmesh

   func OutboundEntryParityRows() [][4]string {
	  return [][4]string{
		   {"new", "POST", "PUT", CodeOutboundOK},
		  {"replay", "POST", "PUT", CodeOutboundReplay},
		  {"conflict", "POST", "PUT", CodeOutboundConflict},
		   {"cancelled", "POST", "PUT", CodeOutboundCancelled},
	  }
  }

  func SameOutboundCode(a, b OutboundResult) bool {
	   if a.Code != b.Code && !replayCodesEqual(a.Code, b.Code) {
		   return false
	   }
	   okA := a.State == OutboundAccepted || a.State == OutboundReplayed || a.State == OutboundClosed
	  okB := b.State == OutboundAccepted || b.State == OutboundReplayed || b.State == OutboundClosed
	  if okA != okB && a.Code != CodeOutboundConflict {
		   return a.Code == b.Code
	  }
	   return true
   }

  func OutboundProofKinds() []string {
	   return []string{ProofReplay,
		   ProofConflict, ProofDupDebit, ProofClosed, ProofKeyInvalid, ProofPayload, ProofRemoteFail}
  }

  func SummarizeOutboundProofs(rows []OutboundProof) map[string]int {
	  out := map[string]int{}
	   for _, k := range OutboundProofKinds() {
		   out[k] = 0
	   }
	   out["blocked"] = 0
	   for _, r := range rows {
		   out[r.Kind]++
		   if r.Blocked {
			  out["blocked"]++
		  }
	  }
	  return out
  }
