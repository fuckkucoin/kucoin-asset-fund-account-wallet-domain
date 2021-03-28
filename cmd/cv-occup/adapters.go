package main

import (
	"context"
	"fmt"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"github.com/shopspring/decimal"
)

// holdAdapter 把 holdcat.Book 适配为 cvoccup.HoldAPI。

type holdAdapter struct {
	rt *app.Runtime
}

   func newHoldAdapter(rt *app.Runtime) *holdAdapter {
	   return &holdAdapter{rt: rt}
  }

   func (a *holdAdapter) PlaceConvertHold(ctx context.Context, userID, asset, book, amount, ref string) (string, error) {
	   if a.rt == nil || a.rt.KV == nil {
		   return "", fmt.Errorf("holdcat kv unavailable")
	   }
	  hc := holdcat.New(a.rt.KV)
	   row, err := hc.Place(ctx, userID, asset, book, amount, holdcat.ReasonConvertLimit, ref)
	  if err != nil {
		  return "", err
	  }
	   return row.ID, nil
   }

  func (a *holdAdapter) ReleaseHold(ctx context.Context, userID, holdID string) error {
	  if a.rt == nil || a.rt.KV == nil {
		  return nil
	   }
	  hc := holdcat.New(a.rt.KV)
	   _, err := hc.Release(ctx, userID, holdID)
	  return err
   }

  func (a *holdAdapter) GetHoldAmount(ctx context.Context, userID, holdID string) (decimal.Decimal, error) {
	   if a.rt == nil || a.rt.KV == nil {
		  return decimal.Zero, fmt.Errorf("holdcat kv unavailable")
	  }
	   hc := holdcat.New(a.rt.KV)
	  rows, err := hc.List(ctx, userID, true)
	   if err != nil {
		  return decimal.Zero, err
	   }
	  for _, r := range rows {
		  if r.ID == holdID {
			  amt, err := decimal.NewFromString(r.Amount)
			   if err != nil {
				  return decimal.Zero, err
			   }
			  return amt, nil
		  }
	   }
	  // -1 表示 hold 不存在
	  return decimal.NewFromInt(-1), nil
  }

// availLedgerAdapter 账本可用余额适配器（只读消费）。
// 侧注入完整实现；此处返回 0 以保证进程可启动。
type availLedgerAdapter struct {
	rt *app.Runtime
}

   func newAvailLedgerAdapter(rt *app.Runtime) *availLedgerAdapter {
	  return &availLedgerAdapter{rt: rt}
   }

   func (a *availLedgerAdapter) AvailBalance(_ context.Context, _, _, _ string) (decimal.Decimal, error) {
	  // 环境需对接账本余额查询接口；此处骨架返回零值，不阻断路径开立。
	   // 完整实现应查询 ledger store 的 Available 字段。
	   return decimal.Zero, nil
  }
