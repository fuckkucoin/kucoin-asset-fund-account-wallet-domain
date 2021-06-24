package fundintgate

import "time"

// ─────────────────────────────────────────────────────────────────────────────
// LeaseState — 租约状态机
// ─────────────────────────────────────────────────────────────────────────────
// LeaseState 描述 int_gate_leases 行的生命周期。
type LeaseState string

const (
	// LeaseGated GateFunc 已通过并签发租约，等待 Send 绑定。
	LeaseGated LeaseState = "gated"
	// LeaseBound SendAssert 已复验并绑定本次提交，尚未 SpotSend 落账。
	LeaseBound LeaseState = "bound"
	// LeaseSent SpotSend 已成功，租约终结。
	LeaseSent LeaseState = "sent"
	// LeaseExpired 超过 TTL 未消费，或过期扫描归档。
	LeaseExpired LeaseState = "expired"
	// LeaseRevoked Halt / 换闸 / 禁提抬升 / 运维吊销。
	LeaseRevoked LeaseState = "revoked"
)

// ─────────────────────────────────────────────────────────────────────────────
// ProofKind — 漂移证明类型
// ─────────────────────────────────────────────────────────────────────────────
// ProofKind 区分拒发送证明来源。只追加，不修改。
type ProofKind string

const (
	// DriftNoLease 缺租约仍尝试 Send。
	DriftNoLease ProofKind = "no_lease"
	// DriftExpired 持过期租约 Send。
	DriftExpired ProofKind = "expired"
	// DriftRevoked 持已吊销租约 Send。
	DriftRevoked ProofKind = "revoked"
	// DriftHalt 闸 Halt 后旧租约复验失败。
	DriftHalt ProofKind = "halt"
	// DriftImplSwap 闸实现替换后指纹不一致。
	DriftImplSwap ProofKind = "impl_swap"
	// DriftForbidRaised 禁提标志抬升。
	DriftForbidRaised ProofKind = "forbid_raised"
	// DriftFingerprint 闸指纹漂移（综合）。
	DriftFingerprint ProofKind = "fingerprint"
	// DriftGeneration 闸世代漂移。
	DriftGeneration ProofKind = "generation"
	// DriftResolve 对手解析结果漂移。
	DriftResolve ProofKind = "resolve"
	// DriftAmount 金额/资产漂移。
	DriftAmount ProofKind = "amount"
	// DriftConsumed 已消费租约重放。
	DriftConsumed ProofKind = "consumed"
	// DriftPatrol 未消费租约巡检发现闸面已变。
	DriftPatrol ProofKind = "patrol"
)

// ─────────────────────────────────────────────────────────────────────────────
// AuditAction — 审计动作
// ─────────────────────────────────────────────────────────────────────────────
// AuditAction 记录闸租约生命周期与运维动作。
type AuditAction string

const (
	AuditIssue AuditAction = "issue"
	AuditBind AuditAction = "bind"
	AuditSend AuditAction = "send"
	AuditExpire     AuditAction = "expire"
	AuditRevokeHalt AuditAction = "revoke_halt"
	AuditRevokeSwap AuditAction = "revoke_impl_swap"
	AuditRevokeForbid AuditAction = "revoke_forbid"
	AuditRevokeOps AuditAction = "revoke_ops"
	AuditProof     AuditAction = "proof"
	AuditPatrol    AuditAction = "patrol"
	AuditGenBump AuditAction = "gen_bump"
	AuditRejectMinAmt AuditAction = "reject_min_amount"
	AuditRejectResolve  AuditAction = "reject_resolve"
	AuditRejectSelfXfer AuditAction = "reject_self_xfer"
)

