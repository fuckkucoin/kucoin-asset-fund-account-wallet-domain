package utamodegate

import "fmt"

var (
	// ErrUnavailable 服务或存储层未初始化。
	ErrUnavailable = fmt.Errorf("utamodegate: service unavailable")
	// ErrUserInvalid 用户 ID 非法（空值或包含非法字符）。
	ErrUserInvalid = fmt.Errorf("utamodegate: user id invalid")
	// ErrModeInvalid 模式值非法（必须为 UNIFIED 或 CLASSIC）。
	ErrModeInvalid = fmt.Errorf("utamodegate: mode invalid: must be UNIFIED or CLASSIC")
	// ErrSameMode 切换目标模式与当前模式相同，无需切换。
	ErrSameMode = fmt.Errorf("utamodegate: target mode identical to current mode")
	// ErrAssetInvalid 资产标识非法（空值或超长）。
	ErrAssetInvalid = fmt.Errorf("utamodegate: asset invalid")
	// ErrAmountInvalid 金额字符串非法（空值/科学计数/负数/非数字）。
	ErrAmountInvalid = fmt.Errorf("utamodegate: amount invalid")
	// ErrStaleGen CAS 世代号已过期，另一并发操作抢先更新了世代。
	// 调用方应读取最新世代后重试；建议不超过 3 次。
	ErrStaleGen = fmt.Errorf("utamodegate: stale generation: CAS conflict, retry with latest gen")
	// ErrInflightLiability 存在未结 LiabilityBind（借出/抵押在途），SetMode 被互斥阻塞。
	// 只有所有在途负债/抵押释放后，模式切换才允许接着。
	// 切换请求进入待切换队列（ModeLease.Status = blocked）。
	ErrInflightLiability = fmt.Errorf("utamodegate: inflight liability/collateral mutex: mode switch blocked")
	// ErrDriftBlocked 存在 open 或 proven ModeDriftCase，后续模式切换与高风险借出被挡写。
	// 挡写持续至四眼收口后 DriftCase 推进至 closed。
	ErrDriftBlocked = fmt.Errorf("utamodegate: mode drift case open: switch/borrow blocked pending four-eye sign-off")
	// ErrLeaseNotFound 指定 lease_id 不存在。
	ErrLeaseNotFound = fmt.Errorf("utamodegate: mode lease not found")
	// ErrLeaseStale 操作目标租约的状态已被其他操作推进，不符当前期望状态。
	ErrLeaseStale = fmt.Errorf("utamodegate: mode lease stale or already terminal")
	// ErrBindNotFound 指定 bind_id 或 ref_id 对应的 LiabilityBind 不存在。
	ErrBindNotFound = fmt.Errorf("utamodegate: liability bind not found")
	// ErrBindAlreadyReleased 试图重复释放已处于 released 状态的 LiabilityBind。
	ErrBindAlreadyReleased = fmt.Errorf("utamodegate: liability bind already released")
	// ErrDriftCaseNotFound 指定 drift_case_id 不存在。
	ErrDriftCaseNotFound = fmt.Errorf("utamodegate: mode drift case not found")
	// ErrDriftAlreadyClosed 试图重复关闭已处于 closed 状态的 DriftCase。
	ErrDriftAlreadyClosed = fmt.Errorf("utamodegate: mode drift case already closed")
	// ErrProofMissing 漂移案尚无有效证明，不得关闭（四眼拒绝收口）。
	ErrProofMissing = fmt.Errorf("utamodegate: drift proof missing: cannot close case without verified proof")
	// ErrProofDigestMismatch 证明字段与摘要重计算不一致，证明内容已被篡改。
	ErrProofDigestMismatch = fmt.Errorf("utamodegate: drift proof digest mismatch: proof integrity violated")
	// ErrProofNotFound 指定 proof_id 不存在。
	ErrProofNotFound = fmt.Errorf("utamodegate: drift proof not found")
	// ErrFourEyeRequired 操作需要两名不同操作员确认（四眼原则），当前仅有单人确认。
	ErrFourEyeRequired = fmt.Errorf("utamodegate: four-eye sign-off required: two distinct operators needed")
	// ErrDuplicateConfirm 同一操作员重复提交确认（四眼拒绝）。
	ErrDuplicateConfirm = fmt.Errorf("utamodegate: duplicate four-eye confirm from same operator")
	// ErrOperatorInvalid 操作员标识非法（空值或超长）。
	ErrOperatorInvalid = fmt.Errorf("utamodegate: operator id invalid")
	// ErrPendingQueueFull 待切换队列已达容量上限，暂时无法入队。
	ErrPendingQueueFull = fmt.Errorf("utamodegate: pending mode-switch queue full")
	// ErrDriftCaseConflict 同用户+资产+Kind 已存在 open DriftCase，幂等保护。
	ErrDriftCaseConflict = fmt.Errorf("utamodegate: drift case already open for this user/asset/kind")
)
