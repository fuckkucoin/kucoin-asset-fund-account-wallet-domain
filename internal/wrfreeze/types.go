package wrfreeze

import (
	"time"
	"github.com/shopspring/decimal"
)

// ──────────────────────────────────────────
// CascadePlan — 事故级联冻结计划
// ──────────────────────────────────────────
// PlanScope 级联范围枚举。
type PlanScope string

const (
	ScopeGlobal PlanScope = "global" // 全站冻结
	ScopeAsset  PlanScope = "asset"  // 单资产跨用户
	ScopeCohort PlanScope = "cohort" // 用户分组（cohort 标签集合）
	ScopeUserSet PlanScope = "user_set" // 显式指定用户集合
)

// PlanStatus Plan 状态机。
// draft → armed → executing → sealed
// → aborted
type PlanStatus string

const (
	PlanDraft     PlanStatus = "draft"     // 草稿：仍在补充波次与用户集合
	PlanArmed     PlanStatus = "armed"     // 武装：已四眼批准，等待触发
	PlanExecuting PlanStatus = "executing" // 执行中：波次推进
	PlanSealed  PlanStatus = "sealed"  // 封存：全部波次完成且覆盖率证明通过
	PlanAborted PlanStatus = "aborted" // 中止：人工终止或超时
)

   func (s PlanStatus) Terminal() bool {
	   return s == PlanSealed || s == PlanAborted
   }

// CascadePlan 事故驱动的级联冻结计划。
type CascadePlan struct {
	ID          string `db:"id"           json:"id"`
	IncidentRef string `db:"incident_ref" json:"incident_ref"` // 事故工单编号
	Scope PlanScope `db:"scope"        json:"scope"`
	Asset string `db:"asset"        json:"asset,omitempty"` // ScopeAsset 时有效
	CohortTag string `db:"cohort_tag"   json:"cohort_tag,omitempty"` // ScopeCohort 时有效
	UserSet  []string   `db:"-"            json:"user_set,omitempty"` // ScopeUserSet 时显式列
	Status   PlanStatus `db:"status"       json:"status"`
	OpenedBy string     `db:"opened_by"    json:"opened_by"`
	ArmedBy     string `db:"armed_by"     json:"armed_by,omitempty"`
	AbortedBy   string `db:"aborted_by"   json:"aborted_by,omitempty"`
	AbortReason string `db:"abort_reason" json:"abort_reason,omitempty"`
	DualReqID   string `db:"dual_req_id"  json:"dual_req_id,omitempty"` // dualctl 四眼请求 ID
	ExpiresAt time.Time `db:"expires_at"   json:"expires_at"`
	SealedAt  *time.Time `db:"sealed_at"    json:"sealed_at,omitempty"`
	CreatedAt time.Time  `db:"created_at"   json:"created_at"`
	UpdatedAt time.Time `db:"updated_at"   json:"updated_at"`
}

// ──────────────────────────────────────────
// FreezeWave — 级联执行波次
// ──────────────────────────────────────────
// WaveStatus 波次状态机。
// pending → running → done / failed / skipped
type WaveStatus string

const (
	WavePending WaveStatus = "pending"
	WaveRunning WaveStatus = "running"
	WaveDone    WaveStatus = "done"
	WaveFailed  WaveStatus = "failed"
	WaveSkipped WaveStatus = "skipped"
)

  func (s WaveStatus) Terminal() bool {
	  return s == WaveDone || s == WaveFailed || s == WaveSkipped
  }

// FreezeWave 计划内的一个执行波次。
// 波次按 SeqNo 升序串行执行；同一波次内的用户并发冻结。
type FreezeWave struct {
	ID     string `db:"id"           json:"id"`
	PlanID string `db:"plan_id"      json:"plan_id"`
	SeqNo int `db:"seq_no"       json:"seq_no"` // 从 1 开始
	Label string `db:"label"        json:"label"` // 人工标签（如 "vip_tier1"）
	TargetUsers []string `db:"-"            json:"target_users"` // 该波次目标用户 ID 列表
	Status      WaveStatus `db:"status"       json:"status"`
	FrozenCount int        `db:"frozen_count" json:"frozen_count"` // 实际冻结成功数
	FailedCount int        `db:"failed_count" json:"failed_count"` // 冻结失败数
	RetryCount int        `db:"retry_count"  json:"retry_count"`
	StartedAt  *time.Time `db:"started_at"   json:"started_at,omitempty"`
	FinishedAt *time.Time `db:"finished_at"  json:"finished_at,omitempty"`
	CreatedAt time.Time `db:"created_at"   json:"created_at"`
}

// ──────────────────────────────────────────
// UnfreezePhase — 分阶段解冻
// ──────────────────────────────────────────
// PhaseKind 解冻阶段类型。
// probe → partial → full → closed
type PhaseKind string

