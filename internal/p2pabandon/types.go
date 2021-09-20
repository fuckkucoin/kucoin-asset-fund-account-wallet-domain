package p2pabandon

import (
	"fmt"
	"time"
)

// ─────────────────────────── AbandonPlan 弃单计划 ───────────────────────────
// PlanStatus 弃单计划状态机。
// armed: 计划已就绪，等待超时触发弃单释放。
// abandoning: 弃单释放流程进行中（已持有释放令牌，互斥 Fill 路径）。
// released: 保证金已成功释放，资金还原用户可用余额。终态。
// filled: 订单在弃单期间被成交（Fill 路径先提交）。终态。
// conflict: 释放与成交两路同时到达且互斥失败，需人工仲裁。终态（需 desk 处理）。
type PlanStatus string

const (
	PlanStatusArmed      PlanStatus = "armed"
	PlanStatusAbandoning PlanStatus = "abandoning"
	PlanStatusReleased PlanStatus = "released"
	PlanStatusFilled   PlanStatus = "filled"
	PlanStatusConflict PlanStatus = "conflict"
)

   func (s PlanStatus) Terminal() bool {
	   switch s {
	  case PlanStatusReleased, PlanStatusFilled, PlanStatusConflict:
		  return true
	  default:
		   return false
	   }
   }

   func (s PlanStatus) Valid() bool {
	  switch s {
	  case PlanStatusArmed,
		   PlanStatusAbandoning, PlanStatusReleased,
		   PlanStatusFilled, PlanStatusConflict:
		   return true
	   default:
		   return false
	  }
  }

   func ParsePlanStatus(s string) (PlanStatus, error) {
	   ps := PlanStatus(s)
	   if !ps.Valid() {
		   return "", fmt.Errorf("p2pabandon: plan status %q unknown", s)
	  }
	  return ps, nil
   }

