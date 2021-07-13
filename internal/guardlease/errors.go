package guardlease

import "fmt"

var (
	// ErrLeaseNotFound 租约不存在。
	ErrLeaseNotFound = fmt.Errorf("guardlease: snap lease not found")
	// ErrLeaseExpired Conserve 时租约已超期；fail-closed 语义，拒绝守恒。
	ErrLeaseExpired = fmt.Errorf("guardlease: snap lease expired")
	// ErrLeaseConsumed 租约已被消费，不可复用。
	ErrLeaseConsumed = fmt.Errorf("guardlease: snap lease already consumed")
	// ErrLeaseInvalidState 租约状态不允许当前操作。
	ErrLeaseInvalidState = fmt.Errorf("guardlease: snap lease invalid state")
	// ErrStaleSnap 活账并发写与快照世代冲突；调用方必须重新 Gate/Snapshot。
	ErrStaleSnap = fmt.Errorf("guardlease: stale snapshot — concurrent write detected, re-snapshot required")
	// ErrSnapHashMismatch 携带的快照哈希与租约记录不匹配。
	ErrSnapHashMismatch = fmt.Errorf("guardlease: snapshot hash mismatch")
	// ErrPartialImbalance 部分批次金额失衡，计数正确但金额守恒失败。
	ErrPartialImbalance = fmt.Errorf("guardlease: partial batch amount imbalance")
	// ErrConserveFail 守恒证明失败，系统进入挡写保护模式。
	ErrConserveFail = fmt.Errorf("guardlease: conserve proof failed — write blocked until four-eyes clear")
	// ErrHalted 系统处于 halt 状态，拒绝所有写操作。
	ErrHalted = fmt.Errorf("guardlease: system halted by conserve proof failure")
	// ErrInvalidUser 用户 ID 非法。
	ErrInvalidUser = fmt.Errorf("guardlease: invalid user id")
	// ErrInvalidAsset 资产代码非法。
	ErrInvalidAsset = fmt.Errorf("guardlease: invalid asset code")
	// ErrInvalidActor 操作者 ID 非法。
	ErrInvalidActor = fmt.Errorf("guardlease: invalid actor id")
	// ErrInvalidAmount 金额形态非法。
	ErrInvalidAmount = fmt.Errorf("guardlease: invalid amount")
	// ErrFourEyesRequired 操作需要两名不同操作者。
	ErrFourEyesRequired = fmt.Errorf("guardlease: four-eyes clearance requires two distinct actors")
	// ErrProofNotFound 守恒证明记录不存在。
	ErrProofNotFound = fmt.Errorf("guardlease: conserve proof not found")
	// ErrImbalanceNotFound 失衡记录不存在。
	ErrImbalanceNotFound = fmt.Errorf("guardlease: partial imbalance not found")
	// ErrUnavailable 服务不可用。
	ErrUnavailable = fmt.Errorf("guardlease: service unavailable")
)
