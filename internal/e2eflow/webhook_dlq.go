package e2eflow

import (
	"context"
	"fmt"
	"account-assets.kucoin.net/funding/internal/webhook"
)

type DLQReport struct {
	Dead int `json:"dead"`
	Replayed bool `json:"replayed"`
}

// WebhookDLQReplay 先投失败进 DLQ，再换可投 sink 回放。成功才摘掉。

  func WebhookDLQReplay(ctx context.Context) (DLQReport, error) {
	  n := 0
	  mem := webhook.MemSink()
	   sink := func(ctx context.Context, ep webhook.Endpoint, env webhook.Envelope) error {
		  n++
		   if n <= 3 {
			  return webhook.FailSink()(ctx, ep, env)
		   }
		  return mem(ctx, ep, env)
	   }
	  d := webhook.New(sink)
	  if err := d.Registry().Put(webhook.Endpoint{
		   ID: "e2e-dlq", URL: "https://hooks.kucoin.net/e2e", Secret: "e2e-secret",
	  }); err != nil {
		  return DLQReport{}, err
	  }
	  rows, err := d.Dispatch(ctx, webhook.KindWithdrawFailed, `{"id":"wd-e2e"}`, "e2e-nonce-1")
	  if err != nil || len(rows) == 0 || rows[0].OK || d.Dead().Len() != 1 {
		  return DLQReport{}, fmt.Errorf("dispatch %v dead=%d", err, d.Dead().Len())
	  }
	   del, err := d.Replay(ctx, 0)
	  if err != nil || !del.OK {
		   return DLQReport{}, fmt.Errorf("replay %v %+v", err, del)
	   }
	   if d.Dead().Len() != 0 {
		  return DLQReport{}, fmt.Errorf("dlq still %d", d.Dead().Len())
	  }
	   return DLQReport{Dead: 0, Replayed: true}, nil
   }
