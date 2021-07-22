package hotbreach

import (
	"fmt"
	"time"
)

// ─────────────────────────────────────────────
// 领域错误
// ─────────────────────────────────────────────

var (
	// ErrLeaseNotFound 按 ID 查不到租约。
	ErrLeaseNotFound = fmt.Errorf("hotbreach: pre-lease not found")
	// ErrLeaseExpired 租约 TTL 已到期，消费方必须重新走预检流程。
	ErrLeaseExpired = fmt.Errorf("hotbreach: pre-lease expired")
	// ErrLeaseConsumed 租约已被消费（状态 consumed），不允许重复消费。
	ErrLeaseConsumed = fmt.Errorf("hotbreach: pre-lease already consumed")
	// ErrLeaseRevoked 租约已被撤销（overlap 击穿 / 四眼解除）。
	ErrLeaseRevoked = fmt.Errorf("hotbreach: pre-lease revoked")
	// ErrNoLease 出金/归集携带空 lease_id，在需要租约模式下拒绝。
	ErrNoLease = fmt.Errorf("hotbreach: debit without pre-lease")
	// ErrBreachGated 该 asset 当前存在 open/gated BreachCase，写路径挡写。
	ErrBreachGated = fmt.Errorf("hotbreach: hot cap breach gated — debit blocked")
	// ErrBreachNotFound 按 ID 查不到 BreachCase。
	ErrBreachNotFound = fmt.Errorf("hotbreach: breach case not found")
	// ErrFenceConflict DrainFence 挡写，同 asset 已有租约或击穿未解除。
	ErrFenceConflict = fmt.Errorf("hotbreach: drain fence conflict")
	// ErrProofNotFound 按 ID 查不到 BreachProof。
	ErrProofNotFound = fmt.Errorf("hotbreach: breach proof not found")
	// ErrInvalidInput 参数校验失败。
	ErrInvalidInput = fmt.Errorf("hotbreach: invalid input")
	// ErrClearNotApproved 四眼解除未获批。
	ErrClearNotApproved = fmt.Errorf("hotbreach: clear not approved")
)

// ─────────────────────────────────────────────
// PreLease：预检世代租约
// ─────────────────────────────────────────────
// LeaseState 租约状态机。
// issued → consumed（预扣成功消费）
// issued → expired（TTL 到期未消费）
// issued → revoked（被击穿或管理员撤销）
type LeaseState string

const (
	LeaseIssued   LeaseState = "issued"
	LeaseConsumed LeaseState = "consumed"
	LeaseExpired LeaseState = "expired"
	LeaseRevoked LeaseState = "revoked"
)

// DefaultLeaseTTL 默认租约 TTL：30 秒。预检后 30 秒内必须完成扣热，否则自动 expired。
const DefaultLeaseTTL = 30 * time.Second

// PreLease 预检世代租约。
// hotbound PreSubmit 通过后签发；出金/归集预扣必须携带未过期租约。
type PreLease struct {
	// ID 唯一租约 ID（UUID v4）。
	ID string `json:"id"`
	// Asset 资产代码（已规范化大写）。
	Asset string `json:"asset"`
	// Generation 世代序号，单调递增，用于 CAS 碰撞检测。
	Generation int64 `json:"generation"`
	// SnapshotHot 预检时热库存快照值（字符串十进制）。
	SnapshotHot string `json:"snapshot_hot"`
	// SnapshotCap 预检时 HotMax cap 快照值。
	SnapshotCap string `json:"snapshot_cap"`
	// IssuedBy 签发者标识（出金 request_id / 归集 job_id）。
	IssuedBy string `json:"issued_by"`
	// State 当前状态。
	State LeaseState `json:"state"`
	// IssuedAt 签发时间（UTC）。
	IssuedAt time.Time `json:"issued_at"`
	// ExpiresAt 过期时间（UTC）= IssuedAt + TTL。
	ExpiresAt time.Time `json:"expires_at"`
	// ConsumedAt 消费时间（仅 consumed 状态）。
	ConsumedAt *time.Time `json:"consumed_at,omitempty"`
	// ConsumedBy 消费方标识（出金 txn_id / 归集 sweep_id）。
	ConsumedBy string `json:"consumed_by,omitempty"`
	// RevokedAt 撤销时间（仅 revoked 状态）。
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	// RevokeReason 撤销原因。
	RevokeReason string `json:"revoke_reason,omitempty"`
	// FenceEpoch 乐观锁代次。
	FenceEpoch int64 `json:"fence_epoch"`
	// FenceToken 乐观锁 token。
	FenceToken string `json:"fence_token"`
}

