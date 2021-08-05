package keycer

import "time"

type SlotState string

const (
	SlotEmpty  SlotState = "empty"
	SlotLoaded SlotState = "loaded"
	SlotLocked SlotState = "locked"
	SlotRetired SlotState = "retired"
)

type TicketStatus string

const (
	TicketOpen TicketStatus = "open"
	TicketApproved TicketStatus = "approved"
	TicketDone TicketStatus = "done"
	TicketDenied TicketStatus = "denied"
)

type Slot struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	State SlotState `json:"state"`
	Fingerprint string `json:"fingerprint,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Ticket struct {
	ID       string       `json:"id"`
	SlotID   string       `json:"slot_id"`
	Kind     string       `json:"kind"`
	Status   TicketStatus `json:"status"`
	OpenedBy string       `json:"opened_by"`
	ClosedBy string `json:"closed_by,omitempty"`
	Note string `json:"note,omitempty"`
	ExternalRef string `json:"external_ref,omitempty"`
	OpenedAt time.Time `json:"opened_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Rotation struct {
	ID   string `json:"id"`
	From string `json:"from"`
	To string `json:"to"`
	Actor string    `json:"actor"`
	Note  string    `json:"note,omitempty"`
	At    time.Time `json:"at"`
}

   func (s Slot) Copy() Slot         { return s }
  func (t Ticket) Copy() Ticket     { return t }
  func (r Rotation) Copy() Rotation { return r }
