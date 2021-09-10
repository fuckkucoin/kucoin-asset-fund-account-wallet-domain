package opsauth

import (
	"fmt"
	"account-assets.kucoin.net/funding/internal/perm"
)

type Pair struct {
	Role perm.Role
	Act perm.Action
	OK  bool
}

// Matrix 角色×动作。用户不得冻账/签管/关案。
   func Matrix() []Pair {
	  return []Pair{
		   {perm.RoleUser, perm.ActAccountRead, true},
		   {perm.RoleUser,
			  perm.ActAccountMove, true},
		   {perm.RoleUser, perm.ActLedgerWrite, false},
		  {perm.RoleUser, perm.ActAccountFreeze, false},
		  {perm.RoleUser, perm.ActCustodySign, false},
		  {perm.RoleUser, perm.ActOpsAdmin, false},
		   {perm.RoleOps, perm.ActAccountFreeze, true},
		  {perm.RoleOps, perm.ActOpsAdmin, true},
		   {perm.RoleReviewer, perm.ActWithdrawDecide, true},
		  {perm.RoleReviewer, perm.ActCustodySign, false},
		   {perm.RoleCustody,
			  perm.ActCustodySign, true},
		   {perm.RoleCustody, perm.ActCustodyApprove, true},
		  {perm.RoleCustody,
			  perm.ActAccountFreeze, false},
	  }
  }

   func CheckMatrix(p Pair) error {
	  got := perm.Allowed(p.Role, p.Act)
	  if got != p.OK {
		  return fmt.Errorf("role %s act %s want %v got %v", p.Role, p.Act, p.OK, got)
	   }
	  actor := perm.Actor{ID: "matrix-" + string(p.Role), Role: p.Role}
	   if p.OK {
		  return actor.Check(p.Act)
	  }
	  if err := actor.Check(p.Act); err == nil {
		   return fmt.Errorf("role %s act %s should deny", p.Role, p.Act)
	   }
	   return nil
   }

  func DualControl(requester, approver perm.Actor, act perm.Action) error {
	  if requester.ID == "" || approver.ID == "" || requester.ID == approver.ID {
		   return perm.ErrDenied
	  }
	  if err := Require(requester, act); err != nil {
		   return err
	  }
	  if HighRisk(act) {
		  return CheckHighRisk(approver,
			  act)
	   }
	  return Require(approver, act)
  }
