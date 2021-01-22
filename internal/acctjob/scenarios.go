package acctjob

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/cases"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/internal/reviewsla"
	"account-assets.kucoin.net/funding/internal/signpipe"
	"account-assets.kucoin.net/funding/internal/sweeppol"
)

// RunSLA 单独扫审查超时。超时只关票，不得当放行。
  func (r *Runner) RunSLA() int {
	   if r == nil || r.SLA == nil {
		  return 0
	  }
	   breached := r.SLA.Scan()
	   for _, t := range breached {
		   r.SLA.Close(t.CaseID, "expired")
	  }
	  return len(breached)
  }

   func (r *Runner) RunExpire() int {
	  if r == nil || r.Pipe == nil {
		  return 0
	  }
	  return len(r.Pipe.ExpireStale())
   }

   func (r *Runner) RunSignExpire() int {
	  if r == nil || r.Sign == nil {
		   return 0
	   }
	  return len(r.Sign.ExpireDue())
   }

   func (r *Runner) RunSweep(ctx context.Context) (int, error) {
	  if r == nil || r.Inv == nil || r.Sweep == nil || r.RT == nil || r.RT.Store == nil {
		   return 0, nil
	  }
	  rows, err := r.Inv.Observe(ctx, r.RT.Store)
	   if err != nil {
		  return 0, err
	  }
	   return len(r.Sweep.Evaluate(rows)),
		  nil
  }

   func (r *Runner) RunRecover(ctx context.Context) (wd, dp int, err error) {
	  if r == nil {
		  return 0, 0, nil
	  }
	   if r.WD != nil {
		  wd, err = r.WD.Recover(ctx, r.WD.Runs(), 50)
		  if err != nil {
			  return wd, 0, err
		  }
	   }
	  if r.DP != nil {
		  dp, err = r.DP.Recover(ctx, r.DP.Runs(), 50)
	   }
	   return wd, dp, err
  }

  func TrackOpenCase(sla *reviewsla.Book, c cases.Case) reviewsla.Ticket {
	  if sla == nil {
		   return reviewsla.Ticket{}
	  }
	   return sla.Track(c)
   }

   func DefaultSweep(hotMax map[string]string) (*hotinv.Book, *sweeppol.Book) {
	  return hotinv.FromHotMax(hotMax), sweeppol.FromHotMax(hotMax)
   }

   func DefaultSign(ttl time.Duration) *signpipe.Pipe {
	  return signpipe.New(ttl)
   }
