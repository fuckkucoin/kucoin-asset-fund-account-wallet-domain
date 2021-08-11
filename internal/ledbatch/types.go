package ledbatch

import "time"

type Status string

const (
	StatusOpen      Status = "open"
	StatusSubmitted Status = "submitted"
	StatusHalted   Status = "halted"
	StatusCanceled Status = "canceled"
)

type ItemStatus string

const (
	ItemPending   ItemStatus = "pending"
	ItemSubmitted ItemStatus = "submitted"
	ItemSkipped   ItemStatus = "skipped"
	ItemFailed ItemStatus = "failed"
)

type Item struct {
	Seq int `json:"seq"`
	UserID    string `json:"user_id"`
	Direction string `json:"direction"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	IdempotencyKey string `json:"idempotency_key"`
	Status ItemStatus `json:"status"`
	Reason string     `json:"reason,omitempty"`
	LedgerID string `json:"ledger_id,omitempty"`
}

type Batch struct {
	ID string `json:"id"`
	IdempotencyKey string `json:"idempotency_key"`
	Actor          string `json:"actor"`
	Status    Status `json:"status"`
	Requested string `json:"requested"`
	Submitted string `json:"submitted"`
	Skipped string `json:"skipped"`
	Failed string `json:"failed"`
	Items     []Item    `json:"items"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type OpenReq struct {
	Actor string `json:"actor"`
	Items          []Item `json:"items"`
	IdempotencyKey string `json:"idempotency_key"`
}

  func (b Batch) Copy() Batch {
	  b.Items = append([]Item(nil), b.Items...)
	  return b
   }
