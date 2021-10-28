package riskscore

import "time"

type Mode string

const (
	ModeEnforce Mode = "enforce"
	ModeShadow  Mode = "shadow"
)

type Event struct {
	ID string `json:"id"`
	UserID string `json:"user_id"`
	Chain  string `json:"chain"`
	Dest   string `json:"dest"`
	Asset  string `json:"asset"`
	Amount string    `json:"amount"`
	At     time.Time `json:"at"`
}

type Window struct {
	Velocity1h  int `json:"velocity_1h"`
	Velocity24h int `json:"velocity_24h"`
	DestCount   int `json:"dest_count"`
	DestCount24h int `json:"dest_count_24h"`
	ClusterSize  int `json:"cluster_size"`
	Correlated int `json:"correlated"`
	NewDests int `json:"new_dests_today"`
}

type Score struct {
	UserID string `json:"user_id"`
	Chain  string `json:"chain"`
	Dest string `json:"dest"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Window Window `json:"window"`
	Allow  bool   `json:"allow"`
	Reason string `json:"reason,omitempty"`
	Mode   Mode   `json:"mode"`
	Pack   string `json:"pack,omitempty"`
	Hold   bool   `json:"hold_review,omitempty"`
}

type Report struct {
	Mode   Mode `json:"mode"`
	Events int  `json:"events"`
	Allowed int `json:"allowed"`
	Blocked  int     `json:"blocked"`
	Shadowed int     `json:"shadowed"`
	Hits     []Score `json:"hits,omitempty"`
}

type EvalReq struct {
	UserID string `json:"user_id"`
	Chain string `json:"chain"`
	Dest  string `json:"dest"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	Pack string `json:"pack"`
	Allowlisted bool `json:"allowlisted"`
	HourUTC     int  `json:"hour_utc"`
	FirstDest   bool `json:"first_dest"`
}

  func ParseMode(s string) Mode {
	   if s == string(ModeShadow) {
		   return ModeShadow
	   }
	   return ModeEnforce
  }
