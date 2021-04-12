package destlife

import (
	"fmt"
	"time"
)

// EntryState 白名单条目生命周期状态机节点。
// 合法转换：Added→Cooling→Active→Revoking→Revoked。
// 非法跳转（如 Active→Added）在 Transition 中强制拒绝。

type EntryState string

const (
	StateAdded   EntryState = "added"   // 已添加，冷却计时未开始
	StateCooling EntryState = "cooling" // 冷却窗口计时中，不可出金
	StateActive  EntryState = "active"  // 冷却已满，可正常出金
	StateRevoking EntryState = "revoking" // 待撤销（持有在途出金引用，物理删除被阻塞）
	StateRevoked EntryState = "revoked" // 已撤销，不可再使用
)

// legalTransitions 合法状态转换表。

var legalTransitions = map[EntryState][]EntryState{
	StateAdded: {StateCooling},
	StateCooling:  {StateActive, StateRevoking},
	StateActive:   {StateRevoking},
	StateRevoking: {StateRevoked},
}

// Transition 验证并返回新状态；不合法转换返回错误。

   func Transition(from, to EntryState) error {
	  targets, ok := legalTransitions[from]
	  if !ok {
		  return fmt.Errorf("destlife: state %q has no allowed transitions", from)
	  }
	   for _, t := range targets {
		   if t == to {
			  return nil
		   }
	  }
	  return fmt.Errorf("destlife: illegal transition %q→%q", from, to)
   }

