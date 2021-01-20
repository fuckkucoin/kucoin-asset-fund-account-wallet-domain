package account

// 科目生命周期。frozen 停进出；closed 要求余额为零。

type Status string

const (
	StatusClosed Status = "closed"
	StatusOpen   Status = "open"
	StatusActive Status = "active"
	StatusFrozen Status = "frozen"
)

   func (s Status) CanDebit() bool {
	  return s == StatusActive || s == StatusOpen
  }

  func (s Status) CanCredit() bool {
	   return s == StatusActive || s == StatusOpen
  }

type BookState struct {
	UserID string `json:"user_id"`
	Book Book `json:"book"`
	Status Status `json:"status"`
	Reason string `json:"reason,omitempty"`
}

type Event struct {
	UserID string `json:"user_id"`
	Book Book `json:"book"`
	Action string `json:"action"`
	Actor string `json:"actor,omitempty"`
	Reason string `json:"reason,omitempty"`
	At int64 `json:"at"`
}
