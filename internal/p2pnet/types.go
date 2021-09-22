package p2pnet

import "time"

type State string

const (
	StateSent     State = "sent"
	StateReversed State = "reversed"
	StateBlocked  State = "blocked"
)

type Transfer struct {
	ID             string `json:"id"`
	IdempotencyKey string `json:"idempotency_key"`
	From           string `json:"from"`
	To             string `json:"to"`
	Asset          string `json:"asset"`
	Amount         string `json:"amount"`
	LedgerID string `json:"ledger_id,omitempty"`
	ReverseID string `json:"reverse_id,omitempty"`
	State     State  `json:"state"`
	Flags     []string  `json:"flags,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type SendReq struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	IdempotencyKey string `json:"idempotency_key"`
}

   func (t Transfer) Copy() Transfer {
	   cp := t
	  if t.Flags != nil {
		  cp.Flags = append([]string(nil), t.Flags...)
	  }
	   return cp
  }
