package reorgorch

import "time"

// ────────────────────────────────────────────────────────────────────────────
// ReverseBatch 状态常量
// ────────────────────────────────────────────────────────────────────────────

const (
	// BatchStateOpen 批次已创建，尚未规划腿。
	BatchStateOpen = "open"
	// BatchStatePlanning 正在规划冲正腿（生成 ReverseLeg 记录）。
	BatchStatePlanning = "planning"
	// BatchStateExecuting 正在顺序执行各腿。
	BatchStateExecuting = "executing"
	// BatchStateCommitted 全部腿执行完成（done / skipped），批次关闭。
	BatchStateCommitted = "committed"
	// BatchStateAborted 存在 failed 腿且补偿完成，批次关闭（需人工复核）。
	BatchStateAborted = "aborted"
)

// ────────────────────────────────────────────────────────────────────────────
// ReverseLeg 状态常量
// ────────────────────────────────────────────────────────────────────────────

const (
	// LegStatePending 腿尚未执行。
	LegStatePending = "pending"
	// LegStateDone 腿已成功执行。
	LegStateDone = "done"
	// LegStateSkipped 腿因前序失败被跳过（依赖序保护）。
	LegStateSkipped = "skipped"
	// LegStateFailed 腿执行失败（超时或调用错误）。
	LegStateFailed = "failed"
	// LegStateCompensated 腿补偿完成（仅针对 failed 腿）。
	LegStateCompensated = "compensated"
	// LegStateManualPending 腿等待人工确认（high-risk / live-hash）。
	LegStateManualPending = "manual_pending"
)

// ────────────────────────────────────────────────────────────────────────────
// LegKind 腿类型
// ────────────────────────────────────────────────────────────────────────────

const (
	// LegKindDebitReversal 账本借方冲正分录（对应 KindCredited）。
	LegKindDebitReversal = "debit_reversal"
	// LegKindOccupancyRelease 风险占用释放请求（对应 KindOccupied）。
	LegKindOccupancyRelease = "occupancy_release"
	// LegKindFreezeRollback 出金冻结回滚（对应 KindFreeze）。
	LegKindFreezeRollback = "freeze_rollback"
	// LegKindHopClose 划转 hop 收口通知（对应 KindHopPartial）。
	LegKindHopClose = "hop_close"
	// LegKindIncident live-hash 事故单（对应 KindLiveHash，不执行解冻）。
	LegKindIncident = "incident"
)

// ImpactKindToLegKind 将 reorgacct 的资金位 Kind 映射到 ReverseLeg Kind。
var ImpactKindToLegKind = map[string]string{
	"credited": LegKindDebitReversal,
	"occupied": LegKindOccupancyRelease,
	"freeze": LegKindFreezeRollback,
	"hop_partial": LegKindHopClose,
	"live_hash": LegKindIncident,
}

// LegExecutionOrder 腿执行依赖顺序（值越小越先执行）。
var LegExecutionOrder = map[string]int{
	LegKindDebitReversal:    1,
	LegKindOccupancyRelease: 2,
	LegKindFreezeRollback: 3,
	LegKindHopClose:       4,
	LegKindIncident: 5,
}

// ────────────────────────────────────────────────────────────────────────────
// ReverseBatch
// ────────────────────────────────────────────────────────────────────────────
// ReverseBatch 冲正编排批次。
type ReverseBatch struct {
	// ID UUID v4，主键。
	ID string `json:"id"`
	// EventID 关联的 reorgacct.ReorgEvent.ID。
	EventID string `json:"event_id"`
	// State 批次状态（open/planning/executing/committed/aborted）。
	State string `json:"state"`
	// ImpactRowIDs 关联的 ImpactRow.ID 列表（JSON 序列化存储）。
	ImpactRowIDs []string `json:"impact_row_ids"`
	// TotalLegs 批次总腿数。
	TotalLegs int `json:"total_legs"`
	// DoneLegs 已完成腿数（done + skipped）。
	DoneLegs int `json:"done_legs"`
	// FailedLegs 失败腿数。
	FailedLegs int `json:"failed_legs"`
	// IdemKey 批次幂等键（SHA-256 of event_id + impact_row_ids 排序）。
	IdemKey string `json:"idem_key"`
	// Note 批次说明（自动填充或人工备注）。
	Note string `json:"note,omitempty"`
	// CreatedAt 批次创建时间（UTC）。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 最近状态变更时间（UTC）。
	UpdatedAt time.Time `json:"updated_at"`
}

// ────────────────────────────────────────────────────────────────────────────
// ReverseLeg
// ────────────────────────────────────────────────────────────────────────────
// ReverseLeg 批次中的单条冲正腿。

