package clrcreditgate

import "fmt"

var (
	// ErrUnavailable 引擎或 Store 不可用（nil 或未初始化）。
	ErrUnavailable = fmt.Errorf("clrcreditgate: engine unavailable")
	// ErrLeaseNotFound 指定 LeaseID 不存在。
	ErrLeaseNotFound = fmt.Errorf("clrcreditgate: lease not found")
	// ErrLeaseConsumed Lease 已被消费（consumed 终态），拒绝二次 Apply。
	ErrLeaseConsumed = fmt.Errorf("clrcreditgate: lease already consumed")
	// ErrLeaseExpired Lease 已过期（expired 终态），Apply fail-closed。
	ErrLeaseExpired = fmt.Errorf("clrcreditgate: lease expired")
	// ErrLeaseVoid Lease 已废弃（void 终态），Apply fail-closed。
	ErrLeaseVoid = fmt.Errorf("clrcreditgate: lease voided")
	// ErrLeaseItemMismatch Lease.ItemID 与请求 ItemID 不匹配（防止跨条目消费）。
	ErrLeaseItemMismatch = fmt.Errorf("clrcreditgate: lease item_id mismatch")
	// ErrLeaseFenceStale Lease Fence 校验失败（epoch 或 token 过时）。
	ErrLeaseFenceStale = fmt.Errorf("clrcreditgate: lease fence stale")
	// ErrDupApply 同一 ItemID 已有 consumed Lease（ProofCreditOK），拒绝重复贷记。
	ErrDupApply = fmt.Errorf("clrcreditgate: duplicate apply for item")
	// ErrDrainBlocked 批次存在未消费 Lease 或在途 Apply，关批被阻断。
	ErrDrainBlocked = fmt.Errorf("clrcreditgate: close batch blocked by unconsumed leases")
	// ErrBatchNotFound 批次 ID 不存在于 drain 状态表。
	ErrBatchNotFound = fmt.Errorf("clrcreditgate: batch drain state not found")
	// ErrBatchAlreadyClosed 批次排空状态已 closed，不可重复关闭。
	ErrBatchAlreadyClosed = fmt.Errorf("clrcreditgate: batch already closed")
	// ErrInvalidTransition 状态迁移非法（from 不匹配当前状态）。
	ErrInvalidTransition = fmt.Errorf("clrcreditgate: invalid state transition")
	// ErrProofNotFound 证明记录不存在。
	ErrProofNotFound = fmt.Errorf("clrcreditgate: proof not found")
	// ErrAlreadyProven 同 ItemID+Kind 已存在证明，幂等写入保护。
	ErrAlreadyProven = fmt.Errorf("clrcreditgate: proof already exists")
	// ErrItemNotReleased Item 状态不为 released，无法签发 Lease。
	ErrItemNotReleased = fmt.Errorf("clrcreditgate: item not in released state")
	// ErrLeaseAlreadyIssued 同 ItemID 已有 issued（未消费）Lease，禁止重复签发。
	ErrLeaseAlreadyIssued = fmt.Errorf("clrcreditgate: lease already issued for item")
	// ErrActor 操作人不合法（空或形态错误）。
	ErrActor = fmt.Errorf("clrcreditgate: actor required")
)
