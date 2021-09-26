package limitgate

import (
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/policy"
	"account-assets.kucoin.net/funding/internal/usage"
)

type Req struct {
	Kind   policy.Kind
	UserID string
	Asset  string
	Amount string
	Dest string
	Allowlisted bool
}

type Out struct {
	Allow bool
	Reason string
	PolicyVer int
	Today     string
	Velocity  int
}

  func Check(reg *policy.Registry, use *usage.Store, req Req) (Out, error) {
	   if reg == nil {
		  return Out{}, fmt.Errorf("policy registry required")
	  }
	   doc, err := reg.Current()
	   if err != nil {
		   return Out{}, err
	   }
	   amt, err := money.Parse(req.Amount)
	   if err != nil {
		   return Out{}, err
	   }
	   uk := usage.KindTransfer
	  switch req.Kind {
	   case policy.KindWithdraw:
		  uk = usage.KindWithdraw
	   case policy.KindDeposit:
		   uk = usage.KindDeposit
	   }
	   today := money.Zero()
	  vel := 0
	   win := doc.VelocityWin
	  if win <= 0 {
		   win = time.Minute
	   }
	  if use != nil {
		   today = use.TodaySum(uk, req.UserID, req.Asset)
		  vel = use.Velocity(uk, req.UserID, req.Asset, win)
	  }
	  dec, err := policy.Eval(doc, policy.Request{
		   Kind: req.Kind, Amount: amt, TodaySum: today, Velocity: vel, Allowlisted: req.Allowlisted,
	   })
	  if err != nil {
		  return Out{PolicyVer: doc.Version, Today: today.String(), Velocity: vel}, err
	  }
	   return Out{
		   Allow: dec.Allow, Reason: dec.Reason, PolicyVer: dec.PolicyVersion,
		   Today: today.String(), Velocity: vel,
	   }, nil
   }
