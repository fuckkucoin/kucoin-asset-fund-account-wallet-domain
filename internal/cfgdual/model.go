package cfgdual

import (
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// 枚举类型
// ─────────────────────────────────────────────────────────────────────────────
// ReleaseState 灰度发布状态机枚举。
// 转移路径：draft → canary → sealed | rolled_back。

type ReleaseState string

const (
	// StateDraft 草稿；尚未激活双评。
	StateDraft ReleaseState = "draft"
	// StateCanary 灰度中；双评并行运行。
	StateCanary ReleaseState = "canary"
	// StateSealed 已封堵；canary 包被强制下线，baseline 生效。
	StateSealed ReleaseState = "sealed"
	// StateRolledBack 已回滚；由运营主动发起，留有证明。
	StateRolledBack ReleaseState = "rolled_back"
)

// SealState 封堵闸状态机枚举。
// 转移路径：open → enforced | waived。

type SealState string

const (
	// SealOpen 封堵闸打开（初始态）；差分超限时触发。
	SealOpen SealState = "open"
	// SealEnforced 已强制执行；高风险写被挡回。
	SealEnforced SealState = "enforced"
	// SealWaived 已豁免；运营审核后明确放行。
	SealWaived SealState = "waived"
)

// DiffKind 差分类型枚举。
type DiffKind string

const (
	// DiffAllow baseline 允许、canary 拒绝——最危险的脑裂场景。
	DiffAllow DiffKind = "allow_deny"
	// DiffDeny baseline 拒绝、canary 允许。
	DiffDeny DiffKind = "deny_allow"
	// DiffLimit 限额漂移（两侧均允许但阈值不同）。
	DiffLimit DiffKind = "limit_drift"
	// DiffKillSwitch 任一侧 kill switch 打开。
	DiffKillSwitch DiffKind = "kill_switch"
)

// CohortKey 切流维度枚举。

type CohortKey string

const (
	CohortUser CohortKey = "user"
	CohortAsset CohortKey = "asset"
	CohortRatio CohortKey = "ratio"
)

// ─────────────────────────────────────────────────────────────────────────────
// GrayRelease 灰度发布聚合根
// ─────────────────────────────────────────────────────────────────────────────
// GrayRelease 代表一次策略包升级的灰度发布窗口。
// 维护 baseline 与 canary 两侧的包名和代次，驱动 Cohort 切流与双评。

type GrayRelease struct {
	// ID 全局唯一标识；形态 "gr_<ulid>"。供其他包只读引用。
	ID string `json:"id" db:"id"`
	// PackName 受灰度管控的策略包名称，对应 cfgcenter.LimitPack.Name。
	PackName string `json:"pack_name" db:"pack_name"`
	// BaselineVersion 已全量的代次号。
	BaselineVersion int `json:"baseline_version" db:"baseline_version"`
	// CanaryVersion 灰度候选代次号，必须大于 BaselineVersion。
	CanaryVersion int `json:"canary_version" db:"canary_version"`
	// State 当前状态，由 FSM 驱动。
	State ReleaseState `json:"state" db:"state"`
	// CohortPercent 命中 canary 的流量比例 [0,100]。
	CohortPercent int `json:"cohort_percent" db:"cohort_percent"`
	// Generation 世代号；每次切流比例变更或回滚后自增，保证可回放。
	Generation int `json:"generation" db:"generation"`
	// PolicyAllowDrift 允许的最大差分比例 [0,100]，超过则触发 Seal。
	PolicyAllowDrift int `json:"policy_allow_drift" db:"policy_allow_drift"`
	// Creator 发布者（运营 UID）。
	Creator string `json:"creator" db:"creator"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	// UpdatedAt 最后变更时间。
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
	// SealedAt 封堵时间（可选）。
	SealedAt *time.Time `json:"sealed_at,omitempty" db:"sealed_at"`
	// RolledBackAt 回滚时间（可选）。
	RolledBackAt *time.Time `json:"rolled_back_at,omitempty" db:"rolled_back_at"`
}

// IsActive 判断该发布是否处于活跃双评窗口。

   func (r *GrayRelease) IsActive() bool {
	   return r != nil && r.State == StateCanary
   }

// ─────────────────────────────────────────────────────────────────────────────
// DualDecision 双评决策记录
// ─────────────────────────────────────────────────────────────────────────────
// DualDecision 记录同一请求对 baseline 与 canary 两侧的评估结果。
// 作为审计证据和差分聚合的源数据落仓。
type DualDecision struct {
	// ID 决策记录唯一标识。
	ID string `json:"id" db:"id"`
	// GrayReleaseID 所属灰度发布 ID；供只读关联。
	GrayReleaseID string `json:"gray_release_id" db:"gray_release_id"`
	// Generation 评估时的发布世代号；用于回放一致性检验。
	Generation int `json:"generation" db:"generation"`
	// UserID 发起请求的用户 ID。
	UserID string `json:"user_id" db:"user_id"`
	// Asset 交易资产符号。
	Asset string `json:"asset" db:"asset"`
	// Amount 请求金额（字符串以避免精度损失）。
	Amount string `json:"amount" db:"amount"`
	// PackName 策略包名称。
	PackName string `json:"pack_name" db:"pack_name"`
	// BaselineAllow baseline 侧评估结果。
	BaselineAllow bool `json:"baseline_allow" db:"baseline_allow"`
	// BaselineReason baseline 侧拒绝原因（allow=true 时为空）。
	BaselineReason string `json:"baseline_reason" db:"baseline_reason"`
	// CanaryAllow canary 侧评估结果。
	CanaryAllow bool `json:"canary_allow" db:"canary_allow"`
	// CanaryReason canary 侧拒绝原因。
	CanaryReason string `json:"canary_reason" db:"canary_reason"`
	// HasDiff 两侧结论不一致。
	HasDiff bool `json:"has_diff" db:"has_diff"`
	// DiffKind 差分类型（HasDiff=true 时有效）。
	DiffKind DiffKind `json:"diff_kind,omitempty" db:"diff_kind"`
	// SplitDiffID 关联的差分聚合记录 ID（由 aggregate worker 填写）。
	SplitDiffID string `json:"split_diff_id,omitempty" db:"split_diff_id"`
	// Applied 实际生效侧："baseline" 或 "canary"。
	Applied string `json:"applied" db:"applied"`
	// Sealed 本次请求是否因 Seal 失败关闭。
	Sealed bool `json:"sealed" db:"sealed"`
	// CreatedAt 评估时间。
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// ─────────────────────────────────────────────────────────────────────────────
// SplitDiff 差分聚合记录
// ─────────────────────────────────────────────────────────────────────────────
// SplitDiff 对灰度窗口内的决策差分进行聚合统计，是封堵闸的触发依据。
type SplitDiff struct {
	// ID 差分记录唯一标识；供其他包只读引用。
	ID string `json:"id" db:"id"`
	// GrayReleaseID 所属发布 ID。
	GrayReleaseID string `json:"gray_release_id" db:"gray_release_id"`
	// Generation 对应世代。
	Generation int `json:"generation" db:"generation"`
	// WindowStart 统计窗口起始时间（滑动窗口左端）。
	WindowStart time.Time `json:"window_start" db:"window_start"`
	// WindowEnd 统计窗口结束时间。
	WindowEnd time.Time `json:"window_end" db:"window_end"`
	// TotalDecisions 窗口内总决策数。
	TotalDecisions int64 `json:"total_decisions" db:"total_decisions"`
	// DiffCount 产生差分的决策数。
	DiffCount int64 `json:"diff_count" db:"diff_count"`
	// DiffRatio 差分比例（百分比，0–100）。
	DiffRatio float64 `json:"diff_ratio" db:"diff_ratio"`
	// AllowDenyCount allow→deny 脑裂数。
	AllowDenyCount int64 `json:"allow_deny_count" db:"allow_deny_count"`
	// DenyAllowCount deny→allow 数。
	DenyAllowCount int64 `json:"deny_allow_count" db:"deny_allow_count"`
	// LimitDriftCount 限额漂移数。
	LimitDriftCount int64 `json:"limit_drift_count" db:"limit_drift_count"`
	// KillSwitchCount kill switch 触发数。
	KillSwitchCount int64 `json:"kill_switch_count" db:"kill_switch_count"`
	// ExceedsPolicy 差分比例已超过 GrayRelease.PolicyAllowDrift。
	ExceedsPolicy bool `json:"exceeds_policy" db:"exceeds_policy"`
	// CreatedAt 聚合写入时间。
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// ─────────────────────────────────────────────────────────────────────────────
// SealAction 封堵动作
// ─────────────────────────────────────────────────────────────────────────────
// SealAction 记录封堵闸的一次状态迁移和触发依据。
type SealAction struct {
	// ID 动作唯一标识。
	ID string `json:"id" db:"id"`
	// GrayReleaseID 所属发布 ID。
	GrayReleaseID string `json:"gray_release_id" db:"gray_release_id"`
	// SplitDiffID 触发本次封堵的差分记录 ID（可选；由 patrol 触发时留空）。
	SplitDiffID string `json:"split_diff_id,omitempty" db:"split_diff_id"`
	// FromState 前序状态。
	FromState SealState `json:"from_state" db:"from_state"`
	// ToState 目标状态。
	ToState SealState `json:"to_state" db:"to_state"`
	// Reason 触发原因描述。
	Reason string `json:"reason" db:"reason"`
	// Actor 操作者（worker 自动封堵填 "system"；人工豁免填运营 UID）。
	Actor string `json:"actor" db:"actor"`
	// CreatedAt 动作时间。
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// ─────────────────────────────────────────────────────────────────────────────
// RollbackProof 回滚证明
// ─────────────────────────────────────────────────────────────────────────────
// RollbackProof 记录 canary 包回滚事件的完整证明材料。
// 回滚后 DualEval 仅使用 baseline，不得静默漂回旧世代。
type RollbackProof struct {
	// ID 证明唯一标识。
	ID string `json:"id" db:"id"`
	// GrayReleaseID 对应的灰度发布 ID。
	GrayReleaseID string `json:"gray_release_id" db:"gray_release_id"`
	// PackName 回滚涉及的策略包名。
	PackName string `json:"pack_name" db:"pack_name"`
	// RolledBackVersion 被回滚的 canary 代次号。
	RolledBackVersion int `json:"rolled_back_version" db:"rolled_back_version"`
	// RestoredVersion 回滚后生效的 baseline 代次号。
	RestoredVersion int `json:"restored_version" db:"restored_version"`
	// GenerationBefore 回滚前世代号。
	GenerationBefore int `json:"generation_before" db:"generation_before"`
	// GenerationAfter 回滚后世代号（自增）。
	GenerationAfter int `json:"generation_after" db:"generation_after"`
	// TriggerSplitDiffID 触发回滚的差分记录 ID（自动触发时有值）。
	TriggerSplitDiffID string `json:"trigger_split_diff_id,omitempty" db:"trigger_split_diff_id"`
	// Actor 回滚操作者；自动回滚填 "system"。
	Actor string `json:"actor" db:"actor"`
	// Reason 回滚原因。
	Reason string `json:"reason" db:"reason"`
	// CreatedAt 回滚时间。
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// ─────────────────────────────────────────────────────────────────────────────
// 值对象
// ─────────────────────────────────────────────────────────────────────────────
// EvalRequest 双评请求值对象，携带切流上下文。

type EvalRequest struct {
	// UserID 用于 Cohort 切流决策。
	UserID string `json:"user_id"`
	// Pack 策略包名称。
	Pack string `json:"pack"`
	// Asset 交易资产符号。
	Asset string `json:"asset"`
	// Amount 请求金额。
	Amount string `json:"amount"`
	// TodaySum 当日已用额度（字符串）。
	TodaySum string `json:"today_sum,omitempty"`
	// Hourly 当前小时请求次数。
	Hourly int `json:"hourly,omitempty"`
	// Daily 当日请求次数。
	Daily int `json:"daily,omitempty"`
}

// EvalSide 单侧评估结果值对象。
type EvalSide struct {
	// Allow 是否允许。
	Allow bool `json:"allow"`
	// Reason 拒绝原因（allow=true 时为空）。
	Reason string `json:"reason,omitempty"`
	// Version 使用的策略包代次号。
	Version int `json:"version"`
	// Killed kill switch 打开标志。
	Killed bool `json:"killed"`
}

// DualResult 双评聚合结果，包含两侧结论与最终决策。
type DualResult struct {
	// Baseline baseline 侧评估。
	Baseline EvalSide `json:"baseline"`
	// Canary canary 侧评估（灰度外流量不填充）。
	Canary *EvalSide `json:"canary,omitempty"`
	// InCohort 本次请求是否命中灰度队列。
	InCohort bool `json:"in_cohort"`
	// HasDiff 双评结论不一致。
	HasDiff bool `json:"has_diff,omitempty"`
	// DiffKind 差分类型。
	DiffKind DiffKind `json:"diff_kind,omitempty"`
	// FinalAllow 最终生效决策。Seal 激活时强制以 baseline 为准。
	FinalAllow bool `json:"final_allow"`
	// FinalReason 最终决策原因。
	FinalReason string `json:"final_reason,omitempty"`
	// Sealed 本次因脑裂封堵失败关闭。
	Sealed bool `json:"sealed,omitempty"`
	// Applied 实际生效侧："baseline" | "canary" | "seal"。
	Applied string `json:"applied"`
	// DecisionID 落仓的 DualDecision.ID（异步写入后可查询）。
	DecisionID string `json:"decision_id,omitempty"`
	// GrayReleaseID 所属灰度发布 ID。
	GrayReleaseID string `json:"gray_release_id,omitempty"`
}
