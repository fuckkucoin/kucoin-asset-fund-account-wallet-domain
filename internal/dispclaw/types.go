package dispclaw

import (
	"fmt"
	"time"
)

// PlanType 资金计划类型：追偿（从对手方收回）、退还（退给用户）、恢复可用（解冻）。
type PlanType string

const (
	PlanTypeClawBack PlanType = "claw_back" // 退单追偿：从对手方扣回资金
	PlanTypeRefund  PlanType = "refund"  // 争议退还：将冻结资金退回发起用户
	PlanTypeRestore PlanType = "restore" // 恢复可用：解除冻结，原路恢复
)

  func ParsePlanType(s string) (PlanType, error) {
	   switch PlanType(s) {
	  case PlanTypeClawBack, PlanTypeRefund, PlanTypeRestore:
		   return PlanType(s), nil
	  default:
		   return "", fmt.Errorf("dispclaw: plan type %q unknown", s)
	  }
  }

// PlanStatus 资金计划状态机：open→executing→closed/aborted。
// open: 计划已建，腿尚未执行。
// executing: 至少一条腿正在推进。
// closed: 所有腿终态，CloseProof 已生成，Case 可结案。
// aborted: 计划因审批拒绝或超时被中止；Case 可强制结案（需管理员特批）。
type PlanStatus string

const (
	PlanStatusOpen      PlanStatus = "open"
	PlanStatusExecuting PlanStatus = "executing"
	PlanStatusClosed    PlanStatus = "closed"
	PlanStatusAborted PlanStatus = "aborted"
)

   func (s PlanStatus) Terminal() bool {
	  return s == PlanStatusClosed || s == PlanStatusAborted
   }

// LegType 追偿腿类型。

type LegType string

const (
	LegTypeFreeze       LegType = "freeze"        // 冻结争议金额
	LegTypeClawDebit    LegType = "claw_debit"    // 追偿借记（从对手方扣除）
	LegTypeRefundCredit LegType = "refund_credit" // 退还贷记（贷回发起用户）
	LegTypeRestoreUnfreeze LegType = "restore_unfreeze" // 恢复解冻（争议撤销/拒绝时释放）
	LegTypeSkipMarker      LegType = "skip_marker"      // 占位跳过标记（条件分支不满足时）
)

  func ParseLegType(s string) (LegType, error) {
	  switch LegType(s) {
	   case LegTypeFreeze, LegTypeClawDebit, LegTypeRefundCredit, LegTypeRestoreUnfreeze, LegTypeSkipMarker:
		  return LegType(s), nil
	   default:
		   return "", fmt.Errorf("dispclaw: leg type %q unknown", s)
	   }
   }

// LegStatus 追偿腿状态机：pending→posting→posted/failed/skipped。
// pending: 等待执行。
// posting: 正在执行（乐观锁占位，防并发重复执行）。
// posted: 执行成功，账务已落账。
// failed: 执行失败，已记录失败原因；可重试或升级。
// skipped: 条件不满足时跳过（如余额不足但决策已改为退还）。

type LegStatus string

const (
	LegStatusPending LegStatus = "pending"
	LegStatusPosting LegStatus = "posting"
	LegStatusPosted  LegStatus = "posted"
	LegStatusFailed  LegStatus = "failed"
	LegStatusSkipped LegStatus = "skipped"
)

   func (s LegStatus) Terminal() bool {
	  return s == LegStatusPosted || s == LegStatusFailed || s == LegStatusSkipped
   }

// GateStatus 收口闸状态。

type GateStatus string

const (
	GateStatusBlocking GateStatus = "blocking" // 有未完成腿，阻断 Resolve
	GateStatusPassed   GateStatus = "passed"   // 所有腿终态，放行 Resolve
	GateStatusBypassed GateStatus = "bypassed" // 管理员特批豁免（需留审计记录）
)

