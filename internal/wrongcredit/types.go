// 功能：错贷记案件、证据信封、阶段定义与风险分层。
// 作者：Kinsley
// 日期：2021.12.30

package wrongcredit

import (
	"strings"
	"time"
)

// ─── 案件生命周期相位（用户/CS 可见层） ─────────────────────────────────────────
// Phase 是用户/CS 端可见的案件状态。追回编排器的内部 Step 驱动 Phase 推进；
// 两层解耦：CS 只关心 Phase，运营操作编排器内部 Step。
type Phase string

const (
	// PhaseInvestigating 已开案，发现引擎正在分值；可附加证据。
	PhaseInvestigating Phase = "investigating"
	// PhaseFrozenPending 可用余额已冻结，等待运营确认执行冲销。
	PhaseFrozenPending Phase = "frozen_pending"
	// PhaseReversed 全额冲销成功，终态。
	PhaseReversed Phase = "reversed"
	// PhasePartial 资金不足，已追回部分，剩余挂坏账，终态。
	PhasePartial Phase = "partial"
	// PhaseWaived 运营豁免，不再追回，终态。
	PhaseWaived Phase = "waived"
	// PhaseBadDebt 全额坏账挂账（无可用余额），终态。
	PhaseBadDebt Phase = "bad_debt"
)

// Terminal 是否已到终态（无需接着编排）。

  func (p Phase) Terminal() bool {
	   switch p {
	  case PhaseReversed, PhasePartial, PhaseWaived, PhaseBadDebt:
		  return true
	  default:
		  return false
	  }
   }

// ParsePhase 解析相位字符串；非法值返回 ErrPhase。
  func ParsePhase(s string) (Phase, error) {
	   switch Phase(strings.TrimSpace(s)) {
	  case PhaseInvestigating, PhaseFrozenPending, PhaseReversed,
		  PhasePartial, PhaseWaived, PhaseBadDebt:
		  return Phase(s), nil
	   default:
		   return "", ErrPhase
	  }
  }

// ─── 编排器内部 Step ────────────────────────────────────────────────────────────
// Step 是追回编排器 FSM 的内部节点；不直接暴露给 CS/用户。
type Step string

const (
	// StepOpen 案件已创建，等待发现引擎首次分值。
	StepOpen Step = "open"
	// StepScored 发现引擎已输出置信度 + 风险层级，等待运营审批冻结。
	StepScored Step = "scored"
	// StepFreezeAvailable 已提交冻结申请，锁定用户可用余额。
	StepFreezeAvailable Step = "freeze_available"
	// StepFrozen 冻结已确认，等待运营批准执行冲销。
	StepFrozen Step = "frozen"
	// StepReversalEntries 冲销分录已提交账本，等待确认。
	StepReversalEntries Step = "reversal_entries"
	// StepPartialClaw 可用余额不足，执行部分追回，剩余转 bad_debt。
	StepPartialClaw Step = "partial_claw"
	// StepBadDebtHold 剩余金额挂入 system.bad_debt 账户（坏账）。
	StepBadDebtHold Step = "bad_debt_hold"
	// StepClose 追回流程完结（reversed/partial/bad_debt 之一）。
	StepClose Step = "close"
	// StepWaived 运营豁免，跳过追回直接关闭。
	StepWaived Step = "waived"
	// StepAborted 编排器内部错误或互斥冲突，终止，需人工介入。
	StepAborted Step = "aborted"
)

// stepTerminal 编排步骤是否为终结点。
   func stepTerminal(s Step) bool {
	   switch s {
	  case StepClose, StepWaived, StepAborted:
		  return true
	   default:
		  return false
	  }
  }

// ─── 编排事件 ────────────────────────────────────────────────────────────────
// Event 编排器 FSM 转移事件。

type Event string

const (
	EventScore         Event = "score"          // 发现引擎完成分值
	EventApproveFreeze Event = "approve_freeze" // 运营批准冻结
	EventFrozeOK Event = "froze_ok" // 冻结分录完成确认
	EventApproveRevert Event = "approve_revert" // 运营批准执行冲销
	EventRevertOK      Event = "revert_ok"      // 全额冲销分录完成
	EventPartialOK     Event = "partial_ok"     // 部分追回完成
	EventBadDebtOK     Event = "bad_debt_ok"    // 坏账挂账完成
	EventWaive         Event = "waive"          // 运营豁免
	EventAbort Event = "abort" // 内部错误/互斥冲突终止
	EventReopenScore Event = "reopen_score" // 补充证据后重新分值
)

// ─── 发现来源 ─────────────────────────────────────────────────────────────────
// EvidenceSource 证据来源标签；同一案件可附多条不同来源的证据。
type EvidenceSource string

