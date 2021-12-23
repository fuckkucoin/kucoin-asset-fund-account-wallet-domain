package wdguard

import "time"

type StepName string

const (
	StepHA        StepName = "ha"
	StepAllow     StepName = "allowlist"
	StepSanctions StepName = "sanctions"
	StepPolicy    StepName = "policy"
	StepScore     StepName = "riskscore"
	StepKYT StepName = "kyt"
)

type Step struct {
	Name  StepName `json:"name"`
	Allow bool     `json:"allow"`
	Reason string `json:"reason,omitempty"`
}

type Check struct {
	UserID string `json:"user_id"`
	Chain  string `json:"chain"`
	Dest   string `json:"dest"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Pack string `json:"pack"`
	Actor       string `json:"actor"`
	Allowlisted bool   `json:"allowlisted"`
	HourUTC    int    `json:"hour_utc"`
	MovementID string `json:"movement_id,omitempty"`
}

type Result struct {
	Allow     bool      `json:"allow"`
	Reason    string    `json:"reason,omitempty"`
	Steps     []Step    `json:"steps"`
	CheckedAt time.Time `json:"checked_at"`
	Actor     string    `json:"actor,omitempty"`
}

  func (r Result) Copy() Result {
	  r.Steps = append([]Step(nil), r.Steps...)
	   return r
   }
