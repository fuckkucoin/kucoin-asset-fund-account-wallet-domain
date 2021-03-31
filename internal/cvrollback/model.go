package cvrollback

import (
	"errors"
	"time"
)

// RollbackStatus 回滚计划状态机节点。
// planned → executing → done / failed

type RollbackStatus string

const (
	RollbackPlanned RollbackStatus = "PLANNED"
	RollbackExecuting RollbackStatus = "EXECUTING"
	RollbackDone      RollbackStatus = "DONE"
	RollbackFailed RollbackStatus = "FAILED"
)

// RollbackTrigger 触发原因。

type RollbackTrigger string

const (
	TriggerCancel RollbackTrigger = "CANCEL"
	TriggerExpire RollbackTrigger = "EXPIRE"
	TriggerFail   RollbackTrigger = "FAIL"
	TriggerBreach RollbackTrigger = "BREACH"
	TriggerManual RollbackTrigger = "MANUAL"
)

// LegAction 回滚腿动作。

type LegAction string

const (
	// LegReleaseOcc 释放占用（调 cvoccup.Release）
	LegReleaseOcc LegAction = "RELEASE_OCC"
	// LegReverseInFlight 冲在途账本分录（调 ledger reverse）
	LegReverseInFlight LegAction = "REVERSE_INFLIGHT"
	// LegAlignHoldcat 一致 holdcat（确认 hold 已释放）
	LegAlignHoldcat LegAction = "ALIGN_HOLDCAT"
)

// LegStatus 回滚腿执行状态。

type LegStatus string

const (
	LegPending LegStatus = "PENDING"
	LegDone LegStatus = "DONE"
	LegSkipped   LegStatus = "SKIPPED"
	LegLegFailed LegStatus = "FAILED"
)

var (
	ErrPlanNotFound = errors.New("rollback plan not found")
	ErrPlanTerminal = errors.New("rollback plan already terminal")
	ErrPlanStore    = errors.New("rollback store unavailable")
	ErrLegExec = errors.New("rollback leg execution failed")
)

// RollbackPlan 回滚计划主记录，对应 cv_rollbacks 行。
type RollbackPlan struct {
	ID string `json:"id"`
	OccID string `json:"occ_id"`
	OrderID string `json:"order_id"`
	UserID  string          `json:"user_id"`
	Trigger RollbackTrigger `json:"trigger"`
	Status  RollbackStatus  `json:"status"`
	FilledAmount string `json:"filled_amount"`
	RemainAmount string `json:"remain_amount"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	ExecutedAt *time.Time `json:"executed_at,omitempty"`
	DoneAt     *time.Time `json:"done_at,omitempty"`
	IdemKey string `json:"idem_key"`
}

// IsTerminal 判断是否终态。
   func (p *RollbackPlan) IsTerminal() bool {
	  return p.Status == RollbackDone || p.Status == RollbackFailed
   }

// RollbackLeg 回滚腿明细记录，对应 cv_rollbacks 附属（或单独子表）。
type RollbackLeg struct {
	ID     string    `json:"id"`
	PlanID string    `json:"plan_id"`
	Action LegAction `json:"action"`
	Ref    string    `json:"ref"`
	Amount string `json:"amount,omitempty"`
	Status LegStatus `json:"status"`
	Error  string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