const (
	// SourceDeskReport 运营/CS 台面人工举报。
	SourceDeskReport EvidenceSource = "desk_report"
	// SourceReconHint 对账引擎输出的异常提示（自动）。
	SourceReconHint EvidenceSource = "recon_hint"
	// SourcePostHocAttrib 贷记后归因系统发现的错误归因（自动）。
	SourcePostHocAttrib EvidenceSource = "post_hoc_attribution"
)

// ParseEvidenceSource 解析来源标签；非法值返回 ErrSource。
   func ParseEvidenceSource(s string) (EvidenceSource, error) {
	  switch EvidenceSource(strings.TrimSpace(s)) {
	  case SourceDeskReport,
		  SourceReconHint, SourcePostHocAttrib:
		  return EvidenceSource(s), nil
	   default:
		  return "", ErrSource
	   }
  }

// ─── 置信度 & 风险层级 ────────────────────────────────────────────────────────
// Confidence 发现引擎对"贷记确实有误"的置信度分层。
// unscored 表示尚未完成分值；达到 high/certain 后编排器方可提交冻结申请。

type Confidence string

const (
	ConfUnscored Confidence = "unscored" // 尚未分值
	ConfLow      Confidence = "low"      // < 40%，证据薄弱
	ConfMedium Confidence = "medium" // 40–70%
	ConfHigh   Confidence = "high"   // 70–90%
	ConfCertain Confidence = "certain" // > 90%，多来源交叉印证
)

// ParseConfidence 解析置信度字符串；非法值返回 ErrConfidence。

  func ParseConfidence(s string) (Confidence, error) {
	   switch Confidence(strings.TrimSpace(s)) {
	   case ConfUnscored, ConfLow, ConfMedium, ConfHigh, ConfCertain:
		   return Confidence(s), nil
	   default:
		   return "", ErrConfidence
	   }
  }

// ActionableConfidence 返回是否可以进入冻结/冲销操作（需达 high 及以上）。

  func (c Confidence) ActionableConfidence() bool {
	   return c == ConfHigh || c == ConfCertain
  }

// RiskTier 风险层级：按金额等级决定审批路径深度。
type RiskTier string

const (
	// RiskTier1 低风险：等值 USD < 100。
	RiskTier1 RiskTier = "tier1"
	// RiskTier2 中风险：等值 USD 100 – 10 000。
	RiskTier2 RiskTier = "tier2"
	// RiskTier3 高风险：等值 USD > 10 000，需双人审批。
	RiskTier3 RiskTier = "tier3"
)

// ParseRiskTier 解析风险层级字符串；非法值返回 ErrRiskTier。
  func ParseRiskTier(s string) (RiskTier, error) {
	   switch RiskTier(strings.TrimSpace(s)) {
	  case RiskTier1, RiskTier2, RiskTier3:
		  return RiskTier(s), nil
	  default:
		  return "", ErrRiskTier
	   }
   }

// DualApprovalRequired Tier3 需要双人审批（四眼原则）。
   func (r RiskTier) DualApprovalRequired() bool {
	  return r == RiskTier3
   }

// ─── 证据信封 ─────────────────────────────────────────────────────────────────
// EvidenceEnvelope 一条证据记录；内容以 SHA-256 做完整性锚定（对象存储文件存对象存储）。
// Payload 只在 desk_report 人工附件小 blob 场景允许内联；其余来源必须为空。

type EvidenceEnvelope struct {
	ID string `json:"id"`
	CaseID string         `json:"case_id"`
	Source EvidenceSource `json:"source"`
	SHA256 string         `json:"sha256"`
	Kind   string         `json:"kind"` // "screenshot" / "csv_extract" / "recon_diff" 等
	Note   string         `json:"note,omitempty"`
	Payload string `json:"payload,omitempty"` // 小 blob 内联，仅 desk_report
	Actor     string    `json:"actor"`
	Score     float64   `json:"score,omitempty"` // 该证据的局部贡献分（0–1）
	CreatedAt time.Time `json:"created_at"`
}

  func (e EvidenceEnvelope) Copy() EvidenceEnvelope { return e }

