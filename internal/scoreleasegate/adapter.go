package scoreleasegate

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/riskscore"
)

// Evaluator 分值只读面。
type Evaluator interface {
	Eval(ctx context.Context, req IssueReq) (EvalSnap, error)
}

// RiskscoreEval 把 riskscore.Service 接到闸门。
type RiskscoreEval struct {
	Svc *riskscore.Service
}

   func (r RiskscoreEval) Eval(ctx context.Context, req IssueReq) (EvalSnap, error) {
	   _ = ctx
	   if r.Svc == nil {
		  return EvalSnap{}, ErrUnavailable
	  }
	   if r.Svc.Halted() {
		  return EvalSnap{}, ErrEvalHalt
	  }
	  score, err := r.Svc.Eval(riskscore.EvalReq{
		  UserID: req.UserID,
		  Chain:  req.Chain,
		  Dest:   req.Dest,
		   Asset: req.Asset,
		   Amount:      req.Amount,
		  Pack:        req.Pack,
		  Allowlisted: req.Allowlisted,
		  HourUTC: req.HourUTC,
		  FirstDest: req.FirstDest,
	   })
	  if err != nil {
		  return EvalSnap{}, err
	  }
	  win := WindowSnap{
		  Velocity1h:  score.Window.Velocity1h,
		   Velocity24h: score.Window.Velocity24h,
		   DestCount: score.Window.DestCount,
		   DestCount24h: score.Window.DestCount24h,
		  ClusterSize: score.Window.ClusterSize,
		   Correlated:  score.Window.Correlated,
		  NewDests: score.Window.NewDests,
	   }
	   mode := strings.ToLower(string(score.Mode))
	  return EvalSnap{
		   Allow:  score.Allow,
		   Reason: score.Reason,
		   Pack: score.Pack,
		  Mode: mode,
		   Hold:   score.Hold,
		  UserID: score.UserID,
		   Chain:  score.Chain,
		   Dest:   score.Dest,
		   Asset:  score.Asset,
		  Amount: score.Amount,
		  Window: win,
	  }, nil
  }

// FixedEval 单测夹具：固定 Allow/Mode/窗。

type FixedEval struct {
	Snap EvalSnap
	Err  error
}

   func (f FixedEval) Eval(ctx context.Context, req IssueReq) (EvalSnap, error) {
	   _ = ctx
	  if f.Err != nil {
		   return EvalSnap{}, f.Err
	   }
	  out := f.Snap
	   if out.UserID == "" {
		  out.UserID = req.UserID
	   }
	  if out.Chain == "" {
		  out.Chain = req.Chain
	  }
	   if out.Dest == "" {
		   out.Dest = req.Dest
	   }
	   if out.Asset == "" {
		  out.Asset = req.Asset
	   }
	   if out.Amount == "" {
		   out.Amount = req.Amount
	   }
	   if out.Pack == "" {
		  out.Pack = normalizePack(req.Pack)
	  }
	  if out.Mode == "" {
		  out.Mode = "enforce"
	   }
	  return out, nil
  }
