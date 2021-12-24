package wdpartial

import "errors"

var (
	// ─── 通用 ──────────────────────────────────────────────────────────────────
	ErrUnavailable = errors.New("wdpartial: service unavailable")
	// ─── 残留 / 批次 / 续提租约（出金批次残留收口） ──────────────────────────────
	ErrNotFound      = errors.New("wdpartial: residue item not found")
	ErrBatchNotFound = errors.New("wdpartial: batch not found")
	ErrActor   = errors.New("wdpartial: actor identifier required")
	ErrBatchID = errors.New("wdpartial: wd_batch_id required (must be wdbatch ID, not ledbatch)")
	ErrResidueID = errors.New("wdpartial: residue_id required")
	ErrAmount    = errors.New("wdpartial: amount must be positive decimal")
	ErrAsset     = errors.New("wdpartial: asset identifier required")
	ErrUserID = errors.New("wdpartial: user_id required")
	ErrTerminal = errors.New("wdpartial: residue item already in terminal state")
	ErrNotOpen    = errors.New("wdpartial: residue item not in open state")
	ErrBatchState = errors.New("wdpartial: batch not in eligible state for this operation")
	ErrNoLease = errors.New("wdpartial: caller does not hold the resume lease for this batch")
	ErrStaleGen = errors.New("wdpartial: resume lease generation is stale — another worker has advanced the lock")
	ErrLeaseExpired  = errors.New("wdpartial: lease has expired")
	ErrLeaseConflict = errors.New("wdpartial: lease conflict — held by another worker or blocked by fill mutex")
	ErrLeaseNotFound = errors.New("wdpartial: lease not found")
	ErrLeaseSelf = errors.New("wdpartial: caller already holds this resume lease")
	ErrProofDuplicate = errors.New("wdpartial: release proof already recorded for this residue item")
	ErrProofMissing  = errors.New("wdpartial: release proof missing — batch cannot be closed")
	ErrProofChecksum = errors.New("wdpartial: release proof checksum mismatch")
	ErrProofStore = errors.New("wdpartial: proof store unavailable")
	ErrConserve       = errors.New("wdpartial: conserve equation violated after closeout")
	ErrCloseoutExists = errors.New("wdpartial: batch closeout already recorded")
	ErrObserver = errors.New("wdpartial: occupancy observer not configured")
	ErrSourceMismatch = errors.New("wdpartial: source batch must be a wdbatch ID, not ledbatch or withdraw run")
	// ─── 分片Hold/Fill会计（wdshard 部分成交占用子系统） ─────────────────────────
	ErrHoldNotFound = errors.New("wdpartial: hold not found")
	ErrFillNotFound = errors.New("wdpartial: fill not found")
	ErrState        = errors.New("wdpartial: invalid state transition")
	ErrConserveHold  = errors.New("wdpartial: hold amount does not conserve")
	ErrAmountInvalid = errors.New("wdpartial: hold amount invalid")
	ErrAmountZero    = errors.New("wdpartial: amount must be positive")
	ErrNoHold = errors.New("wdpartial: hold id required")
	ErrNoFill = errors.New("wdpartial: fill id required")
	ErrNoPlan = errors.New("wdpartial: plan id required")
	ErrNoLeg  = errors.New("wdpartial: leg id required")
	ErrNoUser = errors.New("wdpartial: user id required")
	ErrNoAsset = errors.New("wdpartial: asset required")
	ErrAlreadyOpen = errors.New("wdpartial: hold already open for this leg")
	ErrLiveHash = errors.New("wdpartial: live-hash hold cannot be auto-released")
	ErrOverConsume = errors.New("wdpartial: consume amount exceeds hold amount")
	// ─── 竞态收口模型：RemReleaseLease 租约 ───────────────────────────────────
	// ErrLeaseNotOpen 租约不处于 open 状态。
	ErrLeaseNotOpen = errors.New("wdpartial: rem_release_lease not open")
	// ErrLeaseGenMismatch 请求世代与租约 rem_release_gen 不符。
	ErrLeaseGenMismatch = errors.New("wdpartial: rem_release_gen mismatch")
	// ErrLeaseDuplicate 同 planID 已存在 open 租约（重复颁发防护）。
	ErrLeaseDuplicate = errors.New("wdpartial: rem_release_lease duplicate for plan")
	// ─── 竞态收口模型：FillMutexToken ─────────────────────────────────────────
	// ErrFillMutexNotFound Fill 互斥令牌不存在。
	ErrFillMutexNotFound = errors.New("wdpartial: fill_mutex_token not found")
	// ErrFillMutexExpired Fill 互斥令牌已过期。
	ErrFillMutexExpired = errors.New("wdpartial: fill_mutex_token expired")
	// ErrFillMutexMismatch 令牌与 Fill/Plan 不匹配。
	ErrFillMutexMismatch = errors.New("wdpartial: fill_mutex_token plan/fill mismatch")
	// ErrFillMutexConsumed 令牌已消费，不可重用。
	ErrFillMutexConsumed = errors.New("wdpartial: fill_mutex_token already consumed")
	// ─── 竞态收口模型：ConserveCase ───────────────────────────────────────────
	// ErrConserveCaseOpen planID 存在 open 守恒案件，挡写 ConsumeHold。
	ErrConserveCaseOpen = errors.New("wdpartial: conserve_case open — consume hold blocked")
	// ErrConserveCaseNotFound 守恒案件不存在。
	ErrConserveCaseNotFound = errors.New("wdpartial: conserve_case not found")
	// ErrConserveCaseAlreadyClosed 守恒案件已收口，无需重复操作。
	ErrConserveCaseAlreadyClosed = errors.New("wdpartial: conserve_case already closed")
	// ErrConserveCaseProofRequired proven 迁移须提供 proof_payload。
	ErrConserveCaseProofRequired = errors.New("wdpartial: proof_payload required for proven transition")
	// ErrConserveCaseNotProven close 操作须先 proven。
	ErrConserveCaseNotProven = errors.New("wdpartial: conserve_case must be proven before close")
	// ─── 竞态收口模型：BroadcastGuardGen ─────────────────────────────────────
	// ErrGuardGenNotFound 守卫世代不存在。
	ErrGuardGenNotFound = errors.New("wdpartial: broadcast_guard_gen not found")
	// ErrGuardGenInvalid 守卫世代已失效，skip 腿不得释放。
	ErrGuardGenInvalid = errors.New("wdpartial: broadcast_guard_gen invalidated — skip release denied")
	// ErrGuardGenConsumed 守卫世代已消费。
	ErrGuardGenConsumed = errors.New("wdpartial: broadcast_guard_gen already consumed")
	// ErrGuardGenMismatch Hold 绑定的守卫世代与请求不符。
	ErrGuardGenMismatch = errors.New("wdpartial: broadcast_guard_gen id mismatch on hold")
	// ─── 竞态收口模型：Hold/Fill 操作 ────────────────────────────────────────
	// ErrHoldNotOpen Hold 不处于 open 状态，无法释放或消费。
	ErrHoldNotOpen = errors.New("wdpartial: hold not open")
	// ErrHoldAlreadyReleased Hold 已释放（幂等返回）。
	ErrHoldAlreadyReleased = errors.New("wdpartial: hold already released")
	// ErrHoldDoubleRelease 同 planID 已释放过该 Hold，双释防护。
	ErrHoldDoubleRelease = errors.New("wdpartial: hold double release denied")
	// ErrFillAlreadyConfirmed Fill 已确认，不可重复。
	ErrFillAlreadyConfirmed = errors.New("wdpartial: fill already confirmed")
	// ErrInvalidStatus 非法状态迁移（竞态收口模型）。
	ErrInvalidStatus = errors.New("wdpartial: invalid status transition")
	// ErrAmountMismatch 金额校验不通过（竞态收口模型）。
	ErrAmountMismatch = errors.New("wdpartial: amount mismatch")
	// ErrConservationFail 守恒校验失败（TotalFilled + Remainder ≠ TotalHeld）。
	ErrConservationFail = errors.New("wdpartial: plan conservation failed")
	// ErrForceReleaseDenied 强制释放请求缺少必要字段（双人确认未完成）。
	ErrForceReleaseDenied = errors.New("wdpartial: force release denied — dual approval required")
)