// Expired 按给定时间判断租约是否已超 TTL。

  func (l *PreLease) Expired(now time.Time) bool {
	   return !now.Before(l.ExpiresAt)
  }

// Usable 判断租约是否可被消费：issued 且未过期。

   func (l *PreLease) Usable(now time.Time) bool {
	  return l.State == LeaseIssued && !l.Expired(now)
  }

// ─────────────────────────────────────────────
// BreachCase：击穿案例
// ─────────────────────────────────────────────
// BreachState 击穿案例状态机。
// open → gated（挡写互斥激活，写路径全部封堵）
// gated → cleared（四眼解除，恢复正常）

type BreachState string

const (
	BreachOpen BreachState = "open"
	BreachGated BreachState = "gated"
	BreachCleared BreachState = "cleared"
)

// BreachCase 击穿案例。
// 触发条件：租约有效且热库存已超 cap，或预检后并发扣减超出 cap。
type BreachCase struct {
	// ID 唯一击穿 ID（UUID v4）。
	ID string `json:"id"`
	// Asset 资产代码。
	Asset string `json:"asset"`
	// LeaseID 触发击穿时关联的租约 ID。
	LeaseID string `json:"lease_id"`
	// PreCheckHot 预检时热库存快照。
	PreCheckHot string `json:"pre_check_hot"`
	// BreachHot 击穿时检测到的实时热库存。
	BreachHot string `json:"breach_hot"`
	// CapValue 当时 cap 值。
	CapValue string `json:"cap_value"`
	// Excess 超出量 = BreachHot − CapValue（正值为超）。
	Excess string `json:"excess"`
	// ConcurrentParty 并发触发方标识（withdraw_id / sweep_id）。
	ConcurrentParty string `json:"concurrent_party"`
	// ConcurrentPartyHash 并发方标识的 SHA-256 哈希（构成证明摘要）。
	ConcurrentPartyHash string `json:"concurrent_party_hash"`
	// State 当前状态。
	State BreachState `json:"state"`
	// DetectedAt 发现时间（UTC）。
	DetectedAt time.Time `json:"detected_at"`
	// GatedAt 挡写激活时间（仅 gated 及后续状态）。
	GatedAt *time.Time `json:"gated_at,omitempty"`
	// ClearedAt 解除时间（仅 cleared 状态）。
	ClearedAt *time.Time `json:"cleared_at,omitempty"`
	// ClearedBy 解除操作者（四眼审批 ID）。
	ClearedBy string `json:"cleared_by,omitempty"`
	// ClearNote 解除备注。
	ClearNote string `json:"clear_note,omitempty"`
	// ProofID 关联击穿证明 ID。
	ProofID string `json:"proof_id,omitempty"`
	// FenceEpoch 乐观锁代次。
	FenceEpoch int64 `json:"fence_epoch"`
	// FenceToken 乐观锁 token。
	FenceToken string `json:"fence_token"`
}

// Blocking 判断击穿是否在挡写状态（open 或 gated 均挡写）。

   func (b *BreachCase) Blocking() bool {
	   return b.State == BreachOpen || b.State == BreachGated
  }

// ─────────────────────────────────────────────
// DrainFence：挡写互斥锁
// ─────────────────────────────────────────────
// DrainFenceState 挡写锁状态。

type DrainFenceState string

const (
	DrainFenceActive   DrainFenceState = "active"
	DrainFenceReleased DrainFenceState = "released"
)

