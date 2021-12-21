package warroom

import "time"

type TicketStatus string

const (
	TicketOpen TicketStatus = "open"
	TicketApproved TicketStatus = "approved"
	TicketApplied  TicketStatus = "applied"
	TicketDenied TicketStatus = "denied"
)

type Scope string

const (
	ScopeUser Scope = "user"
	ScopeGlobal Scope = "global"
)

type Switch struct {
	FrozenAll bool   `json:"frozen_all"`
	Actor     string `json:"actor,omitempty"`
	Reason string `json:"reason,omitempty"`
	At time.Time `json:"at,omitempty"`
}

type Ticket struct {
	ID string `json:"id"`
	UserID string       `json:"user_id,omitempty"`
	Scope  Scope        `json:"scope"`
	Status TicketStatus `json:"status"`
	Reason string `json:"reason"`
	OpenedBy string `json:"opened_by"`
	ClosedBy string `json:"closed_by,omitempty"`
	OpenedAt  time.Time `json:"opened_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Event struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Actor string `json:"actor"`
	UserID string `json:"user_id,omitempty"`
	Detail string    `json:"detail,omitempty"`
	At     time.Time `json:"at"`
}

type FreezeReq struct {
	UserID string `json:"user_id,omitempty"`
	Actor string `json:"actor"`
	Reason string `json:"reason"`
}

type TicketReq struct {
	UserID string `json:"user_id,omitempty"`
	Scope  string `json:"scope"`
	Actor  string `json:"actor"`
	Reason string `json:"reason"`
}

  func (t Ticket) Copy() Ticket { return t }

   func (e Event) Copy() Event   { return e }
   func (s Switch) Copy() Switch { return s }

  func (s TicketStatus) Terminal() bool {
	  return s == TicketApplied || s == TicketDenied
   }
