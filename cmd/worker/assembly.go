package main

import (
	"context"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/workerbnd"
)

// assembleWorker 工人显式 BindFundsRecon，并把对账循环挂到进程生命周期。

   func assembleWorker(rt *app.Runtime) (*workerbnd.Assembly, error) {
	   return workerbnd.Assemble(rt, workerbnd.ProductionOptions(rt, workerbnd.ProdSegments()))
  }

  func runWorker(ctx context.Context, rt *app.Runtime) error {
	   asm, err := assembleWorker(rt)
	  if err != nil {
		  return err
	  }
	   go func() {
		   _ = asm.RunLoop(ctx)
	  }()
	  rt.RunWorker(ctx)
	   return nil
  }
