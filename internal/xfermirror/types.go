package xfermirror

import (
	"time"
	"github.com/shopspring/decimal"
)

// MirrorState 对手方镜像状态。

type MirrorState string

const (
	// MirrorProcessing 划转仍在处理中，对手方尚未收到资金。
	MirrorProcessing MirrorState = "processing"
	// MirrorCredited 资金已成功到账对手方账户（backfill 或正向 hop 成功）。
	MirrorCredited MirrorState = "credited"
	// MirrorRolledBack 发起方已执行回滚，对手方确认未到账（rollback_egress 完成）。
	MirrorRolledBack MirrorState = "rolled_back"
	// MirrorManual 需要人工介入核实，状态不确定。
	MirrorManual MirrorState = "manual"
)

// MirrorView 对手方镜像视图记录。

type MirrorView struct {
	// ID 记录唯一标识。
	ID string `json:"id"`
	// HopID 对应 xferclose.HalfHop.HopID（xferstore 幂等键）。
	HopID string `json:"hop_id"`
	// InitiatorUserID 发起方用户 ID。
	InitiatorUserID string `json:"initiator_user_id"`
	// CounterpartyUserID 对手方用户 ID（发起方视角的 ToBook 对应的用户，可能为空）。
	CounterpartyUserID string `json:"counterparty_user_id,omitempty"`
	// Asset 资产代码。
	Asset string `json:"asset"`
	// Amount 划转金额。
	Amount decimal.Decimal `json:"amount"`
	// FromBook 出腿科目。
	FromBook string `json:"from_book"`
	// ToBook 入腿科目。
	ToBook string `json:"to_book"`
	// State 镜像当前状态。
	State MirrorState `json:"state"`
	// StateReason 状态变更原因。
	StateReason string `json:"state_reason,omitempty"`
	// CloseoutPlanID 触发本次状态变更的收口计划 ID（可选）。
	CloseoutPlanID string `json:"closeout_plan_id,omitempty"`
	// EntryRef 账本操作引用 ID（到账或回滚的 entry ID）。
	EntryRef string `json:"entry_ref,omitempty"`
	// Version 乐观锁代次号，每次状态变更递增。
	Version int64 `json:"version"`
	// OverrideActor 人工覆盖操作员 ID（manual 状态时填入）。
	OverrideActor string `json:"override_actor,omitempty"`
	// CreatedAt 镜像首次创建时间。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 最后更新时间。
	UpdatedAt time.Time `json:"updated_at"`
}

// Terminal 报告镜像是否处于终态（credited 或 rolled_back）。
// manual 不视为终态，仍可被人工或自动推进。
   func (m MirrorView) Terminal() bool {
	   return m.State == MirrorCredited || m.State == MirrorRolledBack
   }

// MirrorEvent 镜像状态变更事件，写入审计日志。
type MirrorEvent struct {
	ID string `json:"id"`
	MirrorID  string      `json:"mirror_id"`
	HopID     string      `json:"hop_id"`
	FromState MirrorState `json:"from_state"`
	ToState   MirrorState `json:"to_state"`
	Reason    string      `json:"reason,omitempty"`
	Actor string    `json:"actor,omitempty"`
	At    time.Time `json:"at"`
}

// AlignResult 发起方与对手方状态一致验证结果。
type AlignResult struct {
	HopID string `json:"hop_id"`
	InitiatorState string `json:"initiator_state"`
	MirrorState MirrorState `json:"mirror_state"`
	Aligned  bool   `json:"aligned"`
	Conflict string `json:"conflict,omitempty"`
}

// MirrorQuery 查询过滤器。
type MirrorQuery struct {
	UserID string
	State  MirrorState
	Asset string
	Offset int
	Limit int
}
