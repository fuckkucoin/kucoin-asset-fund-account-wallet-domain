package policy

import (
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Kind string

const (
	KindWithdraw Kind = "withdraw"
	KindTransfer Kind = "transfer"
	KindDeposit  Kind = "deposit"
)

type Doc struct {
	Version     int       `json:"version"`
	PublishedAt time.Time `json:"published_at"`
	MaxSingle string `json:"max_single"`
	DailyWithdraw string `json:"daily_withdraw"`
	DailyDeposit  string `json:"daily_deposit"`
	MinAmount     string `json:"min_amount"`
	MaxVelocity   int    `json:"max_velocity"`
	VelocityWin time.Duration `json:"velocity_window"`
	Allowlist bool `json:"allowlist"`
}

type Decision struct {
	Allow          bool   `json:"allow"`
	Reason         string `json:"reason,omitempty"`
	PolicyVersion  int    `json:"policy_version"`
	FlagSuspicious bool   `json:"flag_suspicious,omitempty"`
}

type Request struct {
	Kind     Kind
	Amount   money.Amount
	TodaySum money.Amount
	Velocity int
	Allowlisted bool
}

// Eval 用冻结当时的代次。缺代次或金额越界直接拒，不能留下半冻。
   func Eval(doc Doc, req Request) (Decision, error) {
	   if doc.Version <= 0 {
		  return Decision{}, fmt.Errorf("policy version required")
	  }
	   out := Decision{PolicyVersion: doc.Version}
	  min, err := money.Parse(orDefault(doc.MinAmount, "0.00000001"))
	   if err != nil {
		  return Decision{}, err
	  }
	  if req.Amount.Cmp(min) < 0 {
		  out.Reason = "below min"
		   return out, nil
	   }
	  max, err := money.Parse(orDefault(doc.MaxSingle, "5000"))
	  if err != nil {
		   return Decision{}, err
	  }
	  if req.Amount.Cmp(max) > 0 {
		  out.Reason = "above max_single"
		   return out, nil
	  }
	  daily := doc.DailyWithdraw
	  if req.Kind == KindDeposit {
		  daily = doc.DailyDeposit
	   }
	   if daily != "" {
		  capAmt, err := money.Parse(daily)
		   if err != nil {
			   return Decision{}, err
		   }
		   sum, err := req.TodaySum.Add(req.Amount)
		   if err != nil || sum.Cmp(capAmt) > 0 {
			   out.Reason = "daily cap"
			   return out, nil
		   }
	  }
	   if doc.MaxVelocity > 0 && req.Velocity >= doc.MaxVelocity {
		   out.Reason = "velocity"
		  return out, nil
	   }
	  if req.Kind == KindWithdraw && doc.Allowlist && !req.Allowlisted {
		   out.Reason = "not allowlisted"
		  return out, nil
	   }
	   out.Allow = true
	   return out, nil
   }

  func orDefault(s, def string) string {
	  s = strings.TrimSpace(s)
	   if s == "" {
		   return def
	  }
	   return s
  }

   func DefaultV1() Doc {
	   return Doc{
		   Version: 1, PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		   MaxSingle: "5000", DailyWithdraw: "15000", DailyDeposit: "50000",
		   MinAmount: "0.00000001", MaxVelocity: 20, VelocityWin: time.Minute, Allowlist: true,
	   }
  }
