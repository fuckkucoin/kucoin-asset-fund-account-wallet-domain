package invcheck

import "time"

type Side struct {
	Asset string `json:"asset"`
	Hot   string `json:"hot"`
	Cold      string `json:"cold"`
	HotFrozen string `json:"hot_frozen,omitempty"`
	Source    string `json:"source"`
}

type Line struct {
	Asset string `json:"asset"`
	LedgerHot  string `json:"ledger_hot"`
	LedgerCold string `json:"ledger_cold"`
	CustHot  string `json:"custody_hot"`
	CustCold string `json:"custody_cold"`
	InvHot string `json:"hotinv_hot"`
	InvCold string `json:"hotinv_cold"`
	Matched bool   `json:"matched"`
	Reason  string `json:"reason,omitempty"`
}

type Report struct {
	ID string `json:"id"`
	Actor string `json:"actor,omitempty"`
	Lines     []Line `json:"lines"`
	Balanced  bool   `json:"balanced"`
	PlanCount int    `json:"plan_count"`
	Disclaimer string    `json:"disclaimer"`
	At         time.Time `json:"at"`
}

const disclaimer = "engineering inventory cross-check; not a custody certificate"

   func (r Report) Copy() Report {
	   r.Lines = append([]Line(nil), r.Lines...)
	   return r
   }

   func (l Line) Copy() Line { return l }
