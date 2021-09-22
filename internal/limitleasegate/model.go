package limitleasegate

import (
	"fmt"
	"time"
)

// ─── LimitCheckLease 限额预检租约 ─────────────────────────────────────────
// LeaseState 限额预检租约状态。
// 状态机（FSM）：
// checked ──(BindLease/占用提交首次绑定)──▶ bound
// bound ──(CommitRecheck 核验通过，占用成功)──▶ consumed [终态]
// checked | bound ──(TTL 超限，LeaseExpiryWorker)──▶ expired [终态]
// checked | bound ──(PolicyVer++/TodaySum越限)──▶ revoked [终态]

type LeaseState string

const (
	// LeaseStateChecked Check 放行，租约已签发。等待占用提交绑定。
	LeaseStateChecked LeaseState = "checked"
	// LeaseStateBound 占用提交已绑定（BindLease 成功）。等待 CommitRecheck 最终消费。
	LeaseStateBound LeaseState = "bound"
	// LeaseStateConsumed 占用提交 CommitRecheck 通过，租约消费完成。终态。
	LeaseStateConsumed LeaseState = "consumed"
	// LeaseStateExpired TTL 超限，租约过期。终态。
	LeaseStateExpired LeaseState = "expired"
	// LeaseStateRevoked PolicyVer 翻转或 TodaySum 越限，租约吊销。终态。
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
	LeaseStateChecked:  {LeaseStateBound, LeaseStateConsumed, LeaseStateExpired, LeaseStateRevoked},
	LeaseStateBound:    {LeaseStateConsumed, LeaseStateExpired, LeaseStateRevoked},
	LeaseStateConsumed: {},
	LeaseStateExpired:  {},
	LeaseStateRevoked:  {},
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
	// RevokeReasonPolicyFlip 策略代次翻转（PolicyVer++ 在 Check→占用提交窗口发生）。
	RevokeReasonPolicyFlip RevokeReason = "policy_flip"
	// RevokeReasonUsageBreach 今日累计越过阈值（TodaySum+Amount > DailyLimit）。
	RevokeReasonUsageBreach RevokeReason = "usage_breach"
	// RevokeReasonVelocityBreach 速度窗口越过阈值（Velocity >= MaxVelocity）。
	RevokeReasonVelocityBreach RevokeReason = "velocity_breach"
	// RevokeReasonManual 管理员手动吊销。
	RevokeReasonManual RevokeReason = "manual"
	// RevokeReasonExpiry TTL 超限自动过期（过期使用 expired 状态，此处仅为标注）。
	RevokeReasonExpiry RevokeReason = "expiry"
)

