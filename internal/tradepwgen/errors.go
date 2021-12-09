package tradepwgen

import "errors"

var (
	// ErrStore 持久化层不可用。
	ErrStore = errors.New("tradepwgen: store unavailable")
	// ErrUserRequired 用户 ID 为空。
	ErrUserRequired = errors.New("tradepwgen: user_id required")
	// ErrGenConflict 世代 CAS 冲突，调用方须重试或报竞态。
	ErrGenConflict = errors.New("tradepwgen: generation CAS conflict")
	// ErrLeaseNotFound 核验令牌不存在或已过期。
	ErrLeaseNotFound = errors.New("tradepwgen: verify lease not found")
	// ErrLeaseRevoked 核验令牌已被吊销（改密后旧令牌失效）。
	ErrLeaseRevoked = errors.New("tradepwgen: verify lease revoked")
	// ErrLeaseConsumed 核验令牌已消费（幂等重放）。
	ErrLeaseConsumed = errors.New("tradepwgen: verify lease already consumed")
	// ErrLeaseExpired 核验令牌已超时。
	ErrLeaseExpired = errors.New("tradepwgen: verify lease expired")
	// ErrLeaseGenMismatch 令牌世代与账本当前世代不符，触发漂移检测。
	ErrLeaseGenMismatch = errors.New("tradepwgen: verify lease generation mismatch")
	// ErrInflightLock 存在进行中核验上下文，Change/Reset 被阻止。
	ErrInflightLock = errors.New("tradepwgen: inflight verify lease blocks password change")
	// ErrDriftOpen 漂移案件（PwDriftCase）未关闭，禁止颁发新核验令牌。
	ErrDriftOpen = errors.New("tradepwgen: pw drift case open, withdraw verify blocked")
	// ErrDriftNotProven 漂移案件尚未固化证明，不可关闭。
	ErrDriftNotProven = errors.New("tradepwgen: pw drift case not yet proven")
	// ErrDriftAlreadyClosed 漂移案件已关闭，重复关闭幂等。
	ErrDriftAlreadyClosed = errors.New("tradepwgen: pw drift case already closed")
	// ErrCoolNotBound 冷却未与当前世代绑定，改密不视为完成。
	ErrCoolNotBound = errors.New("tradepwgen: cool lease not bound to current generation")
	// ErrInvalidStatus 非法状态迁移。
	ErrInvalidStatus = errors.New("tradepwgen: invalid status transition")
)
