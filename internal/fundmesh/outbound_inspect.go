// @FileName: outbound_inspect.go
// @Summary: 出站幂等只读巡检。
// 重点：导出键规则、矩阵与禁止二次扣减状态。
// 风险点：巡检不得提交出站单。
// @Author: Cason
// @Created: 2021.06.14

package fundmesh

  func InspectOutbound() map[string]any {
	  return map[string]any{
		  "scheme":                 IdemSchemeV1,
		  "kinds":                  []string{IdemKindClient, IdemKindDerived},
		  "matrix":                 OutboundMatrixRows(),
		  "parity":                 OutboundEntryParityRows(),
		  "forbidden_second_debit": outboundStates(ForbiddenSecondDebitStates()),
		   "proof_kinds": OutboundProofKinds(),
		  "key_min": idemMinLen,
		   "key_max":         idemMaxLen,
		   "fingerprint_len": FingerprintLen,
		   "cancel_still_homologous": true,
	  }
  }

   func outboundStates(in []OutboundState) []string {
	   out := make([]string, 0, len(in))
	   for _, s := range in {
		   out = append(out, string(s))
	   }
	   return out
   }

  func OutboundKeyRuleRows() [][3]string {
	  return [][3]string{
		  {"empty", "derive", "fmout.v1.d"},
		  {"client_bare", "canonicalize", "fmout.v1.c"},
		   {"client_scheme", "parse", "fmout.v1.c"},
		  {"derived_scheme", "parse", "fmout.v1.d"},
		   {"space", "reject", CodeInput},
		  {"control", "reject", CodeInput},
		   {"too_short", "reject", CodeInput},
		  {"too_long", "reject", CodeInput},
		   {"bad_kind", "reject", CodeInput},
	  }
  }

  func OutboundReplayRuleRows() [][3]string {
	  return [][3]string{
		  {"same_key_same_fp", "replay", CodeOutboundReplay},
		   {"same_key_diff_fp", "conflict", CodeOutboundConflict},
		   {"same_key_after_cancel", "replay", CodeOutboundCancelled},
		  {"header_and_body_same", "unify", CodeOutboundOK},
	  }
   }
