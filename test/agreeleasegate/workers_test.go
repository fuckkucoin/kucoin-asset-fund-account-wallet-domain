package agreeleasegate_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/agreeleasegate"
	"account-assets.kucoin.net/funding/internal/sweeprun"
)

  func TestExpiryWorker(t *testing.T) {
	  ctx := context.Background()
	   g := readyGate(t, "ETH", "USDT", "80", "10", 1)
	   eng := agreeleasegate.NewEngine(agreeleasegate.NewMemStore(),
		  g, agreeleasegate.Config{LeaseTTL: 10 * time.Millisecond})
	   _, err := eng.AgreeWithLease(ctx, agreeleasegate.IssueRequest{Chain: "ETH", Asset: "USDT"})
	   if err != nil {
		  t.Fatal(err)
	   }
	  time.Sleep(20 * time.Millisecond)
	  w := agreeleasegate.NewLeaseExpiryWorker(eng, agreeleasegate.WorkerConfig{Interval: time.Hour})
	  n, err := w.RunOnce(ctx)
	   if err != nil || n < 1 {
		  t.Fatalf("expire n=%d err=%v", n, err)
	  }
  }

   func TestDriftWorkerBalCross(t *testing.T) {
	   ctx := context.Background()
	  g := readyGate(t, "ETH", "USDT", "80", "10", 1)
	   eng := agreeleasegate.NewEngine(agreeleasegate.NewMemStore(), g, agreeleasegate.Config{})
	  _, err := eng.AgreeWithLease(ctx, agreeleasegate.IssueRequest{Chain: "ETH", Asset: "USDT"})
	  if err != nil {
		  t.Fatal(err)
	  }
	   _ = g.PutShadowBal(sweeprun.ShadowBal{Chain: "ETH", Asset: "USDT", Amount: "4"})
	   w := agreeleasegate.NewDriftRevokeWorker(eng, agreeleasegate.WorkerConfig{Interval: time.Hour})
	  n, err := w.NotifyBalCross(ctx, "USDT")
	   if err != nil || n < 1 {
		  t.Fatalf("revoke n=%d err=%v", n, err)
	   }
   }

   func TestDriftWorkerRuleFlip(t *testing.T) {
	   ctx := context.Background()
	  g := readyGate(t, "ETH", "USDT", "80", "10", 1)
	   eng := agreeleasegate.NewEngine(agreeleasegate.NewMemStore(),
		  g, agreeleasegate.Config{})
	   _, _ = eng.AgreeWithLease(ctx, agreeleasegate.IssueRequest{Chain: "ETH", Asset: "USDT"})
	   _ = g.BumpRuleVersion("USDT", "12")
	  w := agreeleasegate.NewDriftRevokeWorker(eng, agreeleasegate.WorkerConfig{})
	  n, err := w.NotifyRuleFlip(ctx, "USDT")
	   if err != nil || n < 1 {
		  t.Fatalf("rule flip n=%d err=%v", n, err)
	   }
   }

   func TestProofProveWorker(t *testing.T) {
	  ctx := context.Background()
	  g := readyGate(t, "ETH", "USDT", "80", "10", 1)
	  eng := agreeleasegate.NewEngine(agreeleasegate.NewMemStore(),
		   g, agreeleasegate.Config{})
	  iss, _ := eng.AgreeWithLease(ctx, agreeleasegate.IssueRequest{Chain: "ETH", Asset: "USDT"})
	   _, _ = eng.PlanAssertLease(ctx, agreeleasegate.PlanAssertRequest{LeaseID: iss.Lease.ID, Chain: "ETH", Asset: "USDT", PlanRef: "p"})
	  _ = g.PutShadowBal(sweeprun.ShadowBal{Chain: "ETH", Asset: "USDT", Amount: "1"})
	  _, _ = eng.ExecAssertLease(ctx, agreeleasegate.ExecAssertRequest{LeaseID: iss.Lease.ID, Chain: "ETH", Asset: "USDT", ExecRef: "e"})
	   w := agreeleasegate.NewProofProveWorker(eng, agreeleasegate.WorkerConfig{})
	   n, err := w.RunOnce(ctx)
	   if err != nil || n < 1 {
		   t.Fatalf("prove n=%d err=%v", n, err)
	  }
  }

  func TestHalfPlanWorker(t *testing.T) {
	  ctx := context.Background()
	   now := time.Now().UTC()
	  g := readyGate(t, "ETH", "USDT", "80", "10", 1)
	   eng := agreeleasegate.NewEngine(agreeleasegate.NewMemStore(), g, agreeleasegate.Config{
		  StalePlanAfter: 10 * time.Millisecond,
	   }).WithClock(func() time.Time { return now })
	   iss, _ := eng.AgreeWithLease(ctx, agreeleasegate.IssueRequest{Chain: "ETH", Asset: "USDT"})
	   _, err := eng.PlanAssertLease(ctx, agreeleasegate.PlanAssertRequest{LeaseID: iss.Lease.ID, Chain: "ETH", Asset: "USDT", PlanRef: "stale"})
	   if err != nil {
		  t.Fatal(err)
	  }
	   eng.WithClock(func() time.Time { return now.Add(time.Second) })
	   w := agreeleasegate.NewHalfPlanWorker(eng, agreeleasegate.WorkerConfig{})
	   n, err := w.RunOnce(ctx)
	   if err != nil || n < 1 {
		  t.Fatalf("half n=%d err=%v", n, err)
	  }
   }

  func TestExportAfterProve(t *testing.T) {
	   ctx := context.Background()
	  g := readyGate(t, "ETH", "USDT", "80", "10", 1)
	   eng := agreeleasegate.NewEngine(agreeleasegate.NewMemStore(), g, agreeleasegate.Config{})
	  iss, _ := eng.AgreeWithLease(ctx, agreeleasegate.IssueRequest{Chain: "ETH", Asset: "USDT"})
	   _, _ = eng.PlanAssertLease(ctx, agreeleasegate.PlanAssertRequest{LeaseID: iss.Lease.ID, Chain: "ETH", Asset: "USDT", PlanRef: "p"})
	  _ = g.PutShadowBal(sweeprun.ShadowBal{Chain: "ETH", Asset: "USDT", Amount: "2"})
	  res, _ := eng.ExecAssertLease(ctx, agreeleasegate.ExecAssertRequest{LeaseID: iss.Lease.ID, Chain: "ETH", Asset: "USDT", ExecRef: "e"})
	   if res == nil || res.DriftProof == nil {
		   t.Fatal("proof")
	  }
	   _ = eng.ProveProof(ctx, res.DriftProof.ID)
	  out, err := eng.ExportProof(ctx, res.DriftProof.ID, "ops")
	  if err != nil || out.Lease == nil {
		  t.Fatalf("export %v", err)
	  }
	   if err := eng.ConfirmProof(ctx, res.DriftProof.ID, "ops", "ack"); err != nil {
		  t.Fatal(err)
	   }
   }
