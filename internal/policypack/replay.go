package policypack

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/policy"
)

type ReplayEvent struct {
	Kind string `json:"kind"`
	Amount string `json:"amount"`
	TodaySum string `json:"today_sum"`
	Velocity1h int `json:"velocity_1h"`
	Velocity24h int  `json:"velocity_24h"`
	FirstDest   bool `json:"first_dest"`
	HourUTC int `json:"hour_utc"`
	Allowlisted bool `json:"allowlisted"`
	Asset string `json:"asset"`
	Weekday      string `json:"weekday"`
	DestCount    int    `json:"dest_count"`
	DestCount24h int    `json:"dest_count_24h"`
	ClusterSize  int    `json:"cluster_size"`
	Correlated   int    `json:"correlated"`
	NewDestsToday int    `json:"new_dests_today"`
	WantAllow     bool   `json:"want_allow"`
	WantReason    string `json:"want_reason"`
}

type ReplayFile struct {
	Pack   string        `json:"pack"`
	Events []ReplayEvent `json:"events"`
}

type ReplayRow struct {
	Index int
	Want  bool
	Got bool
	Reason string
	Err    string
	OK     bool
}

   func LoadReplay(path string) (ReplayFile, error) {
	   raw, err := os.ReadFile(path)
	   if err != nil {
		   return ReplayFile{}, err
	  }
	   var f ReplayFile
	  if err := json.Unmarshal(raw, &f); err != nil {
		   return ReplayFile{}, err
	  }
	   if f.Pack == "" || len(f.Events) == 0 {
		   return ReplayFile{}, fmt.Errorf("replay pack and events required")
	  }
	  return f, nil
   }

// Replay 用历史夹具回放。金额必须过 money.Parse，科学计数拒。
  func (e *Engine) Replay(file ReplayFile) ([]ReplayRow, error) {
	   if e == nil {
		  return nil, fmt.Errorf("policy engine unavailable")
	  }
	   out := make([]ReplayRow, 0, len(file.Events))
	   for i, ev := range file.Events {
		   row := ReplayRow{Index: i, Want: ev.WantAllow}
		   amt, err := money.Parse(ev.Amount)
		   if err != nil {
			   row.Err = err.Error()
			  out = append(out, row)
			   continue
		  }
		   today,
			  err := money.ParseBook(orZeroAmt(ev.TodaySum))
		   if err != nil {
			   row.Err = err.Error()
			  out = append(out, row)
			   continue
		   }
		  in := Input{
			  Kind: policy.Kind(ev.Kind), Amount: amt, TodaySum: today,
			  Velocity1h: ev.Velocity1h, Velocity24h: ev.Velocity24h,
			  FirstDest: ev.FirstDest, HourUTC: ev.HourUTC,
			   Allowlisted: ev.Allowlisted, Asset: ev.Asset, CheckDay: ev.Weekday != "",
			  Weekday: parseWeekday(ev.Weekday), DestCount: ev.DestCount, DestCount24h: ev.DestCount24h,
			  ClusterSize: ev.ClusterSize, Correlated: ev.Correlated, NewDestsToday: ev.NewDestsToday,
		  }
		  d, err := e.Decide(file.Pack, in)
		   if err != nil {
			  row.Err = err.Error()
			  out = append(out, row)
			   continue
		   }
		  row.Got = d.Allow
		  row.Reason = d.Reason
		  row.OK = d.Allow == ev.WantAllow
		  if ev.WantReason != "" && d.Reason != ev.WantReason && !(e.mode == ModeShadow && d.Reason == "shadow:"+ev.WantReason) {
			  row.OK = false
		  }
		  out = append(out, row)
	  }
	  return out, nil
   }

  func orZeroAmt(s string) string {
	   if s == "" {
		   return "0"
	  }
	   return s
  }

   func parseWeekday(s string) time.Weekday {
	  switch s {
	  case "Saturday":
		  return time.Saturday
	   case "Sunday":
		  return time.Sunday
	   case "Monday":
		  return time.Monday
	   default:
		  return time.Thursday
	  }
   }
