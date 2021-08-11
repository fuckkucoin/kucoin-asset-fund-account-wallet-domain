package hotreserve

import "time"

type State string

const (
	StateOpen State = "open"
	StateEarmarked State = "earmarked"
	StateConsumed  State = "consumed"
	StateExpired State = "expired"
	StateCancelled State = "cancelled"
)

const (
	TrackFreeze = "freeze"
	TrackBroadcast = "broadcast"
)

const (
	KindOpen          = "open"
	KindEarmark       = "earmark"
	KindEarmarkFreeze = "earmark_freeze"
	KindConsume       = "consume"
	KindConsumeFreeze = "consume_freeze"
	KindExpire        = "expire"
	KindCancel       = "cancel"
	KindCancelFreeze = "cancel_freeze"
	KindForce        = "force_release"
	KindForceFreeze = "force_release_freeze"
)

const (
	SubHot       = "hot_operational"
	SubOpen      = "reserve_open"
	SubEarmarked = "reserve_earmarked"
	SubFreeze    = "freeze_track"
	SubBroadcast = "broadcast_track"
	SubMemoFrz   = "memo_freeze"
	SubMemoBc    = "memo_broadcast"
)

// HotForecast 单资产窗口预测。Lower 是保守下界，允许为负。
// 做什么：给出点估与下界，供预留和挡签复用。
// 代次只在输入摘要变化时递增；同一输入刷新只更新 AsOf。
// 不变量：Point/Lower 必须等于 Project(输入) 的结果。
type HotForecast struct {
	Asset string `json:"asset"`
	Version int64 `json:"version"`
	WindowSec int64     `json:"window_sec"`
	AsOf      time.Time `json:"as_of"`
	HotAvail  string    `json:"hot_available"`
	HotFrozen string `json:"hot_frozen"`
	Point     string `json:"point"`
	Lower string `json:"lower"`
	Inflow     string `json:"inflow_expected"`
	Outflow    string `json:"outflow_expected"`
	InflowKept string `json:"inflow_kept"`
	OutflowStressed string `json:"outflow_stressed"`
	SweepHold  string `json:"sweep_hold"`
	InTransit  string `json:"in_transit"`
	Buffer     string `json:"buffer"`
	Haircut    string `json:"haircut"`
	Stress     string `json:"stress"`
	InputsHash string `json:"inputs_hash"`
	SweepIDs []string `json:"sweep_plan_ids,omitempty"`
	SampleN  int      `json:"sample_n"`
	Bias     string   `json:"bias,omitempty"`
	Realized string `json:"realized,omitempty"`
	EventID string `json:"event_id"`
}

type ForecastHead struct {
	Asset string `json:"asset"`
	Version int64  `json:"version"`
	Hash    string `json:"inputs_hash"`
	Lower string `json:"lower"`
	Point   string    `json:"point"`
	AsOf    time.Time `json:"as_of"`
	EventID string    `json:"event_id"`
}

// Reservation 一笔出金意图占用。审批票只存引用，不推进审批链。
type Reservation struct {
	ID string `json:"id"`
	Idem             string    `json:"idempotency_key,omitempty"`
	Fingerprint      string    `json:"-"`
	Asset            string    `json:"asset"`
	Amount           string    `json:"amount"`
	State            State     `json:"state"`
	UserID           string    `json:"user_id,omitempty"`
	IntentID         string    `json:"intent_id,omitempty"`
	MovementID       string    `json:"movement_id,omitempty"`
	ApprovalTicketID string    `json:"approval_ticket_id,omitempty"`
	Note             string    `json:"note,omitempty"`
	ExpiresAt        time.Time `json:"expires_at"`
	EarmarkedAt time.Time `json:"earmarked_at,omitempty"`
	ConsumedAt  time.Time `json:"consumed_at,omitempty"`
	ReleasedAt  time.Time `json:"released_at,omitempty"`
	ReleaseKind string    `json:"release_kind,omitempty"`
	Requester string `json:"requester,omitempty"`
	Approver     string    `json:"approver,omitempty"`
	BroadcastRef string    `json:"broadcast_ref,omitempty"`
	FreezeRef    string    `json:"freeze_ref,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

   func (r Reservation) copy() Reservation { return r }

// ReservationJournal 预留分录。一行只有一个借方和一个贷方，金额等于预留本金。
type ReservationJournal struct {
	ID            string `json:"id"`
	ReservationID string `json:"reservation_id"`
	Asset string `json:"asset"`
	Seq   int    `json:"seq"`
	Kind  string `json:"kind"`
	Debit string `json:"debit_subject"`
	Credit string `json:"credit_subject"`
	Amount string `json:"amount"`
	EventID string    `json:"event_id"`
	Note    string    `json:"note,omitempty"`
	At      time.Time `json:"at"`
}

// AlignReport 三段一致结果。Drift 为真时挡该资产的新预留与新签。

type AlignReport struct {
	ID string `json:"id"`
	Asset string `json:"asset"`
	AsOf time.Time `json:"as_of"`
	Held string    `json:"reservation_held"`
	Earmarked string `json:"earmarked"`
	Consumed string `json:"consumed"`
	Open     string `json:"open"`
	Freeze    string `json:"freeze_track"`
	Broadcast string `json:"broadcast_track"`
	Drift     bool   `json:"drift"`
	DriftKind string `json:"drift_kind,omitempty"`
	TicketID string `json:"ticket_id,omitempty"`
	Blocks bool   `json:"blocks"`
	Note   string `json:"note,omitempty"`
	EventID string `json:"event_id"`
}

type DriftTicket struct {
	ID string `json:"id"`
	Asset string `json:"asset"`
	Kind  string `json:"kind"`
	State string `json:"state"`
	Held      string `json:"reservation_held"`
	Freeze    string `json:"freeze_track"`
	Broadcast string `json:"broadcast_track"`
	Note     string    `json:"note,omitempty"`
	ReportID string    `json:"report_id,omitempty"`
	OpenedAt time.Time `json:"opened_at"`
	ClosedAt time.Time `json:"closed_at,omitempty"`
}

type Sample struct {
	ID string `json:"id"`
	Asset string `json:"asset"`
	Inflow  string    `json:"inflow"`
	Outflow string    `json:"outflow"`
	At      time.Time `json:"at"`
}

type PlanHold struct {
	ID     string    `json:"id"`
	Asset  string    `json:"asset"`
	Amount string    `json:"amount"`
	Noted  time.Time `json:"noted_at"`
}

type SweepHint struct {
	PlanID string `json:"plan_id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Reason string `json:"reason"`
	At       time.Time `json:"at"`
	Executed bool      `json:"executed"`
}

type GateLog struct {
	ID    string `json:"id"`
	Phase string `json:"phase"`
	Asset string `json:"asset"`
	Amount     string `json:"amount"`
	MovementID string `json:"movement_id,omitempty"`
	Result     string `json:"result"`
	Reason string    `json:"reason,omitempty"`
	At     time.Time `json:"at"`
}

type CreateReq struct {
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	UserID     string `json:"user_id"`
	IntentID   string `json:"intent_id"`
	MovementID string `json:"movement_id"`
	ApprovalTicketID string `json:"approval_ticket_id"`
	IdempotencyKey   string `json:"idempotency_key"`
	Note             string `json:"note"`
}

type EarmarkReq struct {
	ID         string
	MovementID string
	FreezeRef  string
	Note string
}

type ConsumeReq struct {
	ID           string
	BroadcastRef string
	Note string
}

type ReleaseReq struct {
	ID        string
	Requester string
	Approver  string
	Note string
}
