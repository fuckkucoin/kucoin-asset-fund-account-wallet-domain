package acctlifegate

import "time"

// ─────────────────────────── BookLifeGeneration ─────────────────────────────
// BookLifeGeneration per-(user, book) 生命周期世代记录。
// Open/Freeze/Close 每次成功后 generation 加一并写入新行（不改旧行，
// 以便重放和审计）。GateMove 持有的 LifeLease 必须与最新行的 generation
// 一致，否则拒绝。
type BookLifeGeneration struct {
	GenID  string `json:"gen_id"`
	UserID string `json:"user_id"`
	Book string `json:"book"`
	Generation int64 `json:"generation"`
	Action string `json:"action"` // "open" | "freeze" | "close"
	Actor  string `json:"actor"`  // 操作发起方（系统/ops）
	CreatedAt time.Time `json:"created_at"`
}

// ─────────────────────────── LifeLease ──────────────────────────────────────
// LeaseState 生命周期世代租约状态。
// open → used : Move 执行成功，租约消费
// open → expired : TTL 超期，租约作废
// open → revoked : Freeze/Close 将本租约吊销（在途 Move 须重新获取）
type LeaseState string

const (
	LeaseStateOpen LeaseState = "open"
	LeaseStateUsed    LeaseState = "used"
	LeaseStateExpired LeaseState = "expired"
	LeaseStateRevoked LeaseState = "revoked"
)

// DefaultLeaseTTL 生命周期世代租约默认存活期。
// 超期后 worker 将其标记为 expired；Move 时碰到过期租约立即返回 ErrLeaseExpired。

const DefaultLeaseTTL = 5 * time.Minute

// LifeLease 生命周期世代租约。
// GateMove 调用方在执行 Move 前须先通过 AcquireLease 取得；租约携带
// 当前 generation 并在 TTL 内有效。Freeze/Close 成功时吊销同
// (user,book) 的全部 open 租约；被吊销的 Move 须重新获取租约（彼时
// Freeze/Close 已完成，重新获取将携带新 generation，若 Freeze/Close 后
// 科目不再允许借记则 AcquireLease 直接拒绝）。
type LifeLease struct {
	LeaseID string `json:"lease_id"`
	UserID  string `json:"user_id"`
	Book string `json:"book"`
	Generation int64 `json:"generation"`
	State LeaseState `json:"state"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
	UsedAt     *time.Time `json:"used_at,omitempty"`
	ExpiredAt  *time.Time `json:"expired_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	RevokeNote string     `json:"revoke_note,omitempty"`
}

// IsExpired 判断租约是否已超期（与系统时钟）。

   func (l LifeLease) IsExpired(now time.Time) bool {
	   return now.After(l.ExpiresAt)
  }

// IsUsable 租约是否可被 Move 消费：必须处于 open 且未超期。
  func (l LifeLease) IsUsable(now time.Time) bool {
	   return l.State == LeaseStateOpen && !l.IsExpired(now)
   }

// ─────────────────────────── MoveBind ───────────────────────────────────────
// MoveBindState 在途 Move 绑定状态。
// pending → completed : Move 完成，科目余额已落账
// pending → compensated: Move 被回滚补偿（GateMove 绑意图失败时）
// pending → timed_out : worker 巡检超时，标记为孤儿
type MoveBindState string

const (
	MoveBindPending     MoveBindState = "pending"
	MoveBindCompleted   MoveBindState = "completed"
	MoveBindCompensated MoveBindState = "compensated"
	MoveBindTimedOut MoveBindState = "timed_out"
)

// MoveBindTTL 在途 Move 绑定默认超时期。
// 超过此时间未完成则 worker 标记为 timed_out，并释放 Freeze/Close 等待队列。
const MoveBindTTL = 10 * time.Minute

