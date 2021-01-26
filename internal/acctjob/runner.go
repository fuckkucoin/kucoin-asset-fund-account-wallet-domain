package acctjob

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/compliance"
	"account-assets.kucoin.net/funding/internal/deposit"
	"account-assets.kucoin.net/funding/internal/ha"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/internal/ledgersvc"
	"account-assets.kucoin.net/funding/internal/reviewsla"
	"account-assets.kucoin.net/funding/internal/signpipe"
	"account-assets.kucoin.net/funding/internal/sweeppol"
	"account-assets.kucoin.net/funding/internal/withdraw"
)

type Report struct {
	Reconciled bool `json:"reconciled"`
	Expired    int  `json:"expired_cases"`
	SLABreach int `json:"sla_breaches"`
	Withdraw  int `json:"withdraw_recovered"`
	Deposit int `json:"deposit_recovered"`
	Sweeps int `json:"sweep_plans"`
	SignExp int `json:"sign_expired"`
	Leader bool   `json:"leader"`
	Err    string `json:"error,omitempty"`
	At string `json:"at"`
}

type Runner struct {
	RT *app.Runtime
	WD *withdraw.Service
	DP   *deposit.Service
	Pipe *compliance.Pipeline
	EL *ha.Elector
	SLA *reviewsla.Book
	Inv *hotinv.Book
	Sweep *sweeppol.Book
	Sign *signpipe.Pipe
	Pair *ha.Pair
}

// Tick 一次完整拍。非选主直接跳过写路径，避免双活同时解冻。
// 对账失败记到 Report.Err，本拍仍尝试过期和恢复（恢复只 Confirm，不解冻 live hash）。
  func (r *Runner) Tick(ctx context.Context) Report {
	  out := Report{At: time.Now().UTC().Format(time.RFC3339)}
	   if r == nil || r.RT == nil {
		  out.Err = "runner unavailable"
		  return out
	  }
	   if r.EL != nil {
		   if rec, err := r.EL.Campaign(ctx); err != nil || !rec.Leader {
			   out.Leader = false
			   out.Err = "not leader"
			  return out
		  }
		   out.Leader = true
	  } else {
		   out.Leader = true
	  }
	  if _, err := ledgersvc.ReconcileUsers(ctx, r.RT); err != nil {
		  out.Err = err.Error()
	   } else {
		  out.Reconciled = true
	  }
	  if r.Pair != nil {
		   if err := r.Pair.RequireWrite(); err != nil {
			   out.Leader = false
			   out.Err = err.Error()
			  return out
		   }
	  }
	  if r.SLA != nil {
		   breached := r.SLA.Scan()
		   out.SLABreach = len(breached)
		   for _, t := range breached {
			   r.SLA.Close(t.CaseID, "expired")
		   }
	  }
	  if r.Pipe != nil {
		  out.Expired = len(r.Pipe.ExpireStale())
	   }
	  if r.Sign != nil {
		  out.SignExp = len(r.Sign.ExpireDue())
	  }
	   if r.Inv != nil && r.Sweep != nil && r.RT.Store != nil {
		  rows, err := r.Inv.Observe(ctx, r.RT.Store)
		   if err == nil {
			  plans := r.Sweep.Evaluate(rows)
			  out.Sweeps = len(plans)
		   } else if out.Err == "" {
			   out.Err = err.Error()
		   }
	  }
	  if r.WD != nil {
		  n, err := r.WD.Recover(ctx, r.WD.Runs(), 50)
		  if err != nil && out.Err == "" {
			  out.Err = err.Error()
		  }
		  out.Withdraw = n
	   }
	   if r.DP != nil {
		   n,
			   err := r.DP.Recover(ctx, r.DP.Runs(), 50)
		   if err != nil && out.Err == "" {
			   out.Err = err.Error()
		   }
		  out.Deposit = n
	  }
	  if r.RT.Ledger != nil {
		  r.RT.Ledger.SweepHot(ctx, r.RT.Cfg.Transfer.HotMax)
	   }
	   return out
   }
