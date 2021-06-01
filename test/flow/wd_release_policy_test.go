package flow

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/approvestore"
	"account-assets.kucoin.net/funding/internal/custody"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/wdpath"
	"account-assets.kucoin.net/funding/internal/wdrelease"
	"account-assets.kucoin.net/funding/internal/withdraw"
	"github.com/shopspring/decimal"
)

const relDest = "0x1111111111111111111111111111111111111111"

  func TestWdReleaseChainEdges(t *testing.T) {
	   if err := wdrelease.ValidateBuiltins(); err != nil {
		  t.Fatal(err)
	  }
	   for _, e := range wdrelease.DenyChainInventory() {
		  if wdrelease.AllowChain(e[0], e[1]) {
			  t.Fatalf("chain allow %s → %s", e[0], e[1])
		   }
	  }
	   for _, e := range wdrelease.DenySegmentInventory() {
		   if wdrelease.AllowSegment(e[0], e[1]) {
			  t.Fatalf("segment allow %s → %s", e[0], e[1])
		   }
	   }
	   if wdrelease.AllowChain(wdrelease.StatePending, wdrelease.StatePartial) != true {
		  t.Fatal("pending partial")
	   }
	  if wdrelease.AllowSegment(wdrelease.SegPlanned, wdrelease.SegFrozen) {
		   t.Fatal("planned frozen")
	  }
  }

  func TestWdReleaseFixtures(t *testing.T) {
	  dir := filepath.Join("..", "..", "docs", "fixtures", "wd-release")
	   ents, err := os.ReadDir(dir)
	   if err != nil {
		  t.Fatal(err)
	   }
	  n := 0
	  for _, ent := range ents {
		  if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".json") {
			  continue
		   }
		   n++
		  t.Run(ent.Name(), func(t *testing.T) {
			   fx, err := wdrelease.LoadFixture(filepath.Join(dir, ent.Name()))
			   if err != nil {
				  t.Fatal(err)
			   }
			   got,
				   err := wdrelease.Replay(context.Background(), fx)
			  if err != nil {
				  t.Fatal(err)
			   }
			   if got.ID == "" {
				   t.Fatal("empty ticket")
			  }
		  })
	  }
	  if n < 6 {
		   t.Fatalf("fixtures %d", n)
	  }
  }

   func TestWdReleaseSamePersonAndLiveHash(t *testing.T) {
	   ctx := context.Background()
	  d := wdrelease.New()
	  d.SetNow(time.Date(2021, 8, 2, 0, 0, 0, 0, time.UTC))
	  if err := d.Publish("rel-2021.04"); err != nil {
		   t.Fatal(err)
	   }
	  in := wdrelease.Input{
		   UserID: "u-priv", Asset: "USDT", Amount: "50000", Chain: "ETH", Dest: relDest,
		   DestClass: "external", Risk: 8, Applicant: "maker-priv", Idem: "idem-priv",
	   }
	  tk, err := d.Open(ctx, in)
	  if err != nil {
		  t.Fatal(err)
	  }
	  if _, err := d.Submit(ctx, tk.ID, "maker-priv"); err != nil {
		  t.Fatal(err)
	   }
	   if _, err := d.Advance(ctx, tk.ID, "maker-priv", "reviewer", true); err == nil || wdrelease.CodeOf(err) != wdrelease.CodeApplicant {
		   t.Fatalf("applicant %v", err)
	  }
	   if d.Privilege(wdrelease.CodeApplicant) != 1 {
		   t.Fatalf("privilege applicant %d", d.Privilege(wdrelease.CodeApplicant))
	   }
	  if _, err := d.Advance(ctx, tk.ID, "rev-a", "reviewer", true); err != nil {
		   t.Fatal(err)
	  }
	   got, err := d.Advance(ctx, tk.ID, "rev-a", "senior", true)
	  if err == nil || wdrelease.CodeOf(err) != wdrelease.CodeSame || got.State != wdrelease.StateRejected {
		  t.Fatalf("same %s %v", got.State, err)
	   }
	  if d.Privilege(wdrelease.CodeSame) != 1 {
		  t.Fatalf("privilege same %d", d.Privilege(wdrelease.CodeSame))
	   }
	  if wdrelease.FrozenAmount(got).Sign() != 0 {
		  t.Fatalf("frozen %s", got.Frozen)
	  }
	   rows, err := d.Audits(tk.ID)
	  if err != nil {
		   t.Fatal(err)
	   }
	  if err := wdrelease.VerifyAudit(rows); err != nil {
		  t.Fatal(err)
	  }
	  if err := wdrelease.RefuseLiveUnfreeze("0xabc"); err == nil || wdrelease.CodeOf(err) != wdrelease.CodeLive {
		   t.Fatal(err)
	   }
	   if err := wdrelease.RefuseLiveUnfreeze(""); err == nil {
		   t.Fatal("empty hash")
	   }
   }

   func TestWdReleaseRejectKeepsFrozenSegment(t *testing.T) {
	  ctx := context.Background()
	   d := wdrelease.New()
	   d.SetNow(time.Date(2021, 8, 3, 0, 0, 0, 0, time.UTC))
	   if err := d.Publish("rel-2021.04"); err != nil {
		  t.Fatal(err)
	  }
	   in := wdrelease.Input{
		  UserID: "u-keep", Asset: "USDT", Amount: "50000", Chain: "ETH",
		   Dest:      "0x7777777777777777777777777777777777777777",
		  DestClass: "external", Risk: 3, Applicant: "maker-keep", Idem: "idem-keep",
	   }
	  tk, err := d.Open(ctx, in)
	   if err != nil {
		  t.Fatal(err)
	  }
	   if _, err := d.Submit(ctx, tk.ID, in.Applicant); err != nil {
		  t.Fatal(err)
	  }
	   if _, err := d.Advance(ctx, tk.ID, "rev-keep", "reviewer", true); err != nil {
		  t.Fatal(err)
	  }
	  ask := withdraw.ReleaseAsk{
		   UserID: in.UserID, Asset: in.Asset, Amount: "15000", Chain: in.Chain, Dest: in.Dest,
		  Idem: "idem-keep-frz", TicketID: tk.ID,
	  }
	   g, err := d.BeforeFreeze(ctx, ask)
	   if err != nil {
		   t.Fatal(err)
	  }
	  if err := d.CommitFreeze(ctx, g); err != nil {
		  t.Fatal(err)
	   }
	  got, err := d.Reject(ctx, tk.ID, "rev-keep-2", "senior", wdrelease.CodeRejected, "stop")
	   if err != nil {
		  t.Fatal(err)
	   }
	   if got.Frozen != "15000" {
		  t.Fatalf("frozen %s", got.Frozen)
	  }
	  if got.Segments[0].State != wdrelease.SegFrozen || got.Segments[1].State != wdrelease.SegCancelled {
		  t.Fatalf("segs %+v", got.Segments)
	   }
	   if _, err := d.BeforeFreeze(ctx, withdraw.ReleaseAsk{
		  UserID: in.UserID, Asset: in.Asset, Amount: "35000", Chain: in.Chain, Dest: in.Dest,
		  Idem: "idem-keep-more", TicketID: tk.ID,
	  }); err == nil || wdrelease.CodeOf(err) != wdrelease.CodeQuota {
		  t.Fatalf("second freeze %v", err)
	  }
	  again, err := d.Get(tk.ID)
	   if err != nil {
		  t.Fatal(err)
	   }
	   if again.Frozen != "15000" {
		  t.Fatalf("still %s", again.Frozen)
	  }
   }

  func TestWdReleaseTimeoutRejectAndDrift(t *testing.T) {
	   ctx := context.Background()
	   d := wdrelease.New()
	   d.SetNow(time.Date(2021, 8, 4, 0, 0, 0, 0, time.UTC))
	  if err := d.Publish("rel-2021.04"); err != nil {
		  t.Fatal(err)
	  }
	   in := wdrelease.Input{
		  UserID: "u-exp", Asset: "USDT", Amount: "200000", Chain: "ETH",
		   Dest: "0x8888888888888888888888888888888888888888",
		  DestClass: "external", Risk: 4, Applicant: "maker-exp", Idem: "idem-exp",
	   }
	  tk, err := d.Open(ctx, in)
	  if err != nil {
		  t.Fatal(err)
	   }
	   if tk.PolicyVer != "rel-2021.04" || tk.Timeout != wdrelease.TimeoutReject {
		   t.Fatalf("policy %s %s", tk.PolicyVer, tk.Timeout)
	  }
	   if _, err := d.Submit(ctx, tk.ID, in.Applicant); err != nil {
		  t.Fatal(err)
	  }
	  d.SetNow(time.Date(2021, 8, 5, 2, 0, 0, 0, time.UTC))
	  res, err := d.Tick(ctx)
	  if err != nil {
		  t.Fatal(err)
	   }
	  if res.Expired != 1 {
		   t.Fatalf("expired %d", res.Expired)
	  }
	  got, err := d.Get(tk.ID)
	   if err != nil {
		  t.Fatal(err)
	   }
	  if got.State != wdrelease.StateExpired || got.Code != wdrelease.CodeTimeout {
		  t.Fatalf("state %s %s", got.State, got.Code)
	   }
	  if err := d.Publish("rel-2022.11"); err != nil {
		  t.Fatal(err)
	  }
	   if d.DriftCount() < 1 {
		  t.Fatal("drift")
	   }
	  snap := d.Snapshot()
	   if snap.ActiveVersion != "rel-2022.11" || snap.Drift < 1 {
		  t.Fatalf("snap %+v", snap)
	  }
   }

  func TestWdReleaseSegmentCapAndNoSweepKind(t *testing.T) {
	  ctx := context.Background()
	  d := wdrelease.New()
	  v, err := d.Version("rel-2021.04")
	   if err != nil {
		   t.Fatal(err)
	  }
	   v.ID = "rel-cap"
	   v.Effective = time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	   v.Bands = []wdrelease.Band{
		  {Asset: "*", Min: "0", Max: "10", Depth: 0, Ratios: []int{10000}, SLASec: 0, Timeout: wdrelease.TimeoutNone, Skip: true, SegmentCap: "10", DailyCap: "1000", DestCap: "1000"},
		   {Asset: "*", Min: "10", Depth: 1, Ratios: []int{10000}, SLASec: 3600, Timeout: wdrelease.TimeoutReject, SegmentCap: "5", DailyCap: "1000", DestCap: "1000"},
	   }
	   if err := d.Install(v); err != nil {
		  t.Fatal(err)
	  }
	   if err := d.Publish("rel-cap"); err != nil {
		  t.Fatal(err)
	   }
	  _, err = d.Open(ctx, wdrelease.Input{
		   UserID: "u-cap", Asset: "USDT", Amount: "20", Chain: "ETH", Dest: relDest,
		  DestClass: "external", Applicant: "u-cap", Idem: "idem-cap",
	   })
	   if wdrelease.CodeOf(err) != wdrelease.CodeSegmentCap {
		   t.Fatalf("cap %v", err)
	  }
	   st := approvestore.NewMem()
	  if _, err := st.Open(ctx, custody.ApprovalKind(approvestore.KindWDRelease), "alice", "mv", "ETH", relDest, "1", "note"); !errors.Is(err, custody.ErrIllegalApproval) {
		   t.Fatalf("open kind %v", err)
	  }
	  if err := st.NoteRelease(ctx, approvestore.ReleaseRef{ID: "t-sw", TicketID: "t-sw", Kind: string(custody.KindSweep)}); !errors.Is(err, custody.ErrIllegalApproval) {
		   t.Fatalf("note sweep %v", err)
	   }
	   if err := st.NoteRelease(ctx, approvestore.ReleaseRef{ID: "t-ok", TicketID: "t-ok", Applicant: "alice", Amount: "1", Asset: "USDT", State: "draft"}); err != nil {
		   t.Fatal(err)
	  }
	  ref, err := st.ReleaseOf(ctx, "t-ok")
	  if err != nil || ref.Kind != approvestore.KindWDRelease {
		  t.Fatalf("ref %+v %v", ref, err)
	   }
	  if _, _, err := st.Get(ctx, "t-ok"); !errors.Is(err, custody.ErrApprovalNotFound) {
		   t.Fatalf("approval get %v", err)
	  }
  }

   func TestWdReleaseFailClosedBeforeFreeze(t *testing.T) {
	   rt := openMem(t)
	  ctx := context.Background()
	   uid := seedUser(rt)
	   desk := wdrelease.New()
	  wd := withdraw.New(rt.Ledger).BindRelease(desk)
	  before, err := rt.Store.GetAccount(ctx, uid, ledger.AccountWallet, "USDT")
	  if err != nil {
		   t.Fatal(err)
	  }
	  _, err = wd.Request(ctx, withdraw.Request{
		  UserID: uid, Asset: "USDT", Amount: decimal.RequireFromString("5000"),
		   Chain: "ETH", ToAddress: extDest("ETH"), IdempotencyKey: "wd-rel-big",
	  })
	   if err == nil || wdrelease.CodeOf(err) != wdrelease.CodeNoTicket {
		  t.Fatalf("big %v", err)
	   }
	  after, err := rt.Store.GetAccount(ctx, uid, ledger.AccountWallet, "USDT")
	  if err != nil {
		   t.Fatal(err)
	   }
	  if !before.Available.Equal(after.Available) || !before.Frozen.Equal(after.Frozen) {
		  t.Fatalf("balance moved %s/%s → %s/%s", before.Available, before.Frozen, after.Available, after.Frozen)
	   }
	   tr, err := wd.Request(ctx, withdraw.Request{
		  UserID: uid, Asset: "USDT", Amount: decimal.RequireFromString("1"),
		   Chain: "ETH", ToAddress: extDest("ETH"), IdempotencyKey: "wd-rel-small",
	   })
	   if err != nil || tr == nil {
		   t.Fatalf("small %v", err)
	   }
   }

  func TestWdReleasePathAndDeskHTTP(t *testing.T) {
	  ctx := context.Background()
	  desk := wdrelease.New()
	   path := wdpath.New().BindRelease(desk)
	  _, err := path.Open(ctx, wdpath.OpenReq{
		   UserID: "u-path", Chain: "ETH", Dest: relDest, Asset: "USDT", Amount: "50000",
		   IdempotencyKey: "path-big",
	  })
	   if !errors.Is(err, wdpath.ErrRelease) {
		  t.Fatalf("path big %v", err)
	   }
	  _, err = path.Open(ctx, wdpath.OpenReq{
		  UserID: "u-path", Chain: "ETH", Dest: relDest, Asset: "USDT", Amount: "1",
		   IdempotencyKey: "path-small",
	  })
	  if errors.Is(err,
		   wdpath.ErrRelease) {
		   t.Fatal("small path blocked")
	  }
	  rt, h := router(t)
	   w := doJSON(t, rt, h, "GET", "/v1/ops/wd-release/codes", nil)
	  if w.Code != 200 || !strings.Contains(w.Body.String(), "deny_no_ticket") {
		  t.Fatalf("codes %d %s", w.Code, w.Body.String())
	  }
	   w = doJSON(t, rt, h, "POST", "/v1/ops/wd-release/eval", map[string]any{
		   "user_id": "u-http", "asset": "USDT", "amount": "1", "chain": "ETH",
		  "dest": relDest, "dest_class": "external", "risk": 1,
	  })
	  if w.Code != 200 || !strings.Contains(w.Body.String(), `"skip":true`) {
		   t.Fatalf("eval %d %s", w.Code, w.Body.String())
	   }
	   w = doJSON(t, rt, h, "GET", "/v1/ops/wd-release/queue", nil)
	   if w.Code != 200 || !strings.Contains(w.Body.String(), "queue") {
		  t.Fatalf("queue %d %s", w.Code, w.Body.String())
	  }
	   w = doJSON(t, rt, h, "GET", "/v1/ops/wd-release/metrics", nil)
	   if w.Code != 200 || !strings.Contains(w.Body.String(), "active_version") {
		   t.Fatalf("metrics %d %s", w.Code, w.Body.String())
	   }
   }
