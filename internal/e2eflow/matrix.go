package e2eflow

import (
	"context"
	"fmt"
	"account-assets.kucoin.net/funding/internal/app"
)

type Matrix struct {
	Halt      HaltReport     `json:"halt"`
	Sanctions SanctionReport `json:"sanctions"`
	Drift     DriftReport    `json:"drift"`
	DLQ       DLQReport      `json:"dlq"`
}

// RunMatrix 停浮、制裁、漂移冻、DLQ 回放四条。任一条失败不得当全绿。
  func RunMatrix(ctx context.Context, rt *app.Runtime) (Matrix, error) {
	  if rt == nil {
		   return Matrix{}, fmt.Errorf("e2e runtime unavailable")
	   }
	  var out Matrix
	  var err error
	   out.Halt,
		  err = HaltMidWithdraw(ctx, rt)
	   if err != nil {
		   return out, fmt.Errorf("halt: %w", err)
	  }
	  out.Sanctions,
		   err = SanctionsHit(ctx, rt)
	   if err != nil {
		   return out, fmt.Errorf("sanctions: %w", err)
	  }
	  out.DLQ, err = WebhookDLQReplay(ctx)
	  if err != nil {
		   return out, fmt.Errorf("dlq: %w", err)
	  }
	  // 漂移冻放最后：会冻种子用户，后续出金都挡。
	  out.Drift, err = DriftFreeze(ctx, rt)
	   if err != nil {
		   return out, fmt.Errorf("drift: %w", err)
	  }
	  return out, nil
   }
