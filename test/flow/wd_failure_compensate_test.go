package flow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/wdcomp"
	"account-assets.kucoin.net/funding/internal/wdrollback"
	"account-assets.kucoin.net/funding/internal/withdraw"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/gin-gonic/gin"
)

  func signHint() wdcomp.Hint {
	  return wdcomp.Hint{
		  UserID: "u-sign", FeeReserved: "1", IdempotencyKey: "sign-1",
		   Available: "100", Frozen: "40", Fee: "1",
	  }
   }

   func TestWdCompSignFailAutoUnfreezeIdempotent(t *testing.T) {
	  o := wdcomp.New()
	  ctx := context.Background()
	  row := withdraw.Broadcast{
		   TransferID: "tr-sign", Chain: "ETH", Asset: "USDT", Amount: "40",
		   Status: withdraw.BcSigned,
	   }
	   st := withdraw.Stuck{
		   TransferID: row.TransferID, Status: withdraw.BcSigned,
		   Action: withdraw.StuckRecover, Reason: "sign failed",
	   }
	   if n := o.ObserveStuck([]withdraw.Stuck{st}, map[string]withdraw.Broadcast{row.TransferID: row}, map[string]wdcomp.Hint{row.TransferID: signHint()}); n != 1 {
		  t.Fatalf("observe %d", n)
	  }
	  rep, err := o.Tick(ctx, time.Now().UTC())
	  if err != nil {
		  t.Fatal(err)
	  }
	   if rep.Ingested != 1 || rep.Executed != 1 {
		   t.Fatalf("tick %+v", rep)
	   }
	  rows := o.Failures(wdcomp.ListFilter{TransferID: "tr-sign"})
	  if len(rows) != 1 || rows[0].Code != wdcomp.CodeSignFail || rows[0].Status != wdcomp.FailClosed {
		  t.Fatalf("failure %+v", rows)
	   }
	   view, err := o.Plan(rows[0].PlanID)
	  if err != nil || view.Plan.State != wdcomp.PlanCommitted {
		   t.Fatalf("plan %+v err=%v", view.Plan, err)
	  }
	  pos, err := o.Position("u-sign", "USDT")
	  if err != nil {
		  t.Fatal(err)
	  }
	   if pos.Available != "141" || pos.Frozen != "0" || pos.FeeReserved != "0" || !pos.Conserved {
		   t.Fatalf("pos %+v", pos)
	  }
	  before := len(o.Rollback("u-sign", "USDT", "tr-sign"))
	   if n := o.ObserveStuck([]withdraw.Stuck{st}, map[string]withdraw.Broadcast{row.TransferID: row}, map[string]wdcomp.Hint{row.TransferID: signHint()}); n != 1 {
		   t.Fatal(n)
	   }
	   rep, err = o.Tick(ctx, time.Now().UTC())
	   if err != nil {
		  t.Fatal(err)
	  }
	  if rep.Replayed != 1 || rep.Ingested != 0 || rep.Executed != 0 {
		  t.Fatalf("replay %+v", rep)
	   }
	  pos, err = o.Position("u-sign", "USDT")
	  if err != nil {
		   t.Fatal(err)
	  }
	  if pos.Available != "141" || pos.Frozen != "0" || !pos.Conserved {
		  t.Fatalf("replay pos %+v", pos)
	  }
	   if len(o.Rollback("u-sign", "USDT", "tr-sign")) != before {
		   t.Fatal("rollback grew on replay")
	   }
	  if err := o.AssertPosition("u-sign", "USDT"); err != nil {
		  t.Fatal(err)
	   }
   }

  func TestWdCompLiveHashNeedsHumanRejectsAutoUnfreeze(t *testing.T) {
	   o := wdcomp.New()
	  ctx := context.Background()
	  res, err := o.Ingest(ctx, wdcomp.FailureFact{
		   Source: wdcomp.SourceStuck, TransferID: "tr-live", UserID: "u-live", Asset: "USDT",
		  Amount: "40", FeeReserved: "1", TxHash: "0xabc", BroadcastStatus: "broadcast",
		   Reason: "broadcast seen", IdempotencyKey: "live-1",
		  AvailableSnapshot: "100", FrozenSnapshot: "40", FeeSnapshot: "1",
	   })
	   if err != nil {
		   t.Fatal(err)
	   }
	  if res.Failure.Code != wdcomp.CodeLiveHash || res.Failure.MayAuto {
		  t.Fatalf("class %+v", res.Failure)
	   }
	  view, err := o.Preview(ctx, res.Failure.ID)
	   if err != nil {
		  t.Fatal(err)
	   }
	  if view.Plan.State != wdcomp.PlanNeedsHuman {
		  t.Fatalf("state %s", view.Plan.State)
	   }
	  if _, err := o.Execute(ctx, view.Plan.ID); !errors.Is(err, wdcomp.ErrNeedsHuman) {
		  t.Fatalf("execute %v", err)
	   }
	   if _, err := o.Confirm(ctx, view.Plan.ID, "ada", "bob", "look", true); !errors.Is(err, wdcomp.ErrAutoUnfreezeDenied) {
		  t.Fatalf("confirm %v", err)
	  }
	  if _, err := o.PutPolicy(ctx, wdcomp.AutoPolicy{Code: wdcomp.CodeLiveHash, AutoAllowed: true, UpdatedBy: "ada"}); !errors.Is(err, wdcomp.ErrPolicy) {
		  t.Fatalf("policy %v", err)
	   }
	  if wdrollback.LiveUnfreezeAllowed() {
		  t.Fatal("live unfreeze flag")
	  }
	   pos, err := o.Position("u-live", "USDT")
	   if err != nil {
		  t.Fatal(err)
	  }
	   if pos.Available != "100" || pos.Frozen != "40" || pos.FeeReserved != "1" || !pos.Conserved {
		   t.Fatalf("pos %+v", pos)
	   }
	   if len(o.Audits()) < 2 {
		   t.Fatalf("audits %+v", o.Audits())
	  }
	  for _, a := range o.Audits() {
		  if a.TransferID == "tr-live" && a.Path == "" {
			   t.Fatal("empty audit path")
		   }
	  }
	   if err := wdcomp.Transit(wdcomp.PlanCommitted, wdcomp.PlanExecuting); err == nil {
		  t.Fatal("committed reopened")
	   }
	  again, err := o.Execute(ctx, view.Plan.ID)
	   if !errors.Is(err, wdcomp.ErrNeedsHuman) || again.Plan.State != wdcomp.PlanNeedsHuman {
		  t.Fatalf("again %+v %v", again.Plan, err)
	  }
	  pos, _ = o.Position("u-live", "USDT")
	   if pos.Frozen != "40" || pos.Available != "100" {
		  t.Fatalf("moved %+v", pos)
	   }
   }

   func TestWdCompShardPartialFilledUntouched(t *testing.T) {
	   o := wdcomp.New()
	   ctx := context.Background()
	   _, err := o.IngestShard(ctx, wdcomp.ShardFailEvent{
		  PlanID: "sp-ok", TransferID: "tr-ok", UserID: "u-shard", Asset: "USDT",
		  TotalAmount: "40", FilledAmount: "40", FailedAmount: "0",
		  IdempotencyKey: "shard-ok",
	   })
	  if !errors.Is(err, wdcomp.ErrSuccessPath) {
		  t.Fatalf("success path %v", err)
	   }
	   res, err := o.IngestShard(ctx, wdcomp.ShardFailEvent{
		  PlanID: "sp-1", TransferID: "tr-shard", UserID: "u-shard", Asset: "USDT", Chain: "ETH",
		   ShardID: "sh-3", ShardSeq: 3, ShardCount: 4,
		   TotalAmount: "40", FilledAmount: "10", FailedAmount: "30", FeeReserved: "0.4",
		  FilledTxHashes: []string{"0xfilled"}, Reason: "remainder rejected",
		   IdempotencyKey: "shard-1",
		  Available: "100", Frozen: "40", Fee: "0.4",
	  })
	  if err != nil {
		   t.Fatal(err)
	  }
	  if res.Failure.Code != wdcomp.CodeShardPartial || res.Failure.Compensable != "30" || res.Failure.ProtectedAmount != "10" {
		  t.Fatalf("class %+v", res.Failure)
	  }
	  if res.Failure.LiveHash || res.Failure.TxHash != "" {
		  t.Fatalf("hash %+v", res.Failure)
	  }
	   view, err := o.Preview(ctx, res.Failure.ID)
	   if err != nil {
		   t.Fatal(err)
	   }
	   got, err := o.Execute(ctx, view.Plan.ID)
	   if err != nil {
		   t.Fatal(err)
	  }
	  if got.Plan.State != wdcomp.PlanCommitted {
		  t.Fatalf("state %s", got.Plan.State)
	   }
	   pos, err := o.Position("u-shard", "USDT")
	   if err != nil {
		  t.Fatal(err)
	   }
	   if pos.Frozen != "10" || pos.Available != "130.4" || pos.FeeReserved != "0" || !pos.Conserved {
		   t.Fatalf("pos %+v", pos)
	  }
	   var released money.Amount
	   for _, e := range o.Rollback("u-shard", "USDT", "tr-shard") {
		   if e.Kind != wdrollback.KindFreeze || !e.Applied {
			  continue
		   }
		  amt, err := money.ParseBook(e.Amount)
		  if err != nil {
			   t.Fatal(err)
		   }
		   released = released.MustAdd(amt)
	   }
	   if !released.Equal(money.Must("30")) {
		  t.Fatalf("released %s", released)
	  }
  }

   func TestWdCompRollbackConservesAvailableFrozen(t *testing.T) {
	   o := wdcomp.New()
	   ctx := context.Background()
	  res, err := o.IngestCancel(ctx, wdcomp.FailureFact{
		   TransferID: "tr-cons", UserID: "u-cons", Asset: "USDT", Amount: "25", FeeReserved: "0.5",
		  BroadcastStatus: string(withdraw.BcSigned), Reason: "human cancel", IdempotencyKey: "cons-1",
		  AvailableSnapshot: "10", FrozenSnapshot: "25", FeeSnapshot: "0.5",
	   })
	   if err != nil {
		   t.Fatal(err)
	  }
	   if res.Failure.Code != wdcomp.CodeHumanCancel {
		   t.Fatalf("code %s", res.Failure.Code)
	  }
	   view, err := o.Preview(ctx, res.Failure.ID)
	  if err != nil {
		  t.Fatal(err)
	  }
	   if _, err := o.Execute(ctx, view.Plan.ID); err != nil {
		  t.Fatal(err)
	  }
	   if err := o.AssertPosition("u-cons", "USDT"); err != nil {
		  t.Fatal(err)
	   }
	  pos, err := o.Position("u-cons", "USDT")
	   if err != nil {
		  t.Fatal(err)
	   }
	  avail, _ := money.ParseBook(pos.Available)
	   frozen,
		  _ := money.ParseBook(pos.Frozen)
	   fee, _ := money.ParseBook(pos.FeeReserved)
	  if !frozen.IsZero() || !fee.IsZero() || !avail.Equal(money.Must("35.5")) {
		  t.Fatalf("pos %+v", pos)
	   }
	   eq, err := money.ParseBook(pos.Equity)
	   if err != nil || !eq.Equal(money.Must("35.5")) {
		   t.Fatalf("equity %s %v", pos.Equity, err)
	  }
	   sumFreeze := money.Zero()
	   sumFee := money.Zero()
	  for _, e := range o.Rollback("u-cons", "USDT", "tr-cons") {
		  if !e.Applied {
			  continue
		   }
		   amt, err := money.ParseBook(e.Amount)
		   if err != nil {
			  t.Fatal(err)
		  }
		  switch e.Kind {
		   case wdrollback.KindFreeze:
			  sumFreeze = sumFreeze.MustAdd(amt)
			  if e.AvailableBefore != "10" && e.AvailableBefore != "35" && e.FrozenBefore == "" {
				   t.Fatalf("entry %+v", e)
			  }
		  case wdrollback.KindFee:
			  sumFee = sumFee.MustAdd(amt)
		  }
	  }
	  if !sumFreeze.Equal(money.Must("25")) || !sumFee.Equal(money.Must("0.5")) {
		  t.Fatalf("sums freeze=%s fee=%s", sumFreeze, sumFee)
	   }
  }

   func TestWdCompHTTPSignFail(t *testing.T) {
	   gin.SetMode(gin.TestMode)
	   o := wdcomp.New()
	   r := gin.New()
	  wdcomp.RegisterService(r.Group("/v1"), o)
	  body := []byte(`{
		"source":"stuck","transfer_id":"tr-http","user_id":"u-http","asset":"USDT",
		"amount":"8","fee_reserved":"0.2","broadcast_status":"signed","reason":"sign failed",
		"idempotency_key":"http-1","available_snapshot":"20","frozen_snapshot":"8","fee_snapshot":"0.2"
	}`)
	  w := postJSON(r, "/v1/wdcomp/failures", body)
	   if w.Code != 200 {
		   t.Fatalf("ingest %d %s", w.Code, w.Body.String())
	   }
	  var ing wdcomp.IngestResult
	  if err := json.Unmarshal(w.Body.Bytes(), &ing); err != nil {
		  t.Fatal(err)
	  }
	  prevBody, _ := json.Marshal(map[string]string{"failure_id": ing.Failure.ID})
	   w = postJSON(r, "/v1/wdcomp/plans/preview", prevBody)
	   if w.Code != 200 {
		   t.Fatalf("preview %d %s", w.Code, w.Body.String())
	  }
	  var view wdcomp.PlanView
	  if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		  t.Fatal(err)
	   }
	  w = postJSON(r, "/v1/wdcomp/plans/"+view.Plan.ID+"/execute", []byte(`{}`))
	   if w.Code != 200 {
		   t.Fatalf("execute %d %s", w.Code, w.Body.String())
	  }
	   req := httptest.NewRequest(http.MethodGet, "/v1/wdcomp/positions?user_id=u-http&asset=USDT", nil)
	  req.Header.Set("X-Actor", "ada")
	   req.Header.Set("X-Role", "ops")
	  rec := httptest.NewRecorder()
	   r.ServeHTTP(rec, req)
	  if rec.Code != 200 {
		  t.Fatalf("pos %d %s", rec.Code, rec.Body.String())
	  }
	   var pos wdcomp.PositionView
	  if err := json.Unmarshal(rec.Body.Bytes(), &pos); err != nil {
		   t.Fatal(err)
	  }
	  if pos.Available != "28.2" || pos.Frozen != "0" || !pos.Conserved {
		   t.Fatalf("pos %+v", pos)
	  }
   }

  func postJSON(r *gin.Engine, path string, body []byte) *httptest.ResponseRecorder {
	   req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	  req.Header.Set("Content-Type", "application/json")
	   req.Header.Set("X-Actor", "ada")
	  req.Header.Set("X-Role", "ops")
	   w := httptest.NewRecorder()
	  r.ServeHTTP(w, req)
	   return w
  }
