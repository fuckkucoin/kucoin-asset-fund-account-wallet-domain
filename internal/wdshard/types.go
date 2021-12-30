package wdshard

import "time"

// PlanState 计划状态机状态。

type PlanState string

const (
	PlanDraft        PlanState = "draft"
	PlanApprovedExec PlanState = "approved_exec"
	PlanRunning      PlanState = "running"
	PlanFilled        PlanState = "filled"
	PlanPartialClosed PlanState = "partial_closed"
	PlanAborted       PlanState = "aborted"
)

// LegState 分片腿状态机状态。
type LegState string

const (
	LegPending      LegState = "pending"
	LegFrozen       LegState = "frozen"
	LegBroadcasting LegState = "broadcasting"
	LegFilled       LegState = "filled"
	LegFailed       LegState = "failed"
)

// ShardPlan 大额出金分片计划。
// 一个计划对应一笔用户出金请求，被拆为若干 ShardLeg 依序广播。
type ShardPlan struct {
	ID string `json:"id"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Chain  string `json:"chain"`
	ToAddress   string `json:"to_address"`
	TotalAmount string `json:"total_amount"`
	PolicyID    string `json:"policy_id"`
	BatchID    string    `json:"batch_id,omitempty"`    // 关联 wdbatch 批次
	WithdrawID string    `json:"withdraw_id,omitempty"` // 关联 withdraw 原始单
	State      PlanState `json:"state"`
	LegCount     int    `json:"leg_count"`
	FilledAmount string `json:"filled_amount"`
	InflightAmount string `json:"inflight_amount"`
	RemainAmount string `json:"remain_amount"`
	Actor        string `json:"actor"`
	PauseReason string `json:"pause_reason,omitempty"`
	AbortReason string `json:"abort_reason,omitempty"`
	IdempotencyKey string `json:"idempotency_key"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ShardLeg 分片计划下的单条广播腿。
// pending→frozen→broadcasting→filled/failed。
type ShardLeg struct {
	ID string `json:"id"`
	PlanID string `json:"plan_id"`
	Seq    int    `json:"seq"` // 从 1 开始，有序
	Amount string `json:"amount"`
	State  LegState `json:"state"`
	HoldID string   `json:"hold_id,omitempty"` // 关联 ShardHold
	FillID string `json:"fill_id,omitempty"` // 关联 PartialFill
	WithdrawID string `json:"withdraw_id,omitempty"` // 关联子出金单
	BroadcastRef string     `json:"broadcast_ref,omitempty"` // 链上 tx hash
	FailReason   string     `json:"fail_reason,omitempty"`
	FrozenAt     *time.Time `json:"frozen_at,omitempty"`
	BroadcastAt  *time.Time `json:"broadcast_at,omitempty"`
	FilledAt *time.Time `json:"filled_at,omitempty"`
	FailedAt  *time.Time `json:"failed_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// PlanPolicy 拆分策略参数。
type PlanPolicy struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	MaxLegs int    `json:"max_legs"` // 最大分片数，默认 8
	MinLegAmount string `json:"min_leg_amount"` // 每片最小额
	MaxLegAmount   string `json:"max_leg_amount"`   // 每片最大额（热库可用上限）
	HotCapFraction string `json:"hot_cap_fraction"` // 热库可用乘以此系数作为单片上限，范围 (0,1]
	SplitMode      string `json:"split_mode"`       // "equal" | "cap_first" | "drain"
	AllowPartial   bool   `json:"allow_partial"`    // 允许前 k 成交后剩余保留
	Description    string `json:"description,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// CreatePlanReq 创建计划请求。
type CreatePlanReq struct {
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Chain string `json:"chain"`
	ToAddress string `json:"to_address"`
	TotalAmount string `json:"total_amount"`
	PolicyID    string `json:"policy_id"`
	BatchID     string `json:"batch_id,omitempty"`
	WithdrawID  string `json:"withdraw_id,omitempty"`
	Actor       string `json:"actor"`
	IdempotencyKey string `json:"idempotency_key"`
	// HotAvailable 调用方传入热库当前可用额（来自 hotreserve，只读）。
	HotAvailable string `json:"hot_available,omitempty"`
}

// PreviewReq 预览分片计划请求（不落库，只返回计划草稿）。

type PreviewReq struct {
	Asset string `json:"asset"`
	Chain string `json:"chain"`
	TotalAmount string `json:"total_amount"`
	PolicyID string `json:"policy_id"`
	HotAvailable string `json:"hot_available,omitempty"`
}

// PreviewResp 预览响应。

type PreviewResp struct {
	PolicyID string `json:"policy_id"`
	LegCount int    `json:"leg_count"`
	Amounts []string `json:"amounts"` // 各片面额
	TotalAmount string `json:"total_amount"`
	Warning string `json:"warning,omitempty"`
}

// ProgressResp 整单进度聚合视图。
type ProgressResp struct {
	PlanID string `json:"plan_id"`
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	TotalAmount  string `json:"total_amount"`
	FilledAmount string `json:"filled_amount"`
	InflightAmount string `json:"inflight_amount"`
	RemainAmount string `json:"remain_amount"`
	State PlanState `json:"state"`
	LegCount     int `json:"leg_count"`
	LegsFilled   int `json:"legs_filled"`
	LegsInflight int `json:"legs_inflight"`
	LegsRemain   int `json:"legs_remain"`
	LegsFailed int `json:"legs_failed"`
}

// PauseReq 运维暂停请求。
type PauseReq struct {
	PlanID string `json:"plan_id"`
	Reason string `json:"reason"`
	Actor  string `json:"actor"`
}

// ResumeReq 运维续跑请求。
type ResumeReq struct {
	PlanID string `json:"plan_id"`
	Actor string `json:"actor"`
}

// LegAdvanceReq 内部推进分片腿状态。
type LegAdvanceReq struct {
	LegID string `json:"leg_id"`
	To LegState `json:"to"`
	HoldID     string `json:"hold_id,omitempty"`
	FillID     string `json:"fill_id,omitempty"`
	WithdrawID string `json:"withdraw_id,omitempty"`
	BroadcastRef string `json:"broadcast_ref,omitempty"`
	FailReason string `json:"fail_reason,omitempty"`
	Actor string `json:"actor"`
}

// ShardEvent 分片失败事件（供任务 G 订阅）。

type ShardEvent struct {
	Kind string `json:"kind"` // "shard_leg_failed" | "shard_plan_aborted"
	PlanID string `json:"plan_id"`
	LegID string `json:"leg_id,omitempty"`
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	Amount     string `json:"amount"`
	FailReason string `json:"fail_reason,omitempty"`
	At time.Time `json:"at"`
}

  func (p *ShardPlan) Copy() ShardPlan {
	   if p == nil {
		  return ShardPlan{}
	  }
	   return *p
  }

  func (l *ShardLeg) Copy() ShardLeg {
	   if l == nil {
		   return ShardLeg{}
	   }
	  return *l
  }
