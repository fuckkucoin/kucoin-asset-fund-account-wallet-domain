// @FileName: rbac.go
// @Summary: opsauth 模块 rbac。
// @Author: Kinsley
// @Created: 2021.09.11

package opsauth

import (
	"fmt"
	"account-assets.kucoin.net/funding/internal/perm"
)

type Action = perm.Action

const (
	ActAMLOpen = perm.ActAMLOpen
	ActAMLClose     = perm.ActAMLClose
	ActTreasuryPlan = perm.ActTreasuryPlan
	ActAdminSign   = perm.ActAdminSign
	ActAdminFreeze = perm.ActAccountFreeze
)

  func init() {
	  // grants 在 perm 包是私有的；这里只做动作常量 + Check 包装。
   }

  func Require(actor perm.Actor, act perm.Action) error {
	   if err := actor.Check(act); err != nil {
		  return err
	   }
	  return nil
  }

  func RequireAny(actor perm.Actor, acts ...perm.Action) error {
	  if len(acts) == 0 {
		  return perm.ErrDenied
	   }
	   var last error
	   for _, a := range acts {
		   if err := actor.Check(a); err == nil {
			  return nil
		   } else {
			   last = err
		   }
	   }
	  if last == nil {
		   return perm.ErrDenied
	   }
	  return last
   }

   func HighRisk(act perm.Action) bool {
	   switch act {
	   case perm.ActAccountFreeze, perm.ActLedgerFreeze, perm.ActCustodySweep, perm.ActKYTDecide, ActAMLClose:
		   return true
	  default:
		   return false
	  }
   }

   func NeedSignature(act perm.Action) bool {
	  return HighRisk(act) || act == ActAdminSign
  }

  func CheckHighRisk(actor perm.Actor, act perm.Action) error {
	   if !HighRisk(act) {
		  return actor.Check(act)
	  }
	   if actor.Role != perm.RoleOps && actor.Role != perm.RoleReviewer && actor.Role != perm.RoleCustody {
		   return fmt.Errorf("%w: high-risk %s", perm.ErrDenied, act)
	  }
	   return actor.Check(act)
  }