type ReverseLeg struct {
	// ID UUID v4，主键。
	ID string `json:"id"`
	// BatchID 关联的 ReverseBatch.ID。
	BatchID string `json:"batch_id"`
	// ImpactRowID 关联的 reorgacct.ImpactRow.ID。
	ImpactRowID string `json:"impact_row_id"`
	// Kind 腿类型（LegKind* 常量）。
	Kind string `json:"kind"`
	// ExecutionOrder 执行顺序（值越小越先执行，由 LegExecutionOrder 决定）。
	ExecutionOrder int `json:"execution_order"`
	// State 腿状态（pending/done/skipped/failed/compensated/manual_pending）。
	State string `json:"state"`
	// IdemKey 腿幂等键，SHA-256(batch_id + "|" + impact_row_id + "|" + kind)。
	IdemKey string `json:"idem_key"`
	// ReferenceID 被操作对象的 ID（TransferID / OccupancyID / FreezeID / HopID / WithdrawID）。
	ReferenceID string `json:"reference_id"`
	// Amount 操作金额（字符串 decimal）。
	Amount string `json:"amount"`
	// UserID 用户 ID。
	UserID string `json:"user_id"`
	// Asset 资产代码。
	Asset string `json:"asset"`
	// ResultNote 执行结果说明（成功/失败原因/事故单号）。
	ResultNote string `json:"result_note,omitempty"`
	// HighRisk 是否标记为高危腿（需人工确认闸）。
	HighRisk bool `json:"high_risk"`
	// StartedAt 腿执行开始时间（UTC，pending 时为零）。
	StartedAt *time.Time `json:"started_at,omitempty"`
	// FinishedAt 腿执行完成时间（UTC）。
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	// CreatedAt 腿创建时间（UTC）。
	CreatedAt time.Time `json:"created_at"`
}

// ────────────────────────────────────────────────────────────────────────────
// ReverseGuard 幂等护栏
// ────────────────────────────────────────────────────────────────────────────
// ReverseGuard 防止同一 ImpactRow 被二次冲正的护栏记录。
// 唯一约束：(impact_row_id, leg_kind)。

type ReverseGuard struct {
	// ID UUID v4，主键。
	ID string `json:"id"`
	// ImpactRowID 关联的 ImpactRow.ID。
	ImpactRowID string `json:"impact_row_id"`
	// LegKind 腿类型。
	LegKind string `json:"leg_kind"`
	// BatchID 第一次冲正所属批次 ID。
	BatchID string `json:"batch_id"`
	// LegID 第一次冲正腿 ID。
	LegID string `json:"leg_id"`
	// LockedAt 护栏写入时间（UTC）。
	LockedAt time.Time `json:"locked_at"`
}

// ────────────────────────────────────────────────────────────────────────────
// IncidentTicket 事故单（live-hash / 超时腿）
// ────────────────────────────────────────────────────────────────────────────
// IncidentTicket 记录一次需要人工介入的事故。
type IncidentTicket struct {
	// ID UUID v4，主键。
	ID string `json:"id"`
	// BatchID 关联批次。
	BatchID string `json:"batch_id"`
	// LegID 关联腿（可为空，批次级别事故时无腿 ID）。
	LegID string `json:"leg_id,omitempty"`
	// Kind 事故类型：live_hash_manual / leg_timeout / leg_failed_manual。
	Kind string `json:"kind"`
	// Description 事故描述。
	Description string `json:"description"`
	// State 处理状态：open / acknowledged / resolved。
	State string `json:"state"`
	// CreatedAt 事故创建时间（UTC）。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 最近变更时间。
	UpdatedAt time.Time `json:"updated_at"`
}

// ────────────────────────────────────────────────────────────────────────────
// PlanResult 规划结果
// ────────────────────────────────────────────────────────────────────────────
// PlanResult 冲正腿规划结果摘要。

type PlanResult struct {
	BatchID   string `json:"batch_id"`
	TotalLegs int    `json:"total_legs"`
	HighRisk  int    `json:"high_risk"`
	LiveHash  int    `json:"live_hash"`
}

// ────────────────────────────────────────────────────────────────────────────
// ExecuteResult 执行结果摘要
// ────────────────────────────────────────────────────────────────────────────
// ExecuteResult 单次执行轮次的结果摘要。
type ExecuteResult struct {
	BatchID  string `json:"batch_id"`
	Executed int    `json:"executed"`
	Done     int    `json:"done"`
	Skipped  int    `json:"skipped"`
	Failed int `json:"failed"`
	Manual     int    `json:"manual_pending"`
	Incidents  int    `json:"incidents"`
	BatchState string `json:"batch_state"`
}
