package e2eflow

import (
	"context"
	"fmt"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/account"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/deposit"
	"account-assets.kucoin.net/funding/internal/kyt"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/treasury"
	"account-assets.kucoin.net/funding/internal/wallet"
	"account-assets.kucoin.net/funding/internal/withdraw"
)

// Report 一串入金→入账→ hop →出金→KYT→资金台的工程回放。不是对外验收证书。

type Report struct {
	DepositID  string `json:"deposit_id"`
	MoveID     string `json:"move_id"`
	WithdrawID string `json:"withdraw_id"`
	KYT      string `json:"kyt"`
	Treasury int    `json:"treasury_plans"`
}

  func Run(ctx context.Context, rt *app.Runtime) (Report, error) {
	  if rt == nil || rt.Ledger == nil || rt.Wallet == nil {
		   return Report{}, fmt.Errorf("e2e runtime unavailable")
	  }
	  uid := rt.Cfg.SeedUserID
	   to := rt.Wallet.AddressFor(ctx, uid, "ETH")
	   hash, err := rt.Wallet.InjectInbound("ETH", to, "USDT", decimal.RequireFromString("5"))
	   if err != nil {
		  return Report{}, err
	   }
	   dep := deposit.New(rt.Ledger, rt.Wallet)
	  dtr, err := dep.Observe(ctx, deposit.ObserveReq{
		   UserID: uid, Chain: "ETH", TxHash: hash, Asset: "USDT",
		  IdempotencyKey: "e2e-dep-1", WaitConfirm: true,
	  })
	   if err != nil || dtr == nil {
		   return Report{}, fmt.Errorf("deposit: %w", err)
	  }
	   acct := account.New(rt.Ledger)
	   if _, err := acct.OpenAll(ctx, uid); err != nil {
		  return Report{}, err
	   }
	   mv, err := acct.Move(ctx, account.MoveReq{
		   UserID: uid, From: account.BookFunding, To: account.BookTrading,
		  Asset: "USDT", Amount: decimal.RequireFromString("2"), IdempotencyKey: "e2e-mv-1",
	   })
	   if err != nil || mv == nil {
		   return Report{}, fmt.Errorf("move: %w", err)
	  }
	   back, err := acct.Move(ctx, account.MoveReq{
		  UserID: uid,
		   From:   account.BookTrading, To: account.BookFunding,
		   Asset: "USDT", Amount: decimal.RequireFromString("2"), IdempotencyKey: "e2e-mv-2",
	   })
	  if err != nil || back == nil {
		  return Report{}, fmt.Errorf("move back: %w", err)
	   }
	  wdTo := wallet.DeriveAddress(ledger.ExtWithdrawUser, "ETH")
	  wd := withdraw.New(rt.Ledger)
	  wtr, err := wd.Request(ctx, withdraw.Request{
		  UserID: uid, Asset: "USDT", Amount: decimal.RequireFromString("1"),
		  Chain: "ETH", ToAddress: wdTo, IdempotencyKey: "e2e-wd-1",
	   })
	  if wtr == nil {
		  return Report{}, fmt.Errorf("withdraw: %w", err)
	   }
	  dec, reason, kerr := kyt.Static{Mode: kyt.Allow}.Screen(ctx, kyt.Request{
		   UserID: uid, MovementID: wtr.ID, Chain: "ETH", ToAddress: wdTo, Asset: "USDT", Amount: "1",
	  })
	  if kerr != nil || dec != kyt.Allow {
		  return Report{}, fmt.Errorf("kyt %s %v", reason, kerr)
	   }
	  tr := treasury.New(nil, nil)
	   plans, err := tr.Plan(ctx, rt.Store)
	   n := 0
	   if err == nil {
		   n = len(plans)
	   }
	  return Report{
		   DepositID: dtr.ID, MoveID: mv.ID, WithdrawID: wtr.ID,
		   KYT: string(dec), Treasury: n,
	   }, nil
   }
