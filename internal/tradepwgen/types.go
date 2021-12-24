package tradepwgen

import "time"

// ──────────────────────────────────────────────
// 状态枚举
// ──────────────────────────────────────────────
// GenStatus 密码世代状态。
// active → 当前活跃世代（用户已有有效交易密码）
// superseded → 已被更新世代取代（旧世代）
type GenStatus string

const (
	GenStatusActive GenStatus = "active"
	GenStatusSuperseded GenStatus = "superseded"
)

// LeaseStatus 核验令牌状态。
// open → 已通过核验，令牌活跃（尚未提交）
// consumed → 出金请求已提交，令牌已消费
// revoked → 改密/重置触发吊销，令牌作废
type LeaseStatus string

const (
	LeaseStatusOpen LeaseStatus = "open"
	LeaseStatusConsumed LeaseStatus = "consumed"
	LeaseStatusRevoked  LeaseStatus = "revoked"
)

// IsTerminal 返回 true 表示令牌终态，不可再用。
   func (s LeaseStatus) IsTerminal() bool {
	   return s == LeaseStatusConsumed || s == LeaseStatusRevoked
  }

// DriftStatus 漂移案件状态。
// open → 漂移已检测，待证明
// proven → 证明已固化
// closed → 已收口
type DriftStatus string

const (
	DriftStatusOpen DriftStatus = "open"
	DriftStatusProven DriftStatus = "proven"
	DriftStatusClosed DriftStatus = "closed"
)

// ──────────────────────────────────────────────
// 核心领域模型
// ──────────────────────────────────────────────
// PwGeneration 用户交易密码世代记录。
// 每次 Set/Change/Reset 成功后 Generation 单调+1；CAS 写路径依赖此不变量防止并发竞态。
// 对外只读：UserID、Generation、ActiveLeaseID。

type PwGeneration struct {
	// UserID 用户 ID，与账本用户主键一致。
	UserID string `json:"user_id"`
	// Generation 世代号，从 1 开始单调递增；0 表示用户尚未设置交易密码。
	Generation int64 `json:"generation"`
	// Status 世代状态。
	Status GenStatus `json:"status"`
	// ActiveLeaseID 当前活跃核验令牌 ID（出金已通过但未提交时非空）。
	ActiveLeaseID string `json:"active_lease_id,omitempty"`
	// CoolBound 当前世代冷却是否已生效（markCool 携带正确 pw_gen 且 seclock 已确认）。
	CoolBound bool `json:"cool_bound"`
	// UpdatedAt 世代最后更新时间。
	UpdatedAt time.Time `json:"updated_at"`
	// CreatedAt 世代创建时间。
	CreatedAt time.Time `json:"created_at"`
}

// VerifyLease 出金核验通过后的上下文令牌。
// 每次 RequireForWithdraw 通过后颁发；绑定当前 pw_gen；改密后须吊销同用户全部 open 令牌。
// verify_lease_id 对外只读，供他包 SELECT 引用。
type VerifyLease struct {
	// ID 令牌 ID，对外导出为 verify_lease_id。
	ID string `json:"verify_lease_id"`
	// UserID 用户 ID。
	UserID string `json:"user_id"`
	// PwGen 颁发时绑定的密码世代号。
	PwGen int64 `json:"pw_gen"`
	// Token 令牌字符串（内部随机值），出金请求须携带并核对。不落审计日志。
	Token string `json:"-"`
	// Status 令牌状态。
	Status LeaseStatus `json:"status"`
	// ExpiresAt 有效期；零值表示不过期（环境专用）。
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	// ConsumedAt 消费时间（提交出金时写入）。
	ConsumedAt *time.Time `json:"consumed_at,omitempty"`
	// RevokedAt 吊销时间（改密时写入）。
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 最后更新时间。
	UpdatedAt time.Time `json:"updated_at"`
}

// PwDriftCase 旧 gen 令牌漂移事故证明仓。
// 发现「核验用旧 gen、账本已新 gen」时创建；收口须先固化证明；
// open 状态时禁止颁发新核验令牌。

