package flow

import (
	"context"
	"strings"
	"testing"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/wallet"
	"github.com/shopspring/decimal"
)

  func TestReconcileJournalNetExceedsBook(t *testing.T) {
	   rt, err := app.OpenMem()
	  if err != nil {
		   t.Fatal(err)
	   }
	   ctx := context.Background()
	   uid := "u-recon-journal"
	  if _, err := rt.Ledger.Create(ctx, ledger.CreateReq{
		  UserID: uid, Direction: ledger.FiatOnramp, Asset: "USDT",
		  Amount: decimal.RequireFromString("100"), IdempotencyKey: "recon-on-1",
	   }); err != nil {
		   t.Fatal(err)
	   }
	  rep, err := ledger.Reconcile(ctx, rt.Store, uid)
	   if err != nil || !rep.OK {
		  t.Fatalf("clean %+v %v", rep, err)
	  }
	   acc, err := rt.Store.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	  if err != nil {
		  t.Fatal(err)
	   }
	  acc.Available = acc.Available.Sub(decimal.RequireFromString("40"))
	  if err := rt.Store.UpsertAccount(ctx, acc); err != nil {
		  t.Fatal(err)
	   }
	  rep, err = ledger.Reconcile(ctx, rt.Store, uid)
	  if err != nil {
		  t.Fatal(err)
	  }
	   if rep.OK {
		   t.Fatal("silent book debit must fail reconcile")
	  }
	   found := false
	  for _, m := range rep.Mismatches {
		  if strings.Contains(m.Note, "journal net exceeds book") {
			   found = true
		   }
	   }
	   if !found {
		  t.Fatalf("mismatches %+v", rep.Mismatches)
	   }
   }

  func TestReconcileSeedUserStillOK(t *testing.T) {
	   rt, err := app.OpenMem()
	  if err != nil {
		   t.Fatal(err)
	  }
	  rep, err := ledger.Reconcile(context.Background(), rt.Store, rt.Cfg.SeedUserID)
	  if err != nil || !rep.OK {
		  t.Fatalf("seed %+v %v", rep, err)
	   }
  }

  func TestCoverFundingFromWallet(t *testing.T) {
	  rt, err := app.OpenMem()
	   if err != nil {
		  t.Fatal(err)
	   }
	   ctx := context.Background()
	   uid := rt.Cfg.SeedUserID
	   if err := rt.Ledger.CoverFundingFromWallet(ctx, uid, "USDT", decimal.RequireFromString("11000"), "cover-1"); err != nil {
		  t.Fatal(err)
	  }
	  spot, err := rt.Store.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	   if err != nil {
		  t.Fatal(err)
	   }
	  if !spot.Available.Equal(decimal.RequireFromString("11000")) {
		  t.Fatalf("spot after cover %s", spot.Available)
	   }
	   wal, err := rt.Store.GetAccount(ctx, uid, ledger.AccountWallet, "USDT")
	  if err != nil {
		  t.Fatal(err)
	   }
	   if !wal.Available.Equal(decimal.RequireFromString("1000")) {
		  t.Fatalf("wallet after cover %s", wal.Available)
	  }
	  if err := rt.Ledger.CoverFundingFromWallet(ctx, uid, "USDT", decimal.RequireFromString("11000"), "cover-1"); err != nil {
		  t.Fatal(err)
	  }
   }

   func TestCoverFundingFromTradingAndIncomingInner(t *testing.T) {
	  rt, err := app.OpenMem()
	   if err != nil {
		  t.Fatal(err)
	   }
	  ctx := context.Background()
	   uid := rt.Cfg.SeedUserID
	  if _,
		  err := rt.Ledger.Create(ctx, ledger.CreateReq{
		   UserID: uid, Direction: ledger.FundingToTrading, Asset: "USDT",
		  Amount: decimal.RequireFromString("40"), IdempotencyKey: "gap-hop-1",
	  }); err != nil {
		  t.Fatal(err)
	  }
	   if err := rt.Ledger.CoverFundingFromTrading(ctx, uid, "USDT", decimal.RequireFromString("10020"), "gap-trade-cover"); err != nil {
		  t.Fatal(err)
	   }
	   spot, err := rt.Store.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	   if err != nil {
		   t.Fatal(err)
	  }
	  if !spot.Available.Equal(decimal.RequireFromString("10000")) {
		   t.Fatalf("spot after trade cover %s", spot.Available)
	  }
	  peer := "user-10002"
	   if _, err := rt.Ledger.Create(ctx, ledger.CreateReq{
		  UserID: peer, Direction: ledger.SpotSend, Asset: "USDT",
		   Amount: decimal.RequireFromString("5"), ToAddress: uid, IdempotencyKey: "gap-inner-1",
	   }); err != nil {
		   t.Fatal(err)
	   }
	  got, err := rt.Ledger.ListIncomingInner(ctx, uid, 20)
	  if err != nil || len(got) == 0 {
		   t.Fatalf("incoming %v %d", err, len(got))
	   }
   }

   func TestFrozenUsesWalletWithdrawReason(t *testing.T) {
	  rt, err := app.OpenMem()
	   if err != nil {
		   t.Fatal(err)
	   }
	  uid := rt.Cfg.SeedUserID
	   to := wallet.DeriveAddress(ledger.ExtWithdrawUser, "ETH")
	  tr, err := rt.Ledger.Create(context.Background(), ledger.CreateReq{
		  UserID: uid, Direction: ledger.WalletWithdraw, Asset: "USDT",
		  Amount: decimal.RequireFromString("1.1"), Chain: "ETH", ToAddress: to,
		   IdempotencyKey: "surf-wd-1",
	  })
	   if err != nil {
		  t.Fatal(err)
	  }
	   if !tr.State.Open() {
		   t.Fatalf("want open freeze %s", tr.State)
	  }
	   rows, err := rt.FundSurf.Frozen(context.Background(), uid)
	  if err != nil {
		   t.Fatal(err)
	  }
	   found := false
	  for _, r := range rows {
		   if r.Asset == "USDT" && r.Book == "wallet" && r.Reason == holdcat.ReasonPendingWithdraw {
			   found = true
		  }
	   }
	  if !found {
		   t.Fatalf("frozen %+v", rows)
	   }
  }
