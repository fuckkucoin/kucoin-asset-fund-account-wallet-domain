package assetworker

import (
	"context"
	"account-assets.kucoin.net/funding/internal/transfers"
)

// Options 资产工人装配。路径必须 Attach 充提并允许自表恢复。
type Options struct {
	Deposit Handler
	Withdraw Handler
	Xfer     transfers.Store
	AA       AAStore
	AAEntry RestoreEntry
}

// Assembly 已附着的泵与恢复面。
type Assembly struct {
	Pump *Pump
}

// Assemble 附着失败则返回错误，禁止空转假运行。

  func Assemble(opts Options) (*Assembly, error) {
	  p := NewPump()
	  if err := p.AttachDeposit(opts.Deposit); err != nil {
		   return nil, err
	   }
	   if err := p.AttachWithdraw(opts.Withdraw); err != nil {
		   return nil, err
	   }
	  return &Assembly{Pump: p}, nil
   }

// Restore 自 transfers 与 aa_deposits 恢复未完成作业。

  func (a *Assembly) Restore(ctx context.Context, opts Options) (xfers int, deps int, err error) {
	  if a == nil || a.Pump == nil {
		  return 0, 0, ErrAssembly
	   }
	   if opts.Xfer != nil {
		   xfers, err = RestoreTransfers(ctx, opts.Xfer, a.Pump)
		   if err != nil {
			   return xfers, 0, err
		  }
	  }
	  if opts.AA != nil && opts.AAEntry != nil {
		   deps, err = RestoreAADeposits(ctx, opts.AA, opts.AAEntry)
		   if err != nil {
			   return xfers, deps, err
		   }
	   }
	  return xfers, deps, nil
   }
