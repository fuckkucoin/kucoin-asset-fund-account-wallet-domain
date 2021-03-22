package clrcreditgate

import (
	"time"
)

// ──────────────────────────────────────────────────────────────────────────────
// LeaseState
// ──────────────────────────────────────────────────────────────────────────────
// LeaseState 释放贷记租约状态。

type LeaseState string

const (
	// LeaseIssued 已签发，等待 ApplyWithLease 消费。
	LeaseIssued LeaseState = "issued"
	// LeaseConsumed ApplyWithLease CAS 消费成功，对应唯一一次贷记。
	LeaseConsumed LeaseState = "consumed"
	// LeaseExpired TTL 超时，扫描器标记为过期；过期后 Apply 必须 fail-closed。
	LeaseExpired LeaseState = "expired"
	// LeaseVoid 显式废弃：Credit 失败回滚时、或 ops 手动废弃；可重新签发新租约。
	LeaseVoid LeaseState = "void"
)

// Terminal 报告 Lease 是否已进入终态（expired / void / consumed）。

  func (s LeaseState) Terminal() bool {
	  return s == LeaseConsumed || s == LeaseExpired || s == LeaseVoid
   }

// ──────────────────────────────────────────────────────────────────────────────
// ProofKind
// ──────────────────────────────────────────────────────────────────────────────
// ProofKind 交收证明类型。
type ProofKind string

const (
	// ProofCreditOK 贷记成功证明（Lease consumed + Credit 落账）。
	ProofCreditOK ProofKind = "credit_ok"
	// ProofCreditFail 贷记失败证明（Credit 报错，applied 标记已回滚，Lease 标记 void）。
	ProofCreditFail ProofKind = "credit_fail"
	// ProofDupApply 重复 Apply 拒绝证明（Lease 已消费或重复消费同一 ItemID）。
	ProofDupApply ProofKind = "dup_apply"
	// ProofExpiredLease 过期租约上的 Apply 被拒绝证明。
	ProofExpiredLease ProofKind = "expired_lease"
)

// ──────────────────────────────────────────────────────────────────────────────
// DrainStatus
// ──────────────────────────────────────────────────────────────────────────────
// DrainStatus 批次关闭进度状态。
type DrainStatus string

const (
	// DrainPending 批次尚未进入排空流程（默认状态）。
	DrainPending DrainStatus = "pending"
	// DrainDraining 批次已进入排空，等待所有在途 Lease 消费或过期。
	DrainDraining DrainStatus = "draining"
	// DrainClosed 排空完毕，批次已安全关闭。
	DrainClosed DrainStatus = "closed"
)