const (
	PhaseProbe   PhaseKind = "probe"   // 探针校验：确认冻结全覆盖、残留无遗漏
	PhasePartial PhaseKind = "partial" // 部分解冻：仅放行低风险子集
	PhaseFull    PhaseKind = "full"    // 全量解冻
	PhaseClosed  PhaseKind = "closed"  // 已关闭（完成或放弃）
)

// PhaseStatus 分阶段解冻状态机。
// pending → running → passed / failed

type PhaseStatus string

const (
	PhasePending PhaseStatus = "pending"
	PhaseRunning PhaseStatus = "running"
	PhasePassed PhaseStatus = "passed"
	PhaseFailed PhaseStatus = "failed"
)

  func (s PhaseStatus) Terminal() bool {
	  return s == PhasePassed || s == PhaseFailed
  }

// UnfreezePhase 分阶段解冻记录。
// probe 未通过则 partial/full 不得启动。
// 每阶段须有独立四眼批准。
type UnfreezePhase struct {
	ID     string `db:"id"              json:"id"`
	PlanID string `db:"plan_id"         json:"plan_id"`
	Kind PhaseKind `db:"kind"            json:"kind"`
	Status    PhaseStatus `db:"status"          json:"status"`
	DualReqID string      `db:"dual_req_id"     json:"dual_req_id,omitempty"`
	InitiatedBy string `db:"initiated_by"    json:"initiated_by"`
	ApprovedBy  string `db:"approved_by"     json:"approved_by,omitempty"`
	IntegrityNote string     `db:"integrity_note"  json:"integrity_note,omitempty"` // 完整性校验摘要
	UnfrozenCount int        `db:"unfrozen_count"  json:"unfrozen_count"`
	FailedCount   int        `db:"failed_count"    json:"failed_count"`
	StartedAt     *time.Time `db:"started_at"      json:"started_at,omitempty"`
	FinishedAt *time.Time `db:"finished_at"     json:"finished_at,omitempty"`
	CreatedAt  time.Time  `db:"created_at"      json:"created_at"`
	UpdatedAt  time.Time  `db:"updated_at"      json:"updated_at"`
}

// ──────────────────────────────────────────
// CoverageProof — 覆盖率完整性证明
// ──────────────────────────────────────────
// ProofVerdict 证明结论。
type ProofVerdict string

const (
	VerdictPassed ProofVerdict = "passed" // 100% 覆盖
	VerdictGap     ProofVerdict = "gap"     // 存在差额（应冻未冻）
	VerdictPending ProofVerdict = "pending" // 计算中
)

// CoverageProof 应冻集合 vs 实冻集合的覆盖率证明。
// 应冻集合由波次 TargetUsers 并集；实冻集合由账本冻结位查询得出。
// Delta（差额）不为空则 Plan 不得 sealed。

type CoverageProof struct {
	ID     string `db:"id"             json:"id"`
	PlanID string `db:"plan_id"        json:"plan_id"`
	ExpectedCount int `db:"expected_count" json:"expected_count"` // 应冻用户数
	ActualCount int `db:"actual_count"   json:"actual_count"` // 实冻用户数
	DeltaCount  int `db:"delta_count"    json:"delta_count"`  // 差额（应冻未冻）
	DeltaUsers []string     `db:"-"              json:"delta_users,omitempty"`
	Verdict    ProofVerdict `db:"verdict"        json:"verdict"`
	CheckedAt time.Time `db:"checked_at"     json:"checked_at"`
	CreatedAt time.Time `db:"created_at"     json:"created_at"`
}

// ──────────────────────────────────────────
// RollbackLeg — 误冻回滚腿
// ──────────────────────────────────────────
// RollbackReason 回滚原因分类。

type RollbackReason string

const (
	ReasonMisFreeze       RollbackReason = "mis_freeze"       // 错误冻结（不在应冻集合内）
	ReasonExpiredIncident RollbackReason = "expired_incident" // 事故已超时到期
	ReasonScopeError      RollbackReason = "scope_error"      // 范围配置错误
)

// RollbackStatus 回滚腿状态机。
// pending → executing → restored / failed
type RollbackStatus string

const (
	RollbackPending RollbackStatus = "pending"
	RollbackExecuting RollbackStatus = "executing"
	RollbackRestored RollbackStatus = "restored" // 可用视图已恢复
	RollbackFailed   RollbackStatus = "failed"
)

  func (s RollbackStatus) Terminal() bool {
	  return s == RollbackRestored || s == RollbackFailed
  }

