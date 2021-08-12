package leddiff

import "time"

type Position struct {
	UserID string `json:"user_id"`
	Book  string `json:"book"`
	Asset string `json:"asset"`
	Available string `json:"available"`
	Frozen    string `json:"frozen"`
}

type Snapshot struct {
	ID string `json:"id"`
	Label string `json:"label,omitempty"`
	Positions []Position `json:"positions"`
	TakenAt   time.Time  `json:"taken_at"`
}

type Line struct {
	UserID      string `json:"user_id"`
	Book        string `json:"book"`
	Asset       string `json:"asset"`
	BeforeAvail string `json:"before_avail"`
	AfterAvail string `json:"after_avail"`
	BeforeFrozen string `json:"before_frozen"`
	AfterFrozen string `json:"after_frozen"`
	DeltaAvail  string `json:"delta_avail"`
	DeltaFrozen string `json:"delta_frozen"`
}

type Report struct {
	ID     string `json:"id"`
	LeftID string `json:"left_id"`
	RightID  string `json:"right_id"`
	Lines    []Line `json:"lines"`
	Positive string `json:"positive"`
	Negative string `json:"negative"`
	Balanced bool `json:"balanced"`
	Disclaimer string `json:"disclaimer"`
	At time.Time `json:"at"`
}

const disclaimer = "engineering book diff; not a public attestation"

   func (p Position) Copy() Position { return p }

  func (s Snapshot) Copy() Snapshot {
	  s.Positions = append([]Position(nil), s.Positions...)
	  return s
   }
   func (l Line) Copy() Line { return l }

  func (r Report) Copy() Report {
	  r.Lines = append([]Line(nil), r.Lines...)
	  return r
   }
