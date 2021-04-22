package egressleasegate_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/egressleasegate"
	"account-assets.kucoin.net/funding/internal/floatdesk"
)

   func testEng(t *testing.T, cfg egressleasegate.Config) (*egressleasegate.Engine, *floatdesk.Service, *egressleasegate.MemStore) {
	  t.Helper()
	  desk := floatdesk.New()
	  st := egressleasegate.NewMemStore()
	   return egressleasegate.NewEngine(st, desk, cfg), desk, st
   }

   func TestFSMTransitions(t *testing.T) {
	   cases := []struct {
		   from, to egressleasegate.LeaseState
		   ok bool
	  }{
		   {egressleasegate.LeaseStateIssued, egressleasegate.LeaseStateBound, true},
		  {egressleasegate.LeaseStateIssued, egressleasegate.LeaseStateRevoked, true},
		  {egressleasegate.LeaseStateBound,
			   egressleasegate.LeaseStateConsumed, true},
		   {egressleasegate.LeaseStateConsumed, egressleasegate.LeaseStateIssued, false},
		  {egressleasegate.LeaseStateRevoked,
			  egressleasegate.LeaseStateBound, false},
		  {egressleasegate.LeaseStateExpired, egressleasegate.LeaseStateConsumed, false},
	   }
	   for _, tc := range cases {
		  if egressleasegate.CanTransit(tc.from, tc.to) != tc.ok {
			  t.Fatalf("%s -> %s want %v", tc.from, tc.to, tc.ok)
		   }
	   }
	   if !egressleasegate.LeaseStateConsumed.Closed() || egressleasegate.LeaseStateIssued.Closed() {
		  t.Fatal("closed flags")
	   }
   }

  func TestCheckEgressAllowIssuesLease(t *testing.T) {
	   eng, _, _ := testEng(t, egressleasegate.Config{})
	  res, err := eng.CheckEgressWithLease(context.Background(), egressleasegate.IssueRequest{
		  Chain: "ETH", Asset: "USDT", HotAvail: "80000", IdemKey: "k1",
	  })
	   if err != nil || !res.Allowed || res.Lease == nil || res.Lease.ID == "" {
		  t.Fatalf("issue: %+v %v", res, err)
	  }
	   if res.Lease.FlagsFingerprint == "" || res.Lease.TankDigest == "" {
		   t.Fatal("missing fingerprints")
	   }
	   if res.Lease.State != egressleasegate.LeaseStateIssued {
		  t.Fatalf("state %s", res.Lease.State)
	  }
	   again, err := eng.CheckEgressWithLease(context.Background(), egressleasegate.IssueRequest{
		   Chain: "ETH", Asset: "USDT", HotAvail: "80000", IdemKey: "k1",
	  })
	  if err != nil || again.Lease.ID != res.Lease.ID {
		  t.Fatalf("idem: %+v %v", again, err)
	  }
  }

   func TestHaltDeniesIssue(t *testing.T) {
	   eng, desk, _ := testEng(t, egressleasegate.Config{})
	   if err := desk.Halt().HaltChain("ETH"); err != nil {
		   t.Fatal(err)
	  }
	  res, err := eng.CheckEgressWithLease(context.Background(), egressleasegate.IssueRequest{
		   Chain: "ETH", Asset: "USDT", HotAvail: "80000",
	   })
	  if !errors.Is(err, egressleasegate.ErrCheckDenied) || res.Allowed {
		  t.Fatalf("want deny: %+v %v", res, err)
	   }
  }

  func TestWarnPolicy(t *testing.T) {
	   eng, _, _ := testEng(t, egressleasegate.Config{AllowWarnLease: false})
	   // ETH USDT min 20000 target 80000
	  res, err := eng.CheckEgressWithLease(context.Background(), egressleasegate.IssueRequest{
		  Chain: "ETH", Asset: "USDT", HotAvail: "30000",
	  })
	   if !errors.Is(err, egressleasegate.ErrWarnNotAllowed) {
		  t.Fatalf("want warn deny: %v %+v", err, res)
	  }
	   eng2,
		  _, _ := testEng(t, egressleasegate.Config{AllowWarnLease: true})
	  res2, err := eng2.CheckEgressWithLease(context.Background(), egressleasegate.IssueRequest{
		  Chain: "ETH", Asset: "USDT", HotAvail: "30000",
	   })
	   if err != nil || !res2.Allowed {
		   t.Fatalf("warn allow: %v %+v", err, res2)
	   }
  }

  func TestAcceptHaltChainRevokesBroadcast(t *testing.T) {
	  eng, desk, _ := testEng(t, egressleasegate.Config{})
	  ctx := context.Background()
	  iss, err := eng.CheckEgressWithLease(ctx, egressleasegate.IssueRequest{
		   Chain: "ETH", Asset: "USDT", HotAvail: "90000",
	   })
	  if err != nil {
		  t.Fatal(err)
	   }
	   if err := desk.Halt().HaltChain("ETH"); err != nil {
		  t.Fatal(err)
	   }
	   out, err := eng.BroadcastAssert(ctx, egressleasegate.BroadcastRequest{
		   LeaseID: iss.Lease.ID, Chain: "ETH", Asset: "USDT", BroadcastRef: "b1",
	  })
	   if out == nil || out.Allowed {
		  t.Fatalf("must deny: %+v %v", out, err)
	  }
	  if out.DriftProof == nil {
		  t.Fatal("want EgressDriftProof")
	   }
	  got, _ := eng.GetLease(ctx, iss.Lease.ID)
	  if got.State == egressleasegate.LeaseStateConsumed {
		   t.Fatal("must not consume after halt")
	   }
  }

   func TestLevelDropRevokes(t *testing.T) {
	   eng,
		  desk, _ := testEng(t, egressleasegate.Config{})
	  ctx := context.Background()
	  if err := desk.Tanks().Put(floatdesk.Tank{Chain: "ETH", Native: "ETH", Avail: "10", Min: "2", Target: "8"}); err != nil {
		  t.Fatal(err)
	   }
	   iss, err := eng.CheckEgressWithLease(ctx, egressleasegate.IssueRequest{
		  Chain: "ETH", Asset: "ETH", FromTank: true,
	  })
	  if err != nil || !iss.Allowed {
		  t.Fatalf("issue: %v %+v", err, iss)
	   }
	  if err := desk.Tanks().Put(floatdesk.Tank{Chain: "ETH", Native: "ETH", Avail: "1", Min: "2", Target: "8"}); err != nil {
		   t.Fatal(err)
	   }
	  out, err := eng.BroadcastAssert(ctx, egressleasegate.BroadcastRequest{
		  LeaseID: iss.Lease.ID, Chain: "ETH", Asset: "ETH", BroadcastRef: "b2", FromTank: true,
	   })
	  if out == nil || out.Allowed || out.DriftProof == nil {
		  t.Fatalf("level halt: %+v %v", out, err)
	   }
   }

  func TestMissingLeaseFailClosed(t *testing.T) {
	  eng, _, _ := testEng(t, egressleasegate.Config{})
	  out, err := eng.BroadcastAssert(context.Background(), egressleasegate.BroadcastRequest{
		   LeaseID: "", Chain: "ETH", Asset: "USDT", BroadcastRef: "x",
	   })
	  if !errors.Is(err, egressleasegate.ErrNoLease) {
		  t.Fatalf("want ErrNoLease %v", err)
	   }
	   if out == nil || out.DriftProof == nil || out.DriftProof.DriftType != egressleasegate.DriftMissingLease {
		  t.Fatalf("want missing proof %+v", out)
	  }
   }

  func TestNormalConsumeOnce(t *testing.T) {
	  eng, _, _ := testEng(t, egressleasegate.Config{})
	   ctx := context.Background()
	   iss, err := eng.CheckEgressWithLease(ctx, egressleasegate.IssueRequest{
		   Chain: "BTC", Asset: "BTC", HotAvail: "8",
	  })
	   if err != nil {
		   t.Fatal(err)
	   }
	   ok, err := eng.BroadcastAssert(ctx, egressleasegate.BroadcastRequest{
		   LeaseID: iss.Lease.ID, Chain: "BTC", Asset: "BTC", BroadcastRef: "ok",
	   })
	  if err != nil || !ok.Allowed {
		   t.Fatalf("consume: %v %+v", err, ok)
	   }
	  again, err := eng.BroadcastAssert(ctx, egressleasegate.BroadcastRequest{
		  LeaseID: iss.Lease.ID, Chain: "BTC", Asset: "BTC", BroadcastRef: "ok2",
	   })
	   if !errors.Is(err, egressleasegate.ErrLeaseConsumed) || (again != nil && again.Allowed) {
		   t.Fatalf("reuse: %v %+v", err, again)
	   }
  }

   func TestExpireAndScan(t *testing.T) {
	  eng, _, _ := testEng(t, egressleasegate.Config{LeaseTTL: time.Millisecond})
	  ctx := context.Background()
	   iss, err := eng.CheckEgressWithLease(ctx, egressleasegate.IssueRequest{
		   Chain: "SOL", Asset: "SOL", HotAvail: "200",
	   })
	  if err != nil {
		  t.Fatal(err)
	   }
	  time.Sleep(3 * time.Millisecond)
	   n, err := eng.ExpireDue(ctx)
	  if err != nil || n < 1 {
		   t.Fatalf("expire %d %v", n, err)
	  }
	  _, err = eng.BroadcastAssert(ctx, egressleasegate.BroadcastRequest{
		  LeaseID: iss.Lease.ID, Chain: "SOL", Asset: "SOL", BroadcastRef: "e",
	  })
	   if !errors.Is(err, egressleasegate.ErrLeaseExpired) && !errors.Is(err, egressleasegate.ErrLeaseRevoked) {
		   // 过期后 Broadcast 可能先 Expire 再证明
		  if err == nil {
			   t.Fatal("expired must fail")
		   }
	   }
  }

   func TestRefillRefuse(t *testing.T) {
	   eng, desk, _ := testEng(t, egressleasegate.Config{RefuseNewLeaseWhenRefill: true})
	   desk.RefillObserve().SetActive("ETH", "USDT", true)
	   _, err := eng.CheckEgressWithLease(context.Background(), egressleasegate.IssueRequest{
		   Chain: "ETH", Asset: "USDT", HotAvail: "90000",
	   })
	   if !errors.Is(err, egressleasegate.ErrRefillActive) {
		   t.Fatalf("want refill refuse %v", err)
	   }
  }

  func TestRevokeOnHaltBatch(t *testing.T) {
	  eng,
		   _, _ := testEng(t, egressleasegate.Config{})
	  ctx := context.Background()
	  _, err := eng.CheckEgressWithLease(ctx, egressleasegate.IssueRequest{
		  Chain: "ETH", Asset: "USDT", HotAvail: "90000", IdemKey: "a",
	   })
	  if err != nil {
		   t.Fatal(err)
	  }
	   n, err := eng.RevokeOnHalt(ctx, "chain", "ETH", "", "halt_chain")
	   if err != nil || n < 1 {
		   t.Fatalf("revoke %d %v", n, err)
	  }
   }

  func TestScopeMismatch(t *testing.T) {
	  eng, _, _ := testEng(t, egressleasegate.Config{})
	   ctx := context.Background()
	  iss, _ := eng.CheckEgressWithLease(ctx, egressleasegate.IssueRequest{
		  Chain: "ETH", Asset: "USDT", HotAvail: "90000",
	  })
	   _, err := eng.BroadcastAssert(ctx, egressleasegate.BroadcastRequest{
		   LeaseID: iss.Lease.ID, Chain: "BTC", Asset: "BTC", BroadcastRef: "z",
	  })
	  if !errors.Is(err, egressleasegate.ErrScopeMismatch) {
		   t.Fatalf("scope %v", err)
	   }
  }

   func TestProofLifecycle(t *testing.T) {
	   eng, desk, _ := testEng(t, egressleasegate.Config{})
	  ctx := context.Background()
	   iss, _ := eng.CheckEgressWithLease(ctx, egressleasegate.IssueRequest{
		  Chain: "ETH", Asset: "USDT", HotAvail: "90000",
	  })
	  desk.Halt().HaltAll()
	  out, _ := eng.BroadcastAssert(ctx, egressleasegate.BroadcastRequest{
		  LeaseID: iss.Lease.ID, Chain: "ETH", Asset: "USDT", BroadcastRef: "p",
	  })
	   if out == nil || out.DriftProof == nil {
		  t.Fatal("proof")
	  }
	  if err := eng.ProveProof(ctx, out.DriftProof.ID); err != nil {
		  t.Fatal(err)
	   }
	  if err := eng.ConfirmProof(ctx, out.DriftProof.ID, "ops", "ack"); err != nil {
		   t.Fatal(err)
	   }
	   exp, err := eng.ExportProof(ctx, out.DriftProof.ID, "ops")
	  if err != nil || exp.Proof.ID != out.DriftProof.ID {
		   t.Fatalf("export %v %+v", err, exp)
	  }
  }

   func TestCatalogLevelDropViaHotAvail(t *testing.T) {
	   eng, _, _ := testEng(t, egressleasegate.Config{})
	   ctx := context.Background()
	   iss, err := eng.CheckEgressWithLease(ctx, egressleasegate.IssueRequest{
		  Chain: "ETH", Asset: "USDT", HotAvail: "90000",
	   })
	  if err != nil {
		   t.Fatal(err)
	   }
	   out, err := eng.BroadcastAssert(ctx, egressleasegate.BroadcastRequest{
		   LeaseID: iss.Lease.ID, Chain: "ETH", Asset: "USDT", BroadcastRef: "drop", HotAvail: "1000",
	   })
	   if out == nil || out.Allowed {
		  t.Fatalf("hot drop must fail %+v %v", out, err)
	  }
  }

   func TestDoesNotTouchPlanRefill(t *testing.T) {
	   desk := floatdesk.New()
	   before, err := floatdesk.PlanRefill(floatdesk.Tank{Chain: "TRON", Native: "TRX", Avail: "80", Min: "200", Target: "800"})
	  if err != nil {
		  t.Fatal(err)
	  }
	  eng := egressleasegate.NewEngine(egressleasegate.NewMemStore(), desk, egressleasegate.Config{})
	  _, _ = eng.CheckEgressWithLease(context.Background(), egressleasegate.IssueRequest{
		   Chain: "ETH", Asset: "USDT", HotAvail: "90000",
	   })
	  after, err := floatdesk.PlanRefill(floatdesk.Tank{Chain: "TRON", Native: "TRX", Avail: "80", Min: "200", Target: "800"})
	  if err != nil {
		   t.Fatal(err)
	  }
	  if before.Amount != after.Amount || before.Reason != after.Reason {
		  t.Fatal("PlanRefill formula changed")
	   }
   }