// AllowEntryLife 白名单条目生命周期记录。
// entry_life_id 供他包只读引用；不存储风险簇图谱字段。
type AllowEntryLife struct {
	// 主键
	EntryLifeID string `json:"entry_life_id"`
	// 业务维度
	UserID string `json:"user_id"`
	Chain   string `json:"chain"`
	Address string `json:"address"`
	Label string `json:"label,omitempty"`
	// 生命周期字段
	State EntryState `json:"state"`
	AddedAt time.Time `json:"added_at"`
	CoolingFrom time.Time `json:"cooling_from"`
	ActiveAt time.Time `json:"active_at,omitempty"`
	RevokingAt time.Time `json:"revoking_at,omitempty"`
	RevokedAt  time.Time `json:"revoked_at,omitempty"`
	// 世代（与 Versioned.gen 一致）
	Generation int64 `json:"generation"`
	// 审计
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CooldownDeadline 返回冷却截止时刻（CoolingFrom + 冷却窗口）。
// window 为零则无冷却限制，直接返回 CoolingFrom。

  func (e *AllowEntryLife) CooldownDeadline(window time.Duration) time.Time {
	  if window <= 0 || e.CoolingFrom.IsZero() {
		   return e.CoolingFrom
	   }
	  return e.CoolingFrom.Add(window)
  }

// IsCoolingDone 判断冷却窗口是否已到期。
   func (e *AllowEntryLife) IsCoolingDone(window time.Duration, now time.Time) bool {
	  if e == nil {
		  return false
	   }
	   deadline := e.CooldownDeadline(window)
	  if deadline.IsZero() {
		   return true
	   }
	  return !now.Before(deadline)
  }

// CooldownWindow 冷却策略窗口（按用户维度可覆盖全局默认）。
// 不变量：Seconds >= 0；零值表示无冷却。
type CooldownWindow struct {
	ID string `json:"id"`
	Scope   string `json:"scope"` // "global" | user_id
	Seconds int64  `json:"seconds"`
	Duration  time.Duration `json:"-"` // 运行时缓存，不落库
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// InflightRef 在途出金对白名单条目的冻结引用。
// 仅记引用；不存储出金金额或 tx_hash（不依赖广播 FSM）。
// 不变量：同一 movement_id 只能持有一个 ref；引用数归零才允许物理删除。
type InflightRef struct {
	RefID string `json:"ref_id"`
	EntryLifeID string `json:"entry_life_id"`
	UserID      string `json:"user_id"`
	Chain       string `json:"chain"`
	Address     string `json:"address"`
	MovementID  string `json:"movement_id"` // 出金流水号（只读引用）
	AcquiredAt time.Time `json:"acquired_at"`
	ReleasedAt time.Time `json:"released_at,omitempty"`
	Released bool `json:"released"`
}

// BypassProof 冷却绕过证明记录。
// 仅记录绕过路径与证明；不修改条目状态，不替代 KYT。
// 不变量：一旦写入不可覆盖（append-only）；proof_id 全局唯一。
type BypassProof struct {
	ProofID string `json:"proof_id"`
	EntryLifeID string `json:"entry_life_id"`
	UserID      string `json:"user_id"`
	Chain string `json:"chain"`
	Address    string    `json:"address"`
	Reason     string    `json:"reason"`                   // 绕过原因（audit 字段）
	ClockDrift int64     `json:"clock_drift_ms,omitempty"` // 伪造时钟漂移量（毫秒）
	Blocked    bool      `json:"blocked"`                  // 是否已被挡写
	BlockedBy  string    `json:"blocked_by,omitempty"`
	DetectedAt time.Time `json:"detected_at"`
	BlockedAt  time.Time `json:"blocked_at,omitempty"`
}

// LifecycleAuditRow 条目全轨迹审计行（Add/Cooling/Active/Revoking/Revoked）。
// 不变量：append-only；audit_id 唯一；from_state 空串表示初始化事件。
type LifecycleAuditRow struct {
	AuditID string `json:"audit_id"`
	EntryLifeID string `json:"entry_life_id"`
	UserID  string `json:"user_id"`
	Chain   string `json:"chain"`
	Address string `json:"address"`
	Actor string `json:"actor"`
	Event     string     `json:"event"`
	FromState EntryState `json:"from_state,omitempty"`
	ToState    EntryState `json:"to_state"`
	Note       string     `json:"note,omitempty"`
	Generation int64      `json:"generation"`
	At time.Time `json:"at"`
}

// InflightCount 在途引用计数摘要（供 API 返回）。
type InflightCount struct {
	EntryLifeID string `json:"entry_life_id"`
	UserID string `json:"user_id"`
	Chain  string `json:"chain"`
	Address string `json:"address"`
	Active  int    `json:"active_refs"`
	Total int `json:"total_refs"`
}

// RevokingRequest 运营强制转 revoking 请求。
type RevokingRequest struct {
	EntryLifeID string `json:"entry_life_id"`
	Actor string `json:"actor"`
	Note string `json:"note,omitempty"`
}

// GenMismatch 世代不一致错误详情（供 Strict 闸决策）。
type GenMismatch struct {
	EntryLifeID string `json:"entry_life_id"`
	LifeGen int64 `json:"life_gen"`
	BookGen int64 `json:"book_gen"`
	Discrepancy int64 `json:"discrepancy"`
}

   func (g *GenMismatch) Error() string {
	  return fmt.Sprintf("destlife: generation mismatch entry_life=%s life_gen=%d book_gen=%d",
		   g.EntryLifeID, g.LifeGen, g.BookGen)
   }

// ErrInflightBlock 在途引用阻塞物理删除。
var ErrInflightBlock = fmt.Errorf("destlife: entry blocked by active inflight refs")

// ErrCooldownActive 冷却未满拒绝放行。
var ErrCooldownActive = fmt.Errorf("destlife: entry still in cooldown window")

// ErrStateRevoked 条目已撤销，不可再操作。
var ErrStateRevoked = fmt.Errorf("destlife: entry has been revoked")

// ErrBypassBlocked 冷却绕过被挡写。

var ErrBypassBlocked = fmt.Errorf("destlife: bypass attempt blocked by proof guard")

// ErrEntryNotFound 条目不存在。
var ErrEntryNotFound = fmt.Errorf("destlife: entry life record not found")

// ErrDuplicateRef 重复的在途引用（同一 movement_id 已持有引用）。
var ErrDuplicateRef = fmt.Errorf("destlife: duplicate inflight ref for movement")

// ── 在途世代闸领域对象（与白名单生命周期并存） ──
// RefState 在途引用状态机。
// acquired → bound (BindRef：将引用与出金广播绑定)
// bound → released (ReleaseRef：出金完成或回滚)
// acquired → released (直接释放，无广播绑定)

type RefState string

const (
	RefAcquired RefState = "acquired"
	RefBound    RefState = "bound"
	RefReleased RefState = "released"
)

// EntryActive/EntryRevoking/EntryRevoked 是在途吊销世代状态别名，
// 字符串值与白名单生命周期 StateActive/StateRevoking/StateRevoked 一致。
const (
	EntryActive   = StateActive
	EntryRevoking = StateRevoking
	EntryRevoked = StateRevoked
)

// DriftCaseState 漂移案例状态。

type DriftCaseState string

const (
	DriftCaseOpen   DriftCaseState = "open"
	DriftCaseClosed DriftCaseState = "closed"
)

// DriftKind 漂移类型。

type DriftKind string

const (
	// DriftBroadcastAfterRevoke 条目已 revoked/removed 仍有广播完成。
	DriftBroadcastAfterRevoke DriftKind = "broadcast_after_revoke"
	// DriftBroadcastStaleGen 持旧 inflight_gen 广播，引用已释放。
	DriftBroadcastStaleGen DriftKind = "broadcast_stale_gen"
	// DriftRefLeakTimeout 引用超时未释放，泄漏扫描发现。
	DriftRefLeakTimeout DriftKind = "ref_leak_timeout"
)

// InflightGenLease 在途世代租约。
// AcquireRef 成功时签发；出金广播必须携带有效 LeaseID 与 InflightGen，
// 广播路径验证 State=acquired|bound 且 entry RevokeGen 未超出 BoundRevokeGen。

type InflightGenLease struct {
	LeaseID string `json:"lease_id"`
	EntryID string `json:"entry_id"`
	UserID  string `json:"user_id"`
	Chain   string `json:"chain"`
	Address string `json:"address"`
	MovementID string `json:"movement_id"`
	InflightGen    int64 `json:"inflight_gen"`
	BoundRevokeGen int64 `json:"bound_revoke_gen"`
	State RefState `json:"state"`
	IssuedAt time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
	BoundAt    *time.Time `json:"bound_at,omitempty"`
	ReleasedAt *time.Time `json:"released_at,omitempty"`
	Actor string `json:"actor,omitempty"`
	Note string `json:"note,omitempty"`
}

// RevokeGeneration 吊销世代记录。
// 每次 BeginRevoking 或 ForceRevoke 时抬升 Gen（单调递增）。
// 广播路径要求 bound_revoke_gen == current RevokeGen，否则拒绝。
type RevokeGeneration struct {
	EntryID string `json:"entry_id"`
	UserID  string `json:"user_id"`
	Chain   string `json:"chain"`
	Address string `json:"address"`
	Gen     int64  `json:"gen"`
	State EntryState `json:"state"`
	ActiveRefs  int       `json:"active_refs"`
	InitiatedAt time.Time `json:"initiated_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	Actor string `json:"actor,omitempty"`
}

// InflightDriftCase 在途漂移案例。
// 发现「条目已 revoked/removed 仍广播成功」或「持旧 inflight_gen 广播」时产出。
// 打开后同一 entry_id 后续广播路径被完全阻断，直到四眼收口。
type InflightDriftCase struct {
	CaseID string `json:"case_id"`
	EntryID string `json:"entry_id"`
	UserID  string `json:"user_id"`
	Chain   string `json:"chain"`
	Address string `json:"address"`
	MovementID string `json:"movement_id"`
	LeaseID    string `json:"lease_id,omitempty"`
	InflightGen int64 `json:"inflight_gen"`
	BoundRevokeGen int64 `json:"bound_revoke_gen"`
	CurrentRevokeGen int64 `json:"current_revoke_gen"`
	EntryState EntryState     `json:"entry_state"`
	Kind       DriftKind      `json:"kind"`
	State      DriftCaseState `json:"state"`
	OpenedAt time.Time  `json:"opened_at"`
	ClosedAt *time.Time `json:"closed_at,omitempty"`
	ClosedBy   string `json:"closed_by,omitempty"`
	Resolution string `json:"resolution,omitempty"`
	Note       string `json:"note,omitempty"`
}

// InflightProof 在途广播证明。
// 广播路径成功提交时写入不可变证明，记录世代绑定。
// 证明缺失不得当作广播已成功。

type InflightProof struct {
	ProofID    string `json:"proof_id"`
	LeaseID    string `json:"lease_id"`
	EntryID    string `json:"entry_id"`
	MovementID string `json:"movement_id"`
	InflightGen int64 `json:"inflight_gen"`
	BoundRevokeGen int64  `json:"bound_revoke_gen"`
	BroadcastTxID  string `json:"broadcast_tx_id,omitempty"`
	Chain string `json:"chain"`
	Address string `json:"address"`
	CreatedAt time.Time `json:"created_at"`
}

// AcquireResult AcquireRef 结果。
type AcquireResult struct {
	Lease InflightGenLease `json:"lease"`
	Idempotent bool `json:"idempotent"`
}

// BroadcastVerifyResult 广播路径验证结果。

type BroadcastVerifyResult struct {
	OK      bool   `json:"ok"`
	LeaseID string `json:"lease_id"`
	EntryID        string `json:"entry_id"`
	InflightGen    int64  `json:"inflight_gen"`
	BoundRevokeGen int64  `json:"bound_revoke_gen"`
	CurrentRevokeGen int64 `json:"current_revoke_gen"`
	EntryState   EntryState `json:"entry_state"`
	RejectReason string     `json:"reject_reason,omitempty"`
	DriftCaseID string `json:"drift_case_id,omitempty"`
}

// RevokeResult BeginRevoking/ForceRevoke 结果。
type RevokeResult struct {
	EntryID string `json:"entry_id"`
	Gen   int64      `json:"gen"`
	State EntryState `json:"state"`
	ActiveRefs int  `json:"active_refs"`
	Deferred   bool `json:"deferred"`
}