// ──────────────────────────────────────────────────────────────────────────────
// ReleaseCreditLease
// ──────────────────────────────────────────────────────────────────────────────
// ReleaseCreditLease 释放贷记租约。
// ReleaseWithLease 在将 Item 标记为 released 的同时签发本租约（issued 状态）；
// ApplyWithLease 携带 LeaseID + Fence 做 CAS 消费；消费后状态变 consumed 且不可逆。
// TTL 由签发时刻起算；扫描器每分钟检测 ExpiresAt < now 的 issued Lease 并标记 expired。
// 已过期的 Lease 上的 Apply 操作必须 fail-closed，不得落账。
type ReleaseCreditLease struct {
	// LeaseID 全局唯一租约 ID（UUID v4）。
	LeaseID string `json:"lease_id"`
	// ItemID 绑定的 clearing.Item.ID（外键引用，只读）。
	ItemID string `json:"item_id"`
	// BatchID 绑定的 clearing.Batch.ID（只读）。
	BatchID string `json:"batch_id"`
	// UserID 受益人 ID（与 Item.UserID 一致）。
	UserID string `json:"user_id"`
	// Asset 资产代码（大写，如 "BTC"）。
	Asset string `json:"asset"`
	// Amount 金额字符串（decimal 精确表示，如 "1.250000000000000000"）。
	Amount string `json:"amount"`
	// IssuedAt 签发时刻（UTC）。
	IssuedAt time.Time `json:"issued_at"`
	// ExpiresAt TTL 截止时刻（UTC）；= IssuedAt + TTL。
	ExpiresAt time.Time `json:"expires_at"`
	// State 当前状态。
	State LeaseState `json:"state"`
	// ConsumedAt Apply 成功消费时刻（UTC）；State≠consumed 时为零值。
	ConsumedAt time.Time `json:"consumed_at,omitempty"`
	// ConsumedByProof 消费本 Lease 的 ProofID（State=consumed 时非空）。
	ConsumedByProof string `json:"consumed_by_proof,omitempty"`
	// ExpiredAt 过期标记时刻（UTC）；State=expired 时非空。
	ExpiredAt time.Time `json:"expired_at,omitempty"`
	// VoidAt void 标记时刻（UTC）；State=void 时非空。
	VoidAt time.Time `json:"void_at,omitempty"`
	// VoidReason void 原因（credit_fail / ops_void / drain_timeout）。
	VoidReason string `json:"void_reason,omitempty"`
	// Fence 行级围栏（epoch+token，CAS 保护 State 迁移）。
	Fence Fence `json:"fence"`
	// Actor 签发操作人（clearing actor）。
	Actor string `json:"actor"`
	// CreatedAt 行创建时刻（UTC，= IssuedAt）。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 最近更新时刻（UTC）。
	UpdatedAt time.Time `json:"updated_at"`
}

// Expired 报告 Lease 在给定时刻是否已超过 TTL（不含状态判断）。

  func (l *ReleaseCreditLease) Expired(now time.Time) bool {
	  if l == nil {
		   return true
	   }
	  return !now.Before(l.ExpiresAt)
  }

// IsActive 报告 Lease 是否处于可消费状态（issued 且未超时）。
   func (l *ReleaseCreditLease) IsActive(now time.Time) bool {
	  return l != nil && l.State == LeaseIssued && !l.Expired(now)
  }

// ──────────────────────────────────────────────────────────────────────────────
// ClearingCreditProof
// ──────────────────────────────────────────────────────────────────────────────
// ClearingCreditProof 交收证明（不可变历史记录）。
// Apply 路径产生三类证明：
// - ProofCreditOK：贷记成功，Lease 已消费。
// - ProofCreditFail：Credit 调用失败，applied 已回滚，Lease 已 void。
// - ProofDupApply：同一 ItemID 重复 Apply，被 CAS 拒绝。
// - ProofExpiredLease：Lease 已过期，Apply fail-closed。
// 证明一经写入不得删除或修改（append-only 日志语义）。

type ClearingCreditProof struct {
	// ProofID 全局唯一证明 ID（UUID v4）。
	ProofID string `json:"proof_id"`
	// LeaseID 触发本证明的 Lease ID；ProofDupApply 时可能为"尝试使用的 LeaseID"。
	LeaseID string `json:"lease_id"`
	// ItemID clearing.Item.ID。
	ItemID string `json:"item_id"`
	// BatchID clearing.Batch.ID。
	BatchID string `json:"batch_id"`
	// UserID 受益人 ID。
	UserID string `json:"user_id"`
	// Asset 资产代码。
	Asset string `json:"asset"`
	// Amount 金额。
	Amount string `json:"amount"`
	// Kind 证明类型（credit_ok / credit_fail / dup_apply / expired_lease）。
	Kind ProofKind `json:"kind"`
	// Detail 补充说明（错误消息或拒绝原因）。
	Detail string `json:"detail,omitempty"`
	// Actor 操作人（clearing actor）。
	Actor string `json:"actor"`
	// At 证明落地时刻（UTC）。
	At time.Time `json:"at"`
	// EpochAtProof Fence.Epoch（供审计溯源）。
	EpochAtProof int64 `json:"epoch_at_proof"`
	// CreatedAt 行创建时刻（UTC）。
	CreatedAt time.Time `json:"created_at"`
}

