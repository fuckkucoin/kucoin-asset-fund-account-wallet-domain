package p2pabandon

import "errors"

var (
	// ErrUnavailable 服务不可用（依赖未注入）。
	ErrUnavailable = errors.New("p2pabandon: service unavailable")
	// ErrOrder 订单 ID 为空。
	ErrOrder = errors.New("p2pabandon: order_id required")
	// ErrPlan 弃单计划不存在。
	ErrPlan = errors.New("p2pabandon: abandon plan not found")
	// ErrPlanExists 同一订单已存在弃单计划（幂等约束）。
	ErrPlanExists = errors.New("p2pabandon: abandon plan already exists for order")
	// ErrPlanTerminal 计划已处于终态，不得接着推进。
	ErrPlanTerminal = errors.New("p2pabandon: abandon plan already in terminal state")
	// ErrPlanNotArmed 计划不在 armed 状态，不得发起弃单。
	ErrPlanNotArmed = errors.New("p2pabandon: abandon plan not in armed state")
	// ErrPlanNotAbandoning 计划不在 abandoning 状态，不得提交释放令牌。
	ErrPlanNotAbandoning = errors.New("p2pabandon: abandon plan not in abandoning state")
	// ErrToken 令牌不存在。
	ErrToken = errors.New("p2pabandon: mutex token not found")
	// ErrTokenTerminal 令牌已处于终态（committed 或 aborted）。
	ErrTokenTerminal = errors.New("p2pabandon: mutex token already in terminal state")
	// ErrTokenConflict 互斥令牌冲突：两条路径均尝试 commit，触发竞态冲突流程。
	ErrTokenConflict = errors.New("p2pabandon: mutex token conflict detected (release vs fill race)")
	// ErrTokenAborted 令牌已被对手方抢占，本路径已 aborted。
	ErrTokenAborted = errors.New("p2pabandon: mutex token aborted by competing path")
	// ErrHoldAge 超龄记录不存在。
	ErrHoldAge = errors.New("p2pabandon: hold age record not found")
	// ErrHoldAgeResolved 超龄记录已处置（终态）。
	ErrHoldAgeResolved = errors.New("p2pabandon: hold age already resolved")
	// ErrProof 收口证明不存在。
	ErrProof = errors.New("p2pabandon: close proof not found")
	// ErrProofExists 收口证明已存在（幂等约束）。
	ErrProofExists = errors.New("p2pabandon: close proof already exists for plan")
	// ErrProofTamper 收口证明哈希不符，疑似篡改。
	ErrProofTamper = errors.New("p2pabandon: close proof tamper detected")
	// ErrNoProof 缺失收口证明，不得标 closed。
	ErrNoProof = errors.New("p2pabandon: close proof required before marking closed")
	// ErrFourEye 双人确认校验失败（操作人1 与操作人2 不得相同）。
	ErrFourEye = errors.New("p2pabandon: dual-control required (operator1 != operator2)")
	// ErrActor 操作人为空。
	ErrActor = errors.New("p2pabandon: actor required")
	// ErrIdem 幂等键为空。
	ErrIdem = errors.New("p2pabandon: idempotency_key required")
	// ErrAmount 金额无效。
	ErrAmount = errors.New("p2pabandon: amount invalid")
	// ErrAsset 资产代码无效。
	ErrAsset = errors.New("p2pabandon: asset required")
	// ErrHold hold_id 为空。
	ErrHold = errors.New("p2pabandon: hold_id required")
	// ErrState 状态转换非法。
	ErrState = errors.New("p2pabandon: illegal state transition")
	// ErrReason 强制弃单理由为空。
	ErrReason = errors.New("p2pabandon: force_abandon reason required")
	// ErrInternal 内部数据不一致（需告警）。
	ErrInternal = errors.New("p2pabandon: internal data inconsistency")
)
