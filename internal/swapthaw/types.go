package swapthaw

import "time"

// ──────────────────────────────────────────────
// 状态枚举
// ──────────────────────────────────────────────
// LeaseStatus FreezeLease 状态机。
// open → 冻额已写入，Lease 活跃
// settled → settle 终态腿胜出，Lease 关闭
// thawed → failUnfreeze 终态腿胜出，Lease 关闭
// blocked → 双释检测触发后强制封堵，等待 ThawCase 收口
type LeaseStatus string

const (
	LeaseStatusOpen    LeaseStatus = "open"
	LeaseStatusSettled LeaseStatus = "settled"
	LeaseStatusThawed LeaseStatus = "thawed"
	LeaseStatusBlocked LeaseStatus = "blocked"
)

// IsTerminal 返回 true 表示该状态为终态，Lease 不再可用。

   func (s LeaseStatus) IsTerminal() bool {
	  return s == LeaseStatusSettled || s == LeaseStatusThawed || s == LeaseStatusBlocked
  }

// ThawCaseStatus ThawCase 状态机。
// open → 事故发现，待证明
// proven → 证明已固化，可以关闭
// closed → 已收口（含强制关闭）
type ThawCaseStatus string

const (
	ThawCaseStatusOpen ThawCaseStatus = "open"
	ThawCaseStatusProven ThawCaseStatus = "proven"
	ThawCaseStatusClosed ThawCaseStatus = "closed"
)

// ThawCaseKind 双释事故类型。

type ThawCaseKind string

const (
	// ThawCaseKindDoubleRelease settle 后又触发 failUnfreeze。
	ThawCaseKindDoubleRelease ThawCaseKind = "double_release"
	// ThawCaseKindOrphanFrozen 冻额无对应活跃 Lease（孤儿冻额）。
	ThawCaseKindOrphanFrozen ThawCaseKind = "orphan_frozen"
	// ThawCaseKindConcurrentThaw 并发解冻竞态，两条腿均声称胜出。
	ThawCaseKindConcurrentThaw ThawCaseKind = "concurrent_thaw"
)

// BindLeg SettleBind 终态腿类型。
type BindLeg string

const (
	BindLegSettle BindLeg = "settle"
	BindLegFailUnfreeze BindLeg = "fail_unfreeze"
)

// ──────────────────────────────────────────────
// 核心领域模型
// ──────────────────────────────────────────────
// FreezeGeneration 订单级冻额世代令牌。
// 每次 Execute（冻额动作）将 generation 单调+1；CAS 写路径依赖此不变量防止并发超冻。
// 对外只读字段：OrderID、Generation（供他包引用但不可改）。
type FreezeGeneration struct {
	// OrderID 闪兑订单 ID，与 swap_orders.id 对应。
	OrderID string `json:"order_id"`
	// UserID 用户 ID，用于用户级阻断查询。
	UserID string `json:"user_id"`
	// Generation 单调递增世代号；0 表示从未冻额。
	Generation int64 `json:"generation"`
	// ActiveLeaseID 当前活跃 FreezeLease ID；终态后置空。
	ActiveLeaseID string `json:"active_lease_id,omitempty"`
	// UpdatedAt 最后更新时间。
	UpdatedAt time.Time `json:"updated_at"`
}

