package restoregate

import "fmt"

// 业务级错误——调用方应对这些错误类型做 errors.Is 判断，不应依赖错误字符串。

var (
	// ErrUnavailable 依赖组件未就绪（store 为 nil 等）。
	ErrUnavailable = fmt.Errorf("restoregate: service unavailable")
	// ErrPlanNotFound 指定 RestorePlan 不存在。
	ErrPlanNotFound = fmt.Errorf("restoregate: plan not found")
	// ErrPlanConflict 同一 plan_id 已存在，拒绝重复创建。
	ErrPlanConflict = fmt.Errorf("restoregate: plan already exists")
	// ErrInvalidState FSM 状态转换非法。
	ErrInvalidState = fmt.Errorf("restoregate: invalid state transition")
	// ErrGenConflict 世代冲突：活账水位 > 计划目标世代，整单失败，零写入。
	ErrGenConflict = fmt.Errorf("restoregate: generation conflict — live watermark exceeds plan target")
	// ErrGenLag 世代落后：计划目标世代 < 当前系统世代，拒绝回灌。
	ErrGenLag = fmt.Errorf("restoregate: generation lag — plan target below current system generation")
	// ErrWriteFrozen 活账写互斥锁持有期间，贷记/冻结/划转意图被拒绝。
	ErrWriteFrozen = fmt.Errorf("restoregate: writes frozen during restore window")
	// ErrHalfSealExists 已有 HalfSeal 封印，必须显式续或中止后才可重 Import。
	ErrHalfSealExists = fmt.Errorf("restoregate: half-restore seal exists; explicit resume or abort required")
	// ErrHalfSealNotFound HalfSeal 记录不存在。
	ErrHalfSealNotFound = fmt.Errorf("restoregate: half-seal not found")
	// ErrSplitBrainOpen SplitBrainProof 打开且挡写，必须四眼解除后才可接着写。
	ErrSplitBrainOpen = fmt.Errorf("restoregate: split-brain proof is open; writes blocked until four-eyes clear")
	// ErrSplitBrainNotFound 指定 SplitBrainProof 不存在。
	ErrSplitBrainNotFound = fmt.Errorf("restoregate: split-brain proof not found")
	// ErrChecksumMismatch 余额根 checksum 与计划目标不符。
	ErrChecksumMismatch = fmt.Errorf("restoregate: balance-root checksum mismatch after restore")
	// ErrMutexConflict WriteMutex 冲突：当前 plan 持有锁，其他 plan 不得申请。
	ErrMutexConflict = fmt.Errorf("restoregate: write mutex conflict with existing holder")
	// ErrMutexNotFound 写互斥锁记录不存在。
	ErrMutexNotFound = fmt.Errorf("restoregate: write mutex not found")
	// ErrPlanAborted 计划已中止，操作被拒。
	ErrPlanAborted = fmt.Errorf("restoregate: plan has been aborted")
	// ErrPlanSealed 计划已封印完成，不得重复操作。
	ErrPlanSealed = fmt.Errorf("restoregate: plan is already sealed")
	// ErrEmpty 参数为空。
	ErrEmpty = fmt.Errorf("restoregate: required parameter is empty")
	// ErrProofConflict 四眼解除审批人不足或角色重复。
	ErrProofConflict = fmt.Errorf("restoregate: split-brain proof approval conflict")
)