// LimitCheckLease 限额预检租约。
// 不变量：
// - PolicyVer 签发时固化（来自 policy.Registry.Current.Version）；
// CommitRecheck 时复验当前代次，代次变化则置 revoked + 落 LimitDriftProof。
// - TodaySum 签发时固化（来自 usage.Store.TodaySum 快照）；
// CommitRecheck 时重新计算，若累计越阈则置 revoked + 落 DriftProof。
// - Velocity 签发时固化（来自 usage.Store.Velocity 快照）；
// CommitRecheck 时复验，超速则拒绝。
// - Kind 精确标记操作类型（withdraw/transfer/deposit）。
// - 终态不可逆：consumed / expired / revoked 一旦写入，Engine 禁止再次转换。
type LimitCheckLease struct {
	// ID 租约唯一标识（UUID v4），对外称 limit_check_lease_id。
	ID string
	// UserID 用户 ID（操作主体）。
	UserID string
	// Asset 资产代码（如 BTC、USDT）。
	Asset string
	// Kind 操作类型（withdraw / transfer / deposit）。
	Kind string
	// Amount 本次占用请求金额（字符串十进制）。
	Amount string
	// Dest 提现目标地址（可为空；withdraw 场景必填）。
	Dest string
	// PolicyVer 签发时策略代次（固化）。
	PolicyVer int
	// TodaySumAtIssue 签发时今日累计金额（固化快照字符串）。
	TodaySumAtIssue string
	// VelocityAtIssue 签发时速度窗口命中次数（固化快照）。
	VelocityAtIssue int
	// MaxSingleAtIssue 签发时单笔限额（固化）。
	MaxSingleAtIssue string
	// DailyCapAtIssue 签发时日限额（固化）。
	DailyCapAtIssue string
	// MaxVelocityAtIssue 签发时最大速度值（固化）。
	MaxVelocityAtIssue int
	// Allowlisted 签发时用户是否在白名单。
	Allowlisted bool
	// State 当前状态。
	State LeaseState
	// IdemKey 幂等键，防重复签发（推荐 = SHA256(user+asset+kind+amount+request_id)）。
	IdemKey string
	// CommitRef 绑定的占用提交单据 ID（BindLease 时写入）。
	CommitRef string
	// IssuedAt 签发时刻（Check 通过时刻）。
	IssuedAt time.Time
	// ExpiresAt 过期时刻（IssuedAt + LeaseTTL）。
	ExpiresAt time.Time
	// BoundAt 占用提交绑定时刻（BindLease 时刻）。
	BoundAt *time.Time
	// ConsumedAt 消费完成时刻（CommitRecheck 通过时刻）。
	ConsumedAt *time.Time
	// RevokedAt 吊销时刻。
	RevokedAt *time.Time
	// RevokeReason 吊销原因。
	RevokeReason string
	// DriftProofID 关联的 LimitDriftProof.ID（revoked 路径写入）。
	DriftProofID string
	// CreatedAt 行创建时刻。
	CreatedAt time.Time
	// UpdatedAt 行更新时刻。
	UpdatedAt time.Time
}

// Expired 判断租约是否已超 TTL（不依赖状态位，由 Engine 实时判断）。
  func (l *LimitCheckLease) Expired(now time.Time) bool {
	   return now.After(l.ExpiresAt)
  }

// ─── LimitDriftProof 漂移拒放行证明 ──────────────────────────────────────
// ProofState 证明状态。
// 状态机：
// open ──(ProofProveWorker 固化)──▶ proven
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
	// DriftTypePolicyFlip 策略代次翻转（PolicyVer++ 在 Check→占用提交窗口发生）。
	DriftTypePolicyFlip DriftType = "policy_flip"
	// DriftTypeUsageBreach 今日累计越阈（TodaySum+Amount > DailyLimit）。
	DriftTypeUsageBreach DriftType = "usage_breach"
	// DriftTypeVelocityBreach 速度越阈（Velocity >= MaxVelocity）。
	DriftTypeVelocityBreach DriftType = "velocity_breach"
)