// ─── 主案件记录 ───────────────────────────────────────────────────────────────
// WrongCreditCase 错贷记案件主记录。
// CreditRef 是原始 credited Transfer.ID（账本中 direction=WalletDeposit 或
// FiatOnramp 等），编排器以此为"冲销对象"；CreditRef 在 CreditMutex 中做
// 互斥锁定，防止同时在 reorg-reversal 批次中被双路冲销。
type WrongCreditCase struct {
	// 案件标识
	ID             string `json:"id"`
	IdempotencyKey string `json:"idempotency_key"`
	// 受影响用户
	UserID string `json:"user_id"`
	// 原始贷记单据
	CreditRef   string `json:"credit_ref"` // transfer_id（原始入账）
	Asset       string `json:"asset"`
	CreditedAmt string `json:"credited_amt"` // 原始贷记金额（精度以 shopspring decimal 表示）
	// 发现分值
	Confidence   Confidence `json:"confidence"`
	RiskTier     RiskTier   `json:"risk_tier"`
	ScoreNote    string     `json:"score_note,omitempty"`
	ScoreVersion int        `json:"score_version"` // 每次 Rescore 递增
	// 追回结果
	ClawedAmt   string `json:"clawed_amt,omitempty"`   // 已追回金额
	RemainAmt   string `json:"remain_amt,omitempty"`   // 挂坏账金额
	ReversalRef string `json:"reversal_ref,omitempty"` // 冲销账本 TransferID
	// 生命周期
	Phase Phase `json:"phase"`
	Step Step `json:"step"`
	FenceEpoch int64 `json:"fence_epoch"`
	FenceToken string `json:"fence_token"`
	// 审批
	ApprovedBy string     `json:"approved_by,omitempty"`
	ApprovedAt *time.Time `json:"approved_at,omitempty"`
	ApprovedBy2 string     `json:"approved_by_2,omitempty"` // 双人审批第二人（Tier3）
	ApprovedAt2 *time.Time `json:"approved_at_2,omitempty"`
	// 豁免
	WaivedBy   string     `json:"waived_by,omitempty"`
	WaivedAt   *time.Time `json:"waived_at,omitempty"`
	WaivedNote string     `json:"waived_note,omitempty"`
	// 运营备注
	OpenedBy string `json:"opened_by"`
	ClosedBy string `json:"closed_by,omitempty"`
	Note string `json:"note,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

   func (c WrongCreditCase) Copy() WrongCreditCase { return c }

// ─── 案件开立请求 ─────────────────────────────────────────────────────────────
// OpenReq 运营/CS 开立案件请求。
type OpenReq struct {
	IdempotencyKey string `json:"idempotency_key"`
	UserID         string `json:"user_id"`
	CreditRef      string `json:"credit_ref"` // 原始入账 transfer_id
	Asset string `json:"asset"`
	CreditedAmt string `json:"credited_amt"` // 原始贷记金额（须与账本一致）
	OpenedBy string `json:"opened_by"`
	Note     string `json:"note,omitempty"`
}

// ─── 证据附加请求 ─────────────────────────────────────────────────────────────
// AttachEvidenceReq 附加证据请求。
type AttachEvidenceReq struct {
	CaseID string `json:"case_id"`
	Source EvidenceSource `json:"source"`
	Kind   string         `json:"kind"`
	Note string `json:"note,omitempty"`
	SHA256  string  `json:"sha256"`
	Payload string  `json:"payload,omitempty"` // 仅 desk_report
	Score   float64 `json:"score,omitempty"`
	Actor   string  `json:"actor"`
}

// ─── 追回结果快照 ─────────────────────────────────────────────────────────────
// ClawResult 追回操作的结果快照，用于 HTTP 响应和审计日志。
type ClawResult struct {
	CaseID string `json:"case_id"`
	ReversalRef string `json:"reversal_ref"`
	ClawedAmt   string `json:"clawed_amt"`
	RemainAmt string `json:"remain_amt"`
	Phase Phase `json:"phase"`
	Step  Step  `json:"step"`
}

// ─── 审批请求 ─────────────────────────────────────────────────────────────────
// ApproveReq 运营审批（冻结或冲销）请求。
type ApproveReq struct {
	CaseID string `json:"case_id"`
	Actor      string `json:"actor"`
	FenceEpoch int64  `json:"fence_epoch"`
	FenceToken string `json:"fence_token"`
}

// WaiveReq 运营豁免请求。
type WaiveReq struct {
	CaseID string `json:"case_id"`
	Actor  string `json:"actor"`
	Note   string `json:"note"`
	FenceEpoch int64 `json:"fence_epoch"`
	FenceToken string `json:"fence_token"`
}

// ─── 时间轴事件 ───────────────────────────────────────────────────────────────
// TimelineEvent 案件时间轴事件，只追加不删改。

type TimelineEvent struct {
	CaseID string `json:"case_id"`
	Seq   int    `json:"seq"`
	Kind  string `json:"kind"`
	Actor string `json:"actor"`
	FromPhase Phase `json:"from_phase,omitempty"`
	ToPhase   Phase `json:"to_phase,omitempty"`
	FromStep Step `json:"from_step,omitempty"`
	ToStep Step `json:"to_step,omitempty"`
	Note string `json:"note,omitempty"`
	At time.Time `json:"at"`
}

// ─── 分页 ─────────────────────────────────────────────────────────────────────
// ListReq 案件列表查询请求。
type ListReq struct {
	UserID string `json:"user_id,omitempty"`
	Phase     string `json:"phase,omitempty"`
	Asset     string `json:"asset,omitempty"`
	CreditRef string `json:"credit_ref,omitempty"`
	Limit  int `json:"limit,omitempty"`
	Offset int `json:"offset,omitempty"`
}
