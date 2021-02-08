package amlhold

import (
	"time"
)

// HoldStatus 检疫 hold 生命周期状态机。
// open ──[SLA warn]──► aging ──[force_hold]──► escalated ──[compel]──► compel_review
// open/aging/escalated/compel_review ──[release ticket approved]──► released
// released 与 compel_review 是唯二终态（均不得再流转）。

type HoldStatus string

const (
	HoldStatusOpen HoldStatus = "open" // 初始：Case 开启，检疫 hold 等待审核
	HoldStatusAging HoldStatus = "aging" // SLA warn 触发；资金仍冻结，计时升级
	HoldStatusEscalated HoldStatus = "escalated" // 强制升级；二级审核介入
	HoldStatusCompelReview HoldStatus = "compel_review" // 督查级强制审查；最终闸
	HoldStatusReleased     HoldStatus = "released"      // 解冻完成；Case 可 Resolved
	HoldStatusAborted      HoldStatus = "aborted"       // 异常终止（系统管理员级 override）
)

// AgingTier 老化级别，与 HoldStatus 升级路径一致。

type AgingTier string

const (
	AgingTierNone AgingTier = "none" // hold 尚未进入老化
	AgingTierWarn AgingTier = "warn" // SLA 超时警告
	AgingTierForceHold AgingTier = "force_hold" // 强制保持冻结，不得解冻
	AgingTierEscalated AgingTier = "escalated" // 升级至督查层
	AgingTierCompelReview AgingTier = "compel_review" // 最终督查审查
)

// QuarantineHold 检疫资金冻结令牌，与 AML Case 1:N 绑定（一个 Case 可有多笔资金腿）。
// 账本冻结位以 LedgerFrozenBit 为准；QuarantineHold 是检疫层的占用令牌，
// 漂移时以账本为准并开缺口单（GapTicketID）。
type QuarantineHold struct {
	ID string `json:"id"`
	CaseID          string `json:"case_id"`
	UserID          string `json:"user_id"`
	Asset           string `json:"asset"`
	Amount          string `json:"amount"`            // 冻结金额（字符串保持精度）
	LedgerFrozenBit bool   `json:"ledger_frozen_bit"` // 账本冻结位（只读观察）
	Status    HoldStatus `json:"status"`
	AgingTier AgingTier  `json:"aging_tier"`
	OpenedBy  string     `json:"opened_by"`
	ReleasedBy string `json:"released_by,omitempty"`
	AbortedBy  string `json:"aborted_by,omitempty"`
	GapTicketID string    `json:"gap_ticket_id,omitempty"` // 账本漂移缺口单
	SLALimit    int64     `json:"sla_limit_sec"`           // SLA 时限（秒）
	OpenedAt    time.Time `json:"opened_at"`
	AgingAt     time.Time `json:"aging_at,omitempty"`
	EscalatedAt time.Time `json:"escalated_at,omitempty"`
	ReleasedAt time.Time `json:"released_at,omitempty"`
	AbortedAt  time.Time `json:"aborted_at,omitempty"`
	FenceEpoch int64     `json:"fence_epoch"`
	FenceToken string `json:"fence_token"`
}

// Open 报告 hold 是否处于活跃冻结状态（非终态）。

   func (h QuarantineHold) Open() bool {
	  return h.Status == HoldStatusOpen ||
		   h.Status == HoldStatusAging ||
		   h.Status == HoldStatusEscalated ||
		  h.Status == HoldStatusCompelReview
   }

// Terminal 报告 hold 是否已到终态。
   func (h QuarantineHold) Terminal() bool {
	   return h.Status == HoldStatusReleased || h.Status == HoldStatusAborted
   }

// AgingEvent 老化升级事件，追加式存储（aging_events 表）。

type AgingEvent struct {
	ID string `json:"id"`
	HoldID string `json:"hold_id"`
	CaseID   string    `json:"case_id"`
	FromTier AgingTier `json:"from_tier"`
	ToTier AgingTier `json:"to_tier"`
	FromStatus HoldStatus `json:"from_status"`
	ToStatus   HoldStatus `json:"to_status"`
	Actor string `json:"actor"`
	Reason string    `json:"reason"`
	Age    int64     `json:"age_sec"` // hold 已存活秒数
	At     time.Time `json:"at"`
}

