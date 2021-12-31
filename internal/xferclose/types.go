// 功能：划转半截收口核心类型。
// 模型：HalfHop（半截 hop 工单）、CloseoutPlan（收口计划）、CloseoutLeg（收口腿）、XferSLA（SLA 规则）。
// 状态机约定：
// HalfHop: detected → closing → closed | escalated
// CloseoutPlan: open → running → done | aborted
// CloseoutLeg: pending → running → done | failed
// MirrorView: 在 xfermirror 包中定义（processing/credited/rolled_back/manual）。

// 作者：Lior
// 日期：2021.12.30

package xferclose

import (
	"time"
	"github.com/shopspring/decimal"
)

// HopState 半截 hop 工单的状态。
type HopState string

const (
	HopDetected HopState = "detected"
	HopClosing  HopState = "closing"
	HopClosed   HopState = "closed"
	HopEscalated HopState = "escalated"
)

// HopKind 半截单的成因分类。

type HopKind string

const (
	// HopKindOpen hop 已出腿但入腿尚未到账，对手方未达。
	HopKindOpen HopKind = "open"
	// HopKindStranded 出腿已成功，入腿长时间没有推进记录。
	HopKindStranded HopKind = "stranded"
	// HopKindTimeout hop 整体超出 SLA 时限且不处于终态。
	HopKindTimeout HopKind = "timeout"
)

// CloseoutAction 收口腿的处置动作。

type CloseoutAction string

const (
	// ActionRetryAdvance 重试正向推进（出腿已成功，接着尝试入腿）。
	ActionRetryAdvance CloseoutAction = "retry_advance"
	// ActionRollbackEgress 回滚出腿（撤销已冻结的出腿金额，解冻归还发起方）。
	ActionRollbackEgress CloseoutAction = "rollback_egress"
	// ActionBackfillIngress 补入腿（对手方确认未到账后系统补记入腿）。
	ActionBackfillIngress CloseoutAction = "backfill_ingress"
	// ActionEscalateDesk 转人工处理台。
	ActionEscalateDesk CloseoutAction = "escalate_desk"
	// ActionOpenIncident 开启事故工单。
	ActionOpenIncident CloseoutAction = "open_incident"
)

// PlanState 收口计划状态。
type PlanState string

const (
	PlanOpen PlanState = "open"
	PlanRunning PlanState = "running"
	PlanDone    PlanState = "done"
	PlanAborted PlanState = "aborted"
)

// LegState 收口腿执行状态。

type LegState string

const (
	LegPending LegState = "pending"
	LegRunning LegState = "running"
	LegDone    LegState = "done"
	LegFailed  LegState = "failed"
	LegSkipped LegState = "skipped"
)

// HalfHop 半截 hop 工单。由检测器写入，收口编排器消费。

type HalfHop struct {
	// ID 工单自身唯一标识。
	ID string `json:"id"`
	// HopID 对应 xferstore.Rec.ID 或 xferloop 中的案件 Ref（幂等键）。
	HopID string `json:"hop_id"`
	// TransferID 账本层 Transfer.ID，可选（跨片时为 IntentID）。
	TransferID string `json:"transfer_id,omitempty"`
	// UserID 发起方用户 ID。
	UserID string `json:"user_id"`
	// Asset 资产代码，大写。
	Asset string `json:"asset"`
	// Amount 划转金额。
	Amount decimal.Decimal `json:"amount"`
	// FromBook 出腿科目。
	FromBook string `json:"from_book"`
	// ToBook 入腿科目。
	ToBook string `json:"to_book"`
	// Kind 半截类型。
	Kind HopKind `json:"kind"`
	// State 工单状态。
	State HopState `json:"state"`
	// SLABreached SLA 已超出。
	SLABreached bool `json:"sla_breached"`
	// SLADeadline SLA 截止时间。
	SLADeadline time.Time `json:"sla_deadline"`
	// SLARuleID 匹配的 SLA 规则 ID。
	SLARuleID string `json:"sla_rule_id,omitempty"`
	// CloseoutPlanID 关联的收口计划 ID（生成后填入）。
	CloseoutPlanID string `json:"closeout_plan_id,omitempty"`
	// EscalateReason 升级原因。
	EscalateReason string `json:"escalate_reason,omitempty"`
	// XShard 是否为跨分片划转（跨片不由本包处理）。
	XShard bool `json:"x_shard,omitempty"`
	// DetectedAt 检测到的时间。
	DetectedAt time.Time `json:"detected_at"`
	// UpdatedAt 最后更新时间。
	UpdatedAt time.Time `json:"updated_at"`
	// ClosedAt 收口完成时间（终态后填入）。
	ClosedAt *time.Time `json:"closed_at,omitempty"`
}

// Terminal 报告 hop 工单是否处于终态。

   func (h HalfHop) Terminal() bool {
	   return h.State == HopClosed || h.State == HopEscalated
   }

