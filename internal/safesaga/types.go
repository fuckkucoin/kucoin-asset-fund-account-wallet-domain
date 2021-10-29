package safesaga

import "time"

type Step string

const (
	StepOpen   Step = "open"
	StepFreeze Step = "freeze"
	StepMove Step = "move"
	StepSign       Step = "sign"
	StepCommit     Step = "commit"
	StepCompensate Step = "compensate"
	StepAborted    Step = "aborted"
	StepDone Step = "done"
)

type Run struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Idem   string `json:"idempotency_key"`
	Step Step `json:"step"`
	Epoch int64  `json:"epoch"`
	Token string `json:"token"`
	Note string    `json:"note,omitempty"`
	At   time.Time `json:"at"`
}
