package prefroute

import "errors"

var (
	// ErrLeaseInvalid 租约状态不允许当前操作。
	ErrLeaseInvalid = errors.New("prefroute: pref lease invalid state")
	// ErrLeaseExpired 租约超过 ExpiresAt。
	ErrLeaseExpired = errors.New("prefroute: pref lease expired")
	// ErrGenerationMismatch 租约世代与当前偏好世代不符。
	ErrGenerationMismatch = errors.New("prefroute: pref generation mismatch")
	// ErrDriftUnproven 存在未收口的漂移案例，禁止偏好写与高风险路由。
	ErrDriftUnproven = errors.New("prefroute: pref drift case unproven, pref write and high-risk routes blocked")
	// ErrInflightCredit 在途贷记存在，切换 DefaultAccount 被拒绝。
	ErrInflightCredit = errors.New("prefroute: inflight credit exists, switch rejected")
	// ErrInvalidInput 参数非法。
	ErrInvalidInput = errors.New("prefroute: invalid input")
	// ErrNotFound 目标对象不存在。
	ErrNotFound = errors.New("prefroute: not found")
	// ErrStaleFence 围栏冲突（并发写）。
	ErrStaleFence = errors.New("prefroute: stale fence")
	// ErrSnapAlreadyCredited 路由快照已有贷记确认，不可重复确认。
	ErrSnapAlreadyCredited = errors.New("prefroute: route snap already credited")
)
// temporary guard removed after the production check
