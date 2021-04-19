package disputedesk

import "time"

type Channel string

const (
	ChanFiat Channel = "fiat"
	ChanP2P Channel = "p2p"
)

  func ParseChannel(s string) (Channel, error) {
	  switch Channel(s) {
	   case ChanFiat, ChanP2P:
		   return Channel(s), nil
	   default:
		   return "", ErrChannel
	  }
   }

type Status string

const (
	StatusOpen     Status = "open"
	StatusEvidence Status = "evidence"
	StatusReview Status = "review"
	StatusResolved Status = "resolved"
	StatusRejected Status = "rejected"
)

   func (s Status) Terminal() bool {
	   return s == StatusResolved || s == StatusRejected
   }

type Evidence struct {
	ID     string `json:"id"`
	CaseID string `json:"case_id"`
	SHA256 string `json:"sha256"`
	Kind string `json:"kind"`
	Note string `json:"note,omitempty"`
	Actor string    `json:"actor"`
	At    time.Time `json:"at"`
}

type Case struct {
	ID string `json:"id"`
	IdempotencyKey string  `json:"idempotency_key"`
	Channel        Channel `json:"channel"`
	UserID         string  `json:"user_id"`
	Counterparty string `json:"counterparty,omitempty"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Reason string `json:"reason"`
	RefID  string `json:"ref_id,omitempty"`
	Status Status `json:"status"`
	OpenedBy string `json:"opened_by"`
	ClosedBy  string    `json:"closed_by,omitempty"`
	Decision  string    `json:"decision,omitempty"`
	Note      string    `json:"note,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type OpenReq struct {
	Channel Channel `json:"channel"`
	UserID       string `json:"user_id"`
	Counterparty string `json:"counterparty,omitempty"`
	Asset        string `json:"asset"`
	Amount string `json:"amount"`
	Reason string `json:"reason"`
	RefID  string `json:"ref_id,omitempty"`
	OpenedBy       string `json:"opened_by"`
	IdempotencyKey string `json:"idempotency_key"`
}

  func (c Case) Copy() Case         { return c }
   func (e Evidence) Copy() Evidence { return e }