// FreezeLease 冻额效果令牌。
// Create/Execute 冻额时创建；failUnfreeze 与 settle 必须携带同一未过期 Lease。
// freeze_lease_id 对外只读，供他包 SELECT 引用。
type FreezeLease struct {
	// ID 冻额效果令牌 ID，对外导出为 freeze_lease_id。
	ID string `json:"freeze_lease_id"`
	// OrderID 对应闪兑订单。
	OrderID string `json:"order_id"`
	// UserID 用户 ID。
	UserID string `json:"user_id"`
	// Asset 被冻资产。
	Asset string `json:"asset"`
	// FrozenAmount 被冻金额（字符串精度 18 位）。
	FrozenAmount string `json:"frozen_amount"`
	// Generation 冻额世代，与 FreezeGeneration.Generation 保持一致。
	Generation int64 `json:"generation"`
	// Token 效果令牌字符串（内部 UUID），failUnfreeze/settle 调用时须携带并核对。
	Token string `json:"-"`
	// Status Lease 状态。
	Status LeaseStatus `json:"status"`
	// ExpiresAt 令牌有效期；为零值表示不过期（仅用）。
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	// WinnerLeg 最终胜出的终态腿类型。
	WinnerLeg BindLeg `json:"winner_leg,omitempty"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 最后更新时间。
	UpdatedAt time.Time `json:"updated_at"`
}

// ThawCase 双释/孤儿冻额事故证明仓。
// 发现 double release 或 orphan frozen 时创建；收口须先固化证明。
// thaw_proof_id 对外只读，供他包 SELECT 引用。
type ThawCase struct {
	// ID 案件 ID，对外导出为 thaw_proof_id（证明确认后同一 ID 保持不变）。
	ID string `json:"thaw_proof_id"`
	// OrderID 触发事故的闪兑订单 ID。
	OrderID string `json:"order_id"`
	// UserID 用户 ID，用于阻断新冻额。
	UserID string `json:"user_id"`
	// LeaseID 对应 FreezeLease ID（如有）。
	LeaseID string `json:"lease_id,omitempty"`
	// Asset 事故资产。
	Asset string `json:"asset"`
	// Kind 事故类型。
	Kind ThawCaseKind `json:"kind"`
	// Status 案件状态。
	Status ThawCaseStatus `json:"status"`
	// DetectedAmount 检测到的双释/孤儿金额（精度 18 位字符串）。
	DetectedAmount string `json:"detected_amount"`
	// FrozenSnapshot 事故时账面 Frozen 余额快照，用于离线核对。禁止事后篡改。
	FrozenSnapshot string `json:"frozen_snapshot"`
	// AvailableSnapshot 事故时账面 Available 余额快照。
	AvailableSnapshot string `json:"available_snapshot"`
	// ProofPayload 证明固化载荷（JSON）；proven 态后不可修改。
	ProofPayload string `json:"proof_payload,omitempty"`
	// Approver 证明审核人。
	Approver string `json:"approver,omitempty"`
	// DetectedAt 事故检测时间。
	DetectedAt time.Time `json:"detected_at"`
	// ProvenAt 证明固化时间（如有）。
	ProvenAt *time.Time `json:"proven_at,omitempty"`
	// ClosedAt 收口时间（如有）。
	ClosedAt *time.Time `json:"closed_at,omitempty"`
	// UpdatedAt 最后更新时间。
	UpdatedAt time.Time `json:"updated_at"`
}

// SettleBind 订单终态腿互斥绑定记录。
// 同一 order_id 只允许写入一次；第二次写入时返回 ErrSettleBindConflict。
// 通过 CAS（INSERT IGNORE / ON CONFLICT DO NOTHING）写入，保证互斥。
type SettleBind struct {
	// ID 绑定记录 ID。
	ID string `json:"id"`
	// OrderID 闪兑订单 ID。
	OrderID string `json:"order_id"`
	// LeaseID 对应 FreezeLease ID。
	LeaseID string `json:"lease_id"`
	// Leg 胜出终态腿类型。
	Leg BindLeg `json:"leg"`
	// Actor 发起终态的请求标识（trace ID 或调用方 ID）。
	Actor string `json:"actor,omitempty"`
	// BoundAt 绑定时间。
	BoundAt time.Time `json:"bound_at"`
}

// DoubleReleaseEvent 双释检测事件，由 Detector 产出，交由 Service 决定是否升 ThawCase。

type DoubleReleaseEvent struct {
	// OrderID 触发事故的闪兑订单 ID。
	OrderID string
	// UserID 用户 ID。
	UserID string
	// LeaseID 对应 FreezeLease ID。
	LeaseID string
	// Asset 事故资产。
	Asset string
	// Kind 事故类型。
	Kind ThawCaseKind
	// IncomingLeg 本次发起的终态腿类型。
	IncomingLeg BindLeg
	// ExistingLeg 已存在的终态腿类型（可能与 IncomingLeg 相同或不同）。
	ExistingLeg BindLeg
	// DetectedAmount 事故金额。
	DetectedAmount string
	// FrozenSnapshot 账面快照。
	FrozenSnapshot string
	// AvailableSnapshot 账面快照。
	AvailableSnapshot string
}

// AcquireLeaseReq 获取/创建 FreezeLease 的请求参数。

type AcquireLeaseReq struct {
	// OrderID 闪兑订单 ID。
	OrderID string
	// UserID 用户 ID。
	UserID string
	// Asset 被冻资产。
	Asset string
	// FrozenAmount 被冻金额。
	FrozenAmount string
	// TTL Lease 有效期（0 表示不过期）。
	TTL time.Duration
}

// ClaimTerminalReq 声明终态腿请求（failUnfreeze 或 settle 时调用）。
type ClaimTerminalReq struct {
	// OrderID 闪兑订单 ID。
	OrderID string
	// LeaseID FreezeLease ID，必须与库中记录匹配。
	LeaseID string
	// LeaseToken 效果令牌字符串，必须与库中 Token 匹配。
	LeaseToken string
	// Leg 本次终态腿类型。
	Leg BindLeg
	// Actor 调用方标识（trace ID）。
	Actor string
	// FrozenSnapshot 当前账面 Frozen 快照，用于事故检测。
	FrozenSnapshot string
	// AvailableSnapshot 当前账面 Available 快照，用于事故检测。
	AvailableSnapshot string
}

// ProveThawCaseReq 证明固化请求。

type ProveThawCaseReq struct {
	// CaseID ThawCase ID。
	CaseID string
	// ProofPayload 证明载荷（JSON 形态）。
	ProofPayload string
	// Approver 证明审核人。
	Approver string
}

// CloseThawCaseReq 收口 ThawCase 请求。
type CloseThawCaseReq struct {
	// CaseID ThawCase ID。
	CaseID string
	// Approver 收口审核人。
	Approver string
}

// GenerationQueryResp 冻额世代查询响应。

type GenerationQueryResp struct {
	// OrderID 订单 ID。
	OrderID string `json:"order_id"`
	// Generation 当前世代号。
	Generation int64 `json:"generation"`
	// ActiveLeaseID 活跃 Lease ID（如有）。
	ActiveLeaseID string `json:"active_lease_id,omitempty"`
	// HasOpenThawCase 是否存在未关闭 ThawCase（为 true 时新冻额被阻断）。
	HasOpenThawCase bool `json:"has_open_thaw_case"`
	// UpdatedAt 世代最后更新时间。
	UpdatedAt time.Time `json:"updated_at"`
}
