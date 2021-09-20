package overwdlease

import (
	"fmt"
	"time"
)

// ─── PrecheckBind 预检绑定租约 ────────────────────────────────────────────
// BindState 预检绑定租约状态。
// 状态机：
// open ──(Commit 时 bind_id 匹配且 LeaseGen/Available 核验通过)──▶ committed
// open ──(TTL 超期，BindExpiryWorker 吊销)──▶ expired
// open ──(手动吊销或差异证明升级后吊销)──▶ revoked
// committed / expired / revoked —— 终态，不可再转换

type BindState string

const (
	BindStateOpen      BindState = "open"      // 已开启，等待 Commit 复验
	BindStateCommitted BindState = "committed" // 已提交（Commit 复验通过）
	BindStateExpired   BindState = "expired"   // 已过期（TTL 超限，BindExpiryWorker 吊销）
	BindStateRevoked   BindState = "revoked"   // 已吊销（手动或差异证明升级）
)

// Closed 判断是否为终态。

   func (s BindState) Closed() bool {
	   return s == BindStateCommitted || s == BindStateExpired || s == BindStateRevoked
  }

// validBindTransitions 合法绑定状态转换表。
var validBindTransitions = map[BindState][]BindState{
	BindStateOpen: {BindStateCommitted,
		BindStateExpired, BindStateRevoked},
	BindStateCommitted: {},
	BindStateExpired:   {},
	BindStateRevoked: {},
}

