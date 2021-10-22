package probetokengate

import (
	"fmt"
	"time"
)

// ─── ProbeTokenLease 探针令牌联合租约 ─────────────────────────────────────
// LeaseState 联合租约状态。
// 状态机（FSM）：
// checked ──(BindLease/写提交首次绑定)──▶ bound
// bound ──(CommitCheck 核验通过，写提交成功)──▶ consumed [终态]
// checked | bound ──(TTL 超限，LeaseExpiryWorker)──▶ expired [终态]
// checked | bound ──(探针翻转/撤令，ProbeFlipRevokeWorker)──▶ revoked [终态]

type LeaseState string

const (
	// LeaseStateChecked Check 放行，租约已签发。等待写提交绑定。
	LeaseStateChecked LeaseState = "checked"
	// LeaseStateBound 写提交已绑定（BindLease 成功）。等待 CommitCheck 最终消费。
	LeaseStateBound LeaseState = "bound"
	// LeaseStateConsumed 写提交 CommitCheck 通过，租约消费完成。终态。
	LeaseStateConsumed LeaseState = "consumed"
	// LeaseStateExpired TTL 超限，租约过期。终态。
	LeaseStateExpired LeaseState = "expired"
	// LeaseStateRevoked 探针翻转或毒价撤令，租约吊销。终态。
	LeaseStateRevoked LeaseState = "revoked"
)

// Closed 判断租约是否为终态（不可再转换）。

  func (s LeaseState) Closed() bool {
	  switch s {
	   case LeaseStateConsumed, LeaseStateExpired, LeaseStateRevoked:
		   return true
	  }
	  return false
  }

// ValidLeaseTransitions 合法租约状态转换表。
var ValidLeaseTransitions = map[LeaseState][]LeaseState{
	LeaseStateChecked: {LeaseStateBound, LeaseStateConsumed, LeaseStateExpired, LeaseStateRevoked},
	LeaseStateBound: {LeaseStateConsumed, LeaseStateExpired, LeaseStateRevoked},
	LeaseStateConsumed: {},
	LeaseStateExpired:  {},
	LeaseStateRevoked: {},
}

// CanTransit 校验状态转换是否合法。

  func CanTransit(from, to LeaseState) bool {
	  for _, t := range ValidLeaseTransitions[from] {
		   if t == to {
			   return true
		  }
	   }
	  return false
   }

// RevokeReason 租约吊销原因。

type RevokeReason string

const (
	// RevokeReasonProbeFlip 探针翻转（MarkProbe/ClearProbe 切换）。
	RevokeReasonProbeFlip RevokeReason = "probe_flip"
	// RevokeReasonPoisonRevoke 毒价令牌撤令。
	RevokeReasonPoisonRevoke RevokeReason = "poison_revoke"
	// RevokeReasonTokenAdvance 令牌世代推进（非撤令场景的正常世代轮替）。
	RevokeReasonTokenAdvance RevokeReason = "token_advance"
	// RevokeReasonManual 管理员手动吊销。
	RevokeReasonManual RevokeReason = "manual"
	// RevokeReasonExpiry TTL 超限自动过期（过期使用 expired 状态，此处仅为标注）。
	RevokeReasonExpiry RevokeReason = "expiry"
)

// ProbeTokenLease 探针+令牌联合租约。
// 不变量：
// - ProbeFingerprint 签发时固化（来自 writegate.WriteGateSnap.Probe.StateFingerprint）；
// CommitCheck 时复验当前探针指纹，不匹配则置 revoked + 落 ProbeTokenDriftProof。
// - TokenGeneration 签发时固化（来自 writegate.WriteGateSnap.Token.Generation）；
// CommitCheck 时复验当前令牌世代，世代抬升或令牌撤令则置 revoked + 落 DriftProof。
// - ScopeKey 唯一绑定一次写作用域（Kind|SubjectID|Chain|Asset）。
// - 终态不可逆：consumed / expired / revoked 一旦写入，Engine 禁止再次转换。

