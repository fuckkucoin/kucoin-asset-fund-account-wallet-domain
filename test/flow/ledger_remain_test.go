package flow

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/kcfund"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/p2pgate"
	"account-assets.kucoin.net/funding/internal/subacct"
	"account-assets.kucoin.net/funding/internal/wallet"
	"github.com/shopspring/decimal"
)

   func TestCoverRiskRejectUndoesAndSameIdemRetries(t *testing.T) {
	  rt, err := app.OpenMem()
	  if err != nil {
		   t.Fatal(err)
	  }
	   ctx := context.Background()
	   uid := "u-cover-risk"
	  mem, ok := rt.Store.(*ledger.MemStore)
	   if !ok {
		  t.Fatal("mem store")
	   }
	   mem.Seed(uid, ledger.AccountWallet, "USDT", "0")
	   mem.Seed(uid, ledger.AccountSpot, "USDT", "40")
	   mem.Seed("system", ledger.AccountBridge, "USDT", "1000")
	   to := wallet.DeriveAddress(ledger.ExtWithdrawUser, "ETH")
	   wal0, _ := rt.Store.GetAccount(ctx, uid, ledger.AccountWallet, "USDT")
	   spot0, _ := rt.Store.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	  _, err = rt.Ledger.Create(ctx, ledger.CreateReq{
		   UserID: uid, Direction: ledger.WalletWithdraw, Asset: "USDT",
		  Amount: decimal.RequireFromString("10"), Chain: "ETH", ToAddress: to,
		   IdempotencyKey: "cover-risk-1",
	   })
	   if !errors.Is(err, ledger.ErrRiskRejected) {
		  t.Fatalf("want risk reject %v", err)
	  }
	  wal, err := rt.Store.GetAccount(ctx, uid, ledger.AccountWallet, "USDT")
	   if err != nil {
		  t.Fatal(err)
	  }
	  spot, err := rt.Store.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	   if err != nil {
		   t.Fatal(err)
	  }
	  if !wal.Available.Equal(wal0.Available) || !spot.Available.Equal(spot0.Available) {
		  t.Fatalf("stranded cover wal %s->%s spot %s->%s", wal0.Available, wal.Available, spot0.Available, spot.Available)
	  }
	   mem.Allow(uid, "ETH", to)
	   tr, err := rt.Ledger.Create(ctx, ledger.CreateReq{
		   UserID: uid, Direction: ledger.WalletWithdraw, Asset: "USDT",
		   Amount: decimal.RequireFromString("10"), Chain: "ETH", ToAddress: to,
		   IdempotencyKey: "cover-risk-1",
	  })
	  if err != nil || tr == nil || !tr.State.Open() {
		   t.Fatalf("retry %v %+v", err, tr)
	   }
  }

  func TestParkedHoldcatNotUserFrozenAndReconcileOK(t *testing.T) {
	  rt, err := app.OpenMem()
	   if err != nil {
		   t.Fatal(err)
	  }
	   ctx := context.Background()
	   uid := rt.Cfg.SeedUserID
	   if mem, ok := rt.Store.(*ledger.MemStore); ok {
		  mem.Seed("system", ledger.AccountBridge, "USDC", "10000")
	  }
	  spot, err := rt.Store.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	   if err != nil {
		   t.Fatal(err)
	  }
	   before := spot.Available
	   open, err := rt.Convert.PlaceLimit(ctx, uid, "lim-park-1", "USDT", "USDC", "10", "11", "MAIN")
	   if err != nil || open.Status != "OPEN" {
		   t.Fatalf("open %v %+v", err, open)
	   }
	  spot, err = rt.Store.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	   if err != nil {
		   t.Fatal(err)
	   }
	  if !spot.Available.Equal(before.Sub(decimal.RequireFromString("10"))) {
		   t.Fatalf("available %s want %s-10", spot.Available, before)
	  }
	  if spot.Frozen.IsPositive() {
		  t.Fatalf("user frozen after park %s", spot.Frozen)
	  }
	  rows, err := rt.FundSurf.Frozen(ctx, uid)
	   if err != nil {
		   t.Fatal(err)
	   }
	   parked, extraFunding := false, false
	   for _, r := range rows {
		   if r.Asset == "USDT" && r.Reason == holdcat.ReasonConvertLimit && r.Subject == "parked" {
			  parked = true
		   }
		   if r.Asset == "USDT" && r.Book == "funding" && r.Reason == holdcat.ReasonConvertLimit {
			  extraFunding = true
		  }
		  if r.Asset == "USDT" && r.Book == "spot" && r.Reason == holdcat.ReasonConvertLimit {
			  extraFunding = true
		   }
	  }
	   if !parked {
		   t.Fatalf("want parked occupancy %+v", rows)
	  }
	   if extraFunding {
		  t.Fatalf("holdcat must not inflate user frozen %+v", rows)
	   }
	   extra, mm, err := rt.InflightFrozen(ctx, uid)
	   if err != nil {
		  t.Fatal(err)
	  }
	  rep, err := ledger.ReconcileWith(ctx, rt.Store, uid, extra)
	  if err != nil {
		  t.Fatal(err)
	   }
	   ledger.MergeMismatches(rep, mm)
	  if !rep.OK {
		   t.Fatalf("reconcile %+v", rep)
	   }
	  f := p2pgate.NewFunds(rt.Ledger, holdcat.New(rt.KV))
	  hold, err := f.SellHold(ctx, uid, "USDT", "2", "p2p-park-1")
	   if err != nil || hold.ID == "" {
		  t.Fatalf("p2p hold %v %+v", err, hold)
	  }
	  rows,
		   err = rt.FundSurf.Frozen(ctx, uid)
	   if err != nil {
		   t.Fatal(err)
	  }
	   p2pParked := false
	  for _, r := range rows {
		  if r.Reason == holdcat.ReasonP2PSell && r.Subject == "parked" {
			   p2pParked = true
		   }
		   if r.Reason == holdcat.ReasonP2PSell && (r.Book == "funding" || r.Book == "spot") {
			   t.Fatalf("p2p extra user frozen %+v", rows)
		  }
	  }
	  if !p2pParked {
		  t.Fatalf("want p2p parked %+v", rows)
	  }
	  extra, mm, err = rt.InflightFrozen(ctx, uid)
	  if err != nil {
		  t.Fatal(err)
	  }
	   rep, err = ledger.ReconcileWith(ctx, rt.Store, uid, extra)
	  if err != nil {
		  t.Fatal(err)
	  }
	  ledger.MergeMismatches(rep, mm)
	  if !rep.OK {
		   t.Fatalf("p2p reconcile %+v", rep)
	   }
   }

   func TestHopFundsRetryAfterCompensatedOut(t *testing.T) {
	  rt, err := app.OpenMem()
	  if err != nil {
		   t.Fatal(err)
	   }
	  ctx := context.Background()
	   uid := rt.Cfg.SeedUserID
	  svc := subacct.New()
	   p, err := svc.OpenParent(subacct.OpenParentReq{UserID: uid, Label: "primary"})
	   if err != nil {
		   t.Fatal(err)
	   }
	  child, err := svc.OpenChild(subacct.OpenChildReq{
		  ParentID: p.ID, UserID: "child-remain-1", Label: "desk",
		   Privileges: []subacct.Privilege{subacct.PrivDeposit, subacct.PrivWithdraw, subacct.PrivHop},
	   })
	   if err != nil {
		   t.Fatal(err)
	   }
	  spot0, err := rt.Store.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	  if err != nil {
		   t.Fatal(err)
	  }
	  _, err = kcfund.HopFunds(ctx, rt.Ledger, svc, kcfund.HopFundsReq{
		   UserID: uid, Kind: "PARENT_TO_SUB", ClientOid: "hop-retry-1",
		  Currency: "USDT", Amount: decimal.RequireFromString("3"), AmountRaw: "3",
		   FromOwner: p.ID,
		  ToOwner:   child.ID,
	   })
	  if err == nil {
		   t.Fatal("want hop fail without transfer priv")
	   }
	   spot, err := rt.Store.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	  if err != nil {
		  t.Fatal(err)
	   }
	  if !spot.Available.Equal(spot0.Available) {
		  t.Fatalf("ledger not restored %s want %s", spot.Available, spot0.Available)
	   }
	   if _, err := svc.SetPrivileges(child.ID, subacct.AllPrivileges()); err != nil {
		  t.Fatal(err)
	  }
	   hop, err := kcfund.HopFunds(ctx, rt.Ledger, svc, kcfund.HopFundsReq{
		  UserID: uid, Kind: "PARENT_TO_SUB", ClientOid: "hop-retry-1",
		  Currency: "USDT", Amount: decimal.RequireFromString("3"), AmountRaw: "3",
		  FromOwner: p.ID, ToOwner: child.ID,
	  })
	   if err != nil || hop.ID == "" {
		   t.Fatalf("retry %v %+v", err, hop)
	   }
	  spot, err = rt.Store.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	   if err != nil {
		  t.Fatal(err)
	   }
	   if !spot.Available.Equal(spot0.Available.Sub(decimal.RequireFromString("3"))) {
		   t.Fatalf("after retry funding %s", spot.Available)
	   }
	  cb, err := svc.BookOf(child.ID, "USDT")
	   if err != nil || cb.Available != "3" {
		   t.Fatalf("child overlay %v %+v", err, cb)
	  }
	   back, err := kcfund.HopFunds(ctx, rt.Ledger, svc, kcfund.HopFundsReq{
		   UserID: uid, Kind: "SUB_TO_PARENT", ClientOid: "hop-back-1",
		  Currency: "USDT", Amount: decimal.RequireFromString("1"), AmountRaw: "1",
		   FromOwner: child.ID, ToOwner: p.ID,
	   })
	  if err != nil || back.ID == "" {
		   t.Fatalf("sub to parent %v %+v", err, back)
	  }
  }

   func TestReconcileSeedFixtureStillOK(t *testing.T) {
	  rt, err := app.OpenMem()
	  if err != nil {
		   t.Fatal(err)
	  }
	  rep,
		   err := ledger.Reconcile(context.Background(), rt.Store, rt.Cfg.SeedUserID)
	   if err != nil || !rep.OK {
		   t.Fatalf("seed %+v %v", rep, err)
	   }
	   for _, m := range rep.Mismatches {
		   if strings.Contains(m.Note, "unexplained book") {
			   t.Fatalf("seed fixture flagged %s", m.Note)
		   }
	  }
  }

  func TestCoverUndoRetriesThenFailClosed(t *testing.T) {
	  rt, err := app.OpenMem()
	   if err != nil {
		  t.Fatal(err)
	  }
	  ctx := context.Background()
	  uid := "u-cover-undo-fc"
	  mem := rt.Store.(*ledger.MemStore)
	   mem.Seed(uid, ledger.AccountWallet, "USDT", "0")
	  mem.Seed(uid, ledger.AccountSpot, "USDT", "40")
	  mem.Seed("system", ledger.AccountBridge, "USDT", "1000")
	   to := wallet.DeriveAddress(ledger.ExtWithdrawUser, "ETH")
	   rt.Ledger.SetCoverUndoFaults(1)
	  _, err = rt.Ledger.Create(ctx, ledger.CreateReq{
		   UserID: uid, Direction: ledger.WalletWithdraw, Asset: "USDT",
		   Amount: decimal.RequireFromString("8"), Chain: "ETH", ToAddress: to,
		  IdempotencyKey: "cover-undo-1",
	   })
	  if !errors.Is(err, ledger.ErrRiskRejected) {
		  t.Fatalf("retry undo should still surface risk %v", err)
	   }
	  wal, _ := rt.Store.GetAccount(ctx, uid, ledger.AccountWallet, "USDT")
	  spot, _ := rt.Store.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	  if !wal.Available.IsZero() || !spot.Available.Equal(decimal.RequireFromString("40")) {
		   t.Fatalf("after retried undo wal=%s spot=%s", wal.Available, spot.Available)
	  }
	   rt.Ledger.SetCoverUndoFaults(5)
	  _, err = rt.Ledger.Create(ctx, ledger.CreateReq{
		  UserID: uid, Direction: ledger.WalletWithdraw, Asset: "USDT",
		  Amount: decimal.RequireFromString("8"), Chain: "ETH", ToAddress: to,
		   IdempotencyKey: "cover-undo-2",
	   })
	  if !errors.Is(err, ledger.ErrCoverStranded) {
		  t.Fatalf("want stranded %v", err)
	  }
	   rt.Ledger.SetCoverUndoFaults(0)
	   _, err = rt.Ledger.Create(ctx, ledger.CreateReq{
		   UserID: uid, Direction: ledger.WalletWithdraw, Asset: "USDT",
		   Amount: decimal.RequireFromString("8"), Chain: "ETH", ToAddress: to,
		  IdempotencyKey: "cover-undo-2",
	  })
	  if !errors.Is(err, ledger.ErrRiskRejected) {
		  t.Fatalf("sweep leftover then clean reject %v", err)
	  }
	  wal, _ = rt.Store.GetAccount(ctx, uid, ledger.AccountWallet, "USDT")
	  spot, _ = rt.Store.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	  if !wal.Available.IsZero() || !spot.Available.Equal(decimal.RequireFromString("40")) {
		   t.Fatalf("after sweep wal=%s spot=%s", wal.Available, spot.Available)
	  }
  }

   func TestFiatOfframpParkVisibleAndReconcile(t *testing.T) {
	   rt, err := app.OpenMem()
	   if err != nil {
		  t.Fatal(err)
	   }
	   ctx := context.Background()
	  uid := rt.Cfg.SeedUserID
	   if _, err := rt.Ledger.Create(ctx, ledger.CreateReq{
		   UserID: uid, Direction: ledger.FiatOfframp, Asset: "USDT",
		   Amount: decimal.RequireFromString("4"), IdempotencyKey: "fiat-park-1",
	  }); err != nil {
		  t.Fatal(err)
	  }
	   rows, err := rt.FundSurf.Frozen(ctx, uid)
	   if err != nil {
		   t.Fatal(err)
	   }
	  found := false
	  for _, r := range rows {
		  if r.Reason == "fiat_offramp" && r.Subject == "parked" {
			  found = true
		  }
		   if r.Reason == "fiat_offramp" && (r.Book == "funding" || r.Book == "spot") {
			   t.Fatalf("offramp must not inflate user frozen %+v", rows)
		   }
	   }
	   if !found {
		   t.Fatalf("want fiat parked %+v", rows)
	  }
	  rep, err := ledger.Reconcile(ctx, rt.Store, uid)
	  if err != nil || !rep.OK {
		  t.Fatalf("reconcile %+v %v", rep, err)
	  }
   }

   func TestConcurrentCreatesCoverSerialize(t *testing.T) {
	   rt, err := app.OpenMem()
	  if err != nil {
		  t.Fatal(err)
	  }
	  ctx := context.Background()
	   uid := "u-cover-conc"
	  mem := rt.Store.(*ledger.MemStore)
	  mem.Seed(uid, ledger.AccountWallet, "USDT", "50")
	   mem.Seed(uid, ledger.AccountSpot, "USDT", "0")
	   mem.Seed("system", ledger.AccountBridge, "USDT", "1000")
	  var wg sync.WaitGroup
	   errs := make(chan error, 2)
	   for i := 0; i < 2; i++ {
		   wg.Add(1)
		   go func(n int) {
			   defer wg.Done()
			   _, err := rt.Ledger.Create(ctx, ledger.CreateReq{
				   UserID: uid, Direction: ledger.FiatOfframp, Asset: "USDT",
				   Amount:         decimal.RequireFromString("10"),
				   IdempotencyKey: "conc-off-" + string(rune('a'+n)),
			   })
			  errs <- err
		  }(i)
	  }
	  wg.Wait()
	  close(errs)
	  for err := range errs {
		  if err != nil {
			  t.Fatalf("concurrent create %v", err)
		   }
	  }
	   wal, _ := rt.Store.GetAccount(ctx, uid, ledger.AccountWallet, "USDT")
	   spot, _ := rt.Store.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	   if !wal.Available.Add(spot.Available).Equal(decimal.RequireFromString("30")) {
		  t.Fatalf("MAIN after concurrent cover+park wal=%s spot=%s", wal.Available, spot.Available)
	  }
   }

  func TestCoverSweeperClearsWithoutSameKeyRetry(t *testing.T) {
	  rt, err := app.OpenMem()
	   if err != nil {
		   t.Fatal(err)
	   }
	   ctx := context.Background()
	   uid := "u-cover-sweep"
	   mem := rt.Store.(*ledger.MemStore)
	   mem.Seed(uid, ledger.AccountWallet, "USDT", "0")
	  mem.Seed(uid, ledger.AccountSpot, "USDT", "40")
	  mem.Seed("system", ledger.AccountBridge, "USDT", "1000")
	   to := wallet.DeriveAddress(ledger.ExtWithdrawUser, "ETH")
	  rt.Ledger.SetCoverUndoFaults(5)
	  _, err = rt.Ledger.Create(ctx, ledger.CreateReq{
		   UserID: uid, Direction: ledger.WalletWithdraw, Asset: "USDT",
		   Amount: decimal.RequireFromString("8"), Chain: "ETH", ToAddress: to,
		   IdempotencyKey: "cover-sweep-1",
	  })
	  if !errors.Is(err, ledger.ErrCoverStranded) {
		   t.Fatalf("want stranded %v", err)
	  }
	   rt.Ledger.SetCoverUndoFaults(0)
	   // 清扫 minAge 未设时默认 lockTTL+偏斜（约 14s）；单测压低并略等，避免全包顺序跑时误判 young。
	  rt.Ledger.SetCoverSweepMinAge(time.Microsecond)
	  time.Sleep(5 * time.Millisecond)
	  var alerts int
	   rt.Ledger.OnAlert = func(kind, detail string) {
		  if kind == "cover_stranded" {
			  alerts++
		   }
	   }
	  rep, err := rt.Ledger.SweepStrandedCovers(ctx, 50)
	   if err != nil || rep.Undone < 1 {
		  t.Fatalf("sweep %+v %v", rep, err)
	  }
	   wal, _ := rt.Store.GetAccount(ctx, uid, ledger.AccountWallet, "USDT")
	  spot, _ := rt.Store.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	  if !wal.Available.IsZero() || !spot.Available.Equal(decimal.RequireFromString("40")) {
		   t.Fatalf("after sweep wal=%s spot=%s", wal.Available, spot.Available)
	   }
	  _ = alerts
   }

  func TestFundsReconHookRecoversHopAndCover(t *testing.T) {
	  rt, err := app.OpenMem()
	  if err != nil {
		  t.Fatal(err)
	   }
	  kcfund.BindFundsRecon(rt)
	   if rt.FundsRecon == nil {
		   t.Fatal("hook")
	   }
	   ctx := context.Background()
	  uid := rt.Cfg.SeedUserID
	  svc := subacct.NewWithKV(rt.KV)
	   p, err := svc.OpenParent(subacct.OpenParentReq{UserID: uid, Label: "primary"})
	   if err != nil {
		   t.Fatal(err)
	  }
	  child, err := svc.OpenChild(subacct.OpenChildReq{ParentID: p.ID, UserID: "child-hook-1", Label: "desk"})
	  if err != nil {
		   t.Fatal(err)
	  }
	   _, err = kcfund.HopFunds(ctx, rt.Ledger, svc, kcfund.HopFundsReq{
		  UserID: uid, Kind: "PARENT_TO_SUB", ClientOid: "hop-hook-1",
		  Currency: "USDT", Amount: decimal.RequireFromString("1"), AmountRaw: "1",
		  FromOwner: p.ID, ToOwner: child.ID, Fault: "after_ledger_out",
	   })
	   if !errors.Is(err, kcfund.ErrHopCrash) {
		   t.Fatalf("want crash %v", err)
	   }
	   rt.FundsRecon(ctx)
	   fresh := subacct.NewWithKV(rt.KV)
	  cb, err := fresh.BookOf(child.ID, "USDT")
	   if err != nil || cb.Available != "1" {
		  t.Fatalf("hook recover %v %+v", err, cb)
	   }
  }

   func TestHopSagaCrashRecoverCompletes(t *testing.T) {
	  rt, err := app.OpenMem()
	  if err != nil {
		  t.Fatal(err)
	  }
	  ctx := context.Background()
	  uid := rt.Cfg.SeedUserID
	  svc := subacct.New()
	  p, err := svc.OpenParent(subacct.OpenParentReq{UserID: uid, Label: "primary"})
	  if err != nil {
		  t.Fatal(err)
	  }
	  child, err := svc.OpenChild(subacct.OpenChildReq{
		  ParentID: p.ID, UserID: "child-crash-1", Label: "desk",
	  })
	   if err != nil {
		  t.Fatal(err)
	  }
	   spot0, _ := rt.Store.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	   _, err = kcfund.HopFunds(ctx, rt.Ledger, svc, kcfund.HopFundsReq{
		  UserID: uid, Kind: "PARENT_TO_SUB", ClientOid: "hop-crash-1",
		  Currency: "USDT", Amount: decimal.RequireFromString("2"), AmountRaw: "2",
		  FromOwner: p.ID, ToOwner: child.ID, Fault: "after_ledger_out",
	   })
	   if !errors.Is(err, kcfund.ErrHopCrash) {
		  t.Fatalf("want crash %v", err)
	   }
	   spot, _ := rt.Store.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	   if !spot.Available.Equal(spot0.Available.Sub(decimal.RequireFromString("2"))) {
		  t.Fatalf("after crash funding %s", spot.Available)
	  }
	   hop, err := kcfund.RecoverHop(ctx, rt.Ledger, svc, "hop-crash-1")
	   if err != nil || hop.ID == "" {
		  t.Fatalf("recover %v %+v", err, hop)
	   }
	   cb, err := svc.BookOf(child.ID, "USDT")
	  if err != nil || cb.Available != "2" {
		  t.Fatalf("child after recover %v %+v", err, cb)
	  }
	   spot, _ = rt.Store.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	   if !spot.Available.Equal(spot0.Available.Sub(decimal.RequireFromString("2"))) {
		  t.Fatalf("double debit after recover %s", spot.Available)
	  }
  }

  func TestHopSagaAbortingUsesCompPhaseNotHopped(t *testing.T) {
	  rt, err := app.OpenMem()
	  if err != nil {
		   t.Fatal(err)
	  }
	  ctx := context.Background()
	  uid := rt.Cfg.SeedUserID
	   svc := subacct.New()
	  p, err := svc.OpenParent(subacct.OpenParentReq{UserID: uid, Label: "primary"})
	   if err != nil {
		   t.Fatal(err)
	  }
	   child, err := svc.OpenChild(subacct.OpenChildReq{ParentID: p.ID, UserID: "child-abort-1", Label: "desk"})
	  if err != nil {
		   t.Fatal(err)
	  }
	   spot0, _ := rt.Store.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	  _, err = kcfund.HopFunds(ctx, rt.Ledger, svc, kcfund.HopFundsReq{
		  UserID: uid, Kind: "PARENT_TO_SUB", ClientOid: "hop-abort-1",
		  Currency: "USDT", Amount: decimal.RequireFromString("3"), AmountRaw: "3",
		  FromOwner: p.ID, ToOwner: child.ID, Fault: "after_aborting",
	  })
	   if !errors.Is(err, kcfund.ErrHopCrash) {
		   t.Fatalf("want abort crash %v", err)
	   }
	  _, err = kcfund.RecoverHop(ctx, rt.Ledger, svc, "hop-abort-1")
	   if !errors.Is(err, kcfund.ErrHopCompensate) {
		  t.Fatalf("recover abort %v", err)
	   }
	  spot, _ := rt.Store.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	   if !spot.Available.Equal(spot0.Available) {
		  t.Fatalf("ledger must restore after abort recover %s want %s", spot.Available, spot0.Available)
	   }
	   pb, _ := svc.BookOf(p.ID, "USDT")
	  cb, _ := svc.BookOf(child.ID, "USDT")
	   if pb.Available != "0" || cb.Available != "0" {
		   t.Fatalf("overlay must stay empty parent=%s child=%s", pb.Available, cb.Available)
	  }
   }

  func TestHopSagaProcessRestartHydrateRecover(t *testing.T) {
	   rt, err := app.OpenMem()
	  if err != nil {
		  t.Fatal(err)
	  }
	  ctx := context.Background()
	   uid := rt.Cfg.SeedUserID
	   svc := subacct.New()
	   kv := svc.KV()
	  p, err := svc.OpenParent(subacct.OpenParentReq{UserID: uid, Label: "primary"})
	   if err != nil {
		   t.Fatal(err)
	   }
	   child, err := svc.OpenChild(subacct.OpenChildReq{ParentID: p.ID, UserID: "child-restart-1", Label: "desk"})
	  if err != nil {
		   t.Fatal(err)
	  }
	  _, err = kcfund.HopFunds(ctx, rt.Ledger, svc, kcfund.HopFundsReq{
		  UserID: uid, Kind: "PARENT_TO_SUB", ClientOid: "hop-restart-1",
		   Currency: "USDT", Amount: decimal.RequireFromString("2"), AmountRaw: "2",
		   FromOwner: p.ID, ToOwner: child.ID, Fault: "after_ledger_out",
	   })
	  if !errors.Is(err, kcfund.ErrHopCrash) {
		   t.Fatalf("want crash %v", err)
	   }
	  fresh := subacct.NewWithKV(kv)
	   hop, err := kcfund.RecoverHop(ctx, rt.Ledger, fresh, "hop-restart-1")
	   if err != nil || hop.ID == "" {
		  t.Fatalf("restart recover %v %+v", err, hop)
	  }
	  cb, err := fresh.BookOf(child.ID, "USDT")
	  if err != nil || cb.Available != "2" {
		   t.Fatalf("child after restart %v %+v", err, cb)
	   }
   }

  func TestRecoverOpenHopsSweepsIncomplete(t *testing.T) {
	   rt, err := app.OpenMem()
	  if err != nil {
		  t.Fatal(err)
	  }
	   ctx := context.Background()
	   uid := rt.Cfg.SeedUserID
	  svc := subacct.New()
	   p, err := svc.OpenParent(subacct.OpenParentReq{UserID: uid, Label: "primary"})
	   if err != nil {
		  t.Fatal(err)
	   }
	   child, err := svc.OpenChild(subacct.OpenChildReq{ParentID: p.ID, UserID: "child-sweep-1", Label: "desk"})
	   if err != nil {
		   t.Fatal(err)
	  }
	  _,
		  err = kcfund.HopFunds(ctx, rt.Ledger, svc, kcfund.HopFundsReq{
		  UserID: uid, Kind: "PARENT_TO_SUB", ClientOid: "hop-sweep-1",
		   Currency: "USDT", Amount: decimal.RequireFromString("1"), AmountRaw: "1",
		   FromOwner: p.ID, ToOwner: child.ID, Fault: "after_ledger_out",
	  })
	  if !errors.Is(err, kcfund.ErrHopCrash) {
		   t.Fatalf("want crash %v", err)
	  }
	   reps, err := kcfund.RecoverOpenHops(ctx, rt.Ledger, svc, 50)
	   if err != nil {
		  t.Fatal(err)
	  }
	  found := false
	   for _, r := range reps {
		   if r.ClientOid == "hop-sweep-1" && r.HopID != "" && r.Err == "" {
			  found = true
		   }
	  }
	   if !found {
		   t.Fatalf("sweep %+v", reps)
	  }
	  cb, err := svc.BookOf(child.ID, "USDT")
	   if err != nil || cb.Available != "1" {
		  t.Fatalf("child after sweep %v %+v", err, cb)
	  }
  }
