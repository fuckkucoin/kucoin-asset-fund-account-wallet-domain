package closelatecredit_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/closelatecredit"
)

   func newEng(t *testing.T) *closelatecredit.Engine {
	   t.Helper()
	   return closelatecredit.NewEngine(closelatecredit.NewMemStore(), closelatecredit.Config{}).
		  WithClock(func() time.Time { return time.Date(2021, 8, 20, 10, 0, 0, 0, time.UTC) })
   }

  func ptr(n uint64) *uint64 { return &n }

  func TestCloseRaisesEpoch(t *testing.T) {
	   ctx := context.Background()
	  eng := newEng(t)
	   ep, err := eng.EnsureOpen(ctx, "u1", "trading", "ops")
	  if err != nil {
		  t.Fatal(err)
	  }
	   if ep.N != 1 || ep.State != closelatecredit.EpochOpen {
		   t.Fatalf("ensure: %+v", ep)
	  }
	  closed, err := eng.CloseWithEpoch(ctx, closelatecredit.CloseRequest{UserID: "u1", Book: "trading", Actor: "ops"})
	  if err != nil {
		  t.Fatal(err)
	   }
	   if !closelatecredit.CloseRaisesEpoch(1, closed.N) || closed.State != closelatecredit.EpochClosed {
		   t.Fatalf("close: %+v", closed)
	  }
	   again, err := eng.CloseWithEpoch(ctx, closelatecredit.CloseRequest{UserID: "u1", Book: "trading"})
	  if err != nil {
		  t.Fatal(err)
	  }
	  if again.N != closed.N {
		   t.Fatalf("idempotent close raised epoch: %d → %d", closed.N, again.N)
	   }
  }

   func TestCreditAfterCloseDispute(t *testing.T) {
	   ctx := context.Background()
	  eng := newEng(t)
	  _, _ = eng.EnsureOpen(ctx, "u2", "trading", "ops")
	  ep, err := eng.CloseWithEpoch(ctx, closelatecredit.CloseRequest{UserID: "u2", Book: "trading"})
	   if err != nil {
		  t.Fatal(err)
	   }
	   res, err := eng.AssertObserve(ctx, "u2", "USDT", "12.50", "tx-1", ptr(ep.N))
	  if !errors.Is(err, closelatecredit.ErrDisputeHold) {
		   t.Fatalf("err=%v", err)
	  }
	   if res.Allowed || res.Decision != closelatecredit.DecisionDispute || res.Proof == nil || res.Hold == nil {
		  t.Fatalf("res=%+v", res)
	   }
	   if res.Proof.Reason != closelatecredit.ReasonClosedDispute {
		  t.Fatalf("reason=%s", res.Proof.Reason)
	  }
  }

   func TestReopenStaleRefused(t *testing.T) {
	   ctx := context.Background()
	   eng := newEng(t)
	  _, _ = eng.EnsureOpen(ctx, "u3", "trading", "ops")
	   closed, _ := eng.CloseWithEpoch(ctx, closelatecredit.CloseRequest{UserID: "u3", Book: "trading"})
	   live, err := eng.ReopenWithEpoch(ctx, closelatecredit.ReopenRequest{UserID: "u3", Book: "trading"})
	  if err != nil {
		   t.Fatal(err)
	  }
	   if !closelatecredit.ReopenRaisesEpoch(closed.N, live.N) {
		   t.Fatalf("reopen %d → %d", closed.N, live.N)
	   }
	   res, err := eng.AssertObserve(ctx, "u3", "BTC", "0.15", "tx-old", ptr(closed.N))
	   if !errors.Is(err, closelatecredit.ErrEpochStale) {
		   t.Fatalf("err=%v", err)
	  }
	   if res.Allowed || res.Proof == nil {
		   t.Fatalf("res=%+v", res)
	  }
	  ok, err := eng.AssertObserve(ctx, "u3", "BTC", "0.15", "tx-new", ptr(live.N))
	   if err != nil || !ok.Allowed {
		   t.Fatalf("live admit: %+v %v", ok, err)
	  }
  }

   func TestSourcesShareEpoch(t *testing.T) {
	  ctx := context.Background()
	   eng := newEng(t)
	  _, _ = eng.EnsureOpen(ctx, "u4", "trading", "ops")
	   ep, _ := eng.CloseWithEpoch(ctx, closelatecredit.CloseRequest{UserID: "u4", Book: "trading"})
	   sources := []struct {
		  name string
		   fn   func() (*closelatecredit.GateResult, error)
	  }{
		  {"observe", func() (*closelatecredit.GateResult, error) {
			  return eng.AssertObserve(ctx, "u4", "ETH", "3.00", "o1", ptr(ep.N))
		   }},
		  {"outbox", func() (*closelatecredit.GateResult, error) {
			  return eng.AssertOutbox(ctx, "u4", "ETH", "3.00", "x1", ptr(ep.N))
		   }},
		   {"fiat", func() (*closelatecredit.GateResult, error) {
			   return eng.AssertFiat(ctx, "u4", "ETH", "3.00", "f1", ptr(ep.N))
		   }},
	  }
	   for _, tc := range sources {
		  res, err := tc.fn()
		  if res == nil || res.Allowed || res.Proof == nil {
			  t.Fatalf("%s: %+v %v", tc.name, res, err)
		   }
		   if res.Decision != closelatecredit.DecisionDispute {
			   t.Fatalf("%s decision=%s", tc.name, res.Decision)
		   }
	   }
  }

   func TestMissingEpochFailClosed(t *testing.T) {
	   ctx := context.Background()
	  eng := newEng(t)
	  _, _ = eng.EnsureOpen(ctx, "u5", "trading", "ops")
	  res, err := eng.AssertObserve(ctx, "u5", "USDT", "1.00", "m1", nil)
	  if !errors.Is(err, closelatecredit.ErrMissingEpoch) {
		  t.Fatalf("err=%v", err)
	   }
	  if res.Allowed {
		  t.Fatal("missing epoch admitted")
	  }
   }

  func TestForceLiveDenied(t *testing.T) {
	  ctx := context.Background()
	  eng := newEng(t)
	   _, _ = eng.EnsureOpen(ctx, "u6", "trading", "ops")
	  one := uint64(1)
	   res, err := eng.CreditAssert(ctx, closelatecredit.CreditAssertRequest{
		   UserID: "u6", Source: closelatecredit.SourceObserve, Asset: "USDT", Amount: "1.00",
		   TxRef: "f1", ClaimedEpoch: &one, ForceLive: true,
	  })
	   if !errors.Is(err, closelatecredit.ErrForceDenied) {
		   t.Fatalf("err=%v res=%+v", err, res)
	   }
   }

   func TestDisputeResolveNoLive(t *testing.T) {
	  ctx := context.Background()
	  eng := newEng(t)
	   _, _ = eng.EnsureOpen(ctx, "u7", "trading", "ops")
	  ep, _ := eng.CloseWithEpoch(ctx, closelatecredit.CloseRequest{UserID: "u7", Book: "trading"})
	   res, _ := eng.AssertObserve(ctx, "u7", "USDT", "20.00", "h1", ptr(ep.N))
	  out, err := eng.DisputeResolve(ctx, closelatecredit.DisputeResolveRequest{HoldID: res.Hold.ID, Actor: "ops", Note: "park"})
	   if err != nil {
		   t.Fatal(err)
	   }
	   if out.Allowed || out.Hold.State != closelatecredit.HoldResolved {
		  t.Fatalf("resolve %+v", out)
	  }
	   if out.Proof.State != closelatecredit.LateClosed && out.Proof.State != closelatecredit.LateRejected {
		  t.Fatalf("proof state %s", out.Proof.State)
	   }
   }

  func TestIdempotentProof(t *testing.T) {
	   ctx := context.Background()
	  eng := newEng(t)
	   _, _ = eng.EnsureOpen(ctx, "u8", "trading", "ops")
	   ep, _ := eng.CloseWithEpoch(ctx, closelatecredit.CloseRequest{UserID: "u8", Book: "trading"})
	   a, _ := eng.AssertObserve(ctx, "u8", "USDT", "1.00", "idem-1", ptr(ep.N))
	   b, _ := eng.AssertObserve(ctx, "u8", "USDT", "1.00", "idem-1", ptr(ep.N))
	  if a.Proof.ID != b.Proof.ID {
		   t.Fatalf("proof not idempotent %s %s", a.Proof.ID, b.Proof.ID)
	  }
  }

   func TestScanPending(t *testing.T) {
	  ctx := context.Background()
	   eng := newEng(t)
	  _, _ = eng.EnsureOpen(ctx, "u9", "trading", "ops")
	   ep, _ := eng.CloseWithEpoch(ctx, closelatecredit.CloseRequest{UserID: "u9", Book: "trading"})
	  if err := eng.EnqueuePending(ctx, &closelatecredit.PendingCredit{
		   UserID: "u9", Source: closelatecredit.SourceOutbox, Asset: "USDT", Amount: "2.00",
		   TxRef: "pend-1", ClaimedEpoch: ptr(ep.N),
	  }); err != nil {
		  t.Fatal(err)
	  }
	  n, err := eng.ScanLateCredits(ctx, 10)
	  if err != nil || n != 1 {
		   t.Fatalf("scan n=%d err=%v", n, err)
	   }
	  rows, err := eng.ListProofs(ctx, "u9", 10)
	  if err != nil || len(rows) == 0 {
		   t.Fatalf("proofs %v %v", rows, err)
	   }
  }

  func TestExportRequiresDigest(t *testing.T) {
	   ctx := context.Background()
	  eng := newEng(t)
	   _, _ = eng.EnsureOpen(ctx, "u10", "trading", "ops")
	  ep, _ := eng.CloseWithEpoch(ctx, closelatecredit.CloseRequest{UserID: "u10", Book: "trading"})
	   res, _ := eng.AssertObserve(ctx, "u10", "USDT", "1.00", "ex-1", ptr(ep.N))
	  ex, err := eng.ExportProof(ctx, res.Proof.ID)
	   if err != nil || ex.Digest == "" {
		   t.Fatalf("export %+v %v", ex, err)
	   }
  }

  func TestWrongUser(t *testing.T) {
	   ctx := context.Background()
	   eng := newEng(t)
	   _, _ = eng.EnsureOpen(ctx, "u11", "trading", "ops")
	  one := uint64(1)
	   res, err := eng.CreditAssert(ctx, closelatecredit.CreditAssertRequest{
		   UserID: "u11", ClaimedUser: "u-other", Source: closelatecredit.SourceObserve,
		  Asset: "USDT", Amount: "1.00", TxRef: "w1", ClaimedEpoch: &one,
	  })
	  if !errors.Is(err, closelatecredit.ErrEpochMismatch) || res.Proof == nil || !res.Proof.Misroute() {
		   t.Fatalf("wrong user %+v %v", res, err)
	  }
  }

  func TestUnavailable(t *testing.T) {
	  var eng *closelatecredit.Engine
	   if _, err := eng.EnsureOpen(context.Background(), "u", "trading", ""); !errors.Is(err, closelatecredit.ErrUnavailable) {
		  t.Fatalf("nil engine: %v", err)
	   }
	   eng = closelatecredit.NewEngine(nil, closelatecredit.Config{})
	  if _, err := eng.CurrentEpoch(context.Background(), "u", "trading"); !errors.Is(err, closelatecredit.ErrUnavailable) {
		  t.Fatalf("nil store: %v", err)
	   }
  }
