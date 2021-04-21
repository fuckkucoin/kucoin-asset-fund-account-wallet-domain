package e2eflow

import (
	"context"
	"fmt"
	"account-assets.kucoin.net/funding/internal/addrintel"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/wallet"
)

type SanctionReport struct {
	Address string `json:"address"`
	Hit bool `json:"hit"`
}

// SanctionsHit 出金目标命中进程内名单。命中失败即关，不得广播。

   func SanctionsHit(ctx context.Context, rt *app.Runtime) (SanctionReport, error) {
	  if rt == nil {
		   return SanctionReport{}, fmt.Errorf("e2e runtime unavailable")
	  }
	   _ = ctx
	  to := wallet.DeriveAddress(ledger.ExtWithdrawUser, "ETH")
	   list := addrintel.NewList()
	  if err := list.Add("ETH", to); err != nil {
		   return SanctionReport{}, err
	   }
	  if err := list.Check("ETH", to); err == nil {
		  return SanctionReport{}, fmt.Errorf("sanctions miss")
	  }
	   return SanctionReport{Address: to, Hit: true}, nil
   }