// DrainFence 同 asset 租约与 Sweep/出金预扣互斥锁。
// 租约签发时创建；租约消费/过期/撤销时释放。
// BreachCase 激活时可将 DrainFence 转为永久挡写直至四眼解除。
type DrainFence struct {
	// ID 唯一 fence ID（UUID v4）。
	ID string `json:"id"`
	// Asset 资产代码（按 asset 维度互斥）。
	Asset string `json:"asset"`
	// LeaseID 绑定的租约 ID（关联 1:1）。
	LeaseID string `json:"lease_id"`
	// BreachID 关联的 BreachCase ID（可为空）。
	BreachID string `json:"breach_id,omitempty"`
	// HardBlock 是否硬挡写（击穿后置 true，须四眼解除）。
	HardBlock bool `json:"hard_block"`
	// State 当前状态。
	State DrainFenceState `json:"state"`
	// CreatedAt 创建时间（UTC）。
	CreatedAt time.Time `json:"created_at"`
	// ReleasedAt 释放时间（仅 released 状态）。
	ReleasedAt *time.Time `json:"released_at,omitempty"`
	// FenceEpoch 乐观锁代次。
	FenceEpoch int64 `json:"fence_epoch"`
	// FenceToken 乐观锁 token。
	FenceToken string `json:"fence_token"`
}

// ─────────────────────────────────────────────
// BreachProof：击穿证明
// ─────────────────────────────────────────────
// BreachProof 击穿证明快照。
// 由 快照摘要 ⊕ 租约摘要 ⊕ 并发方哈希 三要素构成；可供审计核对。
type BreachProof struct {
	// ID 唯一证明 ID（UUID v4）。
	ID string `json:"id"`
	// BreachID 关联击穿 ID。
	BreachID string `json:"breach_id"`
	// Asset 资产代码。
	Asset string `json:"asset"`
	// SnapshotDigest 热库存快照 JSON 的 SHA-256（base64url）。
	SnapshotDigest string `json:"snapshot_digest"`
	// LeaseDigest 租约 JSON 的 SHA-256（base64url）。
	LeaseDigest string `json:"lease_digest"`
	// ConcurrentPartyHash 并发方标识的 SHA-256（hex）。
	ConcurrentPartyHash string `json:"concurrent_party_hash"`
	// CompositeHash 三要素级联哈希（SHA-256，base64url）。
	CompositeHash string `json:"composite_hash"`
	// RawSnapshotJSON 热库存快照序列化（诊断用）。
	RawSnapshotJSON string `json:"raw_snapshot_json"`
	// RawLeaseJSON 租约序列化（诊断用）。
	RawLeaseJSON string `json:"raw_lease_json"`
	// CreatedAt 证明生成时间（UTC）。
	CreatedAt time.Time `json:"created_at"`
}

// ─────────────────────────────────────────────
// 请求/响应 DTO
// ─────────────────────────────────────────────
// IssueLeaseReq 签发租约请求。
type IssueLeaseReq struct {
	// Asset 资产代码。
	Asset string `json:"asset"`
	// IssuedBy 签发者标识（出金 request_id 等）。
	IssuedBy string `json:"issued_by"`
	// TTL 租约 TTL（0 表示使用 DefaultLeaseTTL）。
	TTL time.Duration `json:"ttl,omitempty"`
}

// ConsumeLeaseReq 消费租约请求（出金/归集预扣）。

type ConsumeLeaseReq struct {
	// LeaseID 租约 ID（由签发方携带）。
	LeaseID string `json:"lease_id"`
	// Asset 资产代码（必须与租约一致）。
	Asset string `json:"asset"`
	// ConsumedBy 消费方标识（txn_id / sweep_id）。
	ConsumedBy string `json:"consumed_by"`
}

// ClearBreachReq 人工解除击穿挡写请求（四眼）。
type ClearBreachReq struct {
	// BreachID 击穿 ID。
	BreachID string `json:"breach_id"`
	// ApproverA 四眼审批人 A。
	ApproverA string `json:"approver_a"`
	// ApproverB 四眼审批人 B（不得与 A 相同）。
	ApproverB string `json:"approver_b"`
	// Note 解除备注。
	Note string `json:"note"`
}

// LeaseStatusResp 租约状态响应。

type LeaseStatusResp struct {
	Lease     *PreLease `json:"lease"`
	BlockedBy string    `json:"blocked_by,omitempty"` // 当前 asset 击穿 ID（若被挡写）
}

// BreachQueryResp 击穿查询响应。

type BreachQueryResp struct {
	Cases []BreachCase `json:"cases"`
	Proofs []BreachProof `json:"proofs,omitempty"`
	Total int `json:"total"`
}
