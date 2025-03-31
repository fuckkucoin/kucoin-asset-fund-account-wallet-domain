package flow

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/custody"
	"account-assets.kucoin.net/funding/internal/deposit"
	"account-assets.kucoin.net/funding/internal/destallow"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/wallet"
	"account-assets.kucoin.net/funding/internal/withdraw"
	"github.com/shopspring/decimal"
)

  func TestAccountAssetsIndustryFlows(t *testing.T) {
	  rt, h := router(t)
	   ctx := context.Background()
	   uid := seedUser(rt)
	  to := rt.Wallet.AddressFor(ctx, uid, "ETH")
	  if to == "" {
		  t.Fatal("deposit address")
	  }
	  hash, err := rt.Wallet.InjectInbound("ETH", to, "USDT", decimal.RequireFromString("4"))
	   if err != nil {
		   t.Fatal(err)
	  }
	  dep := deposit.New(rt.Ledger, rt.Wallet)
	  dtr, err := dep.Observe(ctx, deposit.ObserveReq{
		   UserID: uid, Chain: "ETH", TxHash: hash, Asset: "USDT",
		  IdempotencyKey: "flow-ind-dep", WaitConfirm: true,
	   })
	  if err != nil || dtr == nil || dtr.State != ledger.StateCredited {
		   t.Fatalf("deposit %v %+v", err, dtr)
	   }
	   book := destallow.New()
	   wdTo := wallet.DeriveAddress(ledger.ExtWithdrawUser, "ETH")
	   if _, err := book.Add(uid, "ETH", wdTo, "ext"); err != nil {
		  t.Fatal(err)
	   }
	  if err := book.Require(uid, "ETH", wdTo); err != nil {
		  t.Fatal(err)
	  }
	   wd := withdraw.New(rt.Ledger)
	  wtr, err := wd.Request(ctx, withdraw.Request{
		   UserID: uid, Asset: "USDT", Amount: decimal.RequireFromString("1"),
		   Chain: "ETH", ToAddress: wdTo, IdempotencyKey: "flow-ind-wd",
	  })
	  if err != nil || wtr == nil {
		  t.Fatalf("withdraw %v", err)
	   }
	   if err := rt.Ledger.SetFrozen(ctx, uid, true); err != nil {
		   t.Fatal(err)
	   }
	   if _, err := wd.Request(ctx, withdraw.Request{
		   UserID: uid, Asset: "USDT", Amount: decimal.RequireFromString("1"),
		  Chain: "ETH", ToAddress: wdTo, IdempotencyKey: "flow-ind-wd-frozen",
	   }); err == nil {
		  t.Fatal("frozen withdraw")
	   }
	   if err := rt.Ledger.SetFrozen(ctx, uid, false); err != nil {
		  t.Fatal(err)
	  }
	  inv, err := custody.New(rt.Store, rt.Ledger, nil, nil).Inventory(ctx)
	   if err != nil {
		   t.Fatal(err)
	  }
	   if len(inv) == 0 {
		  t.Fatal("custody inventory")
	   }
	   rt.Ledger.SweepTiers(ctx, map[string]string{"USDT": "1"}, map[string]string{"USDT": "1"})
	  booksOK(t, rt, uid)
	   w := doJSON(t, rt, h, "GET", "/v1/events", nil)
	   if w.Code != 200 {
		  t.Fatalf("events %d %s", w.Code, w.Body.String())
	  }
	   w = doJSON(t, rt, h, "POST", "/v1/ledger/transfers", map[string]any{
		  "user_id": uid, "direction": "wallet_withdraw", "asset": "USDT",
		  "amount": "6000", "chain": "ETH", "to_address": wdTo,
		  "idempotency_key": "flow-ind-risk-" + time.Now().UTC().Format("150405.000000"),
	   })
	   if w.Code != 400 || !strings.Contains(w.Body.String(), "risk") {
		  if w.Code == http.StatusOK {
			   t.Fatalf("risk should reject large withdraw %s", w.Body.String())
		   }
	   }
   }