// FundPlan 争议资金计划：由 Case 状态派生，一个 Case 对应唯一一条计划。
type FundPlan struct {
	ID      string `json:"id"`
	CaseID  string `json:"case_id"`
	Channel string `json:"channel"`
	UserID      string     `json:"user_id"`
	Asset       string     `json:"asset"`
	Amount      string     `json:"amount"`
	PlanType    PlanType   `json:"plan_type"`
	Status      PlanStatus `json:"status"`
	RailBreakID string     `json:"rail_break_id,omitempty"` // 只读引用 E 包法币断裂 ID
	OpenedBy    string `json:"opened_by"`
	ClosedBy    string `json:"closed_by,omitempty"`
	AbortReason string `json:"abort_reason,omitempty"`
	Version    int       `json:"version"`
	FenceEpoch int64     `json:"fence_epoch,omitempty"`
	FenceToken string    `json:"fence_token,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

   func (p FundPlan) Copy() FundPlan { return p }

// RecoveryLeg 追偿/退还/恢复腿：一个 FundPlan 下有多条，每条对应一个账务动作。
// 幂等键为 plan_id+seq，确保可回放不重复。
type RecoveryLeg struct {
	ID     string `json:"id"`
	PlanID string `json:"plan_id"`
	CaseID string `json:"case_id"`
	Seq    int    `json:"seq"`
	LegType LegType `json:"leg_type"`
	UserID  string  `json:"user_id"`
	Asset   string  `json:"asset"`
	Amount string    `json:"amount"`
	Status LegStatus `json:"status"`
	IdempotencyKey string `json:"idempotency_key"`
	LedgerRef string `json:"ledger_ref,omitempty"` // 账本分录 ID（只读）
	HoldRef   string `json:"hold_ref,omitempty"`   // holdcat hold ID（只读）
	RailBreakRef string `json:"rail_break_ref,omitempty"` // 法币断裂 ID（只读）
	LegHash string `json:"leg_hash"` // SHA256 防篡改
	PostedBy string     `json:"posted_by,omitempty"`
	Note     string     `json:"note,omitempty"`
	PostedAt *time.Time `json:"posted_at,omitempty"`
	FailedAt *time.Time `json:"failed_at,omitempty"`
	FailReason  string     `json:"fail_reason,omitempty"`
	RetryCount  int        `json:"retry_count"`
	NextRetryAt *time.Time `json:"next_retry_at,omitempty"`
	FenceEpoch int64 `json:"fence_epoch,omitempty"`
	FenceToken string    `json:"fence_token,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

  func (l RecoveryLeg) Copy() RecoveryLeg { return l }

// CloseGate 结案收口闸：绑定 Plan 与 Case 结案，在所有腿终态前阻断 Resolve。

type CloseGate struct {
	ID string `json:"id"`
	PlanID string `json:"plan_id"`
	CaseID     string     `json:"case_id"`
	GateStatus GateStatus `json:"gate_status"`
	BlockingReason string `json:"blocking_reason,omitempty"`
	PendingLegIDs string `json:"pending_leg_ids,omitempty"`
	CheckedBy string `json:"checked_by,omitempty"`
	PassedAt     *time.Time `json:"passed_at,omitempty"`
	BypassReason string     `json:"bypass_reason,omitempty"`
	BypassedBy string `json:"bypassed_by,omitempty"`
	Version int `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

   func (g CloseGate) Copy() CloseGate { return g }

// CloseProof 收口证明：Case 结案时生成，含所有腿哈希集合。
// ProofHash = SHA256(各腿 leg_hash 有序拼接)，篡改任何腿后巡检必须检测到不符。
type CloseProof struct {
	ID     string `json:"id"`
	PlanID string `json:"plan_id"`
	CaseID string `json:"case_id"`
	GateID string `json:"gate_id"`
	ProofHash string `json:"proof_hash"`
	LegHashes string `json:"leg_hashes"` // JSON 数组字符串
	LegCount  int    `json:"leg_count"`
	TotalAmount    string `json:"total_amount"`
	Asset          string `json:"asset"`
	DecidedBy      string `json:"decided_by"`
	Decision       string `json:"decision"`
	TamperDetected bool   `json:"tamper_detected"`
	TamperDetail    string     `json:"tamper_detail,omitempty"`
	TamperCheckedAt *time.Time `json:"tamper_checked_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

  func (p CloseProof) Copy() CloseProof { return p }

// GeneratePlanReq 从 Case 生成 FundPlan 的请求。
type GeneratePlanReq struct {
	CaseID   string   `json:"case_id"`
	PlanType PlanType `json:"plan_type"`
	Actor string `json:"actor"`
	RailBreakID    string `json:"rail_break_id,omitempty"` // 可选：只读挂钩 E 包断裂 ID
	IdempotencyKey string `json:"idempotency_key"`
}

// ExecuteLegReq 执行一条追偿腿的请求（需四眼：执行人不得为开单人）。

type ExecuteLegReq struct {
	PlanID string `json:"plan_id"`
	LegID  string `json:"leg_id"`
	Actor  string `json:"actor"` // 执行人，不得与 Plan.OpenedBy 相同
	Note   string `json:"note,omitempty"`
}

// CloseGateCheckReq 收口闸校验请求。

type CloseGateCheckReq struct {
	CaseID string `json:"case_id"`
	Actor string `json:"actor"`
}

// BypassGateReq 管理员特批豁免结案收口闸请求。
type BypassGateReq struct {
	CaseID string `json:"case_id"`
	Actor        string `json:"actor"`
	BypassReason string `json:"bypass_reason"`
}

// AbortPlanReq 中止资金计划请求。

type AbortPlanReq struct {
	PlanID      string `json:"plan_id"`
	Actor       string `json:"actor"`
	AbortReason string `json:"abort_reason"`
}
