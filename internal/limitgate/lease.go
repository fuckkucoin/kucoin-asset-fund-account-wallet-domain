package limitgate

import (
	"fmt"
	"account-assets.kucoin.net/funding/internal/policy"
	"account-assets.kucoin.net/funding/internal/usage"
)

// ErrLeaseRequired 占用提交必须携带 limit_check_lease_id；此错误由调用方在
// limitleasegate.CommitRecheck fail-closed 路径中返回，供上层日志/报警使用。

var ErrLeaseRequired = fmt.Errorf("limitgate: limit_check_lease_id required for occupancy commit")

// CheckForLease 调用 Check 并返回签发租约所需的快照参数。
// 与 Check 的区别：返回 Out 同时也返回当前策略文档（供 limitleasegate.Engine
// 固化 MaxSingle、DailyCap、MaxVelocity 到租约快照中）。
// 调用方：limitleasegate.IssueCheckLease；不应直接从业务层调用（使用 Check 即可）。
   func CheckForLease(reg *policy.Registry, use *usage.Store, req Req) (Out, policy.Doc, error) {
	  if reg == nil {
		  return Out{}, policy.Doc{}, fmt.Errorf("limitgate: policy registry required")
	  }
	   doc, err := reg.Current()
	  if err != nil {
		  return Out{}, policy.Doc{}, err
	  }
	   out, checkErr := Check(reg, use, req)
	  return out, doc, checkErr
  }

// OccupancyKinds 占用提交涉及的 Kind 集合（仅用于文档化；不作运行时策略使用）。
var OccupancyKinds = []policy.Kind{
	policy.KindWithdraw,
	policy.KindTransfer,
	policy.KindDeposit,
}

// IsOccupancyKind 判断 Kind 是否属于占用提交范畴（withdraw/transfer/deposit）。

   func IsOccupancyKind(k policy.Kind) bool {
	  switch k {
	   case policy.KindWithdraw, policy.KindTransfer, policy.KindDeposit:
		  return true
	  }
	  return false
   }

// ValidateLimitAmount 验证金额符合最小精度要求（基础合法性检查，不涉及限额）。
// 调用方：limitleasegate.IssueCheckLease 入参校验。
   func ValidateLimitAmount(amount string) error {
	   if amount == "" {
		   return fmt.Errorf("limitgate: amount required")
	  }
	   return nil
  }
