package egressleasegate_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/egressleasegate"
	"account-assets.kucoin.net/funding/internal/floatdesk"
)

   func TestExpiryWorkerRunOnce(t *testing.T) {
	  desk := floatdesk.New()
	   st := egressleasegate.NewMemStore()
	   eng := egressleasegate.NewEngine(st, desk, egressleasegate.Config{LeaseTTL: time.Millisecond})
	  _, err := eng.CheckEgressWithLease(context.Background(), egressleasegate.IssueRequest{
		   Chain: "ETH", Asset: "USDT", HotAvail: "90000",
	  })
	  if err != nil {
		  t.Fatal(err)
	   }
	   time.Sleep(3 * time.Millisecond)
	   w := egressleasegate.NewLeaseExpiryWorker(eng, egressleasegate.WorkerConfig{Interval: time.Hour})
	  w.RunOnce(context.Background())
	   rows, _ := st.ListActiveLeases(context.Background(), 10)
	   if len(rows) != 0 {
		  t.Fatalf("still active %d", len(rows))
	   }
  }

   func TestLevelScanWorker(t *testing.T) {
	   desk := floatdesk.New()
	  _ = desk.Tanks().Put(floatdesk.Tank{Chain: "ETH", Native: "ETH", Avail: "10", Min: "2", Target: "8"})
	  st := egressleasegate.NewMemStore()
	   eng := egressleasegate.NewEngine(st,
		  desk, egressleasegate.Config{})
	   ctx := context.Background()
	  iss, err := eng.CheckEgressWithLease(ctx, egressleasegate.IssueRequest{Chain: "ETH", Asset: "ETH", FromTank: true})
	   if err != nil {
		   t.Fatal(err)
	  }
	  _ = desk.Tanks().Put(floatdesk.Tank{Chain: "ETH", Native: "ETH", Avail: "0.5", Min: "2", Target: "8"})
	  w := egressleasegate.NewLevelScanWorker(eng, egressleasegate.WorkerConfig{})
	   w.RunOnce(ctx)
	  got, _ := eng.GetLease(ctx, iss.Lease.ID)
	  if got.State != egressleasegate.LeaseStateRevoked {
		   t.Fatalf("state %s", got.State)
	   }
  }

  func TestHaltFlipNotify(t *testing.T) {
	   desk := floatdesk.New()
	   st := egressleasegate.NewMemStore()
	   eng := egressleasegate.NewEngine(st, desk, egressleasegate.Config{})
	   ctx := context.Background()
	   _, err := eng.CheckEgressWithLease(ctx, egressleasegate.IssueRequest{
		   Chain: "ETH", Asset: "USDT", HotAvail: "90000",
	   })
	  if err != nil {
		   t.Fatal(err)
	  }
	  _ = desk.Halt().HaltChain("ETH")
	   w := egressleasegate.NewHaltFlipWorker(eng,
		  egressleasegate.WorkerConfig{})
	  n, err := w.NotifyHaltChain(ctx, "ETH")
	   if err != nil || n < 1 {
		   t.Fatalf("notify %d %v", n, err)
	  }
   }

  func TestProveWorker(t *testing.T) {
	   desk := floatdesk.New()
	  st := egressleasegate.NewMemStore()
	  eng := egressleasegate.NewEngine(st, desk, egressleasegate.Config{})
	  ctx := context.Background()
	   iss, _ := eng.CheckEgressWithLease(ctx, egressleasegate.IssueRequest{
		   Chain: "ETH", Asset: "USDT", HotAvail: "90000",
	  })
	   desk.Halt().HaltAll()
	  _, _ = eng.BroadcastAssert(ctx, egressleasegate.BroadcastRequest{
		   LeaseID: iss.Lease.ID, Chain: "ETH", Asset: "USDT", BroadcastRef: "w",
	  })
	  pw := egressleasegate.NewProofProveWorker(eng, egressleasegate.WorkerConfig{})
	   pw.RunOnce(ctx)
	  open, _ := st.ListProofsByState(ctx, egressleasegate.ProofStateOpen, 10)
	   if len(open) != 0 {
		   t.Fatalf("still open %d", len(open))
	   }
   }

  func TestWorkerStartStop(t *testing.T) {
	  desk := floatdesk.New()
	  eng := egressleasegate.NewEngine(egressleasegate.NewMemStore(), desk, egressleasegate.Config{})
	  cfg := egressleasegate.WorkerConfig{Interval: time.Hour}
	  ctx, cancel := context.WithCancel(context.Background())
	  defer cancel()
	  ew := egressleasegate.NewLeaseExpiryWorker(eng, cfg)
	   hw := egressleasegate.NewHaltFlipWorker(eng, cfg)
	  lw := egressleasegate.NewLevelScanWorker(eng, cfg)
	   pw := egressleasegate.NewProofProveWorker(eng,
		   cfg)
	   ew.Start(ctx)
	   hw.Start(ctx)
	  lw.Start(ctx)
	  pw.Start(ctx)
	  ew.Stop()
	   hw.Stop()
	   lw.Stop()
	   pw.Stop()
  }
