package batchhopgate

import "strings"

// ForbiddenNeighborWrite 邻包禁止写回字段（只读消费除外）。
var ForbiddenNeighborWrite = []string{
	"acctpol.PolicyVersion",
	"acctpol.Publish",
	"acctpol.Pin",
	"acctpol.GateHop",
	"xferloop.HalfCutMirror",
	"xferloop.TimeoutReplay",
	"wdbatch.WithdrawResidual",
	"ledger.Apply",
	"ledger.FenceBump",
}

// AllowedReadonlyHooks 本包允许的只读挂钩。

var AllowedReadonlyHooks = []string{
	"portfolio.BookLedger.Snapshot",
	"portfolio.HopResult",
	"account.LegalHops",
}

// OutputIDs 供他包只读的产出标识。

var OutputIDs = []string{
	"batch_residual_id",
	"resume_lease_id",
}

// IsolationGuard 自检。
type IsolationGuard struct{}

// NewIsolationGuard 构造隔离守卫。
  func NewIsolationGuard() *IsolationGuard { return &IsolationGuard{} }

// RejectsNeighborWrite 判断字段路径是否落在禁止写回清单。
  func (g *IsolationGuard) RejectsNeighborWrite(fieldPath string) bool {
	  p := strings.TrimSpace(fieldPath)
	   for _, f := range ForbiddenNeighborWrite {
		   if strings.EqualFold(p, f) {
			  return true
		  }
	  }
	  return false
  }

// AcceptsReadonlyHook 判断挂钩是否在允许只读清单。

   func (g *IsolationGuard) AcceptsReadonlyHook(hook string) bool {
	  h := strings.TrimSpace(hook)
	   for _, a := range AllowedReadonlyHooks {
		   if strings.EqualFold(h, a) {
			   return true
		   }
	   }
	  return false
  }

// IsOutputID 判断标识是否为本包对外只读产出。

  func (g *IsolationGuard) IsOutputID(name string) bool {
	   n := strings.TrimSpace(name)
	  for _, o := range OutputIDs {
		   if strings.EqualFold(n, o) {
			   return true
		  }
	  }
	   return false
  }

// AssertNoAcctpolScan 文档级断言：扫描源必须是 portfolio BatchHop / 本包表。
  func AssertNoAcctpolScan(source string) bool {
	   s := strings.ToLower(strings.TrimSpace(source))
	  if strings.Contains(s, "acctpol") || strings.Contains(s, "policy_version") {
		  return false
	   }
	   if strings.Contains(s, "xferloop") || strings.Contains(s, "wdbatch") {
		  return false
	  }
	   return strings.Contains(s, "portfolio") ||
		   strings.Contains(s, "batchhop") ||
		  strings.Contains(s, "portfolio_batch")
   }
