package flow

import (
	"context"
	"errors"
	"strings"
	"testing"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/spot"
	"account-assets.kucoin.net/funding/internal/swap"
)

  func TestSpotWalletRoundTripAndIdempotency(t *testing.T) {
	  rt := openMem(t)
	  ctx := context.Background()
	   uid := seedUser(rt)
	  before, err := rt.Store.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	  if err != nil {
		  t.Fatal(err)
	   }
	  tr, err := rt.Ledger.Create(ctx, ledger.CreateReq{
		   UserID: uid, Direction: ledger.SpotToWallet, Asset: "USDT",
		  Amount: decimal.RequireFromString("12.5"), Chain: "ETH",
		   ToAddress: dest(uid, "ETH"), IdempotencyKey: "flow-s2w", WaitConfirm: true,
	   })
	   if err != nil || tr.State != ledger.StateCredited {
		  t.Fatalf("spot_to_wallet %v %+v", err, tr)
	  }
	  again, err := rt.Ledger.Create(ctx, ledger.CreateReq{
		   UserID: uid, Direction: ledger.SpotToWallet, Asset: "USDT",
		   Amount: decimal.RequireFromString("12.5"), Chain: "ETH",
		   ToAddress: dest(uid, "ETH"), IdempotencyKey: "flow-s2w", WaitConfirm: true,
	   })
	  if err != nil || again.ID != tr.ID {
		   t.Fatalf("idempotent resume %v %+v", err, again)
	  }
	  after, err := rt.Store.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	   if err != nil {
		   t.Fatal(err)
	  }
	   if !before.Available.Sub(after.Available).Equal(decimal.RequireFromString("12.5")) {
		   t.Fatalf("spot moved %s -> %s", before.Available, after.Available)
	  }
	  if !after.Frozen.IsZero() {
		  t.Fatalf("spot leftover frozen %s", after.Frozen)
	  }
	  wal, err := rt.Store.GetAccount(ctx, uid, ledger.AccountWallet, "USDT")
	   if err != nil {
		   t.Fatal(err)
	  }
	   if !wal.Available.Equal(decimal.RequireFromString("2012.5")) || !wal.Frozen.IsZero() {
		  t.Fatalf("wallet after s2w avail=%s frozen=%s", wal.Available, wal.Frozen)
	  }
	   if tr.State != ledger.StateCredited || tr.State.AppStatus() != "success" {
		   t.Fatalf("s2w status %s %s", tr.State, tr.State.AppStatus())
	   }
	  booksOK(t, rt, uid)
	  back, err := rt.Ledger.Create(ctx, ledger.CreateReq{
		   UserID: uid, Direction: ledger.WalletToSpot, Asset: "USDT",
		  Amount: decimal.RequireFromString("3"), Chain: "ETH",
		  IdempotencyKey: "flow-w2s", WaitConfirm: true,
	   })
	   if err != nil || back.State != ledger.StateCredited {
		  t.Fatalf("wallet_to_spot %v %+v", err, back)
	  }
	   spot2, err := rt.Store.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	   if err != nil {
		  t.Fatal(err)
	  }
	   wal2, err := rt.Store.GetAccount(ctx, uid, ledger.AccountWallet, "USDT")
	  if err != nil {
		  t.Fatal(err)
	  }
	   if !spot2.Available.Equal(after.Available.Add(decimal.RequireFromString("3"))) || !spot2.Frozen.IsZero() {
		   t.Fatalf("spot after w2s %s frozen %s", spot2.Available, spot2.Frozen)
	  }
	   if !wal2.Available.Equal(wal.Available.Sub(decimal.RequireFromString("3"))) || !wal2.Frozen.IsZero() {
		   t.Fatalf("wallet after w2s %s frozen %s", wal2.Available, wal2.Frozen)
	   }
	   booksOK(t, rt, uid)
   }

   func TestWithdrawSubmitFailRestoresBooks(t *testing.T) {
	  rt := openMem(t)
	  ctx := context.Background()
	   uid := seedUser(rt)
	   before, err := rt.Store.GetAccount(ctx, uid, ledger.AccountWallet, "USDT")
	  if err != nil {
		  t.Fatal(err)
	   }
	  rt.Wallet.Registry().FailNext("ETH", 8)
	  tr, err := rt.Ledger.Create(ctx, ledger.CreateReq{
		  UserID: uid, Direction: ledger.WalletWithdraw, Asset: "USDT",
		   Amount: decimal.RequireFromString("1.5"), Chain: "ETH",
		   ToAddress: extDest("ETH"), IdempotencyKey: "flow-wd-fail",
		   WaitConfirm: true,
	  })
	   if err == nil {
		   t.Fatalf("want submit fail got %+v", tr)
	   }
	  after, err := rt.Store.GetAccount(ctx, uid, ledger.AccountWallet, "USDT")
	   if err != nil {
		   t.Fatal(err)
	   }
	   if !after.Available.Equal(before.Available) || !after.Frozen.IsZero() {
		   t.Fatalf("fail left books avail=%s frozen=%s", after.Available, after.Frozen)
	  }
	   if tr != nil && tr.TxHash != "" && tr.State != ledger.StateRolledBack && tr.State != ledger.StateFailed {
		   t.Fatalf("fail state %+v", tr)
	  }
	  booksOK(t, rt, uid)
  }

  func TestWithdrawGatesAndCredit(t *testing.T) {
	   rt := openMem(t)
	   ctx := context.Background()
	  uid := seedUser(rt)
	   _, err := rt.Ledger.Create(ctx, ledger.CreateReq{
		   UserID: uid, Direction: ledger.WalletWithdraw, Asset: "USDT",
		   Amount: decimal.RequireFromString("1"), Chain: "ETH",
		   ToAddress: "0xabc", IdempotencyKey: "flow-bad-fmt",
	   })
	  if !errors.Is(err, ledger.ErrAddressFormat) {
		   t.Fatalf("format %v", err)
	  }
	  _, err = rt.Ledger.Create(ctx, ledger.CreateReq{
		  UserID: uid, Direction: ledger.WalletWithdraw, Asset: "USDT",
		  Amount: decimal.RequireFromString("1"), Chain: "ETH",
		   ToAddress: dest("blocked", "ETH"), IdempotencyKey: "flow-deny",
	  })
	  if !errors.Is(err, ledger.ErrRiskRejected) {
		   t.Fatalf("allowlist %v", err)
	  }
	  _, err = rt.Ledger.Create(ctx, ledger.CreateReq{
		   UserID: uid, Direction: ledger.WalletWithdraw, Asset: "USDT",
		   Amount: decimal.RequireFromString("1"), Chain: "BTC",
		  ToAddress: dest(uid, "BTC"), IdempotencyKey: "flow-asset",
	  })
	  if !errors.Is(err, ledger.ErrAssetChain) {
		   t.Fatalf("asset/chain %v", err)
	  }
	  wal, err := rt.Store.GetAccount(ctx, uid, ledger.AccountWallet, "USDT")
	   if err != nil {
		  t.Fatal(err)
	  }
	  _, err = rt.Ledger.Create(ctx, ledger.CreateReq{
		  UserID: uid, Direction: ledger.WalletWithdraw, Asset: "USDT",
		  Amount: decimal.RequireFromString("1.5"), Chain: "ETH",
		   ToAddress: dest(uid, "ETH"), IdempotencyKey: "flow-own-in",
	   })
	   if !errors.Is(err, ledger.ErrOwnInbound) {
		   t.Fatalf("own inbound %v", err)
	   }
	   tr, err := rt.Ledger.Create(ctx, ledger.CreateReq{
		   UserID: uid, Direction: ledger.WalletWithdraw, Asset: "USDT",
		  Amount: decimal.RequireFromString("1.5"), Chain: "ETH",
		  ToAddress: extDest("ETH"), IdempotencyKey: "flow-wd", WaitConfirm: true,
	  })
	  if err != nil || tr.State != ledger.StateCredited {
		   t.Fatalf("withdraw %v %+v", err, tr)
	   }
	  after, err := rt.Store.GetAccount(ctx, uid, ledger.AccountWallet, "USDT")
	  if err != nil {
		  t.Fatal(err)
	   }
	   if !after.Available.LessThan(wal.Available) || !after.Frozen.IsZero() {
		   t.Fatalf("wallet after withdraw avail=%s frozen=%s", after.Available, after.Frozen)
	  }
   }

  func TestDepositOwnershipAndReconcile(t *testing.T) {
	  rt := openMem(t)
	   ctx := context.Background()
	   uid := seedUser(rt)
	   _, err := rt.Ledger.Create(ctx, ledger.CreateReq{
		   UserID: uid, Direction: ledger.WalletDeposit, Asset: "USDT",
		   Amount: decimal.RequireFromString("2"), Chain: "ETH",
		  ToAddress: dest("other", "ETH"), TxHash: "0xdep-foreign",
		   IdempotencyKey: "flow-dep-bad",
	  })
	  if !errors.Is(err, ledger.ErrDepositAddress) {
		  t.Fatalf("foreign deposit %v", err)
	  }
	  tr, err := rt.Ledger.Create(ctx, ledger.CreateReq{
		   UserID: uid, Direction: ledger.WalletDeposit, Asset: "USDT",
		  Amount: decimal.RequireFromString("2"), Chain: "ETH",
		  IdempotencyKey: "flow-dep-ok", WaitConfirm: true,
	  })
	   if err != nil || tr.State != ledger.StateCredited {
		  t.Fatalf("deposit %v %+v", err, tr)
	   }
	  if tr.ToAddress != dest(uid, "ETH") {
		   t.Fatalf("stamped %s", tr.ToAddress)
	   }
	  rep, err := ledger.Reconcile(ctx, rt.Store, uid)
	   if err != nil || !rep.OK {
		   t.Fatalf("reconcile %+v %v", rep, err)
	   }
	   wal, err := rt.Store.GetAccount(ctx, uid, ledger.AccountWallet, "USDT")
	   if err != nil {
		  t.Fatal(err)
	   }
	  if !wal.Available.Equal(decimal.RequireFromString("2002")) || !wal.Frozen.IsZero() {
		   t.Fatalf("deposit wallet %s frozen %s", wal.Available, wal.Frozen)
	  }
	  booksOK(t, rt, uid)
   }

   func TestSwapAndInternalSend(t *testing.T) {
	  rt := openMem(t)
	   ctx := context.Background()
	   uid := seedUser(rt)
	  q, err := rt.Swap.Quote(ctx, "ETH", "USDT", "0.05")
	   if err != nil || q.ToAmount == "" {
		  t.Fatalf("quote %v %+v", err, q)
	  }
	   ord, err := rt.Swap.Execute(ctx, swap.ExecReq{
		  UserID: uid, FromAsset: "ETH", ToAsset: "USDT", FromAmount: "0.05",
		  IdempotencyKey: "flow-swap", Chain: "ETH",
	   })
	   if err != nil || ord.Status != swap.StatusFilled {
		  t.Fatalf("swap %v %+v", err, ord)
	  }
	  if q.Source != "spot_ticker" || q.Rate != rt.Market.Cache()["ETH-USDT"].Price {
		  t.Fatalf("quote off ticker src=%s rate=%s", q.Source, q.Rate)
	  }
	   eth, err := rt.Store.GetAccount(ctx, uid, ledger.AccountWallet, "ETH")
	   if err != nil {
		  t.Fatal(err)
	  }
	  usdt, err := rt.Store.GetAccount(ctx, uid, ledger.AccountWallet, "USDT")
	  if err != nil {
		  t.Fatal(err)
	   }
	  if !eth.Frozen.IsZero() || !usdt.Frozen.IsZero() {
		  t.Fatalf("swap leftover frozen eth=%s usdt=%s", eth.Frozen, usdt.Frozen)
	  }
	  if !eth.Available.Equal(decimal.RequireFromString("1.95")) {
		  t.Fatalf("eth after fill %s", eth.Available)
	   }
	  gotUSDT := decimal.RequireFromString("2000").Add(ord.ToAmount)
	  if !usdt.Available.Equal(gotUSDT) {
		  t.Fatalf("usdt after fill %s want %s fee=%s", usdt.Available, gotUSDT, ord.FeeAmount)
	  }
	  booksOK(t, rt, uid)
	  _, err = rt.Swap.Quote(ctx, "FOO", "BAR", "1")
	   if !errors.Is(err, ledger.ErrStaleQuote) {
		  t.Fatalf("missing tick must not use a second book %v", err)
	  }
	  rt.Wallet.Registry().FailNext("ETH", 3)
	  failed,
		  err := rt.Swap.Execute(ctx, swap.ExecReq{
		   UserID: uid, FromAsset: "ETH", ToAsset: "USDT", FromAmount: "0.1",
		  IdempotencyKey: "flow-swap-fail", Chain: "ETH",
	  })
	  if err == nil {
		  t.Fatalf("want swap submit fail %+v", failed)
	  }
	  eth2, _ := rt.Store.GetAccount(ctx, uid, ledger.AccountWallet, "ETH")
	   if !eth2.Available.Equal(eth.Available) || !eth2.Frozen.IsZero() {
		   t.Fatalf("swap fail left eth=%s frozen=%s", eth2.Available, eth2.Frozen)
	  }
	   if failed != nil && failed.Status != swap.StatusFailed {
		   t.Fatalf("swap fail status %s", failed.Status)
	   }
	   part, err := rt.Swap.Execute(ctx, swap.ExecReq{
		  UserID: uid, FromAsset: "ETH", ToAsset: "USDT", FromAmount: "0.2",
		   FillRatio: "0.5", IdempotencyKey: "flow-swap-part", Chain: "ETH",
	  })
	  if err != nil || part.Status != swap.StatusPartial {
		   t.Fatalf("partial %v %+v", err, part)
	  }
	  if !part.FilledAmount.Equal(decimal.RequireFromString("0.1")) || !part.FeeAmount.IsPositive() {
		   t.Fatalf("partial fill/fee %+v", part)
	  }
	  eth3, _ := rt.Store.GetAccount(ctx, uid, ledger.AccountWallet, "ETH")
	  if !eth3.Available.Equal(eth2.Available.Sub(decimal.RequireFromString("0.1"))) || !eth3.Frozen.IsZero() {
		   t.Fatalf("partial books eth=%s frozen=%s", eth3.Available, eth3.Frozen)
	  }
	   booksOK(t, rt, uid)
	   send, err := rt.Ledger.Create(ctx, ledger.CreateReq{
		  UserID: uid, Direction: ledger.SpotSend, Asset: "USDT",
		  Amount: decimal.RequireFromString("5"), ToAddress: "user-10002",
		   IdempotencyKey: "flow-send",
	  })
	  if err != nil || send.State != ledger.StateCredited {
		   t.Fatalf("spot_send %v %+v", err, send)
	   }
  }

   func TestWithdrawSafetyDeeperThanAllowlist(t *testing.T) {
	   rt := openMem(t)
	  ctx := context.Background()
	   uid := seedUser(rt)
	   if ledger.ConfirmNeed("ETH", 1) < 12 || ledger.ConfirmNeed("BTC", 1) < 3 {
		  t.Fatalf("chain confirm floor ETH=%d BTC=%d", ledger.ConfirmNeed("ETH", 1), ledger.ConfirmNeed("BTC", 1))
	   }
	   own := dest(uid, "ETH")
	  poison := evmLookalike(own)
	   if !ledger.LookalikeInbound("ETH", poison, own) {
		  t.Fatalf("lookalike helper %s vs %s", poison, own)
	  }
	  if _, err := rt.Ledger.AddAddressBook(ctx, ledger.AddressBookEntry{
		   UserID: uid, Chain: "ETH", Address: poison, Label: "poison",
	  }); err != nil {
		  t.Fatal(err)
	   }
	  _, err := rt.Ledger.Create(ctx, ledger.CreateReq{
		  UserID: uid, Direction: ledger.WalletWithdraw, Asset: "USDT",
		  Amount: decimal.RequireFromString("1"), Chain: "ETH",
		  ToAddress: poison, IdempotencyKey: "flow-poison",
	  })
	   if !errors.Is(err, ledger.ErrAddressPoison) {
		  t.Fatalf("poison %v", err)
	  }
	   dep, err := rt.Ledger.Create(ctx, ledger.CreateReq{
		  UserID: uid, Direction: ledger.WalletDeposit, Asset: "USDT",
		   Amount: decimal.RequireFromString("2"), Chain: "ETH",
		   IdempotencyKey: "flow-pt-dep", WaitConfirm: true,
	   })
	  if err != nil || dep.State != ledger.StateCredited {
		   t.Fatalf("deposit %v %+v", err, dep)
	   }
	   _, err = rt.Ledger.Create(ctx, ledger.CreateReq{
		   UserID: uid, Direction: ledger.WalletWithdraw, Asset: "USDT",
		  Amount: decimal.RequireFromString("1.8"), Chain: "ETH",
		  ToAddress: extDest("ETH"), IdempotencyKey: "flow-pt-wd",
	   })
	   if !errors.Is(err, ledger.ErrPassThrough) {
		   t.Fatalf("pass-through %v", err)
	  }
	   small, err := rt.Ledger.Create(ctx, ledger.CreateReq{
		   UserID: uid, Direction: ledger.WalletWithdraw, Asset: "USDT",
		  Amount: decimal.RequireFromString("0.2"), Chain: "ETH",
		   ToAddress: extDest("ETH"), IdempotencyKey: "flow-pt-small", WaitConfirm: true,
	   })
	   if err != nil || small.State != ledger.StateCredited {
		   t.Fatalf("small after deposit %v %+v", err, small)
	   }
	  rt.Ledger.SweepHot(ctx, map[string]string{"USDT": "0.5"})
	  _, err = rt.Ledger.Create(ctx, ledger.CreateReq{
		  UserID: uid, Direction: ledger.WalletWithdraw, Asset: "USDT",
		   Amount: decimal.RequireFromString("1.5"), Chain: "ETH",
		   ToAddress: extDest("ETH"), IdempotencyKey: "flow-hot",
	  })
	  if !errors.Is(err, ledger.ErrHotInventory) {
		   t.Fatalf("hot inventory %v", err)
	   }
	   if err := rt.Ledger.SetFrozen(ctx, uid, true); err != nil {
		  t.Fatal(err)
	  }
	  if err := rt.Ledger.SetFrozen(ctx, uid, false); err != nil {
		   t.Fatal(err)
	  }
	  _, err = rt.Ledger.Create(ctx, ledger.CreateReq{
		   UserID: uid, Direction: ledger.WalletWithdraw, Asset: "USDT",
		  Amount: decimal.RequireFromString("0.3"), Chain: "ETH",
		   ToAddress: extDest("ETH"), IdempotencyKey: "flow-cool",
	  })
	  if !errors.Is(err, ledger.ErrWithdrawCooling) {
		  t.Fatalf("cooling %v", err)
	   }
	   booksOK(t, rt, uid)
   }

  func evmLookalike(own string) string {
	  o := strings.ToLower(strings.TrimSpace(own))
	   if !strings.HasPrefix(o, "0x") || len(o) != 42 {
		  return ""
	  }
	  mid := []byte(o[8:36])
	   for i := range mid {
		   if mid[i] >= '0' && mid[i] <= '9' {
			  mid[i] = 'a'
		   } else {
			   mid[i] = '0'
		  }
	   }
	   return o[:8] + string(mid) + o[36:]
   }

  func TestSpotFillFeedsWalletTicker(t *testing.T) {
	  rt := openMem(t)
	   ctx := context.Background()
	  uid := seedUser(rt)
	   _, err := rt.Spot.Place(ctx, spot.PlaceReq{
		   UserID: uid, Symbol: "ETH-USDT", Side: "sell", Type: "limit",
		  Size: "0.01", Price: "3310", IdempotencyKey: "flow-spot-sell",
	   })
	   if err != nil {
		   t.Fatal(err)
	   }
	  _, err = rt.Spot.Place(ctx, spot.PlaceReq{
		   UserID: "user-10002", Symbol: "ETH-USDT", Side: "buy", Type: "limit",
		  Size: "0.01", Price: "3310", IdempotencyKey: "flow-spot-buy",
	  })
	  if err != nil {
		   t.Fatal(err)
	   }
	   tick := rt.Market.Cache()["ETH-USDT"]
	  if tick.Price != "3310" {
		  t.Fatalf("wallet ticker after spot fill %s", tick.Price)
	  }
	  q, err := rt.Swap.Quote(ctx, "ETH", "USDT", "1")
	  if err != nil || q.Rate != "3310" || q.Source != "spot_ticker" {
		  t.Fatalf("swap still off spot print %v %+v", err, q)
	   }
  }