// ──────────────────────────────────────────────────────────────────────────────
// BatchDrainState
// ──────────────────────────────────────────────────────────────────────────────
// BatchDrainState 批次排空状态快照。
// CloseBatchGuard 在尝试关批前查询本结构；若 UnconsumedLeases>0 或 InFlightApplies>0，
// 必须拒绝关批或切换到 draining 模式等待排空完成。
type BatchDrainState struct {
	// BatchID clearing.Batch.ID。
	BatchID string `json:"batch_id"`
	// Status 排空状态。
	Status DrainStatus `json:"status"`
	// TotalItems 批次内总 Item 数。
	TotalItems int `json:"total_items"`
	// ReleasedItems 已 released（含 applied）的 Item 数。
	ReleasedItems int `json:"released_items"`
	// AppliedItems 已成功 Apply（有 ProofCreditOK 证明）的 Item 数。
	AppliedItems int `json:"applied_items"`
	// UnconsumedLeases 当前 issued（未消费、未过期）的 Lease 数量。
	// > 0 时 CloseBatch 必须阻断。
	UnconsumedLeases int `json:"unconsumed_leases"`
	// InFlightApplies 正在执行 ApplyWithLease 中的并发 Apply 数量。
	// > 0 时 CloseBatch 必须等待或拒绝。
	InFlightApplies int `json:"in_flight_applies"`
	// ExceptedItems 已进入异常队列的 Item 数。
	ExceptedItems int `json:"excepted_items"`
	// DrainStartedAt 排空开始时刻（DrainDraining 时非空）。
	DrainStartedAt time.Time `json:"drain_started_at,omitempty"`
	// DrainTimeoutAt 排空超时时刻（超时后强制 void 未消费 Lease）。
	DrainTimeoutAt time.Time `json:"drain_timeout_at,omitempty"`
	// SnapshotAt 快照采集时刻（UTC）。
	SnapshotAt time.Time `json:"snapshot_at"`
	// Fence 批次排空行级围栏。
	Fence Fence `json:"fence"`
	// CreatedAt 行创建时刻（UTC）。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 最近更新时刻（UTC）。
	UpdatedAt time.Time `json:"updated_at"`
}

// Drainable 报告批次是否可以安全关闭（无未消费 Lease、无在途 Apply）。

   func (d *BatchDrainState) Drainable() bool {
	  if d == nil {
		   return false
	   }
	   return d.UnconsumedLeases == 0 && d.InFlightApplies == 0
   }

// ──────────────────────────────────────────────────────────────────────────────
// LeaseConfig
// ──────────────────────────────────────────────────────────────────────────────
// LeaseConfig 租约签发配置。

type LeaseConfig struct {
	// DefaultTTL 默认租约有效期（默认 10 分钟）。
	DefaultTTL time.Duration
	// DrainTimeout 批次进入 draining 后的最大等待时间（默认 5 分钟）。
	DrainTimeout time.Duration
}

// Fill 填充默认值。
  func (c *LeaseConfig) Fill() {
	  if c.DefaultTTL <= 0 {
		   c.DefaultTTL = 10 * time.Minute
	  }
	   if c.DrainTimeout <= 0 {
		   c.DrainTimeout = 5 * time.Minute
	   }
  }

// ──────────────────────────────────────────────────────────────────────────────
// ReleaseWithLeaseResult
// ──────────────────────────────────────────────────────────────────────────────
// ReleaseWithLeaseResult ReleaseWithLease 的返回结构。
type ReleaseWithLeaseResult struct {
	// ItemID 被释放的条目 ID。
	ItemID string `json:"item_id"`
	// BatchID 批次 ID。
	BatchID string `json:"batch_id"`
	// Lease 签发的租约（State=issued）。
	Lease *ReleaseCreditLease `json:"lease"`
}