// LimitDriftProof 限额漂移拒放行证明。
// 不变量：
// - open 状态下，持旧 LeaseID 的占用提交一律拒绝并返回 proof_id。
// - ProofHash 唯一（防重复固化同一证明）；由 ProofProveWorker 计算写入。
// - 终态 closed 不可逆。
type LimitDriftProof struct {
	// ID 证明唯一标识（UUID v4）。
	ID string
	// LeaseID 旧 LimitCheckLease.ID。
	LeaseID string
	// UserID 受影响用户 ID。
	UserID string
	// Asset 资产代码。
	Asset string
	// Kind 操作类型。
	Kind string
	// DriftType 漂移类型。
	DriftType DriftType
	// OldPolicyVer 租约签发时的策略代次（固化）。
	OldPolicyVer int
	// NewPolicyVer 漂移发现时的策略代次（policy_flip 时非零）。
	NewPolicyVer int
	// OldTodaySum 租约签发时今日累计（固化快照）。
	OldTodaySum string
	// NewTodaySum 漂移发现时今日累计（usage_breach 时非空）。
	NewTodaySum string
	// DailyCap 日限额（证明追溯字段）。
	DailyCap string
	// OldVelocity 租约签发时速度（固化快照）。
	OldVelocity int
	// NewVelocity 漂移发现时速度（velocity_breach 时非零）。
	NewVelocity int
	// MaxVelocity 最大速度值（证明追溯字段）。
	MaxVelocity int
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

// ─── UsageSnapshot 占用快照 ──────────────────────────────────────────────
// UsageSnapshot 签发 LimitCheckLease 时固化的 usage 快照。
// 语义：Check 瞬时抓取的 policy+usage 状态，供 CommitRecheck 漂移。
// 不存储 availcas 可用 CAS 内核字段；不存储目的地图谱限额公式参数。
type UsageSnapshot struct {
	// UserID 用户 ID。
	UserID string
	// Asset 资产代码。
	Asset string
	// Kind 操作类型。
	Kind string
	// PolicyVer 策略代次（policy.Registry.Current.Version）。
	PolicyVer int
	// TodaySum 今日累计金额（usage.Store.TodaySum 快照，字符串十进制）。
	TodaySum string
	// Velocity 速度窗口命中次数（usage.Store.Velocity 快照）。
	Velocity int
	// MaxSingle 单笔限额（policy.Doc.MaxSingle 快照）。
	MaxSingle string
	// DailyCap 日限额（policy.Doc.DailyWithdraw / DailyDeposit 快照）。
	DailyCap string
	// MaxVelocity 最大速度值（policy.Doc.MaxVelocity 快照）。
	MaxVelocity int
	// SnappedAt 快照时刻。
	SnappedAt time.Time
}

// ─── 哨兵错误 ─────────────────────────────────────────────────────────────
// ErrDuplicate 主键或幂等键冲突（InsertXxx 遇到已存在记录）。

var ErrDuplicate = fmt.Errorf("limitleasegate duplicate record")

// ErrNotFound 记录不存在。
var ErrNotFound = fmt.Errorf("limitleasegate record not found")

// ErrLeaseExpired 租约已过期或超 TTL。

var ErrLeaseExpired = fmt.Errorf("limitleasegate lease expired")

// ErrLeaseRevoked 租约已吊销（PolicyVer 翻转或 TodaySum 越限）。
var ErrLeaseRevoked = fmt.Errorf("limitleasegate lease revoked")

// ErrLeaseConsumed 租约已消费（不可重复使用）。

var ErrLeaseConsumed = fmt.Errorf("limitleasegate lease already consumed")

// ErrNoLease 占用提交未携带 limit_check_lease_id（fail-closed）。
var ErrNoLease = fmt.Errorf("limitleasegate limit_check_lease_id required")

// ErrPolicyDrift 策略代次漂移（policy_flip 窗口）。

var ErrPolicyDrift = fmt.Errorf("limitleasegate policy version drifted")

// ErrUsageDrift 今日累计漂移越阈（usage_breach）。
var ErrUsageDrift = fmt.Errorf("limitleasegate today usage breached daily cap")

// ErrVelocityDrift 速度漂移越阈（velocity_breach）。
var ErrVelocityDrift = fmt.Errorf("limitleasegate velocity breached max")

// ErrScopeMismatch 用户/资产/操作类型与租约绑定不符。
var ErrScopeMismatch = fmt.Errorf("limitleasegate scope mismatch (user/asset/kind)")

// ErrInvalidTransit 非法状态转换。
var ErrInvalidTransit = fmt.Errorf("limitleasegate invalid state transition")

// ErrProofBlocked 存在 open LimitDriftProof，挡占用提交。
var ErrProofBlocked = fmt.Errorf("limitleasegate blocked by open drift proof")

// ErrCommitRefMismatch 占用提交单据 ID 与 bound 状态租约绑定不符。

var ErrCommitRefMismatch = fmt.Errorf("limitleasegate commit_ref mismatch")

// ErrCheckDenied limitgate.Check 返回 Deny，不得签发租约。

var ErrCheckDenied = fmt.Errorf("limitleasegate check denied by policy/usage")
