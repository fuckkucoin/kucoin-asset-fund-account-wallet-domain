package breachclosegate

import "fmt"

var (
	// ErrBreachBlock Decide(allow/approve) 或出金 Submit 因存在活跃 BreachLease 被拒。
	// 呼叫者须将此错误作为 fail-closed 语义：不得重试为成功；须进入清障流程。
	ErrBreachBlock = fmt.Errorf("sla breach block: active breach_gen exists; allow/submit rejected fail-closed")
	// ErrNoActiveBreach 目标 case/movement 无活跃 BreachLease，TryAllowCheck 通过。
	ErrNoActiveBreach = fmt.Errorf("no active sla breach for this case/movement")
	// ErrLeaseNotFound 指定 breach_gen 或 lease_id 不存在。
	ErrLeaseNotFound = fmt.Errorf("breach lease not found")
	// ErrAlreadyCleared BreachLease 已清障，不可重复清障。
	ErrAlreadyCleared = fmt.Errorf("breach lease already cleared")
	// ErrAlreadyExpiredCase BreachLease 关联案件已过期，状态不可再变更。
	ErrAlreadyExpiredCase = fmt.Errorf("breach lease case already expired")
	// ErrClearanceMakerCheck 四眼清障：Proposer 与 Approver 不能为同一人。
	ErrClearanceMakerCheck = fmt.Errorf("clearance four-eye check: proposer and approver must be distinct actors")
	// ErrClearanceNotFound 清障工单不存在。
	ErrClearanceNotFound = fmt.Errorf("breach clearance not found")
	// ErrClearanceNotPending 清障工单已不在 pending 状态，不可再审批或拒绝。
	ErrClearanceNotPending = fmt.Errorf("breach clearance is not in pending state")
	// ErrClearanceNoAutoAllow 四眼清障完成后不隐式放行出金；须重新进入审查/核验路径。
	// 此错误用于向调用方明确语义，而非终止清障流程本身。
	ErrClearanceNoAutoAllow = fmt.Errorf("clearance approved: withdraw is NOT auto-allowed; re-enter compliance path")
	// ErrDuplicateBreach 同一 case_id 已存在活跃 BreachLease，不可重复签发。
	ErrDuplicateBreach = fmt.Errorf("duplicate breach lease for this case")
	// ErrProofNotFound 指定证明记录不存在。
	ErrProofNotFound = fmt.Errorf("sla breach proof not found")
	// ErrIllegalTransition 非法状态转换。
	ErrIllegalTransition = fmt.Errorf("illegal breach lease state transition")
)
// temporary guard removed after the production check
