package flow

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/chainad"
	"account-assets.kucoin.net/funding/internal/hotbound"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/internal/hotreserve"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/withdraw"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

   func newHotReserve(t *testing.T, hot, cap string) (*hotreserve.Service, *ledger.MemStore) {
	   t.Helper()
	  st := ledger.NewMemStore()
	   st.Seed("system", ledger.AccountBridge, "USDT", hot)
	  book := hotinv.FromHotMax(map[string]string{"USDT": cap})
	   svc := hotreserve.New().BindBooks(st).BindHot(book)
	  pol := hotreserve.DefaultPolicy()
	   pol.ForecastMaxAge = 24 * time.Hour
	  if _, err := svc.WithPolicy(pol); err != nil {
		  t.Fatal(err)
	  }
	   return svc, st
  }

   func decEq(t *testing.T, got, want string) {
	  t.Helper()
	   g, err := decimal.NewFromString(got)
	  if err != nil {
		   t.Fatal(err)
	   }
	  w, err := decimal.NewFromString(want)
	   if err != nil {
		  t.Fatal(err)
	  }
	  if !g.Equal(w) {
		  t.Fatalf("got %s want %s", got, want)
	  }
  }

  func TestHotForecastLowerBoundRecomputesAndVersions(t *testing.T) {
	   ctx := context.Background()
	   svc, st := newHotReserve(t, "100", "1000000")
	   first, err := svc.Refresh(ctx, "USDT")
	  if err != nil {
		   t.Fatal(err)
	  }
	  if first.Version != 1 {
		  t.Fatalf("version %d", first.Version)
	   }
	   again, err := hotreserve.Project(hotreserve.ProjectInput{
		   HotAvail: first.HotAvail, Inflow: first.Inflow, Outflow: first.Outflow,
		   SweepHold: first.SweepHold, InTransit: first.InTransit,
		   Haircut: first.Haircut, Stress: first.Stress, Buffer: first.Buffer,
	   })
	  if err != nil {
		   t.Fatal(err)
	  }
	   decEq(t, again.Lower, first.Lower)
	   decEq(t, again.Point, first.Point)
	   decEq(t, first.Lower, "100")
	  if _, err := svc.RecordSample(ctx, "USDT", "0", "20", time.Now().UTC()); err != nil {
		  t.Fatal(err)
	  }
	  second, err := svc.Refresh(ctx, "USDT")
	  if err != nil {
		   t.Fatal(err)
	  }
	  if second.Version != 2 {
		   t.Fatalf("version %d hash %s", second.Version, second.InputsHash)
	   }
	   re, err := hotreserve.Project(hotreserve.ProjectInput{
		   HotAvail: second.HotAvail,
		  Inflow: second.Inflow, Outflow: second.Outflow,
		   SweepHold: second.SweepHold,
		  InTransit: second.InTransit,
		   Haircut: second.Haircut, Stress: second.Stress, Buffer: second.Buffer,
	   })
	  if err != nil {
		  t.Fatal(err)
	  }
	   decEq(t, re.Lower, second.Lower)
	   decEq(t, second.Outflow, "5")
	   decEq(t, second.Lower, "93.75")
	   third, err := svc.Refresh(ctx, "USDT")
	  if err != nil {
		   t.Fatal(err)
	   }
	  if third.Version != 2 {
		   t.Fatalf("same input bumped to %d", third.Version)
	  }
	   st.Seed("system", ledger.AccountBridge, "USDT", "80")
	   fourth, err := svc.Refresh(ctx, "USDT")
	  if err != nil {
		   t.Fatal(err)
	   }
	  if fourth.Version != 3 {
		   t.Fatalf("version %d", fourth.Version)
	  }
	   decEq(t, fourth.HotAvail, "80")
   }

   func TestReservationLowersOperationalAndConsumeNoDoubleFreeze(t *testing.T) {
	  ctx := context.Background()
	  svc, _ := newHotReserve(t, "100", "1000000")
	  if _, err := svc.Refresh(ctx, "USDT"); err != nil {
		  t.Fatal(err)
	   }
	   first, err := svc.Create(ctx, hotreserve.CreateReq{Asset: "USDT", Amount: "80", UserID: "u1", IntentID: "i1", IdempotencyKey: "idem-1"})
	  if err != nil {
		  t.Fatal(err)
	  }
	  ex, err := svc.Explain("USDT")
	  if err != nil {
		   t.Fatal(err)
	  }
	  decEq(t, ex.Operational, "20")
	  if _, err := svc.Create(ctx, hotreserve.CreateReq{Asset: "USDT", Amount: "30", UserID: "u1", IntentID: "i2"}); !errors.Is(err, hotreserve.ErrShortfall) {
		   t.Fatalf("surface still covers 30 but reserve should reject: %v", err)
	   }
	   same, err := svc.Create(ctx, hotreserve.CreateReq{Asset: "USDT", Amount: "80", UserID: "u1", IntentID: "i1", IdempotencyKey: "idem-1"})
	   if err != nil || same.ID != first.ID {
		  t.Fatalf("idem %v %+v", err, same)
	   }
	   em, err := svc.Earmark(ctx, hotreserve.EarmarkReq{ID: first.ID, MovementID: "m1", FreezeRef: "frz-1"})
	   if err != nil || em.State != hotreserve.StateEarmarked {
		   t.Fatalf("earmark %v %+v", err, em)
	   }
	   ex, err = svc.Explain("USDT")
	   if err != nil {
		   t.Fatal(err)
	   }
	  decEq(t, ex.Reserved, "80")
	   got, err := svc.Consume(ctx, hotreserve.ConsumeReq{ID: first.ID, BroadcastRef: "0xabc"})
	   if err != nil || got.State != hotreserve.StateConsumed {
		  t.Fatalf("consume %v %+v", err, got)
	  }
	  if err := svc.Verify(first.ID); err != nil {
		  t.Fatal(err)
	  }
	   ex, err = svc.Explain("USDT")
	   if err != nil {
		   t.Fatal(err)
	  }
	   decEq(t, ex.Reserved, "0")
	   decEq(t, ex.Extra, "80")
	   rep, err := svc.Patrol(ctx, "USDT")
	   if err != nil || rep.Drift {
		   t.Fatalf("align %+v %v", rep, err)
	  }
	   decEq(t, rep.Earmarked, "0")
	  decEq(t, rep.Freeze, "0")
	  decEq(t, rep.Consumed, "80")
	   decEq(t, rep.Broadcast, "80")
  }

   func TestAlignDriftBlocksReserveAndSign(t *testing.T) {
	   ctx := context.Background()
	   svc, _ := newHotReserve(t, "100", "1000000")
	   if _, err := svc.Refresh(ctx, "USDT"); err != nil {
		  t.Fatal(err)
	  }
	   row, err := svc.Create(ctx, hotreserve.CreateReq{Asset: "USDT", Amount: "40", UserID: "u1", IntentID: "drift"})
	   if err != nil {
		   t.Fatal(err)
	   }
	  if _, err := svc.Earmark(ctx, hotreserve.EarmarkReq{ID: row.ID, FreezeRef: "frz"}); err != nil {
		   t.Fatal(err)
	  }
	  if err := svc.SetObservedFreeze(ctx, "USDT", "10"); err != nil {
		  t.Fatal(err)
	  }
	  rep, err := svc.Patrol(ctx, "USDT")
	   if err != nil || !rep.Drift || !rep.Blocks || rep.TicketID == "" {
		   t.Fatalf("report %+v %v", rep, err)
	  }
	   if _, err := svc.Create(ctx, hotreserve.CreateReq{Asset: "USDT", Amount: "1", UserID: "u1", IntentID: "new"}); !errors.Is(err, hotreserve.ErrAlignDrift) {
		  t.Fatalf("create %v", err)
	   }
	  if err := svc.Admit(ctx, "sign", "USDT", "1", "other"); !errors.Is(err, hotreserve.ErrAlignDrift) {
		   t.Fatalf("sign %v", err)
	   }
	  w := withdraw.New(nil)
	   w.BindReserveAdmit(svc.Admit)
	   _, err = w.SubmitOnChain(ctx, chainad.NewRegistry(), "m-new", "ETH", "from", "to", "USDT", "1", "", "", "", "")
	  if !errors.Is(err, hotreserve.ErrAlignDrift) {
		  t.Fatalf("submit %v", err)
	  }
	   _, err = w.EncodeSignBroadcastFeeBump(ctx, "m-new", "from", "to", "USDT", "1", withdraw.BumpHints{}, true, false)
	  if !errors.Is(err, hotreserve.ErrAlignDrift) {
		  t.Fatalf("fee bump sign %v", err)
	   }
   }

   func TestOpenReserveExpiresBroadcastNotUnfrozen(t *testing.T) {
	  ctx := context.Background()
	   svc, _ := newHotReserve(t, "200", "1000000")
	   cur := time.Date(2021, 6, 1, 8, 0, 0, 0, time.UTC)
	   svc.WithClock(func() time.Time { return cur })
	   if _, err := svc.Refresh(ctx, "USDT"); err != nil {
		  t.Fatal(err)
	  }
	   open, err := svc.Create(ctx, hotreserve.CreateReq{Asset: "USDT", Amount: "10", UserID: "u1", IntentID: "exp"})
	  if err != nil {
		  t.Fatal(err)
	  }
	   held, err := svc.Create(ctx, hotreserve.CreateReq{Asset: "USDT", Amount: "15", UserID: "u1", IntentID: "hold"})
	   if err != nil {
		  t.Fatal(err)
	  }
	   if _, err := svc.Earmark(ctx, hotreserve.EarmarkReq{ID: held.ID, FreezeRef: "frz-hold"}); err != nil {
		  t.Fatal(err)
	   }
	   live, err := svc.Create(ctx, hotreserve.CreateReq{Asset: "USDT", Amount: "12", UserID: "u1", IntentID: "live"})
	   if err != nil {
		   t.Fatal(err)
	   }
	   if _, err := svc.Earmark(ctx, hotreserve.EarmarkReq{ID: live.ID, FreezeRef: "frz-live"}); err != nil {
		  t.Fatal(err)
	  }
	   if _, err := svc.Consume(ctx, hotreserve.ConsumeReq{ID: live.ID, BroadcastRef: "0xlive"}); err != nil {
		  t.Fatal(err)
	   }
	  cur = cur.Add(2 * time.Hour)
	   n, err := svc.ExpireDue(ctx)
	  if err != nil {
		  t.Fatal(err)
	   }
	   if n != 1 {
		  t.Fatalf("expired %d", n)
	  }
	  got, err := svc.Get(open.ID)
	   if err != nil || got.State != hotreserve.StateExpired {
		   t.Fatalf("open %+v %v", got, err)
	   }
	  heldGot, err := svc.Get(held.ID)
	  if err != nil || heldGot.State != hotreserve.StateEarmarked {
		  t.Fatalf("earmarked must stay %+v", heldGot)
	  }
	  liveGot, err := svc.Get(live.ID)
	  if err != nil || liveGot.State != hotreserve.StateConsumed || liveGot.BroadcastRef != "0xlive" {
		  t.Fatalf("broadcast %+v", liveGot)
	   }
	  rep, err := svc.Patrol(ctx, "USDT")
	   if err != nil || rep.Drift {
		   t.Fatalf("patrol %+v %v", rep, err)
	  }
	   if _, err := svc.ForceRelease(ctx, hotreserve.ReleaseReq{ID: held.ID, Requester: "ada", Approver: "ada"}); !errors.Is(err, hotreserve.ErrDual) {
		   t.Fatalf("same actor %v", err)
	  }
	  freed, err := svc.ForceRelease(ctx, hotreserve.ReleaseReq{ID: held.ID, Requester: "ada", Approver: "ben", Note: "ops"})
	   if err != nil || freed.State != hotreserve.StateCancelled {
		  t.Fatalf("force %v %+v", err, freed)
	  }
	  if _, err := svc.ForceRelease(ctx, hotreserve.ReleaseReq{ID: live.ID, Requester: "ada", Approver: "ben"}); !errors.Is(err, hotreserve.ErrBroadcastHeld) {
		  t.Fatalf("broadcast release %v", err)
	   }
  }

  func TestSweepSuggestDoesNotExecuteLegs(t *testing.T) {
	   ctx := context.Background()
	  svc, _ := newHotReserve(t, "5000", "1000")
	  if _, err := svc.Refresh(ctx, "USDT"); err != nil {
		   t.Fatal(err)
	   }
	   before := svc.Plans("USDT")
	   hint, err := svc.Suggest(ctx, "USDT")
	  if err != nil {
		   t.Fatal(err)
	  }
	   if hint.Executed || hint.PlanID == "" || !strings.HasPrefix(hint.PlanID, "hot-suggest-") {
		  t.Fatalf("hint %+v", hint)
	  }
	   if len(svc.Plans("USDT")) != len(before) {
		  t.Fatal("suggest wrote a plan hold")
	   }
	   snap, err := svc.Forecast("USDT")
	   if err != nil {
		  t.Fatal(err)
	   }
	  decEq(t, snap.SweepHold, "0")
	   if err := svc.NoteSweepPlans(ctx, "USDT", []hotreserve.PlanHold{{ID: hint.PlanID, Amount: "10"}}); err != nil {
		   t.Fatal(err)
	   }
	  snap, err = svc.Refresh(ctx, "USDT")
	  if err != nil {
		   t.Fatal(err)
	   }
	  decEq(t, snap.SweepHold, "10")
	   if snap.Version < 2 {
		   t.Fatalf("version %d", snap.Version)
	   }
	  st := ledger.NewMemStore()
	   st.Seed("system", ledger.AccountBridge, "USDT", "5000")
	   hb := hotbound.New().BindStore(st)
	  n0 := len(hb.List(20))
	  plans, err := hb.SuggestPlans(ctx)
	   if err != nil || len(plans) != 1 || plans[0].Execute {
		   t.Fatalf("bound %+v %v", plans, err)
	   }
	   if len(hb.List(20)) != n0 {
		   t.Fatal("suggest persisted a sweep")
	  }
	   cmd := exec.Command("go", "list", "-f", "{{join .Deps \"\\n\"}}", "account-assets.kucoin.net/funding/internal/hotreserve")
	   out, err := cmd.CombinedOutput()
	  if err != nil {
		   t.Fatalf("list %v %s", err, out)
	  }
	   if strings.Contains(string(out), "sweeprun") {
		  t.Fatal("hotreserve depends on sweeprun")
	  }
   }

   func TestHotReserveHTTPLedger(t *testing.T) {
	   gin.SetMode(gin.TestMode)
	   ctx := context.Background()
	  svc, _ := newHotReserve(t, "100", "1000000")
	   if _, err := svc.Refresh(ctx, "USDT"); err != nil {
		  t.Fatal(err)
	  }
	  r := gin.New()
	   hotreserve.Mount(r.Group("/v1"), svc)
	   body := bytes.NewBufferString(`{"asset":"USDT","amount":"25","user_id":"u1","intent_id":"http-1","idempotency_key":"http-idem"}`)
	   req := httptest.NewRequest(http.MethodPost, "/v1/hotreserve/reservations", body)
	  req.Header.Set("Content-Type", "application/json")
	   req.Header.Set("X-Role", "custody")
	  req.Header.Set("X-Actor", "ada")
	  rr := httptest.NewRecorder()
	   r.ServeHTTP(rr, req)
	   if rr.Code != 200 {
		  t.Fatalf("create %d %s", rr.Code, rr.Body.String())
	   }
	   req = httptest.NewRequest(http.MethodGet, "/v1/hotreserve/operational?asset=USDT", nil)
	  req.Header.Set("X-Role", "custody")
	  req.Header.Set("X-Actor", "ada")
	   rr = httptest.NewRecorder()
	   r.ServeHTTP(rr, req)
	  if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"operational"`) {
		   t.Fatalf("operational %d %s", rr.Code, rr.Body.String())
	   }
	   rows := svc.List("USDT", "open")
	   if len(rows) != 1 {
		   t.Fatalf("rows %+v", rows)
	   }
	   decEq(t, rows[0].Amount, "25")
  }
