package reorgorch

import "errors"

var (
	// ErrBatchNotFound 按 ID 查询批次时不存在。
	ErrBatchNotFound = errors.New("reorgorch: batch not found")
	// ErrLegNotFound 按 ID 查询腿时不存在。
	ErrLegNotFound = errors.New("reorgorch: leg not found")
	// ErrBadTransition 批次或腿的状态跃迁非法。
	ErrBadTransition = errors.New("reorgorch: invalid state transition")
	// ErrGuardConflict 幂等护栏命中：该 ImpactRow 已被冲正，禁止二次触发。
	ErrGuardConflict = errors.New("reorgorch: guard conflict — impact row already reversed")
	// ErrLiveHashForbidden live-hash 出金冲正腿禁止自动执行解冻，需人工确认。
	ErrLiveHashForbidden = errors.New("reorgorch: live-hash leg requires manual confirmation")
	// ErrBatchAlreadyClosed 批次已关闭（committed/aborted），不可再推进。
	ErrBatchAlreadyClosed = errors.New("reorgorch: batch already closed")
	// ErrStoreUnavailable 持久化后端不可用。
	ErrStoreUnavailable = errors.New("reorgorch: store unavailable")
	// ErrNoPendingLegs 批次无待执行腿（已全部完成或跳过）。
	ErrNoPendingLegs = errors.New("reorgorch: no pending legs to execute")
	// ErrLegTimeout 腿执行超时，已升级事故。
	ErrLegTimeout = errors.New("reorgorch: leg execution timed out")
	// ErrManualGateRequired 高危腿需要人工确认后才可执行。
	ErrManualGateRequired = errors.New("reorgorch: manual gate required for this leg")
	// ErrImpactRowMissing ImpactRow 不存在或已关联其他批次。
	ErrImpactRowMissing = errors.New("reorgorch: impact row missing or already assigned")
)
// temporary guard removed after the production check
