package wdcomp

import "time"

type FailureCode string

const (
	CodeSignFail FailureCode = "sign_fail"
	CodeBroadcastReject FailureCode = "broadcast_reject"
	CodeStuckAbandon    FailureCode = "stuck_abandon"
	CodePolicyReject FailureCode = "policy_reject"
	CodeHumanCancel FailureCode = "human_cancel"
	CodeShardPartial FailureCode = "shard_partial"
	CodeShardFail    FailureCode = "shard_fail"
	CodeFeeOrphan    FailureCode = "fee_reserve_orphan"
	CodeUnknownChain FailureCode = "unknown_chain"
	CodeLiveHash FailureCode = "live_hash"
)

type PlanState string

const (
	PlanOpen     PlanState = "open"
	PlanPlanning PlanState = "planning"
	PlanExecuting  PlanState = "executing"
	PlanCommitted  PlanState = "committed"
	PlanNeedsHuman PlanState = "needs_human"
	PlanAborted    PlanState = "aborted"
)

type FailureStatus string

const (
	FailOpen FailureStatus = "open"
	FailPlanned      FailureStatus = "planned"
	FailCompensating FailureStatus = "compensating"
	FailClosed       FailureStatus = "closed"
	FailNeedsHuman FailureStatus = "needs_human"
	FailAborted FailureStatus = "aborted"
)

type LegKind string

const (
	LegOccupancy LegKind = "release_occupancy"
	LegShard    LegKind = "release_shard_remainder"
	LegUnfreeze LegKind = "unfreeze_user"
	LegFee      LegKind = "release_fee_reserve"
	LegSeal LegKind = "seal_ledger"
	LegAudit LegKind = "audit_deny"
	LegIncident LegKind = "open_incident"
)

type LegStatus string

const (
	LegPending LegStatus = "pending"
	LegRunning  LegStatus = "running"
	LegDone     LegStatus = "done"
	LegSkipped  LegStatus = "skipped"
	LegRejected LegStatus = "rejected"
	LegFailed   LegStatus = "failed"
	LegBlocked  LegStatus = "blocked"
)

const (
	SourceStuck = "stuck"
	SourceCancel = "cancel"
	SourceRecover = "recover"
	SourceShard   = "shard"
	SourcePolicy  = "policy"
	SourceManual  = "manual"
)

// FailureFact 一次失败或放弃的输入事实。
// 功能：承接卡单、取消、恢复和分片失败事件。
// TxHash 只描述本次要补偿的那一段；已成交分片的哈希放在 FilledTxNote，不参与热态判断。
// 不变量：已成交加在途加失败不得超过总冻结本金。
type FailureFact struct {
	Source string `json:"source"`
	TransferID string `json:"transfer_id"`
	UserID     string `json:"user_id"`
	Asset      string `json:"asset"`
	Chain string `json:"chain,omitempty"`
	Amount         string `json:"amount"`
	FilledAmount   string `json:"filled_amount,omitempty"`
	FailedAmount   string `json:"failed_amount,omitempty"`
	InFlightAmount string `json:"in_flight_amount,omitempty"`
	FeeReserved    string `json:"fee_reserved,omitempty"`
	Occupancy string `json:"occupancy,omitempty"`
	TxHash       string `json:"tx_hash,omitempty"`
	FilledTxNote string `json:"filled_tx_note,omitempty"`
	BroadcastStatus string `json:"broadcast_status,omitempty"`
	FreezeState string `json:"freeze_state,omitempty"`
	ChainState  string `json:"chain_state,omitempty"`
	ShardPlanID string `json:"shard_plan_id,omitempty"`
	ShardID     string `json:"shard_id,omitempty"`
	ShardSeq    int    `json:"shard_seq,omitempty"`
	ShardCount  int    `json:"shard_count,omitempty"`
	Reason string `json:"reason,omitempty"`
	Code           string `json:"code,omitempty"`
	IdempotencyKey string `json:"idempotency_key"`
	OccurredAt        time.Time `json:"occurred_at,omitempty"`
	AvailableSnapshot string    `json:"available_snapshot,omitempty"`
	FrozenSnapshot    string    `json:"frozen_snapshot,omitempty"`
	FeeSnapshot       string `json:"fee_snapshot,omitempty"`
	OccupancySnapshot string `json:"occupancy_snapshot,omitempty"`
}

