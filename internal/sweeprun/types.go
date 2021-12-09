package sweeprun

import "time"

type State string

const (
	StateQueued  State = "queued"
	StateRunning State = "running"
	StatePartial State = "partial"
	StateFilled State = "filled"
	StatePaused    State = "paused"
	StateCancelled State = "cancelled"
)

  func (s State) Terminal() bool {
	  return s == StateFilled || s == StateCancelled
  }

  func (s State) CanFill() bool {
	  return s == StateRunning || s == StatePartial
  }

  func (s State) CanResume() bool {
	  return s == StatePaused || s == StatePartial
  }

type FillRec struct {
	Seq    int    `json:"seq"`
	Amount string `json:"amount"`
	Actor  string `json:"actor"`
	LedgerID string    `json:"ledger_id,omitempty"`
	At       time.Time `json:"at"`
}

type Job struct {
	ID             string `json:"id"`
	IdempotencyKey string `json:"idempotency_key"`
	Chain     string `json:"chain"`
	Asset     string `json:"asset"`
	Amount    string `json:"amount"`
	Filled    string `json:"filled"`
	Remaining string `json:"remaining"`
	JobCap    string `json:"job_cap,omitempty"`
	State     State  `json:"state"`
	From string `json:"from"`
	To           string `json:"to"`
	Actor        string `json:"actor,omitempty"`
	AgreeLeaseID string `json:"agree_lease_id,omitempty"`
	LedgerID string `json:"ledger_id,omitempty"`
	Fills     []FillRec `json:"fills,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Budget struct {
	Chain string `json:"chain,omitempty"`
	Asset string `json:"asset,omitempty"`
	DailyCap string `json:"daily_cap"`
	Used string `json:"used"`
	Day string `json:"day"`
}

type EnqueueReq struct {
	Chain string `json:"chain"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	JobCap string `json:"job_cap,omitempty"`
	From   string `json:"from,omitempty"`
	To             string `json:"to,omitempty"`
	IdempotencyKey string `json:"idempotency_key"`
	AgreeLeaseID string `json:"agree_lease_id,omitempty"`
}

   func (j Job) Copy() Job {
	   cp := j
	   if j.Fills != nil {
		  cp.Fills = append([]FillRec(nil), j.Fills...)
	   }
	   return cp
  }

  func (b Budget) Copy() Budget { return b }
