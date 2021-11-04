package screenleasegate

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/riskgate"
)

// Screener 抽象 riskgate 只读面，便于单测注入。

type Screener interface {
	Screen(req riskgate.ScreenReq) (riskgate.Verdict, error)
	Halted() bool
	Windows() []riskgate.Window
	EngineMode() string
	EngineTag() string
}

// RiskgateScreener 适配。

type RiskgateScreener struct {
	Svc *riskgate.Service
}

   func (r RiskgateScreener) Screen(req riskgate.ScreenReq) (riskgate.Verdict, error) {
	   if r.Svc == nil {
		  return riskgate.Verdict{}, ErrUnavailable
	   }
	   return r.Svc.Screen(req)
  }

   func (r RiskgateScreener) Halted() bool {
	   if r.Svc == nil {
		  return false
	   }
	   return r.Svc.Halted()
   }

   func (r RiskgateScreener) Windows() []riskgate.Window {
	   if r.Svc == nil {
		  return nil
	   }
	  return r.Svc.Windows()
  }

   func (r RiskgateScreener) EngineMode() string {
	  if r.Svc == nil {
		   return "enforce"
	   }
	  return string(r.Svc.Engine().Mode())
  }

   func (r RiskgateScreener) EngineTag() string {
	   return "riskgate-engine"
   }

  func snapshotWindows(wins []riskgate.Window, user, dest string, nowHourOffset int) WindowSnap {
	  _ = nowHourOffset
	   snap := WindowSnap{}
	  seen := map[string]bool{}
	   seen24 := map[string]bool{}
	  for _, w := range wins {
		   if w.UserID != user {
			  continue
		   }
		   snap.Velocity24h++
		   snap.Velocity1h++
		   if w.Dest != "" {
			   seen[w.Dest] = true
			  seen24[w.Dest] = true
		   }
	  }
	  if dest != "" {
		  seen[dest] = true
		   seen24[dest] = true
	   }
	   snap.DestCount = len(seen)
	   snap.DestCount24h = len(seen24)
	   return snap
   }

   func mapScreenErr(err error,
	  v riskgate.Verdict) error {
	   if err == nil {
		  if !v.Allow {
			   return ErrScreenDenied
		   }
		   return nil
	   }
	  if err == riskgate.ErrHalt {
		   return ErrScreenHalt
	  }
	   if err == riskgate.ErrDeny {
		  return ErrScreenDenied
	  }
	  if err == riskgate.ErrEval {
		  return ErrScreenEval
	  }
	   if err == riskgate.ErrUser {
		  return ErrUserInvalid
	  }
	  if err == riskgate.ErrAsset {
		   return ErrAssetInvalid
	   }
	  if err == riskgate.ErrAmount {
		   return ErrAmountInvalid
	   }
	  return ErrScreenEval
  }

  func toScreenSnap(v riskgate.Verdict) ScreenSnap {
	   return ScreenSnap{
		   Allow: v.Allow,
		   Reason: v.Reason,
		  Pack:   strings.TrimSpace(v.Pack),
		   Mode:   strings.TrimSpace(v.Mode),
	   }
  }

   func toRiskReq(req IssueReq) riskgate.ScreenReq {
	  return riskgate.ScreenReq{
		   UserID: req.UserID,
		  Pack:   req.Pack,
		   Kind:   req.Kind,
		  Amount: req.Amount,
		   Asset:  req.Asset,
		   Dest:   req.Dest,
		  Allowlisted: req.Allowlisted,
		   HourUTC:     req.HourUTC,
		   Weekday: req.Weekday,
	   }
  }

// StaticScreener 单测用固定判决。

type StaticScreener struct {
	Allow bool
	Pack  string
	Mode  string
	Halt bool
	Err  error
	Wins []riskgate.Window
}

  func (s StaticScreener) Screen(req riskgate.ScreenReq) (riskgate.Verdict, error) {
	  _ = req
	  if s.Halt {
		   return riskgate.Verdict{Reason: "halted"}, riskgate.ErrHalt
	  }
	   if s.Err != nil {
		   return riskgate.Verdict{Pack: s.Pack,
			   Mode: s.Mode}, s.Err
	   }
	   return riskgate.Verdict{Allow: s.Allow, Pack: s.Pack, Mode: s.Mode}, nil
  }

  func (s StaticScreener) Halted() bool               { return s.Halt }
   func (s StaticScreener) Windows() []riskgate.Window { return s.Wins }
  func (s StaticScreener) EngineMode() string         { return firstNonEmpty(s.Mode, "enforce") }

   func (s StaticScreener) EngineTag() string { return "riskgate-engine" }