// CloseoutPlan 收口计划。一个 HalfHop 对应最多一个有效计划（同一 HopID 不得两个 open/running）。

type CloseoutPlan struct {
	// ID 计划唯一标识。
	ID string `json:"id"`
	// HopID 关联的 HalfHop.HopID（幂等键）。
	HopID string `json:"hop_id"`
	// HalfHopID 关联 HalfHop.ID。
	HalfHopID string `json:"half_hop_id"`
	// State 计划状态。
	State PlanState `json:"state"`
	// SelectedAction 选定的收口动作。
	SelectedAction CloseoutAction `json:"selected_action"`
	// Reason 选定该动作的依据。
	Reason string `json:"reason,omitempty"`
	// Actor 触发收口的身份（系统自动填 "system"，人工填操作员 ID）。
	Actor string `json:"actor"`
	// Attempts 执行尝试次数。
	Attempts int `json:"attempts"`
	// LastError 上次执行错误。
	LastError string `json:"last_error,omitempty"`
	// DoneAt 完成时间。
	DoneAt *time.Time `json:"done_at,omitempty"`
	// AbortReason 中止原因。
	AbortReason string `json:"abort_reason,omitempty"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 最后更新时间。
	UpdatedAt time.Time `json:"updated_at"`
}

// CloseoutLeg 收口腿。每次收口计划执行对应一条或多条腿。

type CloseoutLeg struct {
	// ID 腿唯一标识。
	ID string `json:"id"`
	// PlanID 关联 CloseoutPlan.ID。
	PlanID string `json:"plan_id"`
	// HopID 冗余存储，方便按 hop 查询。
	HopID string `json:"hop_id"`
	// SeqNo 同一计划内的顺序号（从 1 开始）。
	SeqNo int `json:"seq_no"`
	// Action 本腿执行的动作。
	Action CloseoutAction `json:"action"`
	// State 腿状态。
	State LegState `json:"state"`
	// IdemKey 本腿操作的幂等键，防止重放时双补入。
	IdemKey string `json:"idem_key"`
	// EntryRef 账本操作返回的引用 ID（入账/回滚 entry）。
	EntryRef string `json:"entry_ref,omitempty"`
	// Note 执行结果说明。
	Note string `json:"note,omitempty"`
	// Attempts 本腿重试次数。
	Attempts int `json:"attempts"`
	// StartedAt 开始执行时间。
	StartedAt *time.Time `json:"started_at,omitempty"`
	// FinishedAt 执行完成时间。
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	// CreatedAt 腿创建时间。
	CreatedAt time.Time `json:"created_at"`
}

// XferSLA SLA 规则。按资产/金额段/路径匹配，最具体者先。
type XferSLA struct {
	// ID 规则 ID。
	ID string `json:"id"`
	// Name 规则名称（可读标识）。
	Name string `json:"name"`
	// Asset 资产过滤（空表示匹配所有资产）。
	Asset string `json:"asset,omitempty"`
	// FromBook 出腿科目（空=任意）。
	FromBook string `json:"from_book,omitempty"`
	// ToBook 入腿科目（空=任意）。
	ToBook string `json:"to_book,omitempty"`
	// AmtMin 最小金额（含）。
	AmtMin decimal.Decimal `json:"amt_min"`
	// AmtMax 最大金额（0 表示无上限）。
	AmtMax decimal.Decimal `json:"amt_max"`
	// Timeout SLA 超时时限。
	Timeout time.Duration `json:"timeout"`
	// EscalateAfter 超时后自动升级等待时限（0 表示立即升级）。
	EscalateAfter time.Duration `json:"escalate_after"`
	// AutoCloseout 是否自动进入收口流程（否则仅升级告警）。
	AutoCloseout bool `json:"auto_closeout"`
	// DefaultAction 自动收口时使用的默认动作。
	DefaultAction CloseoutAction `json:"default_action"`
	// Enabled 规则是否启用。
	Enabled bool `json:"enabled"`
	// Priority 先级，数值越小越先。
	Priority int `json:"priority"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 最后更新时间。
	UpdatedAt time.Time `json:"updated_at"`
}

// SLAMatch 规则匹配结果。
type SLAMatch struct {
	Rule XferSLA
	Deadline time.Time
	Breached bool
}

// DetectResult 检测器扫描结果摘要。

type DetectResult struct {
	Scanned int `json:"scanned"`
	NewHops       int `json:"new_hops"`
	Breached      int `json:"breached"`
	AlreadyClosed int `json:"already_closed"`
	XShardSkip int `json:"x_shard_skip"`
}

// CloseoutResult 收口执行结果摘要。
type CloseoutResult struct {
	PlanID string         `json:"plan_id"`
	HopID  string         `json:"hop_id"`
	Action CloseoutAction `json:"action"`
	Success bool `json:"success"`
	Error string `json:"error,omitempty"`
}
