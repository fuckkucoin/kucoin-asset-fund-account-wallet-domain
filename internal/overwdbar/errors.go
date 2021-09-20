package overwdbar

import "errors"

var (
	// ErrOverdrawBlocked 超提屏障拒绝：意图未通过 CAS 检验。
	// 调用方应向用户返回余额不足，不得重新冻结或绕过屏障。
	ErrOverdrawBlocked = errors.New("overwdbar: overdraw blocked by CAS barrier")
	// ErrPreCheckRequired Commit 时未找到对应 PreCheck 记录；
	// 必须先调用 PreCheck 再调用 Commit。
	ErrPreCheckRequired = errors.New("overwdbar: precheck required before commit")
	// ErrLeaseRequired 调用方未提供租约 ID。
	ErrLeaseRequired = errors.New("overwdbar: lease_id required")
	// ErrAmountRequired 扣减额必须 > 0。
	ErrAmountRequired = errors.New("overwdbar: amount must be positive")
	// ErrLiveHashFrozen live-hash 冻结拒绝时禁止自动解冻；
	// 屏障仅记录 VarianceEvent，不做任何解冻操作。
	ErrLiveHashFrozen = errors.New("overwdbar: live-hash freeze must not be auto-unfrozen")
)
