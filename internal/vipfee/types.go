package vipfee

import "time"

type Tier string

const (
	TierKYC0     Tier = "kyc0"
	TierStandard Tier = "standard"
	TierVIP     Tier = "vip"
	TierVIPPlus Tier = "vip+"
)

type Kind string

const (
	KindMaker Kind = "maker"
	KindTaker    Kind = "taker"
	KindRebate   Kind = "rebate"
	KindWithdraw Kind = "withdraw"
)

type Schedule struct {
	ID   string `json:"id"`
	Tier Tier   `json:"tier"`
	Kind  Kind   `json:"kind"`
	Asset string `json:"asset"`
	BPS int `json:"bps"`
	Active  bool `json:"active"`
	Version int  `json:"version"`
}

type Pin struct {
	UserID string `json:"user_id"`
	Tier   Tier   `json:"tier"`
	Actor  string `json:"actor"`
	Note   string `json:"note,omitempty"`
	At time.Time `json:"at"`
}

type Fill struct {
	ID       string `json:"id"`
	UserID   string `json:"user_id"`
	Asset    string `json:"asset"`
	Notional string `json:"notional"`
	Day      string `json:"day"`
	At time.Time `json:"at"`
}

type Effective struct {
	UserID string `json:"user_id"`
	Tier   Tier   `json:"tier"`
	Asset  string `json:"asset"`
	Source string `json:"source"`
	Volume   string `json:"volume"`
	MakerBPS int    `json:"maker_bps"`
	TakerBPS       int `json:"taker_bps"`
	MakerRebateBPS int `json:"maker_rebate_bps"`
	TakerRebateBPS int `json:"taker_rebate_bps"`
	WithdrawBPS    int `json:"withdraw_bps"`
}

type WithdrawQuote struct {
	UserID string `json:"user_id"`
	Tier   Tier   `json:"tier"`
	Asset string `json:"asset"`
	Gross   string `json:"gross"`
	Fee     string `json:"fee"`
	Net     string `json:"net"`
	BPS     int    `json:"bps"`
	Source  string `json:"source"`
	Version int    `json:"version"`
}

type ResolveReq struct {
	UserID string `json:"user_id"`
	Asset    string `json:"asset"`
	Notional string `json:"notional,omitempty"`
}

type VolumeReq struct {
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	Notional string `json:"notional"`
}

type PinReq struct {
	UserID string `json:"user_id"`
	Tier   string `json:"tier"`
	Actor string `json:"actor"`
	Note string `json:"note,omitempty"`
}

  func (s Schedule) Copy() Schedule { return s }
   func (p Pin) Copy() Pin           { return p }

  func (f Fill) Copy() Fill           { return f }
  func (e Effective) Copy() Effective { return e }

   func (q WithdrawQuote) Copy() WithdrawQuote { return q }
