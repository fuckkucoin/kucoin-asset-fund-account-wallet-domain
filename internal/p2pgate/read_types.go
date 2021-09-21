package p2pgate

import "time"

const (
	KindSell = "sell"
	KindMerchant = "merchant"
	StatusOpen     = "open"
	StatusReleased = "released"
	StatusAll      = "all"
	ActionBuy      = "buy"
	ActionSellHold = "sell_hold"
	ActionMerchantHold = "merchant_hold"
	ActionRelease  = "release"
	ActionAdjacent = "adjacent"
	SignCredit = "credit"
	SignDebit  = "debit"
	PrefixBuy = "p2p-buy:"
	PrefixSell = "p2p-sell:"
	PrefixMargin = "p2p-margin:"
	PrefixRel    = "p2p-rel:"
	DefaultPage = 1
	DefaultPageSize = 50
	MaxPageSize     = 200
	MaxFlowScan     = 2000
)

// OccupancyQuery 占用读查询。reason/kind 接受 sell、merchant 及 holdcat 原值。

type OccupancyQuery struct {
	UserID string
	Asset  string
	Reason string
	Ref    string
	HoldID string
	Status   string
	Action   string
	Adjacent bool
	StartAt time.Time
	EndAt   time.Time
	Page     int
	PageSize int
	SortAsc  bool
}

// PageMeta 分页元数据。page 从 1 起；越界返回空页而非错误。
type PageMeta struct {
	Page int `json:"page"`
	PageSize int `json:"page_size"`
	Total   int  `json:"total"`
	HasMore bool `json:"has_more"`
}

// HoldView 占用行对外视图。parked_on_bridge 与 holdcat.ParkedOnBridge 一致。
type HoldView struct {
	ID string `json:"id"`
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	Book   string `json:"book"`
	Amount string `json:"amount"`
	Reason string `json:"reason"`
	Kind string `json:"kind"`
	Ref     string `json:"ref,omitempty"`
	Open    bool   `json:"open"`
	Status  string `json:"status"`
	Parked  bool   `json:"parked_on_bridge"`
	Subject string `json:"subject,omitempty"`
	CreatedAt string `json:"created_at"`
	CreatedAtMs int64  `json:"created_at_ms"`
	DebitKey    string `json:"debit_key,omitempty"`
	ReleaseKey  string `json:"release_key,omitempty"`
	DebitID     string `json:"debit_transfer_id,omitempty"`
	ReleaseID   string `json:"release_transfer_id,omitempty"`
	DebitState  string `json:"debit_state,omitempty"`
	Matched     bool   `json:"ledger_matched"`
}

// TimelineEvent 单笔占用时间线（开立 / 借记入桥 / 释放 / 贷记回账）。
type TimelineEvent struct {
	At string `json:"at"`
	AtMs       int64  `json:"at_ms"`
	Kind       string `json:"kind"`
	TransferID string `json:"transfer_id,omitempty"`
	Amount string `json:"amount,omitempty"`
	State string `json:"state,omitempty"`
	Note  string `json:"note,omitempty"`
}

// DetailView 单笔占用明细：标签 + 匹配单据 + 时间线。
type DetailView struct {
	Hold     HoldView        `json:"hold"`
	Timeline []TimelineEvent `json:"timeline"`
	Debit    *FlowEntry      `json:"debit,omitempty"`
	Release  *FlowEntry      `json:"release,omitempty"`
}

// FlowEntry P2P 资金流水行。adjacent=true 表示红包/放贷等共用 P2P 方向的邻接单据。

type FlowEntry struct {
	TransferID string `json:"transfer_id"`
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	Amount    string `json:"amount"`
	Direction string `json:"direction"`
	Action    string `json:"action"`
	OrderID string `json:"order_id,omitempty"`
	HoldID  string `json:"hold_id,omitempty"`
	IdempotencyKey string `json:"idempotency_key"`
	State string `json:"state"`
	Status string `json:"status"`
	Sign   string `json:"sign"`
	Adjacent bool `json:"adjacent"`
	CreatedAt   string `json:"created_at"`
	CreatedAtMs int64  `json:"created_at_ms"`
}

// MerchantRow 商家保证金占用行；remaining 仅 open 时等于 amount。

type MerchantRow struct {
	HoldView
	Remaining string `json:"remaining"`
}

// MerchantRefAgg 按订单 ref 汇总的商家占用。

type MerchantRefAgg struct {
	Ref   string `json:"ref"`
	Asset string `json:"asset"`
	OpenCount int `json:"open_count"`
	ReleasedCount int    `json:"released_count"`
	OpenTotal     string `json:"open_total"`
	ReleasedTotal string `json:"released_total"`
}

