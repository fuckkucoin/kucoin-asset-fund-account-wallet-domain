package dispclaw

import "fmt"

var (
	// ErrUnavailable 服务未初始化。
	ErrUnavailable = fmt.Errorf("dispclaw: service unavailable")
	// ErrPlan 计划不存在。
	ErrPlan = fmt.Errorf("dispclaw: fund plan not found")
	// ErrLeg 追偿腿不存在。
	ErrLeg = fmt.Errorf("dispclaw: recovery leg not found")
	// ErrGate 收口闸不存在。
	ErrGate = fmt.Errorf("dispclaw: close gate not found")
	// ErrProof 收口证明不存在。
	ErrProof = fmt.Errorf("dispclaw: close proof not found")
	// ErrState 非法状态转换。
	ErrState = fmt.Errorf("dispclaw: illegal state transition")
	// ErrCase Case 不存在或状态不符。
	ErrCase = fmt.Errorf("dispclaw: case not found or invalid")
	// ErrFourEye 四眼校验失败：执行人不得为开单人或用户本人。
	ErrFourEye = fmt.Errorf("dispclaw: four-eye check failed, executor must differ from opener")
	// ErrGateBlocking 收口闸阻断：尚有未完成追偿腿。
	ErrGateBlocking = fmt.Errorf("dispclaw: close gate blocking, pending legs exist")
	// ErrActor 操作人无效。
	ErrActor = fmt.Errorf("dispclaw: actor invalid")
	// ErrAmount 金额无效。
	ErrAmount = fmt.Errorf("dispclaw: amount invalid")
	// ErrAsset 资产无效。
	ErrAsset = fmt.Errorf("dispclaw: asset invalid")
	// ErrIdem 幂等键冲突。
	ErrIdem = fmt.Errorf("dispclaw: idempotency conflict")
	// ErrPlanExists 一个 Case 只能有一个计划。
	ErrPlanExists = fmt.Errorf("dispclaw: fund plan already exists for this case")
	// ErrLegNotPending 腿不在 pending 状态，无法执行。
	ErrLegNotPending = fmt.Errorf("dispclaw: recovery leg not in pending status")
	// ErrProofTamper 收口证明哈希不符，已检测到篡改。
	ErrProofTamper = fmt.Errorf("dispclaw: close proof hash mismatch, tampering detected")
	// ErrBypassReason 豁免原因不得为空。
	ErrBypassReason = fmt.Errorf("dispclaw: bypass reason required")
	// ErrAbortReason 中止原因不得为空。
	ErrAbortReason = fmt.Errorf("dispclaw: abort reason required")
)
