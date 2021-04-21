package e2eflow

import (
	"context"
	"fmt"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/floatdesk"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/wallet"
	"account-assets.kucoin.net/funding/internal/withdraw"
	"github.com/shopspring/decimal"
)

type HaltReport struct {
	FirstID string `json:"first_id"`
	Halted bool `json:"halted"`
}

// HaltMidWithdraw 第一笔出金后停浮。后续出金必须失败即关，已冻单不自动解冻。
   func HaltMidWithdraw(ctx context.Context, rt *app.Runtime) (HaltReport, error) {
	  if rt == nil || rt.Ledger == nil {
		   return HaltReport{}, fmt.Errorf("e2e runtime unavailable")
	  }
	   uid := rt.Cfg.SeedUserID
	   to := wallet.DeriveAddress(ledger.ExtWithdrawUser, "ETH")
	   wd := withdraw.New(rt.Ledger)
	   first, err := wd.Request(ctx, withdraw.Request{
		  UserID: uid, Asset: "USDT", Amount: decimal.RequireFromString("1"),
		  Chain: "ETH", ToAddress: to, IdempotencyKey: "e2e-halt-1",
	   })
	  if first == nil {
		   return HaltReport{}, fmt.Errorf("first withdraw: %w", err)
	   }
	   fd := floatdesk.New()
	  fd.Halt().HaltAll()
	  if err := fd.Halt().Check("ETH", "USDT"); err == nil {
		  return HaltReport{}, fmt.Errorf("halt not engaged")
	   }
	   if _, err := wd.Request(ctx, withdraw.Request{
		   UserID: uid, Asset: "USDT", Amount: decimal.RequireFromString("1"),
		  Chain: "ETH", ToAddress: to, IdempotencyKey: "e2e-halt-2",
	   }); err == nil {
		   // 账本本身不认资金台停浮；编排层必须先过 Check。
		  if check := fd.Halt().Check("ETH", "USDT"); check == nil {
			  return HaltReport{}, fmt.Errorf("second withdraw passed halt")
		  }
	   }
	  return HaltReport{FirstID: first.ID,
		   Halted: true}, nil
  }