type ProbeTokenLease struct {
	// ID 租约唯一标识（UUID v4），对外称 probe_token_lease_id。
	ID string
	// ScopeKey 写作用域键（Kind|SubjectID|Chain|Asset）。
	ScopeKey string
	// ScopeKind 写作用域类型（withdraw / transfer / settle / redeem）。
	ScopeKind string
	// SubjectID 操作主体 ID。
	SubjectID string
	// Chain 目标链（可为空）。
	Chain string
	// Asset 资产代码（可为空）。
	Asset string
	// ProbeFingerprint 探针状态指纹（签发时固化）。
	ProbeFingerprint string
	// ProbePairID 探针对 ID（签发时固化；IsProbing=false 时为空）。
	ProbePairID string
	// IsProbingAtIssue 签发时探针是否活跃。
	IsProbingAtIssue bool
	// TokenGeneration 令牌世代（签发时固化）。
	TokenGeneration int64
	// TokenHash 令牌指纹（签发时固化）。
	TokenHash string
	// SnapFingerprint 联合快照指纹（WriteGateSnap.SnapFingerprint）。
	SnapFingerprint string
	// State 当前状态。
	State LeaseState
	// IdemKey 幂等键，防重复签发（推荐 = SHA256(scope_key + subject_id + request_id)）。
	IdemKey string
	// IssuedAt 签发时刻（CheckGate 通过时刻）。
	IssuedAt time.Time
	// ExpiresAt 过期时刻（IssuedAt + LeaseTTL）。
	ExpiresAt time.Time
	// BoundAt 写提交绑定时刻（BindLease 时刻）。
	BoundAt *time.Time
	// CommitRef 绑定的写提交单据 ID（BindLease 写入）。
	CommitRef string
	// ConsumedAt 消费完成时刻（CommitCheck 通过时刻）。
	ConsumedAt *time.Time
	// RevokedAt 吊销时刻。
	RevokedAt *time.Time
	// RevokeReason 吊销原因。
	RevokeReason string
	// DriftProofID 关联的 ProbeTokenDriftProof.ID（revoked 路径写入）。
	DriftProofID string
	// CreatedAt 行创建时刻。
	CreatedAt time.Time
	// UpdatedAt 行更新时刻。
	UpdatedAt time.Time
}

// Expired 判断租约是否已超 TTL（不依赖状态位，由 Engine 实时判断）。
  func (l *ProbeTokenLease) Expired(now time.Time) bool {
	  return now.After(l.ExpiresAt)
  }

// ─── ProbeTokenDriftProof 漂移拒放行证明 ──────────────────────────────────
// ProofState 证明状态。
// 状态机：
// open ──(ProofWorker 固化)──▶ proven
// proven ──(管理员确认)──▶ closed [终态]
// open ──(驳回，无需 proven)──▶ closed [终态]

type ProofState string

const (
	// ProofStateOpen 证明已开启，等待固化。
	ProofStateOpen ProofState = "open"
	// ProofStateProven 证明已固化，等待管理员确认。
	ProofStateProven ProofState = "proven"
	// ProofStateClosed 证明已关闭（管理员确认或驳回）。终态。
	ProofStateClosed ProofState = "closed"
)

// Closed 判断证明是否为终态。

   func (s ProofState) Closed() bool {
	   return s == ProofStateClosed
  }

// DriftType 漂移类型（精确标记，供审计）。
type DriftType string

const (
	// DriftTypeProbeFlip 探针翻转（MarkProbe/ClearProbe 在 Check→写提交窗口发生）。
	DriftTypeProbeFlip DriftType = "probe_flip"
	// DriftTypePoisonRevoke 毒价令牌撤令（PoisonRevoke 在 Check→写提交窗口发生）。
	DriftTypePoisonRevoke DriftType = "poison_revoke"
	// DriftTypeTokenAdvance 令牌世代正常推进（非撤令，但世代已变）。
	DriftTypeTokenAdvance DriftType = "token_advance"
)

// ProbeTokenDriftProof 探针令牌漂移拒放行证明。
// 不变量：
// - open 状态下，持旧 LeaseID 的写提交一律拒绝并返回 proof_id。
// - ProofHash 唯一（防重复固化同一证明）；由 ProofWorker 计算写入。
// - 终态 closed 不可逆。
type ProbeTokenDriftProof struct {
	// ID 证明唯一标识（UUID v4）。
	ID string
	// LeaseID 旧 ProbeTokenLease.ID。
	LeaseID string
	// ScopeKey 写作用域键。
	ScopeKey string
	// SubjectID 受影响操作主体 ID。
	SubjectID string
	// DriftType 漂移类型。
	DriftType DriftType
	// OldProbeFingerprint 租约签发时的探针指纹。
	OldProbeFingerprint string
	// NewProbeFingerprint 漂移发现时的探针指纹（probe_flip 时非空）。
	NewProbeFingerprint string
	// OldTokenGeneration 租约签发时的令牌世代。
	OldTokenGeneration int64
	// NewTokenGeneration 漂移发现时的令牌世代（token_advance/poison_revoke 时非空）。
	NewTokenGeneration int64
	// PoisonRevoked 是否因毒价撤令触发（poison_revoke 时为 true）。
	PoisonRevoked bool
	// State 当前状态。
	State ProofState
	// ProofHash SHA-256 hex 证明指纹（唯一）。
	ProofHash string
	// IdemKey 幂等键，防重复开证明。
	IdemKey string
	// OpenedAt 证明开启时刻。
	OpenedAt time.Time
	// ProvenAt 证明固化时刻（nil = 未固化）。
	ProvenAt *time.Time
	// ClosedAt 证明关闭时刻（nil = 未关闭）。
	ClosedAt *time.Time
	// ClosedBy 关闭操作人 ID。
	ClosedBy string
	// Notes 关闭备注。
	Notes string
	// CreatedAt 行创建时刻。
	CreatedAt time.Time
	// UpdatedAt 行更新时刻。
	UpdatedAt time.Time
}

