package wdshard

import "time"

// LegStatus 分片腿生命周期。本包不改写部分成交成交额公式。

type LegStatus string

const (
	HoldLegPending LegStatus = "pending"
	HoldLegAdvancing    LegStatus = "advancing"
	HoldLegBroadcasting LegStatus = "broadcasting"
	HoldLegFilled LegStatus = "filled"
	HoldLegFailed LegStatus = "failed"
	HoldLegReleased LegStatus = "released"
)

// PlanStatus 分片计划状态。
type PlanStatus string

const (
	PlanOpen     PlanStatus = "open"
	PlanPartial  PlanStatus = "partial"
	PlanSettled  PlanStatus = "settled"
	PlanCanceled PlanStatus = "canceled"
)

// Plan 大额出金分片计划。只读挂钩面。

type Plan struct {
	PlanID string `json:"plan_id"`
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	Chain    string `json:"chain"`
	DestAddr string `json:"dest_addr"`
	TotalAmt int64  `json:"total_amt"`
	SplitCount int `json:"split_count"`
	Status PlanStatus `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Leg 分片腿。Advance 冻结 + 广播；Release 扫 failed。
type Leg struct {
	PlanID string `json:"plan_id"`
	LegID  string `json:"leg_id"`
	Seq    int    `json:"seq"`
	Amount int64 `json:"amount"`
	FilledAmt int64 `json:"filled_amt"`
	RemainAmt int64     `json:"remain_amt"`
	Status    LegStatus `json:"status"`
	HoldID string `json:"hold_id"`
	HoldAmt     int64  `json:"hold_amt"`
	BroadcastID string `json:"broadcast_id"`
	TxHash      string `json:"tx_hash"`
	FailReason string `json:"fail_reason"`
	LiveHash   string `json:"live_hash"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Hold 腿冻结占额。HoldFunc 挂钩。
type Hold struct {
	HoldID string `json:"hold_id"`
	PlanID string `json:"plan_id"`
	LegID  string `json:"leg_id"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount int64  `json:"amount"`
	Released int64 `json:"released"`
	Active bool `json:"active"`
	CreatedAt time.Time `json:"created_at"`
}

  func (h Hold) Residual() int64 {
	  r := h.Amount - h.Released
	   if r < 0 {
		   return 0
	   }
	  return r
   }

// Snapshot 腿即时视图。
type Snapshot struct {
	Plan Plan
	Leg  Leg
	Hold *Hold
}

// AlignHint AlignWorker 只读推进成交提示，不改 r5 公式。
type AlignHint struct {
	PlanID string
	LegID     string
	FillDelta int64
	TxHash    string
}

// HoldFunc 冻结/释放挂钩。shardholdgen 只调用，不实现账本主链。

type HoldAPI interface {
	Freeze(planID, legID, userID, asset string, amount int64) (Hold, error)
	Release(holdID string, amount int64) (Hold, error)
	Get(holdID string) (Hold, error)
}

// AlignHook AlignWorker 挂钩。

type AlignHook interface {
	Align(hint AlignHint) (Leg,
		error)
}
