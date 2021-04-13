package closelatecredit_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/closelatecredit"
)

   func TestWorkersRunOnce(t *testing.T) {
	   eng := newEng(t)
	   ctx := context.Background()
	   _, _ = eng.EnsureOpen(ctx, "uw", "trading", "ops")
	   ep, _ := eng.CloseWithEpoch(ctx, closelatecredit.CloseRequest{UserID: "uw", Book: "trading"})
	  _ = eng.EnqueuePending(ctx, &closelatecredit.PendingCredit{
		   UserID: "uw", Source: closelatecredit.SourceFiatCallback, Asset: "USDT", Amount: "4.00",
		  TxRef: "w-1", ClaimedEpoch: ptr(ep.N),
	   })
	   cfg := closelatecredit.WorkerConfig{Interval: time.Hour, BatchSize: 10}
	  scan := closelatecredit.NewLateScanWorker(eng,
		  cfg)
	  n, err := scan.RunOnce(ctx)
	  if err != nil || n != 1 {
		   t.Fatalf("scan %d %v", n, err)
	   }
	   hold := closelatecredit.NewHoldPumpWorker(eng, cfg)
	  _, err = hold.RunOnce(ctx)
	   if err != nil {
		   t.Fatal(err)
	   }
	  exp := closelatecredit.NewProofExportWorker(eng, cfg)
	  if _, err := exp.RunOnce(ctx); err != nil {
		  t.Fatal(err)
	   }
  }

   func TestPumpHolds(t *testing.T) {
	   eng := newEng(t)
	   ctx := context.Background()
	   _, _ = eng.EnsureOpen(ctx, "uh", "trading", "ops")
	  ep, _ := eng.CloseWithEpoch(ctx, closelatecredit.CloseRequest{UserID: "uh", Book: "trading"})
	   res, _ := eng.AssertObserve(ctx, "uh", "USDT", "1.00", "ph-1", ptr(ep.N))
	   if res.Hold == nil {
		  t.Fatal("hold")
	   }
	  n, err := eng.PumpDisputeHolds(ctx, 10)
	  if err != nil {
		   t.Fatal(err)
	  }
	  _ = n
  }