// ─── WriteScopeBind 写作用域绑定记录 ──────────────────────────────────────
// WriteScopeBind 写提交-联合租约绑定记录。
// 语义：BindLease 成功后写入，记录 lease_id ↔ commit_ref 映射关系。
// 供审计追溯：给定写提交单据 ID，可查询对应的联合租约及其探针/令牌世代。
// 不变量：
// - (LeaseID, CommitRef) 联合唯一。
// - ConsumedAt 非空时表示 CommitCheck 通过，写提交最终成功。

type WriteScopeBind struct {
	// ID 绑定记录唯一标识（UUID v4）。
	ID string
	// LeaseID 关联 ProbeTokenLease.ID。
	LeaseID string
	// CommitRef 写提交单据 ID（出金/划转/结算批次 ID）。
	CommitRef string
	// ScopeKey 写作用域键（冗余存储，供查询）。
	ScopeKey string
	// SubjectID 操作主体 ID（冗余存储）。
	SubjectID string
	// BoundAt 绑定时刻（BindLease 时刻）。
	BoundAt time.Time
	// ConsumedAt 写提交成功消费时刻（nil = 尚未完成）。
	ConsumedAt *time.Time
	// CreatedAt 行创建时刻。
	CreatedAt time.Time
	// UpdatedAt 行更新时刻。
	UpdatedAt time.Time
}

// ─── 哨兵错误 ─────────────────────────────────────────────────────────────
// ErrDuplicate 主键或幂等键冲突（InsertXxx 遇到已存在记录）。
var ErrDuplicate = fmt.Errorf("probetokengate duplicate record")

// ErrNotFound 记录不存在。

var ErrNotFound = fmt.Errorf("probetokengate record not found")

// ErrLeaseExpired 租约已过期或超 TTL。

var ErrLeaseExpired = fmt.Errorf("probetokengate lease expired")

// ErrLeaseRevoked 租约已吊销（探针翻转或毒价撤令）。

var ErrLeaseRevoked = fmt.Errorf("probetokengate lease revoked")

// ErrLeaseConsumed 租约已消费（不可重复使用）。
var ErrLeaseConsumed = fmt.Errorf("probetokengate lease already consumed")

// ErrNoLease 写提交未携带 probe_token_lease_id（fail-closed）。

var ErrNoLease = fmt.Errorf("probetokengate probe_token_lease_id required")

// ErrProbeDrift 探针状态漂移（probe_flip 窗口）。
var ErrProbeDrift = fmt.Errorf("probetokengate probe state drifted")

// ErrTokenDrift 令牌世代漂移（poison_revoke 或 token_advance）。
var ErrTokenDrift = fmt.Errorf("probetokengate gen token drifted")

// ErrScopeMismatch 写作用域与租约绑定不符。
var ErrScopeMismatch = fmt.Errorf("probetokengate scope mismatch")

// ErrSubjectMismatch 操作主体与租约绑定不符。

var ErrSubjectMismatch = fmt.Errorf("probetokengate subject_id mismatch")

// ErrInvalidTransit 非法状态转换。
var ErrInvalidTransit = fmt.Errorf("probetokengate invalid state transition")

// ErrProofBlocked 存在 open ProbeTokenDriftProof，挡写提交。

var ErrProofBlocked = fmt.Errorf("probetokengate blocked by open drift proof")

// ErrCommitRefMismatch 写提交单据 ID 与 bound 状态租约绑定不符。

var ErrCommitRefMismatch = fmt.Errorf("probetokengate commit_ref mismatch")
