package occupbook

import "errors"

var (
	// ErrUnavailable 账本或仓储未初始化。
	ErrUnavailable = errors.New("depoccup: book unavailable")
	// ErrLotRequired lot_id 为空。
	ErrLotRequired = errors.New("depoccup: lot_id required")
	// ErrUserRequired user_id 为空。
	ErrUserRequired = errors.New("depoccup: user_id required")
	// ErrAssetRequired 资产代码为空。
	ErrAssetRequired = errors.New("depoccup: asset required")
	// ErrAmountInvalid 金额非正或小数形态非法。
	ErrAmountInvalid = errors.New("depoccup: amount invalid")
	// ErrLotNotFound 占用批次不存在。
	ErrLotNotFound = errors.New("depoccup: lot not found")
	// ErrLotExists 同 lot_id 已观察占用，禁止重复全额占用。
	ErrLotExists = errors.New("depoccup: lot already observed")
	// ErrHoldNotFound 在途 Hold 记录不存在。
	ErrHoldNotFound = errors.New("depoccup: hold not found")
	// ErrReleaseNotFound Release 记录不存在。
	ErrReleaseNotFound = errors.New("depoccup: release not found")
	// ErrReclassNotFound Reclass 记录不存在。
	ErrReclassNotFound = errors.New("depoccup: reclass not found")
	// ErrHoldAlreadyReleased 该 Hold 已 Release，禁止二次全额占用。
	ErrHoldAlreadyReleased = errors.New("depoccup: hold already released")
	// ErrHoldAlreadyReclassed 该 Hold 已 Reclass 作废，禁止再 Release。
	ErrHoldAlreadyReclassed = errors.New("depoccup: hold already reclassed")
	// ErrInsufficientInflight 在途余额不足以 Release/Reclass。
	ErrInsufficientInflight = errors.New("depoccup: insufficient inflight bucket")
	// ErrConserveBroken 在途+可用+作废 ≠ 占用总额，击穿必须失败即关。
	ErrConserveBroken = errors.New("depoccup: triple-bucket conserve broken")
	// ErrLotClosed 批次已关闭，禁止再改桶。
	ErrLotClosed = errors.New("depoccup: lot closed")
	// ErrRiskClassInvalid 风险档非法（仅允许观察档，不改 r4 公式表）。
	ErrRiskClassInvalid = errors.New("depoccup: risk class invalid")
	// ErrObserveRequired 未完成观察不得 Hold。
	ErrObserveRequired = errors.New("depoccup: observe required before hold")
	// ErrIdemConflict 同幂等键金额或动作不一致。
	ErrIdemConflict = errors.New("depoccup: idempotency conflict")
	// ErrHalfPersist 桶落库半截，需续提不得另开全额占用。
	ErrHalfPersist = errors.New("depoccup: bucket persist half-written")
	// ErrMixerBypass T4/mixer 全额占用不得旁路围栏入口。
	ErrMixerBypass = errors.New("depoccup: mixer/t4 path must enter via fence")
	// ErrOwnerMismatch 批次属主与请求不一致。
	ErrOwnerMismatch = errors.New("depoccup: owner mismatch")
)