// WdFailure 已分类的失败单。
type WdFailure struct {
	ID string `json:"id"`
	IdempotencyKey string `json:"idempotency_key"`
	Fingerprint string `json:"fingerprint"`
	TransferID string `json:"transfer_id"`
	UserID     string `json:"user_id"`
	Asset      string      `json:"asset"`
	Chain      string      `json:"chain,omitempty"`
	Source     string      `json:"source"`
	Code       FailureCode `json:"code"`
	ReasonCode string      `json:"reason_code"`
	Reason     string      `json:"reason,omitempty"`
	UserText     string `json:"user_text"`
	TxHash       string `json:"tx_hash,omitempty"`
	FilledTxNote string `json:"filled_tx_note,omitempty"`
	LiveHash     bool   `json:"live_hash"`
	ChainBlocked bool `json:"chain_blocked"`
	BroadcastStatus string `json:"broadcast_status,omitempty"`
	FreezeState string `json:"freeze_state,omitempty"`
	ChainState  string `json:"chain_state,omitempty"`
	ShardPlanID string `json:"shard_plan_id,omitempty"`
	ShardID  string `json:"shard_id,omitempty"`
	ShardSeq int    `json:"shard_seq,omitempty"`
	Amount   string `json:"amount"`
	FilledAmount string `json:"filled_amount"`
	FailedAmount string `json:"failed_amount"`
	InFlightAmount string `json:"in_flight_amount"`
	FeeReserved     string `json:"fee_reserved"`
	Occupancy       string `json:"occupancy"`
	ProtectedAmount string `json:"protected_amount"`
	Compensable string `json:"compensable"`
	AvailableSnapshot string `json:"available_snapshot,omitempty"`
	FrozenSnapshot string `json:"frozen_snapshot,omitempty"`
	FeeSnapshot string `json:"fee_snapshot,omitempty"`
	Status     FailureStatus `json:"status"`
	PlanID     string        `json:"plan_id,omitempty"`
	MayAuto    bool          `json:"may_auto"`
	OccurredAt time.Time     `json:"occurred_at,omitempty"`
	IngestedAt time.Time `json:"ingested_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CompensatePlan 一张补偿计划。
type CompensatePlan struct {
	ID string `json:"id"`
	FailureID string `json:"failure_id"`
	TransferID string `json:"transfer_id"`
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	State PlanState `json:"state"`
	AutoOK     bool `json:"auto_ok"`
	ForceHuman bool `json:"force_human"`
	LiveHash bool `json:"live_hash"`
	ChainBlocked bool `json:"chain_blocked"`
	PolicyCode     string `json:"policy_code"`
	PolicyVersion  int    `json:"policy_version"`
	IdempotencyKey string `json:"idempotency_key"`
	HumanTicketID string `json:"human_ticket_id,omitempty"`
	HumanActor    string `json:"human_actor,omitempty"`
	HumanApprover string `json:"human_approver,omitempty"`
	HumanReason   string `json:"human_reason,omitempty"`
	HumanAt     time.Time `json:"human_at,omitempty"`
	AbortReason string    `json:"abort_reason,omitempty"`
	TimeoutAt time.Time `json:"timeout_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	CommittedAt time.Time `json:"committed_at,omitempty"`
}

// CompensateLeg 计划中的一条有序补偿腿。
type CompensateLeg struct {
	ID     string `json:"id"`
	PlanID string `json:"plan_id"`
	Seq  int     `json:"seq"`
	Kind LegKind `json:"kind"`
	Status LegStatus `json:"status"`
	Amount string `json:"amount"`
	Asset string `json:"asset,omitempty"`
	UserID     string `json:"user_id,omitempty"`
	TransferID string `json:"transfer_id,omitempty"`
	IdempotencyKey string `json:"idempotency_key"`
	ResultRef string `json:"result_ref,omitempty"`
	ReasonCode string `json:"reason_code,omitempty"`
	ErrText    string `json:"err,omitempty"`
	Attempts int `json:"attempts"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PlanView 计划、腿和失败单一起返回，避免调用方再拼。

type PlanView struct {
	Plan    CompensatePlan  `json:"plan"`
	Legs    []CompensateLeg `json:"legs"`
	Failure WdFailure       `json:"failure"`
}

// IngestResult 摄取结果。Replayed 为真时没有新的失败单，也没有再次动账。
type IngestResult struct {
	Failure WdFailure `json:"failure"`
	Replayed bool `json:"replayed"`
}

// Incident 与值班事故单的挂钩。WarRoomRef 只是本域编号，不指向外部地址。

type Incident struct {
	ID string `json:"id"`
	PlanID string `json:"plan_id"`
	FailureID  string `json:"failure_id"`
	TransferID string `json:"transfer_id"`
	UserID     string `json:"user_id"`
	Kind string `json:"kind"`
	Actor string `json:"actor,omitempty"`
	Approver string `json:"approver,omitempty"`
	Note     string `json:"note,omitempty"`
	WarRoomRef string `json:"war_room_ref"`
	OpenedAt time.Time `json:"opened_at"`
}

// ListFilter 查询过滤。Limit 小于等于 0 时由接口给默认值。
type ListFilter struct {
	TransferID string
	UserID string
	Asset  string
	Status string
	Code   string
	Limit int
}

// TickReport 一轮工人的计数。
type TickReport struct {
	Ingested int `json:"ingested"`
	Replayed int `json:"replayed"`
	Planned int `json:"planned"`
	Executed  int `json:"executed"`
	Escalated int `json:"escalated"`
	Denied    int `json:"denied"`
}

// PositionView 给接口和夹具看的余额。
type PositionView struct {
	UserID      string `json:"user_id"`
	Asset       string `json:"asset"`
	Available   string `json:"available"`
	Frozen      string `json:"frozen"`
	FeeReserved string `json:"fee_reserved"`
	Occupancy   string `json:"occupancy"`
	Equity string `json:"equity"`
	Conserved bool `json:"conserved"`
}
