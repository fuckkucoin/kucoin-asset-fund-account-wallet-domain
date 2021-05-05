package feecredit

import (
	"time"
	"account-assets.kucoin.net/funding/internal/feeeng"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/persist"
)

type Kind string

const (
	KindTake Kind = "take"
	KindRebate Kind = "rebate"
)

   func ParseKind(s string) (Kind, error) {
	  switch Kind(s) {
	   case KindTake, KindRebate:
		  return Kind(s), nil
	  default:
		   return "", ErrKind
	  }
  }

type Credit struct {
	ID string `json:"id"`
	Idem     string `json:"idempotency_key"`
	UserID   string `json:"user_id"`
	Kind     Kind   `json:"kind"`
	Asset    string `json:"asset"`
	Gross    string `json:"gross"`
	Fee      string `json:"fee"`
	Net      string `json:"net"`
	QuoteID  string `json:"quote_id,omitempty"`
	LedgerID string `json:"ledger_id,omitempty"`
	Lines     []money.Line  `json:"-"`
	Fence     persist.Fence `json:"-"`
	CreatedAt time.Time     `json:"created_at"`
}

   func (c Credit) Copy() Credit { return c }

type PostReq struct {
	UserID  string `json:"user_id"`
	Kind    string `json:"kind"`
	FeeKind string `json:"fee_kind"`
	Asset          string `json:"asset"`
	Gross          string `json:"gross"`
	IdempotencyKey string `json:"idempotency_key"`
}

type Applied struct {
	Credit Credit `json:"credit"`
	Quote feeeng.Quote `json:"quote"`
}