// AbandonPlan 弃单计划：每个 P2P 卖单持仓超时后生成唯一一条弃单计划。
// 驱动源：P2P order_id + 超时计时器（由调用方注入，本包不写撮合）。
// 隔离约定：
// - plan_id 供他包只读（abandon_plan_id 外键）。
// - 不引用 dispute case_id；不写争议追偿腿。
// - hold_id 只读引用 holdcat.Book 的 hold 行。
type AbandonPlan struct {
	ID string `json:"id"`
	OrderID string `json:"order_id"` // P2P 卖单 ID（驱动源）
	UserID  string `json:"user_id"`  // 保证金所有人
	Asset  string `json:"asset"`  // 保证金资产
	Amount string `json:"amount"` // 保证金金额（DECIMAL 字符串）
	HoldID string `json:"hold_id"` // holdcat hold 行 ID（只读引用）
	Status       PlanStatus `json:"status"`
	ArmedAt      time.Time  `json:"armed_at"`                // 计划就绪时间
	AbandoningAt *time.Time `json:"abandoning_at,omitempty"` // 进入 abandoning 时间
	TerminalAt   *time.Time `json:"terminal_at,omitempty"`   // 终态时间
	TerminalBy string `json:"terminal_by,omitempty"` // 终态执行人（自动=system，人工=operator ID）
	ConflictNote string `json:"conflict_note,omitempty"` // conflict 状态时的冲突说明
	// SLA 配置（就绪时快照，防止事后修改影响已有计划）
	TimeoutSecs int64 `json:"timeout_secs"` // 保证金超时阈值（秒）
	GraceSecs   int64 `json:"grace_secs"`   // 宽限期（秒）
	SLAEscalateSecs int64 `json:"sla_escalate_secs"` // SLA 升级阈值（秒）
	// 乐观锁
	Version int `json:"version"`
	FenceEpoch int64     `json:"fence_epoch,omitempty"`
	FenceToken string    `json:"fence_token,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

   func (p AbandonPlan) Copy() AbandonPlan { return p }

// ─────────────────────────── 互斥令牌 ───────────────────────────
// TokenKind 令牌类型：弃单释放 vs 迟成交填充。
type TokenKind string

const (
	TokenKindRelease TokenKind = "release" // 弃单释放路径占用令牌
	TokenKindFill    TokenKind = "fill"    // 迟成交贷记路径占用令牌
)

  func (k TokenKind) Valid() bool {
	  return k == TokenKindRelease || k == TokenKindFill
   }

// TokenStatus 互斥令牌状态机。
// reserved: 令牌已预留，对应路径已开始推进。
// committed: 路径成功完成，令牌已 committed（对手方路径将被 aborted）。
// aborted: 路径被另一方抢占或主动放弃（对手方已 committed）。

type TokenStatus string

const (
	TokenStatusReserved TokenStatus = "reserved"
	TokenStatusCommitted TokenStatus = "committed"
	TokenStatusAborted TokenStatus = "aborted"
)

   func (s TokenStatus) Terminal() bool {
	  return s == TokenStatusCommitted || s == TokenStatusAborted
  }

  func (s TokenStatus) Valid() bool {
	  switch s {
	   case TokenStatusReserved, TokenStatusCommitted, TokenStatusAborted:
		   return true
	   default:
		   return false
	  }
   }

// MutexToken 互斥令牌：保证同一订单的 release 与 fill 路径中仅一方能 committed。
// 不变式：
// - 同一 plan_id 下，release 令牌与 fill 令牌互斥。
// - 先 committed 的令牌锁定终态；另一方必须 aborted。
// - 两方均 committed 时为冲突（ConflictDetected=true），计划进入 conflict 状态。

type MutexToken struct {
	ID      string `json:"id"`
	PlanID  string `json:"plan_id"`  // 关联 AbandonPlan.ID
	OrderID string `json:"order_id"` // 冗余，便于日志
	Kind TokenKind `json:"kind"`
	Status TokenStatus `json:"status"`
	ReservedBy  string `json:"reserved_by"`            // 预留操作人（system / fill_worker）
	CommittedBy string `json:"committed_by,omitempty"` // 提交操作人
	AbortedBy   string `json:"aborted_by,omitempty"`   // 中止操作人
	AbortReason string `json:"abort_reason,omitempty"` // 中止原因
	// ConflictDetected 当两个令牌均尝试 commit 时置 true，触发 conflict 流程。
	ConflictDetected bool `json:"conflict_detected"`
	LedgerRef string `json:"ledger_ref,omitempty"` // 账本分录 ID（只读）
	HoldRef string `json:"hold_ref,omitempty"` // holdcat hold ID（只读）
	ReservedAt time.Time  `json:"reserved_at"`
	TerminalAt *time.Time `json:"terminal_at,omitempty"`
	Version    int        `json:"version"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

   func (t MutexToken) Copy() MutexToken { return t }

// ─────────────────────────── HoldAge 保证金超龄 ───────────────────────────
// AgeStage 保证金超龄阶段。
// normal: hold 年龄在 SLA 阈值内，正常监控。
// warning: 临近 SLA 阈值（>75%），触发预警。
// escalated: 超过 SLA 阈值，已升级至 desk 干预。
// resolved: 已由 desk 处理（强制弃单或其他处置）。
type AgeStage string

const (
	AgeStageNormal AgeStage = "normal"
	AgeStageWarning AgeStage = "warning"
	AgeStageEscalated AgeStage = "escalated"
	AgeStageResolved AgeStage = "resolved"
)

  func (s AgeStage) Terminal() bool {
	  return s == AgeStageResolved
  }

   func (s AgeStage) Valid() bool {
	   switch s {
	  case AgeStageNormal, AgeStageWarning, AgeStageEscalated, AgeStageResolved:
		   return true
	   default:
		  return false
	   }
   }

// HoldAge 保证金超龄记录：按 SLA 进行巡检与升级。
// 隔离约定：本表由 P2P order_id + hold 时间驱动，不写争议 Case 升级。
type HoldAge struct {
	ID string `json:"id"`
	PlanID  string `json:"plan_id"`  // 关联 AbandonPlan.ID
	OrderID string `json:"order_id"` // P2P 卖单 ID
	UserID  string `json:"user_id"`
	Asset string `json:"asset"`
	Amount      string   `json:"amount"`
	HoldID      string   `json:"hold_id"` // holdcat hold 行 ID（只读）
	Stage       AgeStage `json:"stage"`
	HoldAgeSecs int64    `json:"hold_age_secs"` // 当前 hold 已持续秒数（快照值）
	SLASecs int64 `json:"sla_secs"` // SLA 阈值（秒）
	// 升级信息
	EscalatedAt *time.Time `json:"escalated_at,omitempty"`
	EscalatedTo string     `json:"escalated_to,omitempty"` // desk 操作人 / 组
	EscalateNote string `json:"escalate_note,omitempty"` // 升级说明
	// 处置信息
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
	ResolvedBy  string     `json:"resolved_by,omitempty"`
	ResolveNote string     `json:"resolve_note,omitempty"`
	// 巡检次数
	PatrolCount int        `json:"patrol_count"`
	LastPatrol  *time.Time `json:"last_patrol,omitempty"`
	Version int `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

   func (h HoldAge) Copy() HoldAge { return h }

// ─────────────────────────── CloseProof 收口证明 ───────────────────────────
// CloseOutcome 收口证明终态：三选一。

type CloseOutcome string

const (
	OutcomeReleased CloseOutcome = "released" // 弃单释放：保证金还原用户可用
	OutcomeFilled   CloseOutcome = "filled"   // 订单成交：保证金已转对手方
	OutcomeConflict CloseOutcome = "conflict" // 竞态冲突：需人工仲裁
)

   func (o CloseOutcome) Valid() bool {
	  switch o {
	   case OutcomeReleased, OutcomeFilled, OutcomeConflict:
		  return true
	   default:
		   return false
	  }
   }

// CloseProof 收口证明：弃单/释放/成交三选一终态证明。
// 不变式：
// - ProofHash = SHA256(plan_id + order_id + outcome + committed_token_id + amount + asset)。
// - CloseProof 缺失时，资金面不得标 closed（调用方须先调用 ValidateClose）。
// - 一个 AbandonPlan 至多一条 CloseProof（幂等生成）。

type CloseProof struct {
	ID     string `json:"id"`
	PlanID string `json:"plan_id"` // 关联 AbandonPlan.ID
	OrderID string `json:"order_id"` // P2P 卖单 ID
	UserID           string       `json:"user_id"`
	Asset            string       `json:"asset"`
	Amount           string       `json:"amount"`
	Outcome          CloseOutcome `json:"outcome"`            // released | filled | conflict
	CommittedTokenID string       `json:"committed_token_id"` // 已提交令牌 ID
	CommittedKind TokenKind `json:"committed_kind"` // release | fill
	ProofHash     string    `json:"proof_hash"`     // SHA256 防篡改
	HoldRef       string    `json:"hold_ref"`       // holdcat hold 行 ID（只读）
	LedgerRef     string    `json:"ledger_ref"`     // 账本分录 ID（只读）
	DecidedBy string `json:"decided_by"` // 生成者
	// 篡改检测
	TamperDetected  bool       `json:"tamper_detected"`
	TamperDetail    string     `json:"tamper_detail,omitempty"`
	TamperCheckedAt *time.Time `json:"tamper_checked_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

   func (p CloseProof) Copy() CloseProof { return p }

// ─────────────────────────── 请求/响应类型 ───────────────────────────
// ArmPlanReq 创建/就绪弃单计划请求（由超时扫描 worker 调用）。

type ArmPlanReq struct {
	OrderID string `json:"order_id"`
	UserID  string `json:"user_id"`
	Asset string `json:"asset"`
	Amount      string `json:"amount"`
	HoldID      string `json:"hold_id"`
	TimeoutSecs int64  `json:"timeout_secs"`
	GraceSecs int64 `json:"grace_secs"`
	SLAEscalateSecs int64 `json:"sla_escalate_secs"`
	IdempotencyKey string `json:"idempotency_key"`
}

// AdvanceAbandonReq 推进弃单释放流程请求（abandon-release advance worker 调用）。

type AdvanceAbandonReq struct {
	PlanID string `json:"plan_id"`
	Actor  string `json:"actor"`
}

// CommitReleaseReq 提交释放令牌（弃单路径成功后调用）。
type CommitReleaseReq struct {
	PlanID    string `json:"plan_id"`
	TokenID   string `json:"token_id"`
	LedgerRef string `json:"ledger_ref,omitempty"`
	HoldRef string `json:"hold_ref,omitempty"`
	Actor   string `json:"actor"`
}

// CommitFillReq 提交成交令牌（P2P 成交贷记成功后调用）。

type CommitFillReq struct {
	PlanID    string `json:"plan_id"`
	TokenID   string `json:"token_id"`
	LedgerRef string `json:"ledger_ref,omitempty"`
	Actor string `json:"actor"`
}

// ForceAbandonReq 强制弃单请求（desk 双人确认后调用）。

type ForceAbandonReq struct {
	PlanID string `json:"plan_id"`
	Operator1 string `json:"operator1"` // 操作人1（申请人）
	Operator2 string `json:"operator2"` // 操作人2（确认人，四眼）
	Reason string `json:"reason"`
	AgeRecordID string `json:"age_record_id,omitempty"` // 关联 HoldAge.ID（可选）
}

// EscalateAgeReq 保证金超龄升级请求。
type EscalateAgeReq struct {
	AgeID        string `json:"age_id"`
	EscalateTo   string `json:"escalate_to"`
	EscalateNote string `json:"escalate_note"`
}

// ResolveAgeReq 超龄记录处置请求。
type ResolveAgeReq struct {
	AgeID      string `json:"age_id"`
	ResolvedBy string `json:"resolved_by"`
	ResolveNote string `json:"resolve_note"`
}

// ValidateCloseReq 收口校验请求（调用方在标 closed 前必须调用）。

type ValidateCloseReq struct {
	PlanID  string `json:"plan_id"`
	OrderID string `json:"order_id"`
}

// ValidateCloseResp 收口校验响应。

type ValidateCloseResp struct {
	PlanID   string `json:"plan_id"`
	OrderID  string `json:"order_id"`
	CanClose bool   `json:"can_close"` // true=可标 closed，false=不得标 closed
	Reason string `json:"reason"` // 拒绝原因（CanClose=false 时填充）
	Outcome CloseOutcome `json:"outcome"` // 终态（CanClose=true 时填充）
	ProofID string `json:"proof_id"` // CloseProof.ID（CanClose=true 时填充）
}

// ─────────────────────────── SLA 配置 ───────────────────────────
// SLAConfig SLA 参数配置（可从 config 注入，也可使用默认值）。
type SLAConfig struct {
	// OrderTimeoutSecs 卖单保证金超时阈值（秒）。超过此阈值自动生成 AbandonPlan。
	// 默认 1800（30 分钟）。
	OrderTimeoutSecs int64 `yaml:"order_timeout_secs" json:"order_timeout_secs"`
	// GraceSecs 宽限期（秒）：超时后再等待此时间才实际发起弃单释放。
	// 防止因网络延迟导致的误弃单。默认 60 秒。
	GraceSecs int64 `yaml:"grace_secs" json:"grace_secs"`
	// SLAEscalateSecs 保证金超龄 SLA 升级阈值（秒）。
	// 超过此值时 HoldAge 升级为 escalated。默认 86400（24 小时）。
	SLAEscalateSecs int64 `yaml:"sla_escalate_secs" json:"sla_escalate_secs"`
	// SLAWarningRatio HoldAge 预警比例（0~1），达到 SLAEscalateSecs*ratio 时进入 warning。
	// 默认 0.75。
	SLAWarningRatio float64 `yaml:"sla_warning_ratio" json:"sla_warning_ratio"`
}

// DefaultSLAConfig 默认 SLA 配置。

var DefaultSLAConfig = SLAConfig{
	OrderTimeoutSecs: 1800,
	GraceSecs:        60,
	SLAEscalateSecs: 86400,
	SLAWarningRatio: 0.75,
}

// ─────────────────────────── 信号类型（只读钩子） ───────────────────────────
// OrderTimeoutSignal 订单超时信号（由调用方注入，本包不写 P2P 撮合）。
// 调用方在 P2P 卖单超时时发射此信号；本包监听并触发弃单计划创建。

type OrderTimeoutSignal struct {
	OrderID string `json:"order_id"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	HoldID string `json:"hold_id"`
	TimedOutAt time.Time `json:"timed_out_at"`
}

// HoldObservation holdcat 只读观察（不写 holdcat 核心 FSM）。

type HoldObservation struct {
	HoldID string `json:"hold_id"`
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	Reason string `json:"reason"`
	Open bool `json:"open"`
	CreatedAt time.Time `json:"created_at"`
}

// CooldownObservation 出金冷却只读观察（复用 p2pgate.Funds.CoolingState 路径，不改冷却语义）。
type CooldownObservation struct {
	UserID string    `json:"user_id"`
	Locked bool      `json:"locked"`
	Until  time.Time `json:"until"`
}
