// 错贷记模块错误定义。

// Author: Kinsley
// Date: 2026.04.07

package wrongcredit

import "fmt"

var (
	// ErrUnavailable 服务未初始化或内部状态不可用。
	ErrUnavailable = fmt.Errorf("wrongcredit: service unavailable")
	// ErrNotFound 案件不存在。
	ErrNotFound = fmt.Errorf("wrongcredit: case not found")
	// ErrIdem 幂等键冲突（已有案件，但参数不一致）。
	ErrIdem = fmt.Errorf("wrongcredit: idempotency conflict")
	// ErrDuplicate 同一 CreditRef 已存在 OPEN 案件，禁止重复开案。
	ErrDuplicate = fmt.Errorf("wrongcredit: duplicate open case for credit_ref")
	// ErrUser 用户 ID 非法或不存在。
	ErrUser = fmt.Errorf("wrongcredit: invalid user_id")
	// ErrAsset 资产代码非法。
	ErrAsset = fmt.Errorf("wrongcredit: invalid asset")
	// ErrAmount 金额形态非法或超出原始贷记额度。
	ErrAmount = fmt.Errorf("wrongcredit: invalid amount")
	// ErrCreditRef CreditRef (transfer_id) 形态非法。
	ErrCreditRef = fmt.Errorf("wrongcredit: invalid credit_ref")
	// ErrPhase Phase 字符串不在允许值集合内。
	ErrPhase = fmt.Errorf("wrongcredit: invalid phase")
	// ErrSource EvidenceSource 字符串不在允许值集合内。
	ErrSource = fmt.Errorf("wrongcredit: invalid evidence source")
	// ErrConfidence Confidence 字符串不在允许值集合内。
	ErrConfidence = fmt.Errorf("wrongcredit: invalid confidence")
	// ErrRiskTier RiskTier 字符串不在允许值集合内。
	ErrRiskTier = fmt.Errorf("wrongcredit: invalid risk_tier")
	// ErrFence 乐观锁 epoch/token 过期，拒绝写入。
	ErrFence = fmt.Errorf("wrongcredit: stale fence")
	// ErrActor 操作人 ID 非法。
	ErrActor = fmt.Errorf("wrongcredit: invalid actor")
	// ErrIllegalTransition 非法状态转移（FSM 拒绝）。
	ErrIllegalTransition = fmt.Errorf("wrongcredit: illegal fsm transition")
	// ErrTerminal 案件已在终态，不可再操作。
	ErrTerminal = fmt.Errorf("wrongcredit: case is in terminal state")
	// ErrMutexLocked 目标 CreditRef 已被 reorg-reversal 批次锁定，无法双路冲销。
	ErrMutexLocked = fmt.Errorf("wrongcredit: credit_ref locked by reorg reversal")
	// ErrMutexConflict 目标 CreditRef 已被另一个 wrong-credit 案件锁定。
	ErrMutexConflict = fmt.Errorf("wrongcredit: credit_ref locked by another wrong-credit case")
	// ErrInsufficientConf 置信度不足，禁止执行冻结或冲销操作。
	ErrInsufficientConf = fmt.Errorf("wrongcredit: confidence too low to proceed")
	// ErrApprovalRequired 需要审批方可接着，当前尚未批准。
	ErrApprovalRequired = fmt.Errorf("wrongcredit: approval required")
	// ErrDualApprovalRequired Tier3 需要第二审批人。
	ErrDualApprovalRequired = fmt.Errorf("wrongcredit: dual approval required for tier3")
	// ErrSameApprover 双人审批不得由同一人签两次。
	ErrSameApprover = fmt.Errorf("wrongcredit: dual approval requires different approvers")
	// ErrJournal 冲销分录不平或金额异常。
	ErrJournal = fmt.Errorf("wrongcredit: reversal journal validation failed")
	// ErrReason 豁免/备注文本非法（空或超长）。
	ErrReason = fmt.Errorf("wrongcredit: reason text invalid")
	// ErrHalt 服务处于维护暂停状态。
	ErrHalt = fmt.Errorf("wrongcredit: service halted for maintenance")
)

// wrapErr 将底层错误包装后附在域错误后，方便 errors.Is 同时保留原始信息。
  func wrapErr(sentinel, cause error) error {
	   if cause == nil {
		   return sentinel
	   }
	  return fmt.Errorf("%w: %v", sentinel, cause)
   }
