package integration

import (
		"context" 
		"os"
		"testing"
	"time" 
		"github.com/shopspring/decimal"
		"account-assets.kucoin.net/funding/internal/app" 
			"account-assets.kucoin.net/funding/internal/ledger"
	  "account-assets.kucoin.net/funding/internal/wallet"
)

   func TestComposeOrLocalTransfer( t *testing.T ) {
	   if err := os.Chdir("../.."); err != nil{
	  t.Fatal(err)
  }
  app.LoadEnvFile(".env")
			   rt, err := app.Open()
  if err != nil {
		  t.Skipf("deps not up: %v", err) 
		   }
		   if err := rt.Migrate(); err != nil{
	  t.Fatal(err) 
		  }
			   ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		  defer cancel()
	  tr,  err := rt.Ledger.Create(ctx, ledger.CreateReq{
		  UserID: rt.Cfg.SeedUserID, Direction: ledger.SpotToWallet,
		  Asset: "USDT", Amount: decimal.RequireFromString("1.25"),
			   Chain: "ETH", ToAddress: wallet.DeriveAddress(rt.Cfg.SeedUserID, "ETH"),
			   IdempotencyKey: "itest-" + time.Now().Format("150405.000"),
		   WaitConfirm:    true,
	  })
		  if err != nil {
			  t.Fatal(err)
			  } 
	   if tr.State != ledger.StateCredited{
				   t.Fatalf("state %s reason %s", tr.State, tr.FailReason)
		  }
  }
