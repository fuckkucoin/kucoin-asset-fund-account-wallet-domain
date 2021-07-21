package hopcompgate

import "fmt"

var (
	// ErrCrossShardIntent 跨片 Intent 不得进入本包补偿主路径。
	ErrCrossShardIntent = fmt.Errorf("hopcompgate: cross-shard intent must not enter compensate path")
	// ErrCaseNotFound 补偿仓不存在。
	ErrCaseNotFound = fmt.Errorf("hopcompgate: case not found")
	// ErrLeaseNotFound 补偿租约不存在。
	ErrLeaseNotFound = fmt.Errorf("hopcompgate: lease not found")
	// ErrLeaseExpired 补偿租约已过期或已完成，须重新申请。
	ErrLeaseExpired = fmt.Errorf("hopcompgate: lease expired or done")
	// ErrGenerationMismatch 世代号与当前 Case 不符。
	ErrGenerationMismatch = fmt.Errorf("hopcompgate: generation mismatch")
	// ErrTokenMismatch 执行令牌与租约不符。
	ErrTokenMismatch = fmt.Errorf("hopcompgate: token mismatch")
	// ErrCaseAlreadyClosed 补偿仓已关闭，不得重复补偿。
	ErrCaseAlreadyClosed = fmt.Errorf("hopcompgate: case already closed")
	// ErrCaseBlocked 补偿仓已 blocked，须人工强制后接着。
	ErrCaseBlocked = fmt.Errorf("hopcompgate: case blocked, force-compensate required")
	// ErrIdemConflict 同 idem 的 Case 参数冲突。
	ErrIdemConflict = fmt.Errorf("hopcompgate: hop_idem conflict")
	// ErrReentryRejected 补偿未关闭时同 idem Hop 重入，已拒绝。
	ErrReentryRejected = fmt.Errorf("hopcompgate: hop reentry rejected while compensate open")
	// ErrForceApproverSameAsActor 双人强制补偿时审批人与操作人不得相同。
	ErrForceApproverSameAsActor = fmt.Errorf("hopcompgate: approver must differ from actor")
	// ErrInvalidTransition 不合法的状态迁移。
	ErrInvalidTransition = fmt.Errorf("hopcompgate: invalid state transition")
	// ErrMoveIDMissing Move 交易 ID 缺失。
	ErrMoveIDMissing = fmt.Errorf("hopcompgate: move_id required")
	// ErrCaseOpen 案例尚在 open 状态（非 compensating），不得直接关闭。
	ErrCaseOpen = fmt.Errorf("hopcompgate: case is open")
	// ErrUnavailable 服务不可用（未初始化）。
	ErrUnavailable = fmt.Errorf("hopcompgate: service unavailable")
)
