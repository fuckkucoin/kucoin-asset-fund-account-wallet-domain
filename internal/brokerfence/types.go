package brokerfence

import "time"

// ──────────────────────────────────────────────
// 状态枚举
// ──────────────────────────────────────────────
// LeaseState 围栏 Lease 状态机。
// open → 初始化，尚未激活
// active → 围栏已生效，禁止划拨
// lifting → 解围申请中，等待守恒证明
// sealed → 已封存（永久，含强制关闭）
type LeaseState string

const (
	LeaseStateOpen LeaseState = "open"
	LeaseStateActive  LeaseState = "active"
	LeaseStateLifting LeaseState = "lifting"
	LeaseStateSealed LeaseState = "sealed"
)

// BypassState 旁路案件状态机。
// open → 已发现，待审核
// proven → 证明已确认
// closed → 已关闭（双人确认或自动收口）

type BypassState string

const (
	BypassStateOpen   BypassState = "open"
	BypassStateProven BypassState = "proven"
	BypassStateClosed BypassState = "closed"
)

// BypassKind 旁路类型。

type BypassKind string

const (
	BypassKindToSub    BypassKind = "to_sub"
	BypassKindToMaster BypassKind = "to_master"
	BypassKindCreditMaster BypassKind = "credit_master"
	BypassKindFill         BypassKind = "fill"
)

// ──────────────────────────────────────────────
// 核心领域模型
// ──────────────────────────────────────────────
// FenceGeneration 围栏世代令牌。
// 每次 Fence/Unfence 世代号单调+1。写路径必须携带当前世代且与库中一致。
type FenceGeneration struct {
	SubID      string `json:"sub_id"`
	Generation int64  `json:"generation"` // 单调递增世代号
	Fenced      bool       `json:"fenced"` // 当前是否处于围栏态
	FencedSince *time.Time `json:"fenced_since,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// FenceLease 围栏租约，绑定子账、master、围栏时段与状态机。
// id 字段对外导出为 fence_lease_id，供他包只读引用。

type FenceLease struct {
	ID    string `json:"fence_lease_id"`
	SubID string `json:"sub_id"`
	MasterID   string     `json:"master_id"`
	Asset      string     `json:"asset"`
	Generation int64      `json:"generation"` // 该次围栏对应世代
	State      LeaseState `json:"state"`
	Actor      string     `json:"actor"`
	ProofID string `json:"proof_id,omitempty"` // 守恒证明 ID
	Reason string `json:"reason,omitempty"`
	OpenedAt time.Time  `json:"opened_at"`
	ActiveAt *time.Time `json:"active_at,omitempty"`
	LiftingAt *time.Time `json:"lifting_at,omitempty"`
	SealedAt  *time.Time `json:"sealed_at,omitempty"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// MutexToken 互斥令牌，持有者在围栏窗口内有限写权限。

type MutexToken struct {
	Token   string `json:"token"`
	LeaseID string `json:"lease_id"`
	SubID   string `json:"sub_id"`
	Actor string `json:"actor"`
	ExpiredAt time.Time `json:"expired_at"`
	IssuedAt  time.Time `json:"issued_at"`
}

// BypassCase 旁路划拨/入账案件。
// bypass_proof_id 对外导出，供他包只读引用。
type BypassCase struct {
	ID string `json:"bypass_proof_id"`
	LeaseID string `json:"lease_id"`
	SubID string `json:"sub_id"`
	MasterID string `json:"master_id"`
	Asset  string      `json:"asset"`
	Amount string      `json:"amount"`
	Kind   BypassKind  `json:"kind"` // to_sub / to_master / credit_master / fill
	State  BypassState `json:"state"`
	HangingAmt string `json:"hanging_amount,omitempty"` // 挂账金额（待收口）
	ApproverOne string `json:"approver_one,omitempty"`
	ApproverTwo string     `json:"approver_two,omitempty"`
	ProvenAt    *time.Time `json:"proven_at,omitempty"`
	ClosedAt    *time.Time `json:"closed_at,omitempty"`
	DetectedAt time.Time `json:"detected_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ConserveProof 主子合计守恒证明快照。

type ConserveProof struct {
	ID      string `json:"id"`
	LeaseID string `json:"lease_id"`
	SubID   string `json:"sub_id"`
	MasterID    string `json:"master_id"`
	Asset       string `json:"asset"`
	BeforeTotal string `json:"before_total"` // 围栏前主+子可用合计
	AfterTotal string `json:"after_total"` // 解围前主+子可用合计
	Delta      string `json:"delta"`       // 漂移量（after-before），正/负均有可能
	IsDrifted bool `json:"is_drifted"` // 漂移超阈值
	SampledAt time.Time `json:"sampled_at"`
	CreatedAt time.Time `json:"created_at"`
}

// ──────────────────────────────────────────────
// 请求/响应 DTO
// ──────────────────────────────────────────────
// FenceReq 发起围栏请求。

type FenceReq struct {
	SubID    string `json:"sub_id"`
	MasterID string `json:"master_id"`
	Asset    string `json:"asset"`
	Actor string `json:"actor"`
	Reason string `json:"reason"`
}

// UnfenceReq 发起解围请求。
type UnfenceReq struct {
	SubID   string `json:"sub_id"`
	LeaseID string `json:"lease_id"`
	Actor   string `json:"actor"`
}

// AllocCheckReq 划拨前世代校验请求。
type AllocCheckReq struct {
	SubID string `json:"sub_id"`
	Generation int64 `json:"generation"`
	Actor string `json:"actor"`
	Token string `json:"token"` // 互斥令牌（围栏期内必填）
}

// BypassConfirmReq 旁路案件双人确认请求。

type BypassConfirmReq struct {
	CaseID string `json:"case_id"`
	Approver string `json:"approver"`
	Step int `json:"step"` // 1 = 第一审，2 = 第二审
}

// GenerationQueryResp 世代查询响应。
type GenerationQueryResp struct {
	FenceGeneration
	ActiveLeaseID string `json:"active_lease_id,omitempty"`
}
