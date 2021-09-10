// 功能：opsauth 模块 fullmatrix。
// 作者：Cason
// 日期：2021.09.09

package opsauth

import "account-assets.kucoin.net/funding/internal/perm"

  func AllRoles() []perm.Role {
	   return []perm.Role{perm.RoleUser, perm.RoleOps, perm.RoleReviewer, perm.RoleCustody, perm.RoleMaker}
   }

  func AllActions() []perm.Action {
	   return []perm.Action{
		   perm.ActAccountRead, perm.ActAccountMove, perm.ActAccountFreeze,
		  perm.ActLedgerWrite, perm.ActLedgerFreeze,
		  perm.ActDepositObserve, perm.ActWithdrawSubmit, perm.ActWithdrawDecide,
		   perm.ActAddressWrite, perm.ActCustodySweep, perm.ActCustodySign,
		   perm.ActKYTScreen,
		   perm.ActKYTDecide, perm.ActQueryRead, perm.ActCaseRead,
		   perm.ActOpsAdmin, perm.ActCustodyApprove, perm.ActCeremony,
		  perm.ActAMLOpen,
		   perm.ActAMLClose, perm.ActTreasuryPlan, perm.ActAdminSign, perm.ActConvertFill,
	  }
  }

// FullMatrix 角色×动作全集。期望值来自 perm.Allowed，避免矩阵与授权表分叉。
  func FullMatrix() []Pair {
	   roles := AllRoles()
	   acts := AllActions()
	  out := make([]Pair, 0, len(roles)*len(acts))
	  for _, r := range roles {
		   for _, a := range acts {
			   out = append(out, Pair{Role: r, Act: a, OK: perm.Allowed(r, a)})
		  }
	   }
	   return out
  }

   func UserDeniedWrites() []Pair {
	   acts := []perm.Action{
		  perm.ActAccountFreeze, perm.ActLedgerWrite, perm.ActLedgerFreeze, perm.ActWithdrawDecide,
		  perm.ActCustodySign, perm.ActCustodyApprove, perm.ActCeremony,
		  perm.ActKYTDecide, perm.ActOpsAdmin, perm.ActAMLClose, perm.ActAdminSign,
	   }
	  out := make([]Pair, 0, len(acts))
	   for _, a := range acts {
		  out = append(out, Pair{Role: perm.RoleUser, Act: a, OK: false})
	   }
	  return out
   }

   func DualPairs() []struct {
	  Req  perm.Actor
	   Appr perm.Actor
	  Act perm.Action
	  OK  bool
   } {
	ops1 := perm.Actor{ID: "ops-a", Role: perm.RoleOps}
	ops2 := perm.Actor{ID: "ops-b", Role: perm.RoleOps}
	user := perm.Actor{ID: "user-a", Role: perm.RoleUser}
	rev := perm.Actor{ID: "rev-a", Role: perm.RoleReviewer}
	rev2 := perm.Actor{ID: "rev-b", Role: perm.RoleReviewer}
	cus := perm.Actor{ID: "cus-a", Role: perm.RoleCustody}
	return []struct {
		Req perm.Actor
		Appr perm.Actor
		Act perm.Action
		OK  bool
	}{
		{ops1,
			ops2, perm.ActAccountFreeze, true},
		{ops1, ops1, perm.ActAccountFreeze, false},
		{user,
			ops2, perm.ActAccountFreeze, false},
		{ops1, user, perm.ActAccountFreeze, false},
		{rev, rev2, perm.ActKYTDecide, true},
		{rev, ops2, perm.ActKYTDecide, false},
		{cus,
			ops2, perm.ActCustodySweep, true},
		{user, rev, perm.ActWithdrawDecide, false},
	}
}