// RollbackLeg 单用户的误冻回滚腿。
// 执行后恢复用户可用视图解释（AvailabilityNote）。
// 约束：不自动解冻 live-hash 出金锁定（须走提现 FSM）。
type RollbackLeg struct {
	ID        string         `db:"id"                json:"id"`
	PlanID    string         `db:"plan_id"           json:"plan_id"`
	UserID    string         `db:"user_id"           json:"user_id"`
	Reason    RollbackReason `db:"reason"            json:"reason"`
	Status    RollbackStatus `db:"status"            json:"status"`
	DualReqID string         `db:"dual_req_id"       json:"dual_req_id,omitempty"`
	RequestedBy string `db:"requested_by"      json:"requested_by"`
	ExecutedBy string `db:"executed_by"       json:"executed_by,omitempty"`
	AvailabilityNote string          `db:"availability_note" json:"availability_note,omitempty"` // 恢复后的可用视图解释
	HasLiveHash      bool            `db:"has_live_hash"     json:"has_live_hash"`               // 是否存在 live-hash 出金（不自动解）
	FrozenByPlan     bool            `db:"frozen_by_plan"    json:"frozen_by_plan"`              // 冻结是否由本计划造成
	Amount           decimal.Decimal `db:"amount"           json:"amount,omitempty"`             // 本腿涉及冻结量（仅统计用）
	CreatedAt        time.Time       `db:"created_at"        json:"created_at"`
	UpdatedAt        time.Time       `db:"updated_at"        json:"updated_at"`
}

// ──────────────────────────────────────────
// WaveUserEntry — 波次用户冻结明细
// ──────────────────────────────────────────
// FreezeResult 单用户冻结结果。
type FreezeResult string

const (
	FreezeOK      FreezeResult = "ok"
	FreezeSkipped FreezeResult = "skipped" // 已冻，幂等跳过
	FreezeFailed FreezeResult = "failed"
)

// WaveUserEntry 波次内单用户冻结执行记录（存 SQL）。
type WaveUserEntry struct {
	ID     string `db:"id"        json:"id"`
	WaveID string `db:"wave_id"   json:"wave_id"`
	PlanID string `db:"plan_id"   json:"plan_id"`
	UserID string `db:"user_id"   json:"user_id"`
	Result FreezeResult `db:"result"    json:"result"`
	ErrMsg   string    `db:"err_msg"   json:"err_msg,omitempty"`
	AttemptN int       `db:"attempt_n" json:"attempt_n"`
	At       time.Time `db:"at"        json:"at"`
}

// ──────────────────────────────────────────
// API 请求/响应结构
// ──────────────────────────────────────────
// OpenPlanReq 开启级联冻结计划请求。

type OpenPlanReq struct {
	IncidentRef string    `json:"incident_ref"`
	Scope       PlanScope `json:"scope"`
	Asset     string   `json:"asset,omitempty"`
	CohortTag string   `json:"cohort_tag,omitempty"`
	UserSet   []string `json:"user_set,omitempty"`
	Actor  string `json:"actor"`
	Reason string `json:"reason"`
	TTLHours int `json:"ttl_hours,omitempty"` // 默认 24h
}

// ArmPlanReq 武装计划请求（需四眼批准后触发）。
type ArmPlanReq struct {
	PlanID string `json:"plan_id"`
	Actor  string `json:"actor"`
}

// ExecutePlanReq 触发计划执行。

type ExecutePlanReq struct {
	PlanID string `json:"plan_id"`
	Actor  string `json:"actor"`
}

// AbortPlanReq 中止计划。
type AbortPlanReq struct {
	PlanID string `json:"plan_id"`
	Actor  string `json:"actor"`
	Reason string `json:"reason"`
}

// InitPhaseReq 发起解冻阶段。

type InitPhaseReq struct {
	PlanID string    `json:"plan_id"`
	Kind   PhaseKind `json:"kind"`
	Actor  string    `json:"actor"`
}

// ApprovePhaseReq 批准解冻阶段（四眼）。

type ApprovePhaseReq struct {
	PhaseID string `json:"phase_id"`
	Actor   string `json:"actor"`
}

// RollbackReq 申请回滚腿。

type RollbackReq struct {
	PlanID string         `json:"plan_id"`
	UserID string         `json:"user_id"`
	Reason RollbackReason `json:"reason"`
	Actor  string         `json:"actor"`
}

// ExecuteRollbackReq 执行回滚腿（四眼通过后）。
type ExecuteRollbackReq struct {
	LegID string `json:"leg_id"`
	Actor string `json:"actor"`
}

// GlobalPlanProjection FreezeAll 布尔开关映射为只读全局 Plan 投影。
// 不执行双写；仅供只读查询。
type GlobalPlanProjection struct {
	Source    string `json:"source"` // "legacy_switch"
	FrozenAll bool   `json:"frozen_all"`
	Actor string `json:"actor,omitempty"`
	Reason string    `json:"reason,omitempty"`
	At     time.Time `json:"at,omitempty"`
	PlanID string    `json:"plan_id,omitempty"` // 若存在对应全局 Plan 则填充
}
