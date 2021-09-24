package polhopgate

import (
	"fmt"
	"time"
)

// ─── HopToken 状态机 ──────────────────────────────────────────────────────
// TokenState hop 执行令牌状态。
// 状态机：
// issued ──(hop 执行绑定)──▶ consumed
// issued ──(超过 TTL)──▶ expired
// issued ──(策略世代抬升复验失败)──▶ blocked
// consumed / expired / blocked —— 终态，不可再转换

type TokenState string

const (
	TokenStateIssued   TokenState = "issued"   // 已签发，尚未消费
	TokenStateConsumed TokenState = "consumed" // 已消费（hop 成功绑定）
	TokenStateExpired  TokenState = "expired"  // 已过期（TTL 超限）
	TokenStateBlocked  TokenState = "blocked"  // 已挡写（策略世代不符或 DualRead 分歧升级）
)

// Closed 判断是否为终态。
   func (s TokenState) Closed() bool {
	   return s == TokenStateConsumed || s == TokenStateExpired || s == TokenStateBlocked
   }

// validTokenTransitions 合法令牌状态转换。

var validTokenTransitions = map[TokenState][]TokenState{
	TokenStateIssued:   {TokenStateConsumed, TokenStateExpired, TokenStateBlocked},
	TokenStateConsumed: {},
	TokenStateExpired: {},
	TokenStateBlocked: {},
}

// HopToken hop 执行令牌（世代绑定，防止越权 hop）。
// 不变量：
// - TokenID 全局唯一（UUID v4）。
// - PolicyGen 在签发时锁定；hop 执行时必须与当前策略世代一致，否则→ErrGenMismatch。
// - TokenHash = SHA-256(user_id ‖ ":" ‖ biz_kind ‖ ":" ‖ policy_gen ‖ ":" ‖ issued_at_unix)。
// - 令牌一次性消费（consumed 后不可重用），幂等通过 IdemKey 检查。
// - ConsumedAt 不得早于 IssuedAt；ExpiresAt = IssuedAt + TTL（默认 30s）。

