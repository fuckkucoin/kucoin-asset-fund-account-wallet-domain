package acctjob

import (
	"context"
	"account-assets.kucoin.net/funding/internal/outboxpump"
	"account-assets.kucoin.net/funding/internal/persist"
)

   func (r *Runner) RunOutbox(ctx context.Context, pump *outboxpump.Runner, h persist.Handler) outboxpump.Report {
	  if pump == nil {
		   return outboxpump.Report{Err: "pump unavailable"}
	  }
	   return pump.Tick(ctx, h)
   }
