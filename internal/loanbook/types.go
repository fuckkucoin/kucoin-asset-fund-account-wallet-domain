package loanbook

import "time"

type State string

const (
	StateOpen State = "open"
	StateCurrent State = "current"
	StateOverdue State = "overdue"
	StateFrozen  State = "frozen"
	StatePaid    State = "paid"
	StateDefault State = "default"
)

type InstStatus string

const (
	InstDue     InstStatus = "due"
	InstPaid    InstStatus = "paid"
	InstOverdue InstStatus = "overdue"
)

type TicketKind string

const (
	TicketTopUp TicketKind = "collateral_topup"
)

type TicketStatus string

const (
	TicketOpen   TicketStatus = "open"
	TicketFilled TicketStatus = "filled"
	TicketClosed TicketStatus = "closed"
	TicketRejected TicketStatus = "rejected"
)

type Loan struct {
	ID             string `json:"id"`
	IdempotencyKey string `json:"idempotency_key"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Principal string `json:"principal"`
	Remaining  string `json:"remaining"`
	AnnualBPS  int    `json:"annual_bps"`
	TermMonths int    `json:"term_months"`
	Payment    string    `json:"payment"`
	Collateral string    `json:"collateral"`
	ColAsset   string    `json:"collateral_asset"`
	CapBPS     int       `json:"cap_bps"`
	State      State     `json:"state"`
	Frozen     bool      `json:"frozen"`
	OpenedAt   time.Time `json:"opened_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Installment struct {
	LoanID string `json:"loan_id"`
	Seq int `json:"seq"`
	DueAt     time.Time `json:"due_at"`
	Principal string    `json:"principal"`
	Interest  string    `json:"interest"`
	Payment string `json:"payment"`
	Remaining string `json:"remaining"`
	Status InstStatus `json:"status"`
	PaidAt time.Time  `json:"paid_at,omitempty"`
}

type Ticket struct {
	ID string `json:"id"`
	LoanID string `json:"loan_id"`
	UserID string     `json:"user_id"`
	Kind   TicketKind `json:"kind"`
	Need   string     `json:"need"`
	Asset  string     `json:"asset"`
	Status TicketStatus `json:"status"`
	Note     string `json:"note,omitempty"`
	OpenedBy string `json:"opened_by"`
	ClosedBy  string    `json:"closed_by,omitempty"`
	OpenedAt  time.Time `json:"opened_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type OpenReq struct {
	UserID string `json:"user_id"`
	Asset     string `json:"asset"`
	Principal string `json:"principal"`
	AnnualBPS int    `json:"annual_bps"`
	TermMonths int    `json:"term_months"`
	Collateral string `json:"collateral"`
	ColAsset   string `json:"collateral_asset"`
	CapBPS     int    `json:"cap_bps"`
	IdempotencyKey string `json:"idempotency_key"`
}

  func (l Loan) Copy() Loan { return l }

   func (i Installment) Copy() Installment { return i }

  func (t Ticket) Copy() Ticket { return t }
