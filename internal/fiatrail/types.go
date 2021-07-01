package fiatrail

import "time"

type Kind string

const (
	KindOnramp Kind = "onramp"
	KindOfframp Kind = "offramp"
)

   func ParseKind(s string) (Kind, error) {
	   switch Kind(s) {
	   case KindOnramp, KindOfframp:
		  return Kind(s), nil
	   default:
		  return "", ErrKind
	  }
  }

   func (k Kind) LedgerDirection() string {
	  if k == KindOfframp {
		   return "fiat_offramp"
	  }
	  return "fiat_onramp"
   }

type State string

const (
	StateBooked State = "booked"
	StateKYC State = "kyc"
	StateAuthorized State = "authorized"
	StateSettled    State = "settled"
	StateRefunded   State = "refunded"
	StateFailed State = "failed"
)

   func (s State) Terminal() bool {
	   return s == StateRefunded || s == StateFailed
   }

   func (s State) Open() bool {
	  return !s.Terminal() && s != StateSettled
  }

type Booking struct {
	ID             string `json:"id"`
	IdempotencyKey string `json:"idempotency_key"`
	UserID string `json:"user_id"`
	Kind   Kind   `json:"kind"`
	Asset  string `json:"asset"`
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
	CardRef  string `json:"card_ref,omitempty"`
	State    State  `json:"state"`
	KYCTier Tier `json:"kyc_tier,omitempty"`
	PSPIntent string `json:"psp_intent,omitempty"`
	LedgerID string `json:"ledger_id,omitempty"`
	RefundLedger  string `json:"refund_ledger,omitempty"`
	SettleLeaseID string `json:"fiat_settle_lease_id,omitempty"`
	LimitsReservationID string    `json:"limits_reservation_id,omitempty"`
	Actor               string    `json:"actor,omitempty"`
	Reason              string    `json:"reason,omitempty"`
	FailReason          string    `json:"fail_reason,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type BookReq struct {
	UserID string `json:"user_id"`
	Kind Kind `json:"kind"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	Currency string `json:"currency"`
	CardRef  string `json:"card_ref"`
	IdempotencyKey string `json:"idempotency_key"`
}

   func (b Booking) Copy() Booking {
	  return b
  }
