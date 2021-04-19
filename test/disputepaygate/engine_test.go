package disputepaygate_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/disputedesk"
	"account-assets.kucoin.net/funding/internal/disputepaygate"
)

   func newEng(clock func() time.Time) *disputepaygate.Engine {
	   eng := disputepaygate.NewEngine(
		disputedesk.New(),
		disputepaygate.NewMemPayLeaseRepo(),
		disputepaygate.NewMemClawLeaseRepo(),
		disputepaygate.NewMemProofRepo(),
		disputepaygate.NewMemAuditRepo(),
		disputepaygate.NewMemGenRepo(),
	)
	if clock != nil {
		eng = eng.WithClock(clock)
	}
	return eng
}

  func openReady(t *testing.T, desk *disputedesk.Service, idem string) disputedesk.Case {
	  t.Helper()
	   c, err := desk.Open(disputedesk.OpenReq{
		  Channel: disputedesk.ChanP2P, UserID: "u-payee-01", Asset: "USDT",
		   Amount: "12.50", Reason: "p2p unpaid release", OpenedBy: "op-open-01",
		   IdempotencyKey: idem,
	   })
	  if err != nil {
		   t.Fatalf("open: %v", err)
	   }
	   if _, err := desk.AttachPayload(c.ID, "op-open-01", "chat", "shot", []byte("evidence-body-1")); err != nil {
		   t.Fatalf("attach: %v", err)
	   }
	   if _, err := desk.MarkReview(c.ID, "op-rev-01"); err != nil {
		   t.Fatalf("review: %v", err)
	   }
	   return c
   }

  func loadGolden(t *testing.T, name string) map[string]any {
	  t.Helper()
	  p := filepath.Join("testdata", "disputepaygate", name)
	   if _, err := os.Stat(p); err != nil {
		   p = filepath.Join("..", "..", "testdata", "disputepaygate", name)
	  }
	   b, err := os.ReadFile(p)
	   if err != nil {
		  t.Fatalf("golden %s: %v", name, err)
	   }
	   var m map[string]any
	  if err := json.Unmarshal(b, &m); err != nil {
		  t.Fatalf("json %s: %v", name, err)
	  }
	  return m
  }

  func TestResolveIssuesPayLease(t *testing.T) {
	   base := time.Date(2020, 11, 18, 10, 0, 0, 0, time.UTC)
	  eng := newEng(func() time.Time { return base })
	  ctx := context.Background()
	  c := openReady(t, eng.Desk(), "idem-pay-main")
	  res, err := eng.ResolveWithPayLease(ctx, disputepaygate.ResolveReq{
		  CaseID: c.ID, Actor: "op-close-09", Decision: "resolve",
	   })
	  if err != nil {
		   t.Fatalf("resolve: %v", err)
	   }
	  if res.PayLeaseID == "" || res.PayLease == nil {
		   t.Fatal("want dispute_pay_lease_id")
	  }
	   if res.PayLease.State != disputepaygate.PayIssued {
		   t.Fatalf("state %s", res.PayLease.State)
	   }
	  if res.PayLease.Amount != "12.5" && res.PayLease.Amount != "12.50" {
		  t.Fatalf("amount %s", res.PayLease.Amount)
	  }
	   if res.PayLease.EvidenceRoot == "" {
		   t.Fatal("evidence root empty")
	   }
   }

   func TestNoLeaseCreditFails(t *testing.T) {
	   g := loadGolden(t, "no_lease_credit.json")
	  _ = g
	  eng := newEng(nil)
	  ctx := context.Background()
	   c := openReady(t, eng.Desk(), "idem-nolease")
	  _, err := eng.ResolveWithPayLease(ctx, disputepaygate.ResolveReq{
		   CaseID: c.ID, Actor: "op-close-09", Decision: "resolve",
	  })
	   if err != nil {
		   t.Fatalf("resolve: %v", err)
	   }
	   ar, aerr := eng.AssertPayLease(ctx, disputepaygate.AssertPayInput{CaseID: c.ID, Amount: "12.50", Asset: "USDT"})
	  if !errors.Is(aerr, disputepaygate.ErrNoLeaseForCredit) {
		  t.Fatalf("want no lease, got %v", aerr)
	  }
	  if ar == nil || ar.Allowed || ar.Reason != disputepaygate.OutcomeBlockedNoLease || ar.ProofID == "" {
		   t.Fatalf("assert %+v", ar)
	   }
	   b := disputepaygate.NewBinder(eng, disputepaygate.NewMemoryPoster())
	   _, berr := b.CreditWithLease(ctx, disputepaygate.LedgerPost{CaseID: c.ID, Amount: "12.50", Asset: "USDT", IdemKey: "led-1"})
	   if !errors.Is(berr, disputepaygate.ErrNoLeaseForCredit) {
		  t.Fatalf("credit: %v", berr)
	  }
   }

   func TestDoubleResolveRejected(t *testing.T) {
	  loadGolden(t, "double_resolve.json")
	  eng := newEng(nil)
	   ctx := context.Background()
	   c := openReady(t, eng.Desk(), "idem-double")
	   if _, err := eng.ResolveWithPayLease(ctx, disputepaygate.ResolveReq{CaseID: c.ID, Actor: "op-close-09", Decision: "uphold"}); err != nil {
		   t.Fatalf("first: %v", err)
	  }
	  res, err := eng.ResolveWithPayLease(ctx, disputepaygate.ResolveReq{CaseID: c.ID, Actor: "op-close-10", Decision: "uphold"})
	  if !errors.Is(err, disputepaygate.ErrDoubleResolve) {
		   t.Fatalf("second: %v", err)
	   }
	   if res == nil || res.ProofID == "" {
		  t.Fatal("want proof on double resolve")
	  }
	  rows, _ := eng.ListProofs(ctx, c.ID, 20)
	   found := false
	  for _, p := range rows {
		  if p.Outcome == disputepaygate.OutcomeBlockedDouble {
			  found = true
		   }
	   }
	   if !found {
		   t.Fatal("missing double proof")
	   }
   }

   func TestEvidenceRootDrift(t *testing.T) {
	   loadGolden(t, "evidence_drift.json")
	   eng := newEng(nil)
	  ctx := context.Background()
	  c := openReady(t, eng.Desk(), "idem-drift")
	  ev := eng.Desk().EvidenceOf(c.ID)
	   res, err := eng.ResolveWithPayLease(ctx, disputepaygate.ResolveReq{CaseID: c.ID, Actor: "op-close-09", Decision: "resolve"})
	   if err != nil {
		   t.Fatalf("resolve: %v", err)
	  }
	  oldRoot := res.PayLease.EvidenceRoot
	  if _, err := eng.Desk().OverlayEvidenceHash(c.ID, ev[0].SHA256, disputedesk.EvidenceHash([]byte("tampered-root")), "op-forensics"); err != nil {
		  t.Fatalf("overlay: %v", err)
	  }
	   newRoot, _ := eng.Desk().EvidenceRoot(c.ID)
	   if newRoot == oldRoot {
		  t.Fatal("root should drift")
	  }
	  ar, aerr := eng.AssertPayLease(ctx, disputepaygate.AssertPayInput{
		  LeaseID: res.PayLeaseID, CaseID: c.ID, Amount: "12.50", Asset: "USDT",
	  })
	   if !errors.Is(aerr, disputepaygate.ErrEvidenceDrift) {
		   t.Fatalf("want drift, got %v", aerr)
	   }
	  if ar.Reason != disputepaygate.OutcomeBlockedDrift {
		   t.Fatalf("reason %s", ar.Reason)
	  }
  }

  func TestIdemOpenAfterResolve(t *testing.T) {
	  loadGolden(t, "idem_replay.json")
	  eng := newEng(nil)
	  ctx := context.Background()
	   c := openReady(t, eng.Desk(), "desk-idem-pay-001")
	   if _, err := eng.ResolveWithPayLease(ctx, disputepaygate.ResolveReq{CaseID: c.ID, Actor: "op-close-09", Decision: "resolve"}); err != nil {
		   t.Fatal(err)
	  }
	   _, err := eng.OpenAfterResolve(ctx, disputedesk.OpenReq{
		  Channel: disputedesk.ChanP2P, UserID: "u-payee-01", Asset: "USDT",
		   Amount: "12.50", Reason: "p2p unpaid release", OpenedBy: "op-open-01",
		   IdempotencyKey: "desk-idem-pay-001",
	   })
	  if !errors.Is(err, disputepaygate.ErrIdemReplayPay) {
		   t.Fatalf("open after: %v", err)
	  }
  }

   func TestFourEyeStillHolds(t *testing.T) {
	  eng := newEng(nil)
	   ctx := context.Background()
	   c := openReady(t, eng.Desk(), "idem-4e")
	   if _, err := eng.ResolveWithPayLease(ctx, disputepaygate.ResolveReq{CaseID: c.ID, Actor: "op-open-01", Decision: "resolve"}); !errors.Is(err, disputepaygate.ErrFourEye) {
		   t.Fatalf("opener: %v", err)
	  }
	   if _, err := eng.ResolveWithPayLease(ctx, disputepaygate.ResolveReq{CaseID: c.ID, Actor: "u-payee-01", Decision: "resolve"}); !errors.Is(err, disputepaygate.ErrFourEye) {
		   t.Fatalf("self: %v", err)
	   }
   }

  func TestRejectIssuesClawAndBind(t *testing.T) {
	  eng := newEng(nil)
	   ctx := context.Background()
	  c := openReady(t, eng.Desk(), "idem-claw")
	   res, err := eng.ResolveWithPayLease(ctx, disputepaygate.ResolveReq{
		   CaseID: c.ID, Actor: "op-close-09", Decision: "deny", ReleaseRef: "occ-rel-1",
	  })
	  if err != nil {
		  t.Fatal(err)
	   }
	  if res.ClawLease == nil || res.ClawLease.State != disputepaygate.ClawIssued {
		  t.Fatalf("claw %+v", res.ClawLease)
	   }
	   if res.ClawLease.ReleaseRef != "occ-rel-1" {
		   t.Fatalf("release ref %s", res.ClawLease.ReleaseRef)
	  }
	  b := disputepaygate.NewBinder(eng, disputepaygate.NewMemoryPoster())
	   got, err := b.DebitWithClaw(ctx, disputepaygate.LedgerPost{
		  CaseID: c.ID, UserID: c.UserID, Asset: "USDT", Amount: "12.50",
		   LeaseID: res.PayLeaseID, IdemKey: "deb-1",
	  })
	   if err != nil || !got.Consumed {
		   t.Fatalf("debit %v %+v", err, got)
	   }
  }

   func TestCreditConsumeIdempotentLedger(t *testing.T) {
	   eng := newEng(nil)
	  ctx := context.Background()
	   c := openReady(t, eng.Desk(), "idem-credit")
	  res, err := eng.ResolveWithPayLease(ctx, disputepaygate.ResolveReq{CaseID: c.ID, Actor: "op-close-09", Decision: "close"})
	   if err != nil {
		   t.Fatal(err)
	   }
	  b := disputepaygate.NewBinder(eng, disputepaygate.NewMemoryPoster())
	  got, err := b.CreditWithLease(ctx, disputepaygate.LedgerPost{
		  CaseID: c.ID, UserID: c.UserID, Asset: "USDT", Amount: "12.50",
		  LeaseID: res.PayLeaseID, IdemKey: "cr-1",
	   })
	  if err != nil || !got.Consumed {
		  t.Fatalf("credit %v %+v", err, got)
	  }
	  _, err = b.CreditWithLease(ctx, disputepaygate.LedgerPost{
		  CaseID: c.ID, UserID: c.UserID, Asset: "USDT", Amount: "12.50",
		  LeaseID: res.PayLeaseID, IdemKey: "cr-2",
	  })
	   if !errors.Is(err, disputepaygate.ErrLeaseConsumed) {
		   t.Fatalf("second credit: %v", err)
	   }
  }

   func TestExpireWorker(t *testing.T) {
	  now := time.Date(2021, 4, 12, 8, 0, 0, 0, time.UTC)
	   cur := now
	  eng := newEng(func() time.Time { return cur })
	   ctx := context.Background()
	  c := openReady(t, eng.Desk(), "idem-exp")
	   res, err := eng.ResolveWithPayLease(ctx, disputepaygate.ResolveReq{
		  CaseID: c.ID, Actor: "op-close-09", Decision: "resolve", TTL: time.Minute,
	   })
	  if err != nil {
		   t.Fatal(err)
	   }
	  cur = now.Add(2 * time.Minute)
	  w := disputepaygate.NewExpiryWorker(eng, disputepaygate.WorkerConfig{BatchSize: 20})
	  n, err := w.TickOnce(ctx)
	   if err != nil || n == 0 {
		  t.Fatalf("expire n=%d err=%v", n, err)
	  }
	  _, aerr := eng.AssertPayLease(ctx, disputepaygate.AssertPayInput{LeaseID: res.PayLeaseID, CaseID: c.ID})
	  if !errors.Is(aerr, disputepaygate.ErrLeaseExpired) {
		   t.Fatalf("expired assert: %v", aerr)
	   }
   }

   func TestUnconsumedAndDoublePump(t *testing.T) {
	   eng := newEng(nil)
	  ctx := context.Background()
	  c := openReady(t, eng.Desk(), "idem-pump")
	  if _, err := eng.ResolveWithPayLease(ctx, disputepaygate.ResolveReq{CaseID: c.ID, Actor: "op-close-09", Decision: "resolve"}); err != nil {
		  t.Fatal(err)
	   }
	   uw := disputepaygate.NewUnconsumedWorker(eng, disputepaygate.WorkerConfig{BatchSize: 10})
	  n, err := uw.TickOnce(ctx)
	   if err != nil || n == 0 {
		   t.Fatalf("unconsumed n=%d err=%v", n, err)
	   }
	   dw := disputepaygate.NewDoubleResolveWorker(eng, disputepaygate.WorkerConfig{BatchSize: 10})
	   if _, err := dw.TickOnce(ctx); err != nil {
		  t.Fatal(err)
	   }
   }

  func TestRevokeThenAssert(t *testing.T) {
	   eng := newEng(nil)
	   ctx := context.Background()
	  c := openReady(t, eng.Desk(), "idem-rev")
	   res, _ := eng.ResolveWithPayLease(ctx, disputepaygate.ResolveReq{CaseID: c.ID, Actor: "op-close-09", Decision: "resolve"})
	  if err := eng.RevokeLease(ctx, res.PayLeaseID, "op-admin", "halt"); err != nil {
		   t.Fatal(err)
	  }
	  _,
		   err := eng.AssertPayLease(ctx, disputepaygate.AssertPayInput{LeaseID: res.PayLeaseID, CaseID: c.ID})
	  if !errors.Is(err, disputepaygate.ErrLeaseRevoked) {
		  t.Fatalf("revoked: %v", err)
	  }
  }

   func TestFSMMatrixGolden(t *testing.T) {
	   g := loadGolden(t, "fsm_transitions.json")
	  allowed, _ := g["allowed"].([]any)
	   for _, row := range allowed {
		   pair, _ := row.([]any)
		  from := disputepaygate.LeaseState(pair[0].(string))
		   to := disputepaygate.LeaseState(pair[1].(string))
		   if !disputepaygate.CanTransition(from, to) {
			  t.Fatalf("want allow %s->%s", from, to)
		   }
	  }
	   forb, _ := g["forbidden"].([]any)
	   for _, row := range forb {
		   pair, _ := row.([]any)
		  from := disputepaygate.LeaseState(pair[0].(string))
		  to := disputepaygate.LeaseState(pair[1].(string))
		   if disputepaygate.CanTransition(from, to) {
			   t.Fatalf("want forbid %s->%s", from, to)
		   }
	   }
  }

   func TestConcurrentResolveOneLease(t *testing.T) {
	   eng := newEng(nil)
	  ctx := context.Background()
	   c := openReady(t, eng.Desk(), "idem-race")
	   var ok, fail int32
	   var wg sync.WaitGroup
	   for i := 0; i < 8; i++ {
		   wg.Add(1)
		  go func() {
			   defer wg.Done()
			   _, err := eng.ResolveWithPayLease(ctx, disputepaygate.ResolveReq{CaseID: c.ID, Actor: "op-close-09", Decision: "resolve"})
			   if err == nil {
				   atomic.AddInt32(&ok, 1)
			   } else {
				  atomic.AddInt32(&fail, 1)
			   }
		  }()
	  }
	   wg.Wait()
	  if ok != 1 {
		   t.Fatalf("want 1 success, ok=%d fail=%d", ok, fail)
	   }
   }

  func TestAmountAssetMismatch(t *testing.T) {
	   eng := newEng(nil)
	  ctx := context.Background()
	   c := openReady(t, eng.Desk(), "idem-mm")
	   res, _ := eng.ResolveWithPayLease(ctx, disputepaygate.ResolveReq{CaseID: c.ID, Actor: "op-close-09", Decision: "resolve"})
	  if _, err := eng.AssertPayLease(ctx, disputepaygate.AssertPayInput{LeaseID: res.PayLeaseID, CaseID: c.ID, Amount: "99", Asset: "USDT"}); !errors.Is(err, disputepaygate.ErrAmountMismatch) {
		  t.Fatalf("amount: %v", err)
	   }
	  if _, err := eng.AssertPayLease(ctx, disputepaygate.AssertPayInput{LeaseID: res.PayLeaseID, CaseID: c.ID, Amount: "12.50", Asset: "BTC"}); !errors.Is(err, disputepaygate.ErrAssetMismatch) {
		   t.Fatalf("asset: %v", err)
	  }
  }

   func TestSnapshotAndExport(t *testing.T) {
	   eng := newEng(nil)
	  ctx := context.Background()
	  c := openReady(t, eng.Desk(), "idem-snap")
	  res, _ := eng.ResolveWithPayLease(ctx, disputepaygate.ResolveReq{CaseID: c.ID, Actor: "op-close-09", Decision: "resolve"})
	  snap := eng.Snapshot(ctx)
	   if snap.PayIssued < 1 {
		   t.Fatalf("snap %+v", snap)
	   }
	  if disputepaygate.ReadLeaseID(res.PayLease) != res.PayLeaseID {
		  t.Fatal("read id")
	   }
	  p, err := eng.ExportProof(ctx, res.ProofID)
	   if err != nil || p.CaseID != c.ID || p.PayGen == 0 {
		  t.Fatalf("proof %v %+v", err, p)
	  }
  }

   func TestMustHavePayLease(t *testing.T) {
	  if err := disputepaygate.MustHavePayLease(""); !errors.Is(err, disputepaygate.ErrNoLeaseForCredit) {
		   t.Fatalf("%v", err)
	   }
	  if err := disputepaygate.MustHavePayLease("x"); err != nil {
		  t.Fatal(err)
	   }
   }

  func TestTableResolveDecisions(t *testing.T) {
	   cases := []struct {
		  dec  string
		   kind disputepaygate.LeaseKind
	  }{
		   {"resolve", disputepaygate.KindPay},
		  {"uphold", disputepaygate.KindPay},
		   {"close", disputepaygate.KindPay},
		  {"reject", disputepaygate.KindClaw},
		   {"deny", disputepaygate.KindClaw},
	   }
	   for _, tc := range cases {
		  t.Run(tc.dec, func(t *testing.T) {
			  eng := newEng(nil)
			   c := openReady(t, eng.Desk(), "idem-"+tc.dec)
			   res, err := eng.ResolveWithPayLease(context.Background(), disputepaygate.ResolveReq{CaseID: c.ID, Actor: "op-close-09", Decision: tc.dec})
			   if err != nil {
				   t.Fatal(err)
			  }
			   if res.Kind != tc.kind {
				  t.Fatalf("kind %s", res.Kind)
			   }
		  })
	  }
  }
