package destlife

import "errors"

var (
	// ErrLeaseNotFound 在途世代租约不存在。
	ErrLeaseNotFound = errors.New("destlife: lease not found")
	// ErrRefConflict 同一 movement_id 在不同 entry 已有活跃引用，跨 entry 冲突。
	ErrRefConflict = errors.New("destlife: ref conflict across entries")
	// ErrRefAlreadyReleased 引用已释放，不可重复释放或绑定。
	ErrRefAlreadyReleased = errors.New("destlife: ref already released")
	// ErrEntryRevoked 条目已处于 revoked 状态，拒绝新引用。
	ErrEntryRevoked = errors.New("destlife: entry is revoked")
	// ErrEntryRevoking 条目正在 revoking，不允许在广播路径接着。
	ErrEntryRevoking = errors.New("destlife: entry is revoking")
	// ErrStaleGen 持旧 inflight_gen 广播，对应引用已释放或世代已抬升。
	ErrStaleGen = errors.New("destlife: stale inflight generation")
	// ErrRevokeMutex 存在活跃引用时不允许立即完成 Revoke（需先进入 revoking 状态）。
	ErrRevokeMutex = errors.New("destlife: revoke mutex: active refs present")
	// ErrProofMissing 广播路径缺少在途证明，不得放行。
	ErrProofMissing = errors.New("destlife: inflight proof missing")
	// ErrDriftBlocked 该条目存在未关闭漂移案例，广播路径被阻断。
	ErrDriftBlocked = errors.New("destlife: entry blocked by open drift case")
	// ErrStore 持久化层故障（不可伪装为条目缺失）。
	ErrStore = errors.New("destlife: store error")
	// ErrInvalidState 状态机非法转换。
	ErrInvalidState = errors.New("destlife: invalid state transition")
	// ErrMovementIDEmpty movement_id 不得为空。
	ErrMovementIDEmpty = errors.New("destlife: movement_id required")
	// ErrEntryIDEmpty entry_id 不得为空。
	ErrEntryIDEmpty = errors.New("destlife: entry_id required")
)