type HopToken struct {
	ID     string `json:"id"`      // UUID v4
	UserID string `json:"user_id"` // hop 发起用户
	Asset string `json:"asset"` // 币种
	BizKind   string     `json:"biz_kind"`   // 业务类型（acctpol.HopBizKind）
	PolicyGen int64      `json:"policy_gen"` // 签发时的策略世代号
	PolicyID  string     `json:"policy_id"`  // 关联 PolicyVersion.ID
	State     TokenState `json:"state"`      // 令牌状态
	TokenHash string    `json:"token_hash"` // 内容指纹
	IdemKey   string    `json:"idem_key"`   // 幂等键（防重复签发）
	IssuedAt  time.Time `json:"issued_at"`  // 签发时刻
	ExpiresAt time.Time `json:"expires_at"` // 过期时刻
	ConsumedAt *time.Time `json:"consumed_at,omitempty"` // 消费时刻
	BlockReason string `json:"block_reason,omitempty"` // 挡写原因
	HopRef    string    `json:"hop_ref,omitempty"` // 绑定 hop 的业务单据 ID
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Transition 状态机转换校验。
   func (t *HopToken) Transition(next TokenState) error {
	  for _, s := range validTokenTransitions[t.State] {
		   if s == next {
			  return nil
		  }
	  }
	   return fmt.Errorf("polhopgate: illegal token transition %s→%s for token %s", t.State, next, t.ID)
  }

// ─── PolLease 策略发布互斥租约 ───────────────────────────────────────────
// LeaseState 策略发布互斥租约状态。
// 状态机：
// acquired ──(Publish/Pin 完成)──▶ released
// acquired ──(TTL 超期)──▶ expired

type LeaseState string

const (
	LeaseStateAcquired LeaseState = "acquired" // 已获取，Publish/Pin 进行中
	LeaseStateReleased LeaseState = "released" // 已释放（发布完成）
	LeaseStateExpired LeaseState = "expired" // 已过期（TTL 超限，自动清理）
)

// PolLease 策略发布互斥租约。
// 约束：
// - 同一 (user_id, asset) 键同一时刻至多一个 acquired 租约。
// - Gate.Check 在 Publish 窗口（有 acquired 租约）时必须等待或返回 ErrPublishInWindow。
// - Holder 为发布操作人 ID；不同操作人抢占租约时 TTL 内后者返回 ErrLeaseLocked。
// - Gen 在租约被 Release 时写入实际完成的策略世代号。
type PolLease struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"` // 空=全局租约
	Asset  string `json:"asset"`
	Holder string `json:"holder"` // 租约持有人（操作人 ID）
	State LeaseState `json:"state"`
	PolicyGen  int64     `json:"policy_gen"` // 租约释放后记录完成的世代号
	AcquiredAt time.Time `json:"acquired_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	ReleasedAt *time.Time `json:"released_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ─── PolDriftCase 策略漂移案例 ───────────────────────────────────────────
// DriftCaseState 策略漂移案例状态。
// 状态机：
// open ──(扫描器写入证明)──▶ proven
// proven ──(管理员确认)──▶ closed
// open ──(管理员驳回)──▶ closed

type DriftCaseState string

const (
	DriftCaseOpen DriftCaseState = "open" // 案例开启，挡写生效
	DriftCaseProven DriftCaseState = "proven" // 证明已产出，待人工确认
	DriftCaseClosed DriftCaseState = "closed" // 案例关闭，挡写解除
)

// DriftTrigger 漂移案例触发原因。

type DriftTrigger string

const (
	DriftTriggerDualRead     DriftTrigger = "dual_read"    // DualRead 分歧超阈
	DriftTriggerUnauthorized DriftTrigger = "unauthorized" // 越权 hop（令牌世代不符）
	DriftTriggerWindowRace   DriftTrigger = "window_race"  // 发布窗口竞态（有租约时 hop 通过）
)

// validDriftTransitions 合法漂移案例状态转换。

var validDriftTransitions = map[DriftCaseState][]DriftCaseState{
	DriftCaseOpen: {DriftCaseProven,
		DriftCaseClosed},
	DriftCaseProven: {DriftCaseClosed},
	DriftCaseClosed: {},
}

// PolDriftCase 策略漂移案例（越权/分歧证明落仓）。
// 不变量：
// - CaseID 唯一（UUID v4）；UserID+BizKind+TriggerGen 不得重复开案（幂等键）。
// - ProofID 在 proven 状态后填写；closed 可无 ProofID（Dismiss 路径）。
// - HopTokenID 仅在 DriftTriggerUnauthorized 时填写（标记越权令牌）。
// - BlockedAt 开案时同步写入；CloseReason 在 closed 时写入。
type PolDriftCase struct {
	ID     string `json:"id"`      // UUID v4
	UserID string `json:"user_id"` // 受影响用户（空=全局）
	Asset   string       `json:"asset"`    // 币种（空=全资产）
	BizKind string       `json:"biz_kind"` // 触发案例的业务类型
	Trigger DriftTrigger `json:"trigger"`  // 触发原因
	TriggerGen int64 `json:"trigger_gen"` // 触发时的策略世代（主库视角）
	ReplicaGen   int64 `json:"replica_gen"`   // 触发时的副库世代（dual_read 触发时有效）
	DivergeDelta int64 `json:"diverge_delta"` // 分歧量（trigger_gen - replica_gen）
	HopTokenID string         `json:"hop_token_id,omitempty"` // 关联越权令牌（可空）
	State      DriftCaseState `json:"state"`
	ProofID string `json:"proof_id,omitempty"` // DriftProof.ID（proven 后填）
	ProofHash string `json:"proof_hash,omitempty"`
	ConfirmedBy string     `json:"confirmed_by,omitempty"`
	ConfirmedAt *time.Time `json:"confirmed_at,omitempty"`
	BlockedAt   time.Time  `json:"blocked_at"` // 挡写生效时刻
	CloseReason string     `json:"close_reason,omitempty"`
	ScanNote    string     `json:"scan_note,omitempty"` // 扫描器说明
	IdemKey   string    `json:"idem_key"` // 幂等键（防重复开案）
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Transition 状态机转换校验。
  func (c *PolDriftCase) Transition(next DriftCaseState) error {
	  for _, s := range validDriftTransitions[c.State] {
		   if s == next {
			  return nil
		  }
	   }
	  return fmt.Errorf("polhopgate: illegal drift case transition %s→%s for case %s", c.State, next, c.ID)
  }

// ─── DualEscalate 双读升级挡写记录 ───────────────────────────────────────
// EscalateState 双读升级状态。

type EscalateState string

const (
	EscalateStateActive   EscalateState = "active"   // 挡写生效中
	EscalateStateResolved EscalateState = "resolved" // 挡写已解除（分歧修复后）
)

// DualEscalate 双读分歧超阈后的升级挡写记录。
// 不变量：
// - 同一 (user_id, asset, biz_kind) 最多有一个 active 升级记录。
// - ThresholdCount 为触发升级的分歧次数；DivergeDelta 为最大分歧量。
// - CaseID 关联对应的 PolDriftCase。
// - 挡写生效期间，polhopgate.Engine.CheckHop 对该 BizKind 返回 ErrDualEscalateBlock。

type DualEscalate struct {
	ID     string `json:"id"`      // UUID v4
	UserID string `json:"user_id"` // 空=全局
	Asset  string `json:"asset"`   // 空=全资产
	BizKind         string `json:"biz_kind"`        // 挡写的业务类型
	ThresholdCount  int    `json:"threshold_count"` // 触发升级的分歧次数
	MaxDivergeDelta int64  `json:"max_diverge_delta"`
	CaseID string `json:"case_id"` // 关联 PolDriftCase.ID
	State       EscalateState `json:"state"`
	EscalatedAt time.Time     `json:"escalated_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// ─── DriftProof 漂移证明 ──────────────────────────────────────────────────
// DriftProof 漂移案例证明记录（由 proof seal worker 产出）。
// 证明语义：
// - ProofHash = SHA-256(case_id ‖ ":" ‖ trigger_gen ‖ ":" ‖ replica_gen ‖ ":" ‖ sealed_at_unix)
// - 证明缺失时禁止将 PolDriftCase 标记为 proven 或 closed。
type DriftProof struct {
	ID     string `json:"id"`      // UUID v4
	CaseID string `json:"case_id"` // PolDriftCase.ID
	TriggerGen int64 `json:"trigger_gen"` // 主库世代快照
	ReplicaGen int64 `json:"replica_gen"` // 副库世代快照
	ProofHash string    `json:"proof_hash"` // SHA-256 hex
	SealedAt  time.Time `json:"sealed_at"`
	CreatedAt time.Time `json:"created_at"`
}

// ─── 错误定义 ─────────────────────────────────────────────────────────────

var (
	// ErrGenMismatch 令牌世代与当前策略世代不符；hop 执行必须重新获取令牌。
	ErrGenMismatch = fmt.Errorf("polhopgate: token policy_gen mismatch; hop re-check required")
	// ErrTokenExpired 令牌已过期，不得用于 hop 绑定。
	ErrTokenExpired = fmt.Errorf("polhopgate: hop token expired")
	// ErrTokenBlocked 令牌已被挡写（世代不符或 DualRead 升级）。
	ErrTokenBlocked = fmt.Errorf("polhopgate: hop token blocked")
	// ErrTokenConsumed 令牌已消费，不可重用。
	ErrTokenConsumed = fmt.Errorf("polhopgate: hop token already consumed")
	// ErrPublishInWindow 策略发布窗口进行中，hop Gate 必须等待。
	ErrPublishInWindow = fmt.Errorf("polhopgate: policy publish window active; hop check deferred")
	// ErrDualEscalateBlock 双读分歧升级后挡写，该 BizKind hop 被阻断。
	ErrDualEscalateBlock = fmt.Errorf("polhopgate: dual-read escalation active; hop blocked")
	// ErrHopDenied hop 被策略规则明确拒绝。
	ErrHopDenied = fmt.Errorf("polhopgate: hop denied by policy rule")
	// ErrLeaseLocked 策略租约被他人持有。
	ErrLeaseLocked = fmt.Errorf("polhopgate: pol lease locked by another holder")
	// ErrProofMissing 证明缺失，不得关闭案例。
	ErrProofMissing = fmt.Errorf("polhopgate: drift proof missing; cannot close case")
	// ErrNotFound 记录不存在。
	ErrNotFound = fmt.Errorf("polhopgate: not found")
	// ErrDuplicate 主键冲突。
	ErrDuplicate = fmt.Errorf("polhopgate: duplicate key")
	// ErrBadState 非法状态转换。
	ErrBadState = fmt.Errorf("polhopgate: illegal state transition")
)