// ReleaseTicket 解冻申请单。解冻与结案必须不同 actor（换人四眼）。
// 解冻申请须挂起直到审批，审批通过后方可执行解冻并更新 QuarantineHold.Status→released。

type ReleaseTicket struct {
	ID     string `json:"id"`
	HoldID string `json:"hold_id"`
	CaseID string `json:"case_id"`
	RequestedBy string `json:"requested_by"`
	ApprovedBy  string `json:"approved_by,omitempty"`
	RejectedBy string `json:"rejected_by,omitempty"`
	Status TicketStatus `json:"status"`
	Reason       string `json:"reason"`
	RejectReason string `json:"reject_reason,omitempty"`
	ProofID      string `json:"proof_id,omitempty"` // 关联 HoldProof
	CreatedAt time.Time `json:"created_at"`
	DecidedAt  time.Time `json:"decided_at,omitempty"`
	FenceEpoch int64     `json:"fence_epoch"`
	FenceToken string `json:"fence_token"`
}

// TicketStatus 解冻申请单状态。
type TicketStatus string

const (
	TicketStatusPending TicketStatus = "pending" // 申请待审批
	TicketStatusApproved TicketStatus = "approved" // 已批准，解冻执行中
	TicketStatusRejected TicketStatus = "rejected" // 被拒绝
	TicketStatusDone TicketStatus = "done" // 解冻已落账
)

// HoldProof 检疫占用/升级/解冻操作证明，只追加，供 desk 与审计只读。
type HoldProof struct {
	ID     string `json:"id"`
	HoldID string `json:"hold_id"`
	CaseID string `json:"case_id"`
	TicketID string `json:"ticket_id,omitempty"`
	EventID  string `json:"event_id,omitempty"`
	ProofKind ProofKind `json:"proof_kind"`
	Actor     string    `json:"actor"`
	Summary   string    `json:"summary"`
	Checksum  string    `json:"checksum"` // SHA-256(kind+hold_id+case_id+actor+summary)
	At        time.Time `json:"at"`
}

// ProofKind 证明类型。

type ProofKind string

const (
	ProofKindOccupy ProofKind = "occupy" // 检疫占用开启证明
	ProofKindAging   ProofKind = "aging"   // 老化升级证明
	ProofKindRelease ProofKind = "release" // 解冻证明
	ProofKindAbort   ProofKind = "abort"   // 异常终止证明
	ProofKindDrift   ProofKind = "drift"   // 账本漂移缺口证明
	ProofKindEscalate ProofKind = "escalate" // 督查升级证明
)

// GapTicket 账本漂移缺口单。检疫令牌与账本冻结位不一致时自动创建。
type GapTicket struct {
	ID string `json:"id"`
	HoldID string `json:"hold_id"`
	CaseID string `json:"case_id"`
	UserID string `json:"user_id"`
	Asset       string `json:"asset"`
	HoldAmount  string `json:"hold_amount"`  // 检疫层认为应冻结量
	LedgerState string `json:"ledger_state"` // 账本实际状态描述
	Reason      string `json:"reason"`
	Resolved   bool      `json:"resolved"`
	CreatedAt  time.Time `json:"created_at"`
	ResolvedAt time.Time `json:"resolved_at,omitempty"`
}

// HoldFilter 查询过滤器。

type HoldFilter struct {
	CaseID string
	UserID string
	Status HoldStatus
	AgingTier AgingTier
	OpenOnly  bool
	Limit     int
}

// AgingConfig 老化升级配置。

type AgingConfig struct {
	WarnAfter      int64 // 秒：超过 SLA 多久后进入 warn（通常=SLA 时限）
	ForceHoldAfter int64 // 秒：warn 后多久强制 force_hold
	EscalateAfter int64 // 秒：force_hold 后多久升级 escalated
	CompelReviewAfter int64 // 秒：escalated 后多久强制 compel_review
}

// DefaultAgingConfig 默认老化配置（SLA=24h，之后每 12h 一阶）。

var DefaultAgingConfig = AgingConfig{
	WarnAfter:         86400,  // 24h
	ForceHoldAfter:    129600, // 36h
	EscalateAfter:     172800, // 48h
	CompelReviewAfter: 259200, // 72h
}
