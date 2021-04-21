package e2eflow

import (
	"context"
	"errors"
	"fmt"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/recon"
	"account-assets.kucoin.net/funding/internal/wallet"
	"account-assets.kucoin.net/funding/internal/withdraw"
)

type DriftReport struct {
	Frozen bool `json:"frozen"`
	Blocked bool `json:"blocked"`
}

// DriftFreeze 对账建议冻账户后，新出金必须被挡住。不解冻。

  func DriftFreeze(ctx context.Context, rt *app.Runtime) (DriftReport, error) {
	   if rt == nil || rt.Ledger == nil {
		  return DriftReport{}, fmt.Errorf("e2e runtime unavailable")
	   }
	  uid := rt.Cfg.SeedUserID
	  hooks := recon.DriftHooks{}
	   if rt.Durable.Recon != nil {
		   hooks.WF = recon.WrapBook(rt.Durable.Recon, 0)
	   }
	  if rt.FundSurf != nil {
		  hooks.Halt = rt.FundSurf.HaltGate()
	  }
	  act, err := recon.ApplyFreezeOnDriftWith(ctx, rt.Ledger, uid, []recon.Suggestion{
		  {Action: recon.ActFreezeUser, UserID: uid, Note: "e2e drift"},
	   }, hooks)
	  if err != nil {
		   return DriftReport{}, err
	  }
	   if !act.Frozen {
		   return DriftReport{}, fmt.Errorf("drift freeze not applied")
	   }
	   to := wallet.DeriveAddress(ledger.ExtWithdrawUser, "ETH")
	   wd := withdraw.New(rt.Ledger)
	   _, werr := wd.Request(ctx, withdraw.Request{
		  UserID: uid, Asset: "USDT", Amount: decimal.RequireFromString("1"),
		  Chain: "ETH", ToAddress: to, IdempotencyKey: "e2e-drift-1",
	   })
	   if werr == nil {
		  return DriftReport{}, fmt.Errorf("withdraw after drift freeze")
	   }
	  if !errors.Is(werr, ledger.ErrAccountFrozen) && !errors.Is(werr, ledger.ErrBooksDrift) {
		   // 冻账户或漂移闸都可以，但不能当成功。
		   if rt.Ledger.CheckUser(ctx, uid) == nil {
			  return DriftReport{}, fmt.Errorf("user still open: %v", werr)
		  }
	   }
	  return DriftReport{Frozen: act.Frozen,
		  Blocked: true}, nil
  }
