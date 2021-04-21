package e2eflow

import (
	"context"
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/internal/addrintel"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/floatdesk"
	"account-assets.kucoin.net/funding/internal/por"
	"account-assets.kucoin.net/funding/internal/wallet"
)

type Extra struct {
	Report
	Root string `json:"por_root"`
	Leaves int `json:"por_leaves"`
	Halt bool `json:"float_halt"`
}

// Stitch 在 Run 之后再打 PoR 树和资金台停浮。停浮不得影响已经完成的入金。

  func Stitch(ctx context.Context, rt *app.Runtime) (Extra, error) {
	  rep, err := Run(ctx, rt)
	  if err != nil {
		  return Extra{}, err
	  }
	  snap, err := por.BuildFromStore(ctx, rt.Store, []string{rt.Cfg.SeedUserID}, time.Now().UTC())
	   if err != nil {
		   return Extra{}, err
	   }
	  fd := floatdesk.New()
	   fd.Halt().HaltAll()
	  if err := fd.Halt().Check("ETH", "USDT"); err == nil {
		   return Extra{}, fmt.Errorf("halt not engaged")
	  }
	   intel := addrintel.New()
	   addr := wallet.DeriveAddress(rt.Cfg.SeedUserID, "ETH")
	  if _, err := intel.Observe("ETH", addr, rt.Cfg.SeedUserID, snap.TakenAt); err != nil {
		   return Extra{}, err
	  }
	   return Extra{Report: rep, Root: snap.Root, Leaves: snap.Count, Halt: true}, nil
   }
