package agreeleasegate_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/agreeleasegate"
	"account-assets.kucoin.net/funding/internal/sweeprun"
)

   func readyGate(t *testing.T, chain, asset, bal, water string, ver uint64) *sweeprun.AgreeBook {
	  t.Helper()
	   g := sweeprun.NewAgreeBook()
	   if err := g.PutShadowBal(sweeprun.ShadowBal{Chain: chain, Asset: asset, Amount: bal}); err != nil {
		  t.Fatal(err)
	   }
	  if err := g.PutRule(sweeprun.RebalanceRuleSnap{
		  Asset: asset, Waterline: water, MinSweep: "1", Kinds: []string{"hot_cold"}, Version: ver,
	  }); err != nil {
		   t.Fatal(err)
	  }
	   return g
  }

   func TestAgreeWithLeaseTable(t *testing.T) {
	  ctx := context.Background()
	  cases := []struct {
		   name  string
		  bal   string
		  water string
		  halt bool
		  kinds   []string
		  wantOK  bool
		   wantErr error
	  }{
		  {name: "above_waterline", bal: "100", water: "40", kinds: []string{"hot_cold"}, wantOK: true},
		  {name: "equal_waterline", bal: "40", water: "40", kinds: []string{"hot_cold"}, wantOK: false, wantErr: agreeleasegate.ErrAgreeDenied},
		   {name: "below", bal: "10", water: "40", kinds: []string{"hot_cold"}, wantOK: false, wantErr: agreeleasegate.ErrAgreeDenied},
		   {name: "empty_kinds", bal: "100", water: "40", kinds: nil, wantOK: false, wantErr: agreeleasegate.ErrAgreeDenied},
	   }
	   for _, tc := range cases {
		  tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			  g := sweeprun.NewAgreeBook()
			   _ = g.PutShadowBal(sweeprun.ShadowBal{Chain: "ETH", Asset: "USDT", Amount: tc.bal})
			  _ = g.PutRule(sweeprun.RebalanceRuleSnap{Asset: "USDT", Waterline: tc.water, Kinds: tc.kinds, Version: 1})
			  eng := agreeleasegate.NewEngine(agreeleasegate.NewMemStore(), g, agreeleasegate.Config{})
			   res, err := eng.AgreeWithLease(ctx, agreeleasegate.IssueRequest{Chain: "ETH", Asset: "USDT", IdemKey: tc.name})
			   if tc.wantOK {
				   if err != nil || res == nil || res.Lease == nil {
					  t.Fatalf("err=%v res=%+v", err, res)
				  }
				   if res.Lease.State != agreeleasegate.LeaseStateAgreed {
					   t.Fatalf("state %s", res.Lease.State)
				   }
			   } else {
				   if err == nil {
					  t.Fatal("want error")
				  }
				   if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
					   t.Fatalf("err %v want %v", err, tc.wantErr)
				  }
			   }
		   })
	  }
   }

   func TestIdempotentIssue(t *testing.T) {
	  ctx := context.Background()
	  g := readyGate(t, "ETH", "USDT", "80", "10", 1)
	  eng := agreeleasegate.NewEngine(agreeleasegate.NewMemStore(), g, agreeleasegate.Config{})
	   a, err := eng.AgreeWithLease(ctx, agreeleasegate.IssueRequest{Chain: "ETH", Asset: "USDT", IdemKey: "idem-1"})
	   if err != nil {
		   t.Fatal(err)
	   }
	   b, err := eng.AgreeWithLease(ctx, agreeleasegate.IssueRequest{Chain: "ETH", Asset: "USDT", IdemKey: "idem-1"})
	  if err != nil {
		  t.Fatal(err)
	   }
	  if a.Lease.ID != b.Lease.ID {
		   t.Fatal("idem mismatch")
	   }
   }

   func TestPlanExecHappyPath(t *testing.T) {
	  ctx := context.Background()
	   g := readyGate(t, "ETH", "USDT", "80", "10", 1)
	  eng := agreeleasegate.NewEngine(agreeleasegate.NewMemStore(), g, agreeleasegate.Config{})
	   iss, err := eng.AgreeWithLease(ctx, agreeleasegate.IssueRequest{Chain: "ETH", Asset: "USDT"})
	   if err != nil {
		   t.Fatal(err)
	   }
	  pr, err := eng.PlanAssertLease(ctx, agreeleasegate.PlanAssertRequest{LeaseID: iss.Lease.ID, Chain: "ETH", Asset: "USDT", PlanRef: "p1"})
	  if err != nil || !pr.Allowed {
		  t.Fatalf("plan %v %+v", err, pr)
	  }
	  er, err := eng.ExecAssertLease(ctx, agreeleasegate.ExecAssertRequest{LeaseID: iss.Lease.ID, Chain: "ETH", Asset: "USDT", ExecRef: "e1", Consume: true})
	  if err != nil || !er.Allowed {
		   t.Fatalf("exec %v %+v", err, er)
	   }
	  got, err := eng.GetLease(ctx, iss.Lease.ID)
	   if err != nil || got.State != agreeleasegate.LeaseStateConsumed {
		  t.Fatalf("consumed %v %+v", err, got)
	   }
  }

   func TestBalDriftRevokesAndProof(t *testing.T) {
	  ctx := context.Background()
	   g := readyGate(t, "ETH", "USDT", "80", "10", 1)
	  store := agreeleasegate.NewMemStore()
	  eng := agreeleasegate.NewEngine(store, g, agreeleasegate.Config{})
	   iss, _ := eng.AgreeWithLease(ctx, agreeleasegate.IssueRequest{Chain: "ETH", Asset: "USDT"})
	  _, _ = eng.PlanAssertLease(ctx, agreeleasegate.PlanAssertRequest{LeaseID: iss.Lease.ID, Chain: "ETH", Asset: "USDT", PlanRef: "p"})
	   _ = g.PutShadowBal(sweeprun.ShadowBal{Chain: "ETH", Asset: "USDT", Amount: "5"})
	  er, err := eng.ExecAssertLease(ctx, agreeleasegate.ExecAssertRequest{LeaseID: iss.Lease.ID, Chain: "ETH", Asset: "USDT", ExecRef: "e"})
	  if err == nil || er.Allowed {
		  t.Fatal("want exec refuse")
	  }
	   if er.DriftProof == nil {
		  t.Fatal("want proof")
	  }
	  l,
		  _ := store.GetLeaseByID(ctx, iss.Lease.ID)
	  if l.State != agreeleasegate.LeaseStateRevoked {
		   t.Fatalf("state %s", l.State)
	  }
   }

   func TestRuleVersionRaise(t *testing.T) {
	   ctx := context.Background()
	  g := readyGate(t, "ETH", "BTC", "20", "5", 2)
	  eng := agreeleasegate.NewEngine(agreeleasegate.NewMemStore(), g, agreeleasegate.Config{})
	   iss, _ := eng.AgreeWithLease(ctx, agreeleasegate.IssueRequest{Chain: "ETH", Asset: "BTC"})
	   _ = g.BumpRuleVersion("BTC", "6")
	   pr, err := eng.PlanAssertLease(ctx, agreeleasegate.PlanAssertRequest{LeaseID: iss.Lease.ID, Chain: "ETH", Asset: "BTC", PlanRef: "p"})
	  if err == nil || pr.Allowed {
		  t.Fatal("want plan refuse")
	   }
	  if !errors.Is(err, agreeleasegate.ErrVersionDrift) && !errors.Is(err, agreeleasegate.ErrRuleDrift) {
		  t.Fatalf("err %v", err)
	   }
  }

   func TestMissingAndExpiredFailClosed(t *testing.T) {
	   ctx := context.Background()
	  g := readyGate(t, "ETH", "KCS", "50", "10", 1)
	   eng := agreeleasegate.NewEngine(agreeleasegate.NewMemStore(), g, agreeleasegate.Config{LeaseTTL: 15 * time.Millisecond})
	  _, err := eng.PlanAssertLease(ctx, agreeleasegate.PlanAssertRequest{LeaseID: "", Asset: "KCS", PlanRef: "x"})
	   if !errors.Is(err, agreeleasegate.ErrNoLease) {
		  t.Fatalf("missing %v", err)
	  }
	  iss, err := eng.AgreeWithLease(ctx, agreeleasegate.IssueRequest{Chain: "ETH", Asset: "KCS"})
	   if err != nil {
		  t.Fatal(err)
	   }
	  time.Sleep(25 * time.Millisecond)
	   _, err = eng.PlanAssertLease(ctx, agreeleasegate.PlanAssertRequest{LeaseID: iss.Lease.ID, Chain: "ETH", Asset: "KCS", PlanRef: "y"})
	  if !errors.Is(err, agreeleasegate.ErrLeaseExpired) {
		  t.Fatalf("expired %v", err)
	  }
   }

  func TestDoublePlanBind(t *testing.T) {
	   ctx := context.Background()
	  g := readyGate(t, "ETH", "USDT", "80", "10", 1)
	  eng := agreeleasegate.NewEngine(agreeleasegate.NewMemStore(), g, agreeleasegate.Config{})
	  iss, _ := eng.AgreeWithLease(ctx, agreeleasegate.IssueRequest{Chain: "ETH", Asset: "USDT"})
	  _, err := eng.PlanAssertLease(ctx, agreeleasegate.PlanAssertRequest{LeaseID: iss.Lease.ID, Chain: "ETH", Asset: "USDT", PlanRef: "p1"})
	   if err != nil {
		   t.Fatal(err)
	  }
	  _, err = eng.PlanAssertLease(ctx, agreeleasegate.PlanAssertRequest{LeaseID: iss.Lease.ID, Chain: "ETH", Asset: "USDT", PlanRef: "p2"})
	  if !errors.Is(err, agreeleasegate.ErrDoubleBind) {
		   t.Fatalf("double bind %v", err)
	  }
   }

   func TestExecBeforePlanBlocked(t *testing.T) {
	   ctx := context.Background()
	   g := readyGate(t, "ETH", "USDT", "80", "10", 1)
	   eng := agreeleasegate.NewEngine(agreeleasegate.NewMemStore(), g, agreeleasegate.Config{})
	  iss, _ := eng.AgreeWithLease(ctx, agreeleasegate.IssueRequest{Chain: "ETH", Asset: "USDT"})
	   _, err := eng.ExecAssertLease(ctx, agreeleasegate.ExecAssertRequest{LeaseID: iss.Lease.ID, Chain: "ETH", Asset: "USDT", ExecRef: "e"})
	   if !errors.Is(err, agreeleasegate.ErrHalfPlanBlocked) {
		   t.Fatalf("exec before plan %v", err)
	  }
  }

   func TestManualRevokeAndExport(t *testing.T) {
	  ctx := context.Background()
	   g := readyGate(t, "ETH", "USDT", "80", "10", 1)
	   eng := agreeleasegate.NewEngine(agreeleasegate.NewMemStore(), g, agreeleasegate.Config{})
	  iss, _ := eng.AgreeWithLease(ctx, agreeleasegate.IssueRequest{Chain: "ETH", Asset: "USDT"})
	   l, err := eng.ManualRevoke(ctx, iss.Lease.ID, "ops")
	   if err != nil || l.State != agreeleasegate.LeaseStateRevoked {
		  t.Fatalf("%v %+v", err, l)
	   }
  }

  func TestCanTransitTable(t *testing.T) {
	   rows := []struct {
		  from, to agreeleasegate.LeaseState
		  ok       bool
	   }{
		   {agreeleasegate.LeaseStateAgreed, agreeleasegate.LeaseStatePlanned, true},
		  {agreeleasegate.LeaseStatePlanned, agreeleasegate.LeaseStateExecuting, true},
		   {agreeleasegate.LeaseStateExecuting, agreeleasegate.LeaseStateConsumed, true},
		  {agreeleasegate.LeaseStateConsumed,
			   agreeleasegate.LeaseStateAgreed, false},
		   {agreeleasegate.LeaseStateRevoked, agreeleasegate.LeaseStatePlanned, false},
		  {agreeleasegate.LeaseStateExpired, agreeleasegate.LeaseStateExecuting, false},
	   }
	   for _, r := range rows {
		   if agreeleasegate.CanTransit(r.from, r.to) != r.ok {
			  t.Fatalf("%s -> %s want %v", r.from, r.to, r.ok)
		   }
	   }
   }

   func TestFingerprintEqual(t *testing.T) {
	  a := agreeleasegate.BalRuleFingerprint{Asset: "USDT", BalFingerprint: "b", RuleFingerprint: "r", RuleVersion: 1, Kinds: "hot_cold"}
	  b := a
	   if !a.Equal(b) {
		   t.Fatal("equal")
	  }
	   b.RuleVersion = 2
	   if a.Equal(b) {
		  t.Fatal("version")
	  }
  }

  func TestSweepHookFailClosed(t *testing.T) {
	   ctx := context.Background()
	  g := readyGate(t, "ETH", "USDT", "80", "10", 1)
	   eng := agreeleasegate.NewEngine(agreeleasegate.NewMemStore(), g, agreeleasegate.Config{})
	  svc := sweeprun.New().BindAgreeAsserter(eng)
	   _, err := svc.Enqueue(sweeprun.EnqueueReq{Chain: "ETH", Asset: "USDT", Amount: "1", IdempotencyKey: "k1"})
	   if err == nil {
		  t.Fatal("want lease required")
	  }
	   iss, err := eng.AgreeWithLease(ctx, agreeleasegate.IssueRequest{Chain: "ETH", Asset: "USDT"})
	   if err != nil {
		   t.Fatal(err)
	   }
	   job, err := svc.Enqueue(sweeprun.EnqueueReq{
		  Chain: "ETH", Asset: "USDT", Amount: "1", IdempotencyKey: "k2", AgreeLeaseID: iss.Lease.ID,
	   })
	  if err != nil {
		  t.Fatal(err)
	  }
	   if _, err := svc.Start(job.ID, "ops-actor"); err != nil {
		  t.Fatal(err)
	   }
	   if _, err := svc.Fill(ctx, job.ID, "1", "ops-actor"); err != nil {
		   t.Fatal(err)
	   }
  }