// ──────────────────────────────────────────────────────────────────────────────
// ApplyWithLeaseResult
// ──────────────────────────────────────────────────────────────────────────────
// ApplyWithLeaseResult ApplyWithLease 的返回结构。

type ApplyWithLeaseResult struct {
	// ItemID 条目 ID。
	ItemID string `json:"item_id"`
	// LeaseID 被消费的 Lease ID。
	LeaseID string `json:"lease_id"`
	// Proof 交收证明。
	Proof *ClearingCreditProof `json:"proof"`
	// PositionAfter 贷记后的账本头寸（Position 序列化为字符串映射）。
	PositionAfter map[string]string `json:"position_after,omitempty"`
}

// ──────────────────────────────────────────────────────────────────────────────
// CloseBatchGuardResult
// ──────────────────────────────────────────────────────────────────────────────
// CloseBatchGuardResult CloseBatchGuard 的返回结构。

type CloseBatchGuardResult struct {
	// BatchID 批次 ID。
	BatchID string `json:"batch_id"`
	// Allowed 是否允许关批（true=已排空，false=阻断）。
	Allowed bool `json:"allowed"`
	// DrainState 当前排空快照。
	DrainState *BatchDrainState `json:"drain_state"`
	// Reason 阻断原因（Allowed=false 时非空）。
	Reason string `json:"reason,omitempty"`
}

// ──────────────────────────────────────────────────────────────────────────────
// LeaseListFilter
// ──────────────────────────────────────────────────────────────────────────────
// LeaseListFilter Lease 查询过滤参数。

type LeaseListFilter struct {
	// BatchID 按批次过滤（空=不过滤）。
	BatchID string
	// ItemID 按条目过滤（空=不过滤）。
	ItemID string
	// States 按状态列表过滤（空=返回所有状态）。
	States []LeaseState
	// ExpiredBefore 返回 expires_at < ExpiredBefore 的 Lease（零值=不过滤）。
	ExpiredBefore time.Time
	// IssuedBefore 返回 issued_at < IssuedBefore 的 Lease（零值=不过滤）。
	IssuedBefore time.Time
	// Limit 最大返回数量（0=默认 200）。
	Limit int
}

// Fill 填充默认值。

   func (f *LeaseListFilter) Fill() {
	   if f.Limit <= 0 {
		  f.Limit = 200
	  }
   }

// ──────────────────────────────────────────────────────────────────────────────
// ProofListFilter
// ──────────────────────────────────────────────────────────────────────────────
// ProofListFilter Proof 查询过滤参数。

type ProofListFilter struct {
	// BatchID 按批次过滤（空=不过滤）。
	BatchID string
	// ItemID 按条目过滤（空=不过滤）。
	ItemID string
	// Kinds 按类型列表过滤（空=返回所有类型）。
	Kinds []ProofKind
	// Limit 最大返回数量（0=默认 200）。
	Limit int
}

// Fill 填充默认值。

   func (f *ProofListFilter) Fill() {
	   if f.Limit <= 0 {
		   f.Limit = 200
	  }
   }

// ──────────────────────────────────────────────────────────────────────────────
// ScanResult
// ──────────────────────────────────────────────────────────────────────────────
// ScanResult 扫描器单次运行结果。
type ScanResult struct {
	// Scanned 本轮扫描的 Lease 或 BatchDrainState 数量。
	Scanned int `json:"scanned"`
	// Expired 本轮标记为过期的 Lease 数量。
	Expired int `json:"expired"`
	// Voided 本轮标记为 void 的 Lease 数量（drain 超时强制废弃）。
	Voided int `json:"voided"`
	// DrainClosed 本轮完成排空关闭的批次数量。
	DrainClosed int `json:"drain_closed"`
	// Errors 本轮遇到的错误数量。
	Errors int `json:"errors"`
	// At 扫描时刻（UTC）。
	At time.Time `json:"at"`
}
