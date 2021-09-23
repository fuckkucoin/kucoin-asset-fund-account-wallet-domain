package perm

import (
	"fmt"
	"net/http"
	"strings"
)

type Role string

const (
	RoleUser Role = "user"
	RoleOps Role = "ops"
	RoleReviewer Role = "reviewer"
	RoleCustody  Role = "custody"
	RoleMaker Role = "maker" // 闪兑做市/撮合回调
)

type Action string

const (
	ActAccountRead Action = "account.read"
	ActAccountMove Action = "account.move"
	ActAccountFreeze Action = "account.freeze"
	ActLedgerWrite    Action = "ledger.write"
	ActLedgerFreeze   Action = "ledger.freeze"
	ActDepositObserve Action = "deposit.observe"
	ActWithdrawSubmit Action = "withdraw.submit"
	ActWithdrawDecide Action = "withdraw.decide"
	ActAddressWrite Action = "address.write"
	ActCustodySweep Action = "custody.sweep"
	ActCustodySign Action = "custody.sign"
	ActKYTScreen Action = "kyt.screen"
	ActKYTDecide Action = "kyt.decide"
	ActQueryRead Action = "query.read"
	ActCaseRead       Action = "case.read"
	ActOpsAdmin       Action = "ops.admin"
	ActCustodyApprove Action = "custody.approve"
	ActCeremony Action = "custody.ceremony"
	ActAMLOpen  Action = "aml.open"
	ActAMLClose Action = "aml.close"
	ActTreasuryPlan Action = "treasury.plan"
	ActAdminSign    Action = "admin.sign"
	ActConvertFill Action = "convert.fill" // 限价闪兑部分成交回调
)

var grants = map[Role][]Action{
	RoleUser: {
		ActAccountRead, ActAccountMove, ActDepositObserve, ActWithdrawSubmit,
		ActAddressWrite, ActQueryRead, ActKYTScreen,
	},
	RoleOps: {
		ActAccountRead, ActAccountMove, ActAccountFreeze, ActLedgerWrite, ActLedgerFreeze,
		ActDepositObserve, ActWithdrawSubmit, ActAddressWrite, ActCustodySweep,
		ActKYTScreen, ActQueryRead, ActCaseRead, ActOpsAdmin,
		ActAMLOpen, ActAMLClose, ActTreasuryPlan, ActAdminSign, ActConvertFill,
	},
	RoleReviewer: {ActKYTDecide,
		ActWithdrawDecide, ActCaseRead, ActQueryRead, ActKYTScreen, ActAMLOpen, ActAMLClose},
	RoleCustody: {ActCustodySweep, ActCustodySign, ActCustodyApprove, ActCeremony, ActQueryRead, ActCaseRead, ActTreasuryPlan},
	RoleMaker: {ActConvertFill,
		ActQueryRead},
}

  func Allowed(role Role, act Action) bool {
	   for _, a := range grants[role] {
		  if a == act {
			   return true
		   }
	  }
	  return false
   }

type Actor struct {
	ID   string
	Role Role
}

// FromRequest 读 X-Actor / X-Role。缺省按普通用户，避免把运维口默认打开。
  func FromRequest(r *http.Request, fallbackUser string) Actor {
	  id := strings.TrimSpace(r.Header.Get("X-Actor"))
	   if id == "" {
		   id = fallbackUser
	  }
	   role := Role(strings.ToLower(strings.TrimSpace(r.Header.Get("X-Role"))))
	  if role == "" {
		   role = RoleUser
	  }
	  return Actor{ID: id, Role: role}
   }

   func (a Actor) Check(act Action) error {
	  if !Allowed(a.Role, act) {
		  return fmt.Errorf("%w: %s cannot %s", ErrDenied, a.Role, act)
	  }
	  return nil
  }

var ErrDenied = fmt.Errorf("permission denied")
