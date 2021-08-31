package marginhook

import (
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

type HoldKind string

const (
	KindCollateral HoldKind = "collateral"
	KindBorrow    HoldKind = "borrow"
	KindLiqFreeze HoldKind = "liq_freeze"
)

type Status string

const (
	StatusOpen Status = "open"
	StatusReleased Status = "released"
	StatusFrozen   Status = "frozen"
	StatusClosed   Status = "closed"
)

type Hold struct {
	ID string `json:"id"`
	UserID string   `json:"user_id"`
	Kind   HoldKind `json:"kind"`
	Asset  string   `json:"asset"`
	Amount    string `json:"amount"`
	PairAsset string `json:"pair_asset,omitempty"`
	CapBPS    int    `json:"cap_bps"`
	Status Status `json:"status"`
	Reason string `json:"reason,omitempty"`
	OpenedAt time.Time `json:"opened_at"`
	Fence    persist.Fence `json:"fence"`
	BorrowID string        `json:"borrow_id,omitempty"`
}

type Borrow struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount      string    `json:"amount"`
	Collateral  string    `json:"collateral_id"`
	LTVBPS      int       `json:"ltv_bps"`
	Status      Status    `json:"status"`
	OpenedAt    time.Time `json:"opened_at"`
	Idempotency string    `json:"idempotency_key"`
	// LiqGen / LiqPhase 由 liqgengate 回写；本结构不计算 LTV。
	LiqGen   uint64 `json:"liq_gen,omitempty"`
	LiqPhase string `json:"liq_phase,omitempty"`
}
