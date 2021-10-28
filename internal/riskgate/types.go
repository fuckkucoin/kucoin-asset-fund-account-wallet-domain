package riskgate

import "time"

type Window struct {
	UserID string `json:"user_id"`
	Dest  string    `json:"dest,omitempty"`
	Asset string    `json:"asset"`
	At    time.Time `json:"at"`
	Amount string `json:"amount"`
}

type Cluster struct {
	Root string `json:"root"`
	Nodes []string `json:"nodes"`
}

type Verdict struct {
	Allow bool `json:"allow"`
	Reason string `json:"reason,omitempty"`
	Pack string `json:"pack"`
	Mode string `json:"mode"`
	HoldReview bool `json:"hold_review,omitempty"`
	ShadowHit bool `json:"shadow_hit,omitempty"`
}

type ScreenReq struct {
	UserID string `json:"user_id"`
	Pack   string `json:"pack"`
	Kind   string `json:"kind"`
	Amount string `json:"amount"`
	Asset string `json:"asset"`
	Dest  string `json:"dest,omitempty"`
	Allowlisted bool `json:"allowlisted"`
	HourUTC     int  `json:"hour_utc"`
	Weekday string `json:"weekday,omitempty"`
}