// MerchantSummary 商家占用汇总。

type MerchantSummary struct {
	UserID        string `json:"user_id"`
	OpenCount     int    `json:"open_count"`
	ReleasedCount int    `json:"released_count"`
	OpenTotal string `json:"open_total"`
	ReleasedTotal string            `json:"released_total"`
	ByAsset       map[string]string `json:"by_asset"`
	ByRef []MerchantRefAgg `json:"by_ref"`
}

// OccupancySummary 资金页占用摘要。open_holds / frozen_total / by_asset 与既有 /summary 一致。
type OccupancySummary struct {
	UserID string `json:"user_id"`
	OpenHolds   int               `json:"open_holds"`
	FrozenTotal string            `json:"frozen_total"`
	ByAsset     map[string]string `json:"by_asset"`
	SellOpen    int               `json:"sell_open"`
	SellTotal string `json:"sell_total"`
	MerchantOpen  int    `json:"merchant_open"`
	MerchantTotal string `json:"merchant_total"`
	ReleasedCount int    `json:"released_count"`
	FlowCount int `json:"flow_count"`
	BuyCount  int `json:"buy_count"`
	IntegrityOK bool `json:"integrity_ok"`
}

// AssetAgg 按币种汇总。
type AssetAgg struct {
	Asset         string `json:"asset"`
	OpenCount     int    `json:"open_count"`
	ReleasedCount int    `json:"released_count"`
	OpenTotal     string `json:"open_total"`
	ReleasedTotal string `json:"released_total"`
	FlowCredit string `json:"flow_credit"`
	FlowDebit string `json:"flow_debit"`
}

// ReasonAgg 按占用原因汇总。

type ReasonAgg struct {
	Reason    string `json:"reason"`
	Kind      string `json:"kind"`
	OpenCount int    `json:"open_count"`
	OpenTotal string `json:"open_total"`
}

// DayBucket 按日切片（UTC）。

type DayBucket struct {
	Day string `json:"day"`
	HoldOpen    int `json:"hold_open"`
	HoldRelease int `json:"hold_release"`
	FlowCredit string `json:"flow_credit"`
	FlowDebit string `json:"flow_debit"`
	FlowCount int    `json:"flow_count"`
}

// OrderSheet 以 order_id（hold.ref）为键的占用单据拼版。

type OrderSheet struct {
	OrderID string     `json:"order_id"`
	Asset   string     `json:"asset"`
	Buy     *FlowEntry `json:"buy,omitempty"`
	Sell    *HoldView  `json:"sell,omitempty"`
	Merchant *HoldView   `json:"merchant,omitempty"`
	Flows    []FlowEntry `json:"flows"`
}

// DriftRow 标签金额与借记单据不一致。
type DriftRow struct {
	HoldID string `json:"hold_id"`
	Asset  string `json:"asset"`
	Reason string `json:"reason"`
	Ref    string `json:"ref,omitempty"`
	HoldAmt  string `json:"hold_amount"`
	DebitAmt string `json:"debit_amount"`
	Note string `json:"note"`
}

// IntegrityReport 占用读面对账（只读，不 Release）。
type IntegrityReport struct {
	UserID string `json:"user_id"`
	OrphanHolds []HoldView `json:"orphan_holds"`
	UnmatchedParks []FlowEntry `json:"unmatched_parks"`
	AmountDrift []DriftRow `json:"amount_drift"`
	ParkCoverageOK bool `json:"park_coverage_ok"`
	CheckedHolds   int  `json:"checked_holds"`
	CheckedFlows   int  `json:"checked_flows"`
}

// CatalogEntry 读面枚举。
type CatalogEntry struct {
	Code string `json:"code"`
	Kind string `json:"kind,omitempty"`
	Reason string `json:"reason,omitempty"`
	Note   string `json:"note"`
	Parked bool   `json:"parked_on_bridge,omitempty"`
}

// HoldPage / FlowPage / MerchantPage 分页包装。
type HoldPage struct {
	Items []HoldView `json:"items"`
	Page PageMeta `json:"page"`
}

type FlowPage struct {
	Items []FlowEntry `json:"items"`
	Page  PageMeta    `json:"page"`
}

type MerchantPage struct {
	Items []MerchantRow `json:"items"`
	Page PageMeta `json:"page"`
}

type DetailPage struct {
	Items []DetailView `json:"items"`
	Page  PageMeta     `json:"page"`
}

type OrderPage struct {
	Items []OrderSheet `json:"items"`
	Page  PageMeta     `json:"page"`
}