// ─────────────────────────────────────────────────────────────────────────────
// IntGateLease — 闸绑定提交租约
// ─────────────────────────────────────────────────────────────────────────────
// IntGateLease 封存 GateFunc 通过瞬间的 from/to 解析结果、asset、amount、
// 闸指纹与闸世代。Send/SendResolved 必须携带未过期租约并复验上述钉扎。
// 对外只读标识：int_gate_lease_id。
// 表：int_gate_leases
type IntGateLease struct {
	// LeaseID 全局唯一标识（UUID v4），即 int_gate_lease_id。
	LeaseID string `json:"int_gate_lease_id" db:"lease_id"`
	// FromUserID 转出用户。
	FromUserID string `json:"from_user_id" db:"from_user_id"`
	// ToUserID 解析后的收款用户，禁止在 Send 时改写。
	ToUserID string `json:"to_user_id" db:"to_user_id"`
	// ResolveChannel 解析渠道：uid | email | phone。
	ResolveChannel string `json:"resolve_channel" db:"resolve_channel"`
	// ToHint 签发时的对手提示（原始 ToUID/ToEmail/ToPhone 规范化拼接）。
	ToHint string `json:"to_hint" db:"to_hint"`
	// Asset 资产代码。
	Asset string `json:"asset" db:"asset"`
	// Amount 金额（十进制规范串）。
	Amount string `json:"amount" db:"amount"`
	// GateFingerprint 闸实现 + Halt + 禁提 + 策略盐的 SHA-256 hex。
	GateFingerprint string `json:"gate_fingerprint" db:"gate_fingerprint"`
	// GateImplID 闸实现标识（换闸即变）。
	GateImplID string `json:"gate_impl_id" db:"gate_impl_id"`
	// GateGen 签发时的闸世代序号。
	GateGen int64 `json:"gate_gen" db:"gate_gen"`
	// Halted 签发时闸是否 Halt（应为 false 才可签发）。
	Halted bool `json:"halted" db:"halted"`
	// ForbidWithdraw 签发时禁提标志。
	ForbidWithdraw bool `json:"forbid_withdraw" db:"forbid_withdraw"`
	// State 当前状态。
	State LeaseState `json:"state" db:"state"`
	// TTLSeconds 租约有效期；默认 120 秒。
	TTLSeconds int64 `json:"ttl_seconds" db:"ttl_seconds"`
	// IssuedAt 签发时刻（UTC）。
	IssuedAt time.Time `json:"issued_at" db:"issued_at"`
	// BoundAt 绑定 Send 时刻。
	BoundAt *time.Time `json:"bound_at,omitempty" db:"bound_at"`
	// SentAt SpotSend 成功时刻。
	SentAt *time.Time `json:"sent_at,omitempty" db:"sent_at"`
	// ExpiredAt 过期归档时刻。
	ExpiredAt *time.Time `json:"expired_at,omitempty" db:"expired_at"`
	// RevokedAt 吊销时刻。
	RevokedAt *time.Time `json:"revoked_at,omitempty" db:"revoked_at"`
	// RevokeReason 吊销原因。
	RevokeReason string `json:"revoke_reason,omitempty" db:"revoke_reason"`
	// TransferID SpotSend 成功后的账本转账 ID。
	TransferID string `json:"transfer_id,omitempty" db:"transfer_id"`
	// IdempotencyKey 签发幂等键。
	IdempotencyKey string `json:"idempotency_key,omitempty" db:"idempotency_key"`
	// SendIdempotencyKey Send 路径幂等键。
	SendIdempotencyKey string `json:"send_idempotency_key,omitempty" db:"send_idempotency_key"`
	// CreatedAt 行创建时刻。
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	// UpdatedAt 行最后更新时刻。
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// IsConsumable 租约是否仍可进入 Send 绑定。

   func (l *IntGateLease) IsConsumable(now time.Time) bool {
	   if l == nil {
		  return false
	   }
	  if l.State != LeaseGated && l.State != LeaseBound {
		  return false
	  }
	   if now.After(l.ExpiresAt()) {
		   return false
	   }
	  return true
  }

// ExpiresAt 绝对过期时刻。

  func (l *IntGateLease) ExpiresAt() time.Time {
	  if l == nil {
		   return time.Time{}
	   }
	  return l.IssuedAt.Add(time.Duration(l.TTLSeconds) * time.Second)
  }

// Terminal 是否终态。

  func (l *IntGateLease) Terminal() bool {
	  if l == nil {
		  return true
	   }
	  return l.State == LeaseSent || l.State == LeaseExpired || l.State == LeaseRevoked
  }

// ─────────────────────────────────────────────────────────────────────────────
// GateGeneration — 闸世代
// ─────────────────────────────────────────────────────────────────────────────
// GateGeneration 记录闸面单调世代。Halt、换闸实现、禁提抬升均 bump gen，
// 使旧租约在复验时立即失败，并由吊销泵批量迁入 revoked。
// 表：int_gate_generations（与审计分离；任务表清单以 leases/proofs/audit 为主，
// 世代表为闸面单调序号的实现载体）。

type GateGeneration struct {
	// ID 世代行标识。
	ID string `json:"id" db:"id"`
	// Scope 世代作用域，默认 global（全站闸面）。
	Scope string `json:"scope" db:"scope"`
	// Gen 单调递增世代序号，从 1 起。
	Gen int64 `json:"gen" db:"gen"`
	// GateImplID 当前闸实现标识。
	GateImplID string `json:"gate_impl_id" db:"gate_impl_id"`
	// Halted 当前是否 Halt。
	Halted bool `json:"halted" db:"halted"`
	// ForbidWithdraw 当前禁提标志。
	ForbidWithdraw bool `json:"forbid_withdraw" db:"forbid_withdraw"`
	// Fingerprint 当前闸指纹。
	Fingerprint string `json:"fingerprint" db:"fingerprint"`
	// BumpReason 最近一次抬升原因。
	BumpReason string `json:"bump_reason" db:"bump_reason"`
	// BumpedAt 最近抬升时刻。
	BumpedAt time.Time `json:"bumped_at" db:"bumped_at"`
	// BumpedBy 操作来源。
	BumpedBy string `json:"bumped_by" db:"bumped_by"`
}

// ScopeGlobal 默认全站闸面作用域。

const ScopeGlobal = "global"

// ─────────────────────────────────────────────────────────────────────────────
// IntGateDriftProof — 拒发送 / 漂移证明
// ─────────────────────────────────────────────────────────────────────────────
// IntGateDriftProof 在 Send 缺租约、过期、Halt、换闸、禁提、指纹/解析漂移时
// 追加的不可变证明。供 ops 导出与他包只读对账。
// 表：int_gate_drift_proofs

type IntGateDriftProof struct {
	// ProofID 证明唯一标识。
	ProofID string `json:"proof_id" db:"proof_id"`
	// Kind 证明类型。
	Kind ProofKind `json:"kind" db:"kind"`
	// LeaseID 关联租约；缺租约时为空。
	LeaseID string `json:"int_gate_lease_id,omitempty" db:"lease_id"`
	// FromUserID 转出用户。
	FromUserID string `json:"from_user_id" db:"from_user_id"`
	// ToUserID 请求或租约中的收款用户。
	ToUserID string `json:"to_user_id" db:"to_user_id"`
	// Asset 资产。
	Asset string `json:"asset" db:"asset"`
	// Amount 金额。
	Amount string `json:"amount" db:"amount"`
	// BindFingerprint 租约钉扎指纹。
	BindFingerprint string `json:"bind_fingerprint,omitempty" db:"bind_fingerprint"`
	// LiveFingerprint 复验时活闸指纹。
	LiveFingerprint string `json:"live_fingerprint,omitempty" db:"live_fingerprint"`
	// BindGen 租约钉扎世代。
	BindGen int64 `json:"bind_gen" db:"bind_gen"`
	// LiveGen 复验时活世代。
	LiveGen int64 `json:"live_gen" db:"live_gen"`
	// BindToUser 租约钉扎收款方。
	BindToUser string `json:"bind_to_user,omitempty" db:"bind_to_user"`
	// LiveToUser 复验时解析收款方。
	LiveToUser string `json:"live_to_user,omitempty" db:"live_to_user"`
	// Evidence 现场摘要（JSON）。
	Evidence string `json:"evidence" db:"evidence"`
	// DetectedAt 检测时刻。
	DetectedAt time.Time `json:"detected_at" db:"detected_at"`
	// ExportedAt 导出时刻。
	ExportedAt *time.Time `json:"exported_at,omitempty" db:"exported_at"`
	// CreatedAt 行创建时刻。
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// ─────────────────────────────────────────────────────────────────────────────
// IntGateAudit — 审计仓
// ─────────────────────────────────────────────────────────────────────────────
// IntGateAudit 闸租约审计行。表：int_gate_audit
type IntGateAudit struct {
	// AuditID 审计行标识。
	AuditID string `json:"audit_id" db:"audit_id"`
	// LeaseID 关联租约。
	LeaseID string `json:"int_gate_lease_id,omitempty" db:"lease_id"`
	// Action 动作。
	Action AuditAction `json:"action" db:"action"`
	// Actor 操作者。
	Actor string `json:"actor" db:"actor"`
	// Detail 文本细节。
	Detail string `json:"detail" db:"detail"`
	// FromState 迁移前状态。
	FromState string `json:"from_state,omitempty" db:"from_state"`
	// ToState 迁移后状态。
	ToState string `json:"to_state,omitempty" db:"to_state"`
	// CreatedAt 记录时刻。
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// ─────────────────────────────────────────────────────────────────────────────
// 请求 / 结果 DTO
// ─────────────────────────────────────────────────────────────────────────────
// CheckWithLeaseReq GateFunc 通过后签发租约的请求。

type CheckWithLeaseReq struct {
	UserID  string
	ToUID   string
	ToEmail string
	ToPhone string
	Asset string
	Amount string
	TTLSeconds int64
	IdempotencyKey string
	Actor          string
}

// SendAssertReq 绑定租约并尝试 SpotSend 的请求。

type SendAssertReq struct {
	LeaseID string
	UserID string
	ToUID   string
	ToEmail string
	ToPhone string
	Asset   string
	Amount  string
	IdempotencyKey string
	Remark string
	AllowMissingSender bool // 仅：禁止在打开
}

// SendAssertResult 绑定发送结果。成功时 TransferID 非空。
type SendAssertResult struct {
	Lease IntGateLease
	TransferID string
	ToUserID   string
	Channel string
}

// ResolveResult 对手解析只读结果。

type ResolveResult struct {
	ToUserID string
	Channel  string
	Hint string
}

// GateSurface 当前闸面快照（Halt / 实现 / 禁提 / 指纹 / 世代）。
type GateSurface struct {
	ImplID         string
	Halted         bool
	ForbidWithdraw bool
	Fingerprint string
	Gen         int64
}

// OpsRevokeReq 运维吊销请求。

type OpsRevokeReq struct {
	LeaseID string
	Reason  string
	Actor string
}

// ExportProofFilter 证明导出过滤。

type ExportProofFilter struct {
	Kind    ProofKind
	LeaseID string
	Limit   int
}
