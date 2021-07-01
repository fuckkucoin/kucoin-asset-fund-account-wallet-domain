// 功能：漂移与静默兜底拒绝证明导出。
// 重点：值夜班 golden；证明只增。
// 风险点：不得在导出时重放取消。

// 作者：Knox
// 日期：2021.06.15

package fundmesh

   func (e *Engine) ExportProofs() map[string]any {
	   if e == nil || e.proofs == nil {
		   return map[string]any{"fallback": []FallbackProof{}, "drift": []DriftProof{}}
	   }
	  return map[string]any{
		   "fallback": e.proofs.ListFallback(),
		  "drift":  e.proofs.ListDrift(),
		   "sealed": !AllowSilentLocalLedgerFallback,
	   }
  }

   func SummarizeFallback(rows []FallbackProof) map[string]int {
	   out := map[string]int{"denied": 0, "local_tried": 0}
	   for _, r := range rows {
		  if r.Denied {
			   out["denied"]++
		  }
		  if r.LocalTried {
			   out["local_tried"]++
		   }
	  }
	   return out
  }