type PwDriftCase struct {
	// ID 案件 ID，对外导出为 drift_case_id。
	ID string `json:"drift_case_id"`
	// UserID 用户 ID。
	UserID string `json:"user_id"`
	// LeaseID 触发漂移的核验令牌 ID。
	LeaseID string `json:"lease_id"`
	// VerifyGen 核验时令牌绑定的旧世代号。
	VerifyGen int64 `json:"verify_gen"`
	// CurrentGen 检测时账本实际当前世代号。
	CurrentGen int64 `json:"current_gen"`
	// Status 案件状态。
	Status DriftStatus `json:"status"`
	// VerifySnapshot 检测时核验令牌的元数据摘要（JSON），落库后禁止 UPDATE。
	VerifySnapshot string `json:"verify_snapshot"`
	// GenSnapshot 检测时账本世代快照（JSON），落库后禁止 UPDATE。
	GenSnapshot string `json:"gen_snapshot"`
	// ProofPayload 证明固化载荷（JSON）；proven 态后不可修改。
	ProofPayload string `json:"proof_payload,omitempty"`
	// Approver 证明审核人。
	Approver string `json:"approver,omitempty"`
	// Approver2 双人确认第二审核人。
	Approver2 string `json:"approver2,omitempty"`
	// DetectedAt 事故检测时间。
	DetectedAt time.Time `json:"detected_at"`
	// ProvenAt 证明固化时间（如有）。
	ProvenAt *time.Time `json:"proven_at,omitempty"`
	// ClosedAt 收口时间（如有）。
	ClosedAt *time.Time `json:"closed_at,omitempty"`
	// UpdatedAt 最后更新时间。
	UpdatedAt time.Time `json:"updated_at"`
}

// CoolBinding 冷却绑定记录。
// markCool 成功后写入，绑定用户 ID 与当前 pw_gen。
// CoolBound=false 时出金预检可选拦截（配置可关闭）。

type CoolBinding struct {
	// UserID 用户 ID。
	UserID string `json:"user_id"`
	// PwGen 绑定的密码世代号。
	PwGen int64 `json:"pw_gen"`
	// Reason 冷却原因（trading_password_change / trading_password_reset）。
	Reason string `json:"reason"`
	// BoundAt 冷却绑定时间。
	BoundAt time.Time `json:"bound_at"`
}

// ──────────────────────────────────────────────
// 请求/响应 DTO
// ──────────────────────────────────────────────
// IssueLeaseReq 颁发核验令牌请求（RequireForWithdraw 通过后调用）。

type IssueLeaseReq struct {
	// UserID 用户 ID。
	UserID string
	// TTL 令牌有效期（0 表示不过期）。
	TTL time.Duration
}

// ConsumeLeaseReq 消费核验令牌请求（出金提交时调用）。
type ConsumeLeaseReq struct {
	// UserID 用户 ID。
	UserID string
	// LeaseID 令牌 ID。
	LeaseID string
	// Token 令牌字符串。
	Token string
}

// RevokeInflightReq 吊销用户全部在途核验令牌（Change/Reset 时调用）。
type RevokeInflightReq struct {
	// UserID 用户 ID。
	UserID string
	// PwGen 已抬升的新世代号（吊销该世代之前的全部 open 令牌）。
	PwGen int64
}

// ProveThawReq 证明固化请求。
type ProveThawReq = ProveGenDriftReq

// ProveGenDriftReq 漂移案件证明固化请求。

type ProveGenDriftReq struct {
	// CaseID 案件 ID。
	CaseID string
	// ProofPayload 证明载荷（JSON 形态）。
	ProofPayload string
	// Approver 第一审核人。
	Approver string
	// Approver2 第二审核人（双人确认）。
	Approver2 string
}

// CloseDriftReq 漂移案件收口请求。

type CloseDriftReq struct {
	// CaseID 案件 ID。
	CaseID string
	// Approver 收口审核人。
	Approver string
}

// GenQueryResp 世代查询响应。
type GenQueryResp struct {
	// UserID 用户 ID。
	UserID string `json:"user_id"`
	// Generation 当前世代号。
	Generation int64 `json:"generation"`
	// Status 世代状态。
	Status GenStatus `json:"status"`
	// CoolBound 当前世代冷却是否已生效。
	CoolBound bool `json:"cool_bound"`
	// HasOpenDrift 是否存在未关闭漂移案件（为 true 时新核验令牌被阻断）。
	HasOpenDrift bool `json:"has_open_drift"`
	// ActiveLeaseID 当前活跃核验令牌 ID。
	ActiveLeaseID string `json:"active_lease_id,omitempty"`
	// UpdatedAt 世代最后更新时间。
	UpdatedAt time.Time `json:"updated_at"`
}

// DriftEvent 漂移检测事件，由 ConsumeWithCheck 产出，交由 DriftDetector 创建案件。
type DriftEvent struct {
	// UserID 用户 ID。
	UserID string
	// LeaseID 核验令牌 ID。
	LeaseID string
	// VerifyGen 令牌绑定的旧世代号。
	VerifyGen int64
	// CurrentGen 账本当前世代号。
	CurrentGen int64
	// VerifySnapshot 核验令牌快照（JSON）。
	VerifySnapshot string
	// GenSnapshot 账本世代快照（JSON）。
	GenSnapshot string
}
