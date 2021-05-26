package flow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/fundmat"
	"account-assets.kucoin.net/funding/internal/fundread"
	"account-assets.kucoin.net/funding/internal/httpapi"
	"account-assets.kucoin.net/funding/internal/ledger"
	"github.com/shopspring/decimal"
)

   func newMat() *fundmat.Service {
	  return fundmat.New(fundmat.NewMem())
   }

  func ingest(t *testing.T, svc *fundmat.Service, ev fundmat.FundingEvent) fundmat.FundingEvent {
	   t.Helper()
	   got, err := svc.Ingest(context.Background(), ev)
	  if err != nil {
		   t.Fatal(err)
	   }
	  return got
   }

  func rowOf(t *testing.T, svc *fundmat.Service, user, asset, subject string) fundmat.MatRow {
	  t.Helper()
	  rows,
		  err := svc.Rows(context.Background(), user, asset, subject)
	  if err != nil {
		  t.Fatal(err)
	  }
	  if len(rows) != 1 {
		  t.Fatalf("rows %d", len(rows))
	  }
	  return rows[0]
  }

  func TestFundMatFreezeCatchUpStrongRead(t *testing.T) {
	   svc := newMat()
	  read := fundread.New(svc)
	  ctx := context.Background()
	   ingest(t, svc, fundmat.FundingEvent{
		   ID: "cr-1", Seq: 1, UserID: "u1", Asset: "USDT", Subject: "funding",
		  Op: fundmat.OpCreditAvailable, Amount: "100", Stream: fundmat.StreamLedger,
	   })
	  ingest(t, svc, fundmat.FundingEvent{
		   ID: "fz-1", Seq: 1, UserID: "u1", Asset: "USDT", Subject: "funding",
		   Op: fundmat.OpFreeze, Amount: "40", Stream: fundmat.StreamFreeze,
	   })
	  stats, err := svc.Tick(ctx)
	  if err != nil {
		  t.Fatal(err)
	  }
	   if stats.Applied != 2 || stats.Paused {
		  t.Fatalf("stats %+v", stats)
	  }
	  lag, err := svc.Lag(ctx)
	  if err != nil {
		  t.Fatal(err)
	   }
	   if !lag.CaughtUp || !lag.WithinSLA {
		   t.Fatalf("lag %+v", lag)
	   }
	  view, err := read.Strong(ctx, fundread.Query{UserID: "u1", Asset: "USDT", Subject: "funding"})
	   if err != nil {
		   t.Fatal(err)
	   }
	   if view.Stale || len(view.Rows) != 1 {
		   t.Fatalf("view %+v", view)
	  }
	  if view.Rows[0].Frozen != "40" || view.Rows[0].Available != "60" {
		   t.Fatalf("cols %+v", view.Rows[0])
	   }
	  if view.Version.Version < 1 || view.Version.Digest == "" {
		  t.Fatalf("version %+v", view.Version)
	  }
	  if len(view.Assets) != 1 || view.Assets[0].Frozen != "40" {
		  t.Fatalf("assets %+v", view.Assets)
	   }
   }

  func TestFundMatStrongReadRejectsPumpDelay(t *testing.T) {
	  svc := newMat()
	   read := fundread.New(svc)
	   ctx := context.Background()
	  ingest(t, svc, fundmat.FundingEvent{
		   ID: "cr-d", Seq: 1, UserID: "u1", Asset: "USDT", Subject: "funding",
		  Op: fundmat.OpCreditAvailable, Amount: "80", Stream: fundmat.StreamLedger,
		   OccurredAt: time.Now().Add(-5 * time.Second),
	   })
	  svc.Pause(time.Minute)
	   stats, err := svc.Tick(ctx)
	  if err != nil {
		   t.Fatal(err)
	  }
	   if !stats.Paused || stats.Applied != 0 {
		  t.Fatalf("paused stats %+v", stats)
	  }
	  view, err := read.Strong(ctx, fundread.Query{UserID: "u1"})
	  if !errors.Is(err, fundread.ErrNotCaughtUp) {
		  t.Fatalf("strong err %v", err)
	   }
	  if !view.Stale || len(view.Watermarks) == 0 {
		  t.Fatalf("strong view %+v", view)
	   }
	  var behind bool
	  for _, wm := range view.Watermarks {
		   if wm.Stream == fundmat.StreamLedger && wm.ContiguousSeq < wm.HeadSeq {
			  behind = true
		   }
	   }
	   if !behind {
		   t.Fatalf("watermarks %+v", view.Watermarks)
	   }
	  ev, err := read.Eventual(ctx, fundread.Query{UserID: "u1", Asset: "USDT", Subject: "funding"})
	   if err != nil {
		  t.Fatal(err)
	   }
	  if !ev.Stale {
	   }
   }

   func TestFundMatRebuildMatchesLedger(t *testing.T) {
	  rt, err := app.OpenMem()
	  if err != nil {
		  t.Fatal(err)
	   }
	   ctx := context.Background()
	  uid := rt.Cfg.SeedUserID
	  svc := fundmat.New(fundmat.NewMem())
	   svc.BindLedger(rt.Ledger)
	   base, err := svc.RebuildNow(ctx, uid, "user", "0")
	   if err != nil {
		  t.Fatal(err)
	   }
	  if base.Status != fundmat.JobDone || base.DriftCount != 0 {
		  t.Fatalf("base job %+v", base)
	  }
	  if _, err := rt.Ledger.Create(ctx, ledger.CreateReq{
		  UserID: uid, Direction: ledger.SpotToWallet, Asset: "USDT",
		  Amount: decimal.RequireFromString("10"), IdempotencyKey: "mat-xfer-1",
	  }); err != nil {
		   t.Fatal(err)
	  }
	  n, err := svc.Pull(ctx, 500)
	   if err != nil {
		  t.Fatal(err)
	   }
	   if n < 2 {
		   t.Fatalf("pulled %d", n)
	   }
	   if _, err := svc.Tick(ctx); err != nil {
		   t.Fatal(err)
	  }
	  books, err := rt.Ledger.Accounts(ctx, uid)
	  if err != nil {
		   t.Fatal(err)
	  }
	   rows, err := svc.Rows(ctx, uid, "", "")
	   if err != nil {
		   t.Fatal(err)
	  }
	   lines := make([]fundmat.BookLine, 0, len(books))
	  for _, b := range books {
		  lines = append(lines, fundmat.BookLine{
			  UserID: uid,
			   Asset:  b.Asset, Subject: b.Subject, Available: b.Available, Frozen: b.Frozen,
		  })
	  }
	  drifts, err := fundmat.Compare(rows, lines, "0")
	  if err != nil {
		  t.Fatal(err)
	  }
	   if len(drifts) != 0 {
		  t.Fatalf("incremental drift %+v", drifts)
	   }
	   funding := rowOf(t, svc, uid, "USDT", "funding")
	  wallet := rowOf(t, svc, uid, "USDT", "wallet")
	   if funding.Available != "9990" || funding.Frozen != "0" {
		  t.Fatalf("funding %+v", funding)
	  }
	   if wallet.Available != "2010" {
		  t.Fatalf("wallet %+v", wallet)
	   }
	  funding.Available = "1"
	   funding.Frozen = "9"
	   if err := svc.Store().PutRow(ctx, funding); err != nil {
		  t.Fatal(err)
	  }
	   dirty, err := fundmat.Compare(mustRows(t, svc, uid), lines, "0")
	   if err != nil || len(dirty) == 0 {
		   t.Fatalf("want drift before rebuild %v %+v", err, dirty)
	   }
	  job, err := svc.RebuildNow(ctx, uid, "user", "0")
	  if err != nil {
		  t.Fatal(err)
	  }
	  if job.Status != fundmat.JobDone || job.DriftCount != 0 {
		  t.Fatalf("rebuild %+v", job)
	  }
	  again, err := fundmat.Compare(mustRows(t, svc, uid), lines, "0")
	   if err != nil || len(again) != 0 {
		  t.Fatalf("post rebuild %v %+v", err, again)
	  }
	   if fundmat.CanStep(fundmat.JobQueued, fundmat.JobDone) == nil {
		   t.Fatal("queued must not skip to done")
	   }
  }

  func mustRows(t *testing.T, svc *fundmat.Service, user string) []fundmat.MatRow {
	  t.Helper()
	  rows, err := svc.Rows(context.Background(), user, "", "")
	  if err != nil {
		  t.Fatal(err)
	  }
	  return rows
   }

  func TestFundMatOutOfOrderIdempotent(t *testing.T) {
	   svc := newMat()
	  ctx := context.Background()
	   ingest(t, svc, fundmat.FundingEvent{
		  ID: "oc-2", Seq: 2, UserID: "u1", Asset: "USDT", Subject: "funding",
		  Op: fundmat.OpOccupy, Amount: "3", Bucket: fundmat.BucketRisk, Stream: fundmat.StreamOccupancy,
	  })
	  dup := ingest(t, svc, fundmat.FundingEvent{
		  ID: "oc-2", Seq: 9, UserID: "u1", Asset: "USDT", Subject: "funding",
		   Op: fundmat.OpOccupy, Amount: "3", Bucket: fundmat.BucketRisk, Stream: fundmat.StreamOccupancy,
	   })
	  if dup.Seq != 2 {
		   t.Fatalf("dup seq %d", dup.Seq)
	  }
	  ingest(t, svc, fundmat.FundingEvent{
		   ID: "oc-1", Seq: 1, UserID: "u1", Asset: "USDT", Subject: "funding",
		  Op: fundmat.OpOccupy, Amount: "4", Bucket: fundmat.BucketRisk, Stream: fundmat.StreamOccupancy,
	  })
	  lag, err := svc.Lag(ctx)
	   if err != nil {
		  t.Fatal(err)
	   }
	   if lag.CaughtUp {
		   t.Fatal("gap must block catch-up before tick")
	   }
	   stats, err := svc.Tick(ctx)
	  if err != nil {
		   t.Fatal(err)
	   }
	  if stats.Applied != 2 {
		  t.Fatalf("applied %d", stats.Applied)
	  }
	  again, err := svc.Tick(ctx)
	  if err != nil {
		   t.Fatal(err)
	   }
	   if again.Applied != 0 {
		  t.Fatalf("second tick applied %d", again.Applied)
	  }
	  row := rowOf(t, svc, "u1", "USDT", "funding")
	  if row.Occupancy != "7" || row.OccupancyRisk != "7" {
		   t.Fatalf("occupancy %+v", row)
	  }
	  ingest(t, svc, fundmat.FundingEvent{
		  ID: "av-1", Seq: 1, UserID: "u1", Asset: "USDT", Subject: "funding",
		  Op: fundmat.OpCreditAvailable, Amount: "50", Stream: fundmat.StreamLedger,
	   })
	   first := ingest(t, svc, fundmat.FundingEvent{
		   ID: "fz-a", Seq: 1, UserID: "u1", Asset: "USDT", Subject: "funding",
		   Op: fundmat.OpFreeze, Amount: "20", Stream: fundmat.StreamFreeze,
	   })
	   againFreeze := ingest(t, svc, fundmat.FundingEvent{
		   ID: "fz-a", Seq: 3, UserID: "u1", Asset: "USDT", Subject: "funding",
		  Op: fundmat.OpFreeze, Amount: "20", Stream: fundmat.StreamFreeze,
	  })
	  if againFreeze.Seq != first.Seq {
		   t.Fatalf("duplicate allocated seq %d", againFreeze.Seq)
	  }
	  if _, err := svc.Tick(ctx); err != nil {
		   t.Fatal(err)
	   }
	   row = rowOf(t, svc, "u1", "USDT", "funding")
	   if row.Available != "30" || row.Frozen != "20" || row.Occupancy != "7" {
		   t.Fatalf("after freeze %+v", row)
	  }
	  lag, err = svc.Lag(ctx)
	  if err != nil || !lag.CaughtUp {
		  t.Fatalf("lag %+v %v", lag, err)
	  }
   }

  func TestFundMatHTTPStrongAndEventual(t *testing.T) {
	  rt, err := app.OpenMem()
	  if err != nil {
		  t.Fatal(err)
	   }
	  h := httpapi.Router(rt)
	  tok := rt.Cfg.Auth.BearerTokens[0]
	  uid := rt.Cfg.SeedUserID
	   call := func(method, path, role string, body any) *httptest.ResponseRecorder {
		  t.Helper()
		  var rdr io.Reader
		  if body != nil {
			  raw, _ := json.Marshal(body)
			   rdr = bytes.NewReader(raw)
		   }
		  req := httptest.NewRequest(method,
			   path, rdr)
		  if body != nil {
			  req.Header.Set("Content-Type", "application/json")
		   }
		  req.Header.Set("Authorization", "Bearer "+tok)
		   req.Header.Set("X-User-Id", uid)
		   if role != "" {
			   req.Header.Set("X-Role", role)
		   }
		  w := httptest.NewRecorder()
		   h.ServeHTTP(w, req)
		   return w
	  }
	   cr := call(http.MethodPost, "/v1/funding/views/events", "ops", map[string]any{
		  "id": "http-cr", "stream": "ledger", "seq": 1,
		  "user_id": uid, "asset": "USDT", "subject": "funding",
		  "op": "credit_available", "amount": "15",
	   })
	  if cr.Code != 200 {
		   t.Fatalf("credit %d %s", cr.Code, cr.Body.String())
	  }
	  ev := call(http.MethodPost, "/v1/funding/views/events", "ops", map[string]any{
		   "id": "http-fz", "stream": "freeze", "seq": 1,
		   "user_id": uid, "asset": "USDT", "subject": "funding",
		   "op": "freeze", "amount": "15",
	   })
	  if ev.Code != 200 {
		   t.Fatalf("freeze ingest %d %s", ev.Code, ev.Body.String())
	   }
	   strong := call(http.MethodGet, "/v1/funding/views/strong?asset=USDT&subject=funding", "", nil)
	   if strong.Code != 409 || !bytes.Contains(strong.Body.Bytes(), []byte("投影未追平")) {
		  t.Fatalf("strong delay %d %s", strong.Code, strong.Body.String())
	   }
	  eventual := call(http.MethodGet, "/v1/funding/views/eventual?asset=USDT&subject=funding", "", nil)
	   if eventual.Code != 200 || !bytes.Contains(eventual.Body.Bytes(), []byte(`"stale":true`)) {
		   t.Fatalf("eventual %d %s", eventual.Code, eventual.Body.String())
	  }
	   if !bytes.Contains(eventual.Body.Bytes(), []byte("watermarks")) {
		  t.Fatalf("eventual watermark %s", eventual.Body.String())
	  }
	  pump := call(http.MethodPost, "/v1/funding/views/pump", "ops", map[string]any{})
	  if pump.Code != 200 {
		  t.Fatalf("pump %d %s", pump.Code, pump.Body.String())
	   }
	   strong = call(http.MethodGet, "/v1/funding/views/strong?asset=USDT&subject=funding", "", nil)
	   if strong.Code != 200 || !bytes.Contains(strong.Body.Bytes(), []byte(`"frozen":"15"`)) {
		   t.Fatalf("strong caught %d %s", strong.Code, strong.Body.String())
	  }
	   fr := call(http.MethodGet, "/v1/assets/frozen?source=projection&consistency=strong", "", nil)
	  if fr.Code != 200 || !bytes.Contains(fr.Body.Bytes(), []byte(`"frozen":"15"`)) {
		  t.Fatalf("frozen projection %d %s", fr.Code, fr.Body.String())
	   }
   }
