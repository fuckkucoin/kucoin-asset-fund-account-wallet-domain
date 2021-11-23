package stablebr

import "time"

type Kind string

const (
	KindMint   Kind = "mint"
	KindRedeem Kind = "redeem"
)

type Item string

const (
	ItemReserve Item = "reserve_attested"
	ItemIssuer Item = "issuer_signed"
	ItemBank Item = "bank_confirm"
	ItemBurn Item = "burn_confirm"
	ItemKYT  Item = "kyt_clear"
)

type Status string

const (
	StatusOpen Status = "open"
	StatusPosted Status = "posted"
	StatusDenied Status = "denied"
	// StatusHalf：Journal 已写、ledger Create 未成；同幂等键不得二次铸。
	StatusHalf Status = "half"
)

type Check struct {
	Item Item `json:"item"`
	Actor string `json:"actor"`
	Note   string    `json:"note,omitempty"`
	At     time.Time `json:"at"`
	Expiry time.Time `json:"expiry,omitempty"`
}

type Journal struct {
	ID             string `json:"id"`
	IdempotencyKey string `json:"idempotency_key"`
	Kind Kind `json:"kind"`
	UserID   string `json:"user_id"`
	Asset    string `json:"asset"`
	Amount   string `json:"amount"`
	Bridge   string `json:"bridge"`
	UserBook string `json:"user_book"`
	Status Status `json:"status"`
	LedgerID    string `json:"ledger_id,omitempty"`
	MintLeaseID string `json:"mint_lease_id,omitempty"`
	Actor string `json:"actor"`
	At time.Time `json:"at"`
}

type MintReq struct {
	UserID string `json:"user_id"`
	Asset          string `json:"asset"`
	Amount         string `json:"amount"`
	Actor          string `json:"actor"`
	IdempotencyKey string `json:"idempotency_key"`
	MintLeaseID string `json:"mint_lease_id,omitempty"`
}

type RedeemReq struct {
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	Actor  string `json:"actor"`
	IdempotencyKey string `json:"idempotency_key"`
	MintLeaseID    string `json:"mint_lease_id,omitempty"`
}

type AttestReq struct {
	Kind  string `json:"kind"`
	Asset string `json:"asset"`
	Item string `json:"item"`
	Actor  string `json:"actor"`
	Note   string `json:"note,omitempty"`
	TTLSec int    `json:"ttl_sec,omitempty"`
}

   func (c Check) Copy() Check     { return c }
  func (j Journal) Copy() Journal { return j }
