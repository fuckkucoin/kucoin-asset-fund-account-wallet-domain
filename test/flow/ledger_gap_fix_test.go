package flow

import (
	"context"
	"testing"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/earn"
	"account-assets.kucoin.net/funding/internal/fxmark"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/p2pgate"
	"account-assets.kucoin.net/funding/internal/subacct"
	"account-assets.kucoin.net/funding/internal/wallet"
	"github.com/shopspring/decimal"
)

   func TestConvertCreditFailRestoresDebit(t *testing.T) {
	  rt, err := app.OpenMem()
	  if err != nil {
		   t.Fatal(err)
	  }
	   ctx := context.Background()
	  uid := rt.Cfg.SeedUserID
	   spot, err := rt.Store.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	   if err != nil {
		  t.Fatal(err)
	  }
	  before := spot.Available
	   if _, err := rt.Convert.Place(uid, "USDT", "USDC", "10", "1", "funding"); err == nil {
		  t.Fatal("want credit fail without USDC bridge")
	   }
	  spot, err = rt.Store.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	  if err != nil {
		  t.Fatal(err)
	  }
	  if !spot.Available.Equal(before) {
		   t.Fatalf("debit not restored %s want %s", spot.Available, before)
	  }
  }

   func TestLimitFillCapsToSize(t *testing.T) {
	  rt, err := app.OpenMem()
	   if err != nil {
		   t.Fatal(err)
	  }
	   ctx := context.Background()
	   if mem, ok := rt.Store.(*ledger.MemStore); ok {
		   mem.Seed("system", ledger.AccountBridge, "USDC", "10000")
	   }
	   open, err := rt.Convert.PlaceLimit(ctx, rt.Cfg.SeedUserID, "lim-cap-1", "USDT", "USDC", "10", "11", "MAIN")
	  if err != nil || open.Status != "OPEN" {
		   t.Fatalf("open %v %+v", err, open)
	   }
	  if err := fxmark.New(rt.KV).Put(ctx, "USDC", "0.8"); err != nil {
		  t.Fatal(err)
	   }
	   got, err := rt.Convert.GetLimit(ctx, rt.Cfg.SeedUserID, open.ID, "")
	   if err != nil || got.Status != "FILLED" {
		  t.Fatalf("fill %v %+v", err, got)
	  }
	   if got.ToSize != "11" {
		  t.Fatalf("cap toSize %s", got.ToSize)
	   }
   }

  func TestEarnClaimCreditsFunding(t *testing.T) {
	  rt, err := app.OpenMem()
	  if err != nil {
		   t.Fatal(err)
	  }
	   ctx := context.Background()
	  uid := rt.Cfg.SeedUserID
	  svc := earn.New(rt.Ledger).BindKV(rt.KV)
	   pos, err := svc.Subscribe(ctx, earn.SubscribeReq{UserID: uid, ProductID: "flex-usdt", Asset: "USDT", Amount: "20", IdempotencyKey: "gap-earn-1"})
	  if err != nil {
		  t.Fatal(err)
	  }
	   if _, err := svc.AccrueUser(uid); err != nil {
		  t.Fatal(err)
	  }
	   spot, err := rt.Store.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	  if err != nil {
		   t.Fatal(err)
	   }
	  before := spot.Available
	   cl, err := svc.Claim(pos.ID)
	  if err != nil || cl.Amount == "0" {
		  t.Fatalf("claim %v %+v", err, cl)
	   }
	   spot, err = rt.Store.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	  if err != nil {
		   t.Fatal(err)
	  }
	  if !spot.Available.GreaterThan(before) {
		  t.Fatalf("claim must credit funding %s -> %s", before, spot.Available)
	  }
	   if _, err := svc.Claim(pos.ID); err != nil {
		  t.Fatal(err)
	   }
	  spot2, err := rt.Store.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	  if err != nil {
		   t.Fatal(err)
	   }
	  if !spot2.Available.Equal(spot.Available) {
		   t.Fatalf("second claim minted %s -> %s", spot.Available, spot2.Available)
	   }
   }

   func TestSubOverlayCreditIdempotent(t *testing.T) {
	  s := subacct.New()
	  p, err := s.OpenParent(subacct.OpenParentReq{UserID: config.DefaultSeedUserID, Label: "primary"})
	  if err != nil {
		  t.Fatal(err)
	   }
	  a, err := s.Credit(subacct.PostReq{OwnerID: p.ID, Asset: "USDT", Amount: "7", IdempotencyKey: "ov-1"})
	  if err != nil {
		   t.Fatal(err)
	   }
	   b, err := s.Credit(subacct.PostReq{OwnerID: p.ID, Asset: "USDT", Amount: "7", IdempotencyKey: "ov-1"})
	  if err != nil {
		   t.Fatal(err)
	  }
	  if a.Available != b.Available {
		  t.Fatalf("idem credit %s %s", a.Available, b.Available)
	  }
	   got, err := s.BookOf(p.ID, "USDT")
	  if err != nil || got.Available != "7" {
		  t.Fatalf("book %v %+v", err, got)
	   }
  }

   func TestEnsureFundingRejectsShort(t *testing.T) {
	  rt, err := app.OpenMem()
	  if err != nil {
		   t.Fatal(err)
	   }
	  if err := rt.Ledger.EnsureFunding(context.Background(), rt.Cfg.SeedUserID, "USDT", decimal.RequireFromString("999999"), "ens-1", false); err != ledger.ErrInsufficient {
		   t.Fatalf("short %v", err)
	   }
  }

   func TestConvertDebitCoversWalletAndBizType(t *testing.T) {
	  rt, err := app.OpenMem()
	   if err != nil {
		  t.Fatal(err)
	  }
	   ctx := context.Background()
	  uid := "u-cv-cover"
	  mem,
		   ok := rt.Store.(*ledger.MemStore)
	  if !ok {
		   t.Fatal("mem store")
	   }
	   mem.Seed(uid, ledger.AccountWallet, "USDT", "40")
	  mem.Seed(uid, ledger.AccountSpot, "USDT", "0")
	  mem.Seed("system", ledger.AccountBridge, "USDC", "1000")
	  ord, err := rt.Convert.Place(uid, "USDT", "USDC", "10", "1", "funding")
	  if err != nil || ord.Status != "filled" {
		  t.Fatalf("place %v %+v", err, ord)
	  }
	  tr,
		   err := rt.Ledger.Get(ctx, ord.DebitID)
	  if err != nil || tr.Direction != ledger.ConvertDebit {
		  t.Fatalf("debit biz %v %+v", err, tr)
	  }
	  wal, err := rt.Store.GetAccount(ctx, uid, ledger.AccountWallet, "USDT")
	  if err != nil || !wal.Available.Equal(decimal.RequireFromString("30")) {
		  t.Fatalf("wallet after cover+debit %v %s", err, wal.Available)
	   }
	   spot, err := rt.Store.GetAccount(ctx, uid, ledger.AccountSpot, "USDC")
	   if err != nil || !spot.Available.Equal(decimal.RequireFromString("10")) {
		   t.Fatalf("usdc %v %s", err, spot.Available)
	  }
   }

  func TestSubOverlayForgetAllowsRetryAfterUndo(t *testing.T) {
	   s := subacct.New()
	   p, err := s.OpenParent(subacct.OpenParentReq{UserID: config.DefaultSeedUserID, Label: "primary"})
	   if err != nil {
		   t.Fatal(err)
	  }
	   if _, err := s.Credit(subacct.PostReq{OwnerID: p.ID, Asset: "USDT", Amount: "9", IdempotencyKey: "ov-retry"}); err != nil {
		  t.Fatal(err)
	  }
	   if _, err := s.Debit(subacct.PostReq{OwnerID: p.ID, Asset: "USDT", Amount: "9", IdempotencyKey: "ov-undo"}); err != nil {
		  t.Fatal(err)
	   }
	  s.ForgetPost("credit", subacct.PostReq{OwnerID: p.ID, IdempotencyKey: "ov-retry"})
	  if _, err := s.Credit(subacct.PostReq{OwnerID: p.ID, Asset: "USDT", Amount: "9", IdempotencyKey: "ov-retry"}); err != nil {
		   t.Fatal(err)
	   }
	   got, err := s.BookOf(p.ID, "USDT")
	   if err != nil || got.Available != "9" {
		   t.Fatalf("retry after forget %v %+v", err, got)
	   }
   }

   func TestInboundEarnLandsFundingNotEarnBook(t *testing.T) {
	  rt, err := app.OpenMem()
	  if err != nil {
		  t.Fatal(err)
	  }
	   ctx := context.Background()
	  uid := rt.Cfg.SeedUserID
	   if err := rt.FundSurf.SetDepositTo(ctx, uid, "ETH", "EARN"); err != nil {
		  t.Fatal(err)
	   }
	   spot, err := rt.Store.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	   if err != nil {
		   t.Fatal(err)
	   }
	  beforeSpot, beforeEarn := spot.Available, decimal.Zero
	  if earn, err := rt.Store.GetAccount(ctx, uid, ledger.AccountEarn, "USDT"); err == nil {
		   beforeEarn = earn.Available
	   }
	   if err := rt.FundSurf.ApplyInbound(ctx, uid, "ETH", "USDT", "6", "in-earn-1"); err != nil {
		  t.Fatal(err)
	   }
	   spot, err = rt.Store.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	   if err != nil {
		  t.Fatal(err)
	   }
	  if !spot.Available.Equal(beforeSpot.Add(decimal.RequireFromString("6"))) {
		  t.Fatalf("funding %s want +6 from %s", spot.Available, beforeSpot)
	  }
	  earn, err := rt.Store.GetAccount(ctx, uid, ledger.AccountEarn, "USDT")
	   if err != nil {
		  t.Fatal(err)
	   }
	   if !earn.Available.Equal(beforeEarn) {
		  t.Fatalf("earn book changed %s -> %s", beforeEarn, earn.Available)
	  }
  }

  func TestWithdrawCoversFromFunding(t *testing.T) {
	  rt, err := app.OpenMem()
	  if err != nil {
		   t.Fatal(err)
	   }
	   ctx := context.Background()
	  uid := "u-wd-cover"
	  mem, ok := rt.Store.(*ledger.MemStore)
	  if !ok {
		  t.Fatal("mem store")
	  }
	  mem.Seed(uid, ledger.AccountWallet, "USDT", "0")
	   mem.Seed(uid, ledger.AccountSpot, "USDT", "40")
	   mem.Seed("system", ledger.AccountBridge, "USDT", "1000")
	   to := wallet.DeriveAddress(ledger.ExtWithdrawUser, "ETH")
	   mem.Allow(uid, "ETH", to)
	   mem.Allow(uid, "EVM", to)
	   tr, err := rt.Ledger.Create(ctx, ledger.CreateReq{
		  UserID: uid, Direction: ledger.WalletWithdraw, Asset: "USDT",
		  Amount: decimal.RequireFromString("10"), Chain: "ETH", ToAddress: to,
		   IdempotencyKey: "wd-cover-1",
	  })
	   if err != nil && tr == nil {
		   t.Fatal(err)
	   }
	   if tr.State == ledger.StateFailed || tr.State == ledger.StateRolledBack {
		  t.Fatalf("cover withdraw %s %s", tr.State, tr.FailReason)
	  }
	   wal, err := rt.Store.GetAccount(ctx, uid, ledger.AccountWallet, "USDT")
	   if err != nil {
		   t.Fatal(err)
	  }
	  if wal.Frozen.LessThan(decimal.RequireFromString("10")) && wal.Available.IsZero() && tr.State != ledger.StateCredited {
		  t.Fatalf("wallet not covered %+v frozen=%s avail=%s", tr, wal.Frozen, wal.Available)
	  }
	  spot, err := rt.Store.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	  if err != nil {
		   t.Fatal(err)
	   }
	  if !spot.Available.Equal(decimal.RequireFromString("30")) {
		  t.Fatalf("spot leftover %s", spot.Available)
	   }
  }

   func TestP2PPostsP2PDirections(t *testing.T) {
	   rt, err := app.OpenMem()
	   if err != nil {
		  t.Fatal(err)
	  }
	  ctx := context.Background()
	  uid := rt.Cfg.SeedUserID
	  f := p2pgate.NewFunds(rt.Ledger, holdcat.New(rt.KV))
	   buy, err := f.Buy(ctx, uid, "USDT", "5", "biz-p2p-1")
	   if err != nil || buy.Direction != ledger.P2PCredit {
		  t.Fatalf("buy %v %+v", err, buy)
	  }
	   row, err := f.SellHold(ctx, uid, "USDT", "3", "biz-p2p-2")
	  if err != nil {
		  t.Fatal(err)
	  }
	  list, err := rt.Ledger.List(ctx, uid, 50)
	   if err != nil {
		   t.Fatal(err)
	  }
	  found := false
	  for _, x := range list {
		  if x != nil && x.IdempotencyKey == "p2p-sell:biz-p2p-2" && x.Direction == ledger.P2PDebit {
			   found = true
		   }
	   }
	   if !found {
		   t.Fatalf("p2p debit missing %+v hold=%s", list, row.ID)
	   }
   }
