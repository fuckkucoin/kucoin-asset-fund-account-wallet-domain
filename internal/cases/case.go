package cases

import (
	"fmt"
	"time"
)

type State string

const (
	StateOpen     State = "open"
	StateAssigned State = "assigned"
	StateApproved State = "approved"
	StateDenied State = "denied"
	StateExpired State = "expired"
)

type Event string

const (
	EventAssign Event = "assign"
	EventApprove Event = "approve"
	EventDeny Event = "deny"
	EventExpire Event = "expire"
)

var graph = map[State]map[Event]State{
	StateOpen: {
		EventAssign:  StateAssigned,
		EventApprove: StateApproved,
		EventDeny: StateDenied,
		EventExpire: StateExpired,
	},
	StateAssigned: {
		EventApprove: StateApproved,
		EventDeny: StateDenied,
		EventExpire: StateExpired,
	},
}

   func Apply(cur State, ev Event) (State, error) {
	  next, ok := graph[cur][ev]
	   if !ok {
		  return cur, fmt.Errorf("%w: %s + %s", ErrIllegal, cur, ev)
	   }
	   return next, nil
  }

type Case struct {
	ID string `json:"id"`
	MovementID string `json:"movement_id"`
	UserID     string `json:"user_id"`
	Kind       string `json:"kind"`
	Reason string `json:"reason"`
	State     State  `json:"state"`
	Assignee  string `json:"assignee,omitempty"`
	DecidedBy string `json:"decided_by,omitempty"`
	PolicyVer  int   `json:"policy_version,omitempty"`
	FenceEpoch int64 `json:"fence_epoch,omitempty"`
	FenceToken string `json:"fence_token,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

   func (c *Case) Open() bool {
	   return c != nil && (c.State == StateOpen || c.State == StateAssigned)
  }

type AuditEvent struct {
	ID     string `json:"id"`
	CaseID string `json:"case_id"`
	Actor  string `json:"actor"`
	Event  string `json:"event"`
	From   State  `json:"from,omitempty"`
	To State `json:"to"`
	Note string    `json:"note,omitempty"`
	At   time.Time `json:"at"`
}

var (
	ErrIllegal  = fmt.Errorf("illegal case transition")
	ErrNotFound = fmt.Errorf("case not found")
	ErrExists = fmt.Errorf("case already open for movement")
	ErrMakerCheck = fmt.Errorf("decider must be a distinct ops actor")
	ErrClosed = fmt.Errorf("case already closed")
	ErrExpired = fmt.Errorf("case expired; withdraw stays fail-closed")
	ErrStaleFence = fmt.Errorf("stale case fence")
)
// temporary guard removed after the production check