// PrecheckBind 预检绑定租约。
// 不变量：
// - BindID 全局唯一（UUID v4）。
// - PreCheckAt 早于 ExpiresAt（ExpiresAt = PreCheckAt + TTL，默认 60s）。
// - LeaseGen 与 ExpectedAvail 在 PreCheck 通过时锁定；Commit 时必须与当前 availcas
// 观测值一致，否则视为漂移并升级差异证明。
// - BindHash = SHA-256(user_id ‖ ":" ‖ asset ‖ ":" ‖ lease_gen ‖ ":" ‖ expected_avail ‖ ":" ‖ precheck_at_unix)。
// - IdemKey 防止重复发起 PreCheck 绑定（同一出金请求幂等）。
// - CommitRef 在 committed 后写入实际 Commit 单据 ID。
type PrecheckBind struct {
	ID     string `json:"id"`      // UUID v4
	UserID string `json:"user_id"` // 发起用户
	Asset  string `json:"asset"`  // 币种
	Amount string `json:"amount"` // 本次出金请求金额（decimal string）
	LeaseGen int64 `json:"lease_gen"` // PreCheck 时 availcas 观测 LeaseGen
	ExpectedAvail string `json:"expected_avail"` // PreCheck 时观测 Available（decimal string）
	State BindState `json:"state"` // 绑定状态
	BindHash string `json:"bind_hash"` // 内容指纹（防篡改）
	IdemKey      string     `json:"idem_key"`                // 幂等键（防重复 PreCheck）
	PreCheckAt   time.Time  `json:"precheck_at"`             // PreCheck 通过时刻
	ExpiresAt    time.Time  `json:"expires_at"`              // 绑定过期时刻（TTL 内必须 Commit）
	CommittedAt  *time.Time `json:"committed_at,omitempty"`  // Commit 完成时刻
	CommitRef    string     `json:"commit_ref,omitempty"`    // 实际 Commit 单据 ID
	RevokeReason string     `json:"revoke_reason,omitempty"` // 吊销原因
	ProofID      string     `json:"proof_id,omitempty"`      // 关联 VarianceProof.ID（漂移路径）
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// Transition 状态机转换校验。
  func (b *PrecheckBind) Transition(next BindState) error {
	  for _, s := range validBindTransitions[b.State] {
		  if s == next {
			  return nil
		   }
	  }
	   return fmt.Errorf("overwdlease: illegal bind transition %s→%s for bind %s", b.State, next, b.ID)
   }

// ─── VarianceProof 差异证明 ───────────────────────────────────────────────
// ProofState 差异证明状态。
// 状态机：
// open ──(四眼确认/驳回)──▶ closed
// open 状态期间挡同用户同资产的再次 Commit

type ProofState string

const (
	ProofStateOpen ProofState = "open" // 证明开启，挡写生效
	ProofStateClosed ProofState = "closed" // 证明关闭，挡写解除
)

// ProofTrigger 差异证明触发类型。
type ProofTrigger string

const (
	ProofTriggerGenDrift ProofTrigger = "gen_drift" // LeaseGen 漂移（Commit 时 gen 已变）
	ProofTriggerAvailDrift ProofTrigger = "avail_drift" // Available 漂移（Commit 时可用余额已变）
	ProofTriggerBothDrift ProofTrigger = "both_drift" // Gen+Available 同时漂移
)

// validProofTransitions 合法证明状态转换表。

var validProofTransitions = map[ProofState][]ProofState{
	ProofStateOpen:   {ProofStateClosed},
	ProofStateClosed: {},
}

// VarianceProof 差异证明（漂移事件闭合仓）。
// 不变量：
// - ProofID 全局唯一（UUID v4）。
// - BindID 关联触发漂移的 PrecheckBind.ID。
// - DriftGen = CommitGen - BindLeaseGen（正值表示 gen 已向前移动）。
// - ProofHash = SHA-256(proof_id ‖ ":" ‖ bind_id ‖ ":" ‖ trigger ‖ ":" ‖ drift_gen ‖ ":" ‖ sealed_at_unix)。
// - open 期间，同 (user_id, asset) 的任何 Commit 均返回 ErrProofBlocked。
// - live-hash 差异路径不得在此包自动解冻（保持现有冻结状态，proofhash 仅供审计）。
type VarianceProof struct {
	ID     string `json:"id"`      // UUID v4
	BindID string `json:"bind_id"` // 触发漂移的 PrecheckBind.ID
	UserID string `json:"user_id"` // 受影响用户
	Asset string `json:"asset"` // 币种
	Trigger ProofTrigger `json:"trigger"` // 漂移触发类型
	BindLeaseGen int64  `json:"bind_lease_gen"` // 绑定时 LeaseGen
	CommitGen    int64  `json:"commit_gen"`     // Commit 时观测 LeaseGen
	DriftGen     int64  `json:"drift_gen"`      // CommitGen - BindLeaseGen
	BindAvail    string `json:"bind_avail"`     // 绑定时 ExpectedAvail
	CommitAvail  string `json:"commit_avail"`   // Commit 时观测 Available
	State ProofState `json:"state"` // 证明状态
	ProofHash string    `json:"proof_hash"` // SHA-256 hex（防篡改）
	SealedAt  time.Time `json:"sealed_at"`  // 证明固化时刻
	ConfirmedBy string     `json:"confirmed_by,omitempty"` // 四眼确认人 ID
	ConfirmedAt *time.Time `json:"confirmed_at,omitempty"` // 四眼确认时刻
	CloseReason string `json:"close_reason,omitempty"` // 关闭原因
	DriftCaseID string `json:"drift_case_id,omitempty"` // 关联 OverwdDriftCase.ID
	LiveHashFreeze bool      `json:"live_hash_freeze"` // 是否命中 live-hash 冻结路径（只读标记）
	IdemKey        string    `json:"idem_key"`         // 幂等键（防重复开证明）
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Transition 状态机转换校验。

  func (p *VarianceProof) Transition(next ProofState) error {
	   for _, s := range validProofTransitions[p.State] {
		  if s == next {
			  return nil
		  }
	  }
	   return fmt.Errorf("overwdlease: illegal proof transition %s→%s for proof %s", p.State, next, p.ID)
  }

// ─── OverwdDriftCase 漂移案例 ─────────────────────────────────────────────
// DriftCaseState 漂移案例状态。
// 状态机：
// open ──(DriftPatrolWorker 固化证明)──▶ proven
// proven ──(管理员四眼确认)──▶ closed
// open ──(管理员驳回)──▶ closed
type DriftCaseState string

const (
	DriftCaseOpen DriftCaseState = "open" // 案例开启，挡写生效
	DriftCaseProven DriftCaseState = "proven" // 证明已产出，待管理员确认
	DriftCaseClosed DriftCaseState = "closed" // 案例关闭（挡写解除或驳回）
)

// DriftCaseTrigger 漂移案例触发类型。

type DriftCaseTrigger string

const (
	DriftTriggerGenDrift   DriftCaseTrigger = "gen_drift"   // LeaseGen 漂移
	DriftTriggerAvailDrift DriftCaseTrigger = "avail_drift" // Available 漂移
	DriftTriggerBothDrift  DriftCaseTrigger = "both_drift"  // 双重漂移
	DriftTriggerNoBind      DriftCaseTrigger = "no_bind"      // Commit 缺少 bind_id
	DriftTriggerExpiredBind DriftCaseTrigger = "expired_bind" // bind 已过期或已吊销
)

// validDriftTransitions 合法漂移案例状态转换表。
var validDriftTransitions = map[DriftCaseState][]DriftCaseState{
	DriftCaseOpen:   {DriftCaseProven, DriftCaseClosed},
	DriftCaseProven: {DriftCaseClosed},
	DriftCaseClosed: {},
}

// OverwdDriftCase 超提屏障漂移案例（事故轨迹全记录）。
// 不变量：
// - CaseID 唯一（UUID v4）。
// - IdemKey 唯一（防重复开案）。
// - ProofID 在 proven 状态后填写；closed 可无 ProofID（Dismiss 路径）。
// - BindID 关联触发案例的 PrecheckBind（可空，no_bind 路径无 bind）。
// - BlockedAt 开案时同步写入，挡写立即生效。
type OverwdDriftCase struct {
	ID string `json:"id"` // UUID v4
	UserID string `json:"user_id"` // 受影响用户
	Asset  string `json:"asset"`   // 币种
	Trigger DriftCaseTrigger `json:"trigger"` // 触发原因
	BindID string `json:"bind_id,omitempty"` // 关联 PrecheckBind.ID（可空）
	ProofID string `json:"proof_id,omitempty"` // 关联 VarianceProof.ID（proven 后填）
	ProofHash string         `json:"proof_hash,omitempty"` // 防篡改
	State     DriftCaseState `json:"state"`                // 案例状态
	DriftGen  int64          `json:"drift_gen"`            // 漂移量（gen 维度）
	ConfirmedBy string     `json:"confirmed_by,omitempty"` // 关闭操作人 ID
	ConfirmedAt *time.Time `json:"confirmed_at,omitempty"` // 关闭时刻
	BlockedAt   time.Time  `json:"blocked_at"`             // 挡写生效时刻
	CloseReason string     `json:"close_reason,omitempty"` // 关闭原因
	ScanNote    string     `json:"scan_note,omitempty"`    // 巡检器说明
	IdemKey string `json:"idem_key"` // 幂等键
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Transition 状态机转换校验。

  func (c *OverwdDriftCase) Transition(next DriftCaseState) error {
	   for _, s := range validDriftTransitions[c.State] {
		  if s == next {
			   return nil
		  }
	   }
	   return fmt.Errorf("overwdlease: illegal drift case transition %s→%s for case %s", c.State, next, c.ID)
  }

// ─── LeaseObservation availcas 只读观测 ──────────────────────────────────
// LeaseObservation 来自 availcas 的只读观测快照（不修改 CAS 内核公式）。
// overwdlease 只读消费此结构，用于在 PreCheck 和 Commit 时核对 LeaseGen/Available。
// 不得通过此包改写 availcas.Submit 公式或 CAS 逻辑。
type LeaseObservation struct {
	UserID   string `json:"user_id"`   // 观测用户
	Asset    string `json:"asset"`     // 币种
	LeaseGen int64  `json:"lease_gen"` // availcas 当前 LeaseGen（只读快照）
	Available  string    `json:"available"`   // availcas 当前 Available（decimal string，只读）
	ObservedAt time.Time `json:"observed_at"` // 观测时刻
}

// ─── BarrierDecision overwdbar 屏障决策观测 ─────────────────────────────
// BarrierDecision 来自 overwdbar 屏障决策的只读观测（不修改 overwdbar 内核）。
// PreCheck 通过时携带此结构创建 PrecheckBind；Commit 时核对并检查漂移。

type BarrierDecision struct {
	Allowed  bool  `json:"allowed"`   // PreCheck 是否通过
	LeaseGen int64 `json:"lease_gen"` // 屏障决策时的 LeaseGen
	Available string `json:"available"` // 屏障决策时的 Available（decimal string）
	BarrierTraceID string `json:"barrier_trace_id"` // 屏障决策追踪 ID
}

// ─── 错误定义 ─────────────────────────────────────────────────────────────

var (
	// ErrNoBind Commit 未携带 bind_id，拒绝放行。
	ErrNoBind = fmt.Errorf("overwdlease: commit requires precheck_bind_id; none provided")
	// ErrBindNotFound 指定的 bind_id 不存在。
	ErrBindNotFound = fmt.Errorf("overwdlease: precheck bind not found")
	// ErrBindExpired bind 已过期，不得用于 Commit。
	ErrBindExpired = fmt.Errorf("overwdlease: precheck bind expired; re-precheck required")
	// ErrBindRevoked bind 已被吊销，不得用于 Commit。
	ErrBindRevoked = fmt.Errorf("overwdlease: precheck bind revoked; re-precheck required")
	// ErrBindConsumed bind 已被消费（Commit 成功），不可重用。
	ErrBindConsumed = fmt.Errorf("overwdlease: precheck bind already committed; idempotent ok")
	// ErrBindUserMismatch Commit 用户与 bind 用户不一致。
	ErrBindUserMismatch = fmt.Errorf("overwdlease: commit user does not match bind user")
	// ErrOverdrawBlocked LeaseGen 或 Available 漂移，超提屏障拒绝并升差异证明。
	ErrOverdrawBlocked = fmt.Errorf("overwdlease: overdraw barrier blocked; lease gen or available drifted; variance proof opened")
	// ErrProofBlocked 存在未闭合的差异证明，同用户同资产 Commit 被阻断。
	ErrProofBlocked = fmt.Errorf("overwdlease: open variance proof exists; commit blocked until proof confirmed")
	// ErrProofMissing 证明缺失，案例不能推进至 proven。
	ErrProofMissing = fmt.Errorf("overwdlease: variance proof missing; cannot close drift case")
	// ErrNotFound 记录不存在。
	ErrNotFound = fmt.Errorf("overwdlease: not found")
	// ErrDuplicate 主键或幂等键冲突。
	ErrDuplicate = fmt.Errorf("overwdlease: duplicate key")
	// ErrBadState 非法状态转换。
	ErrBadState = fmt.Errorf("overwdlease: illegal state transition")
)