// MoveBind 在途 Move 绑定记录。
// Move 执行前由 acctlifegate 写入（状态 pending）；Move 成功/补偿/超时后
// 更新状态。Freeze/Close 发现 pending 绑定时拒绝（或在配置为排队模式时挂起）。
// pending 条目是 Freeze/Close 的互斥屏障。
type MoveBind struct {
	BindID   string `json:"bind_id"`
	UserID   string `json:"user_id"`
	FromBook string `json:"from_book"`
	ToBook string `json:"to_book"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	LeaseID    string `json:"lease_id"`
	Generation int64  `json:"generation"`
	IdemKey    string        `json:"idem_key"`
	State      MoveBindState `json:"state"`
	BoundAt    time.Time     `json:"bound_at"`
	ExpiresAt  time.Time     `json:"expires_at"`
	ResolvedAt *time.Time    `json:"resolved_at,omitempty"`
	Note       string        `json:"note,omitempty"`
}

// IsPending 绑定是否仍在途。
  func (m MoveBind) IsPending() bool { return m.State == MoveBindPending }

// IsTimedOut 绑定是否已超时。
   func (m MoveBind) IsTimedOut(now time.Time) bool {
	  return m.State == MoveBindPending && now.After(m.ExpiresAt)
   }

// ─────────────────────────── LifeDriftCase ──────────────────────────────────
// DriftState 生命周期漂移案件状态。
// open → proven : ops 双人确认证明字段通过
// proven → closed : 修复完成，挡写屏障释放
type DriftState string

const (
	DriftStateOpen DriftState = "open"
	DriftStateProven DriftState = "proven"
	DriftStateClosed DriftState = "closed"
)

// LifeDriftCase 半开科目漂移案件。
// 发现路径：CheckHop 时科目 active → GateMove 时科目已 frozen/closed。
// 触发时写入案件（状态 open）并将 BlockedAt 置为当前时间；
// 此后同 (user, book) 的借记操作（CanDebit 调用路径）须先检查是否存在
// open/proven 的漂移案件，若有则拒绝（ErrDriftBlockWrite）。
// 案件收口（closed）后屏障解除。
// ProofFields 是漂移证明必须包含的字段；ops 须填写全部字段后方可 Prove。
type LifeDriftCase struct {
	CaseID string `json:"case_id"`
	UserID     string    `json:"user_id"`
	Book       string    `json:"book"`
	DetectedAt time.Time `json:"detected_at"`
	CheckHopAt    time.Time `json:"check_hop_at"`    // CheckHop 通过时刻
	MoveAttemptAt time.Time `json:"move_attempt_at"` // Move 碰到冻结时刻
	StatusAtCheck string `json:"status_at_check"` // CheckHop 时的状态（active）
	StatusAtMove string `json:"status_at_move"` // Move 时的状态（frozen/closed）
	LeaseID        string `json:"lease_id"`          // 触发 Move 所持的 LifeLease
	MoveIdemKey    string `json:"move_idem_key"`     // 触发 Move 的幂等键
	LifeGenAtCheck int64  `json:"life_gen_at_check"` // CheckHop 时的 generation
	LifeGenAtMove int64      `json:"life_gen_at_move"` // Move 时的 generation（已抬升）
	State         DriftState `json:"state"`
	BlockedAt     *time.Time `json:"blocked_at,omitempty"` // 挡写屏障开始时刻
	ProvenAt      *time.Time `json:"proven_at,omitempty"`
	ClosedAt      *time.Time `json:"closed_at,omitempty"`
	Actor1 string `json:"actor1,omitempty"`
	Actor2    string `json:"actor2,omitempty"`
	ProofNote string `json:"proof_note,omitempty"`
	CloseNote string `json:"close_note,omitempty"`
}

// IsBlockingDebit 案件是否仍在屏蔽借记。
// open 与 proven 状态均屏蔽；仅 closed 状态释放。
  func (c LifeDriftCase) IsBlockingDebit() bool {
	  return c.State == DriftStateOpen || c.State == DriftStateProven
  }

// ─────────────────────────── Request / Response ─────────────────────────────
// AcquireLeaseRequest 发放生命周期世代租约请求。
// GateMove 调用前须发起此请求；服务端校验科目当前可借记后发放。

type AcquireLeaseRequest struct {
	UserID string `json:"user_id"`
	Book   string `json:"book"`
	Actor string `json:"actor,omitempty"`
	TTL time.Duration `json:"ttl,omitempty"`
}

// BindMoveRequest 在 Move 执行前创建 MoveBind 并持有互斥屏障。

type BindMoveRequest struct {
	UserID string `json:"user_id"`
	FromBook string `json:"from_book"`
	ToBook string `json:"to_book"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	LeaseID string `json:"lease_id"`
	IdemKey string `json:"idem_key"`
}

// ReleaseMoveRequest Move 结束后释放 MoveBind。
type ReleaseMoveRequest struct {
	BindID string        `json:"bind_id"`
	State  MoveBindState `json:"state"` // completed | compensated
	Note   string        `json:"note,omitempty"`
}

// FreezeRequest 冻结请求（含生命周期世代联动）。
type FreezeRequest struct {
	UserID string `json:"user_id"`
	Book   string `json:"book"`
	Frozen bool `json:"frozen"`
	Actor  string `json:"actor,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// CloseRequest 关闭科目请求（含生命周期世代联动）。

type CloseRequest struct {
	UserID string `json:"user_id"`
	Book   string `json:"book"`
	Actor  string `json:"actor,omitempty"`
}

// OpenRequest 开通科目请求（含生命周期世代联动）。
type OpenRequest struct {
	UserID string `json:"user_id"`
	Book   string `json:"book"`
	Actor string `json:"actor,omitempty"`
}

// ReportDriftRequest 上报半开漂移（CheckHop active → Move frozen/closed）。

type ReportDriftRequest struct {
	UserID     string    `json:"user_id"`
	Book       string    `json:"book"`
	CheckHopAt time.Time `json:"check_hop_at"`
	MoveAttemptAt time.Time `json:"move_attempt_at"`
	StatusAtCheck string    `json:"status_at_check"`
	StatusAtMove string `json:"status_at_move"`
	LeaseID      string `json:"lease_id"`
	MoveIdemKey    string `json:"move_idem_key"`
	LifeGenAtCheck int64  `json:"life_gen_at_check"`
	LifeGenAtMove int64 `json:"life_gen_at_move"`
}

// ProveDriftRequest ops 双人确认漂移证明。
type ProveDriftRequest struct {
	CaseID string `json:"case_id"`
	Actor1 string `json:"actor1"`
	Actor2 string `json:"actor2"`
	ProofNote string `json:"proof_note"`
}

// CloseDriftRequest ops 关闭漂移案件（修复完成）。
type CloseDriftRequest struct {
	CaseID string `json:"case_id"`
	CloseNote string `json:"close_note"`
	ClosedBy  string `json:"closed_by"`
}

// LifeGenView 世代查询响应。

type LifeGenView struct {
	UserID string `json:"user_id"`
	Book string `json:"book"`
	Generation int64     `json:"generation"`
	LastAction string    `json:"last_action"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// BookDriftSummary ops 查询科目漂移概要。

type BookDriftSummary struct {
	UserID string `json:"user_id"`
	Book   string `json:"book"`
	Generation   int64           `json:"generation"`
	OpenCases    []LifeDriftCase `json:"open_cases,omitempty"`
	DebitBlocked bool            `json:"debit_blocked"`
}
