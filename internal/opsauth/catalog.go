package opsauth

import "account-assets.kucoin.net/funding/internal/perm"

type ActionInfo struct {
	Action   perm.Action `json:"action"`
	HighRisk bool        `json:"high_risk"`
	NeedSig bool `json:"need_signature"`
	Ops     bool `json:"ops"`
	Reviewer bool `json:"reviewer"`
	Custody bool `json:"custody"`
}

  func Catalog() []ActionInfo {
	   acts := []perm.Action{
		  perm.ActAccountFreeze, perm.ActLedgerFreeze, perm.ActCustodySweep,
		  perm.ActKYTDecide, perm.ActAMLOpen, perm.ActAMLClose,
		  perm.ActTreasuryPlan, perm.ActAdminSign, perm.ActOpsAdmin,
		  perm.ActCaseRead, perm.ActQueryRead,
	   }
	  out := make([]ActionInfo, 0, len(acts))
	  for _, a := range acts {
		  out = append(out, ActionInfo{
			  Action: a, HighRisk: HighRisk(a), NeedSig: NeedSignature(a),
			  Ops: perm.Allowed(perm.RoleOps, a), Reviewer: perm.Allowed(perm.RoleReviewer, a),
			  Custody: perm.Allowed(perm.RoleCustody, a),
		  })
	  }
	  return out
  }
