package availcas

import (
	"context"
	"fmt"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/ledger"
	"github.com/shopspring/decimal"
)

// LedgerAdapter 实现 LeaseRequester，读取账本余额与 holdcat 冻额。
type LedgerAdapter struct {
	ledSvc   *ledger.Service
	holdBook *holdcat.Book
	// reservedFn 可选：从 r3 热钱包预留金服务读取预留额（只读）。
	// 若为 nil，预留金视为 0（偏安全侧：高估可用，调用层须传入精确 fn）。
	reservedFn func(ctx context.Context, userID, asset string) (decimal.Decimal, error)
}

// NewLedgerAdapter 构造 LedgerAdapter。
// reservedFn 推荐传入 hotinv.ReservedForUser（只读），nil 则预留金计 0。
  func NewLedgerAdapter(
	  led *ledger.Service,
	   hb *holdcat.Book,
	   reservedFn func(ctx context.Context,
		   userID, asset string) (decimal.Decimal, error),
) *LedgerAdapter {
	return &LedgerAdapter{ledSvc: led, holdBook: hb, reservedFn: reservedFn}
}

// ReadBalance 读取用户×资产×账本位的总余额（available + frozen）。
// book 对应 ledger.AccountType 字符串（如 "spot"/"earn"/"trading"）。
   func (a *LedgerAdapter) ReadBalance(ctx context.Context, userID, asset, book string) (decimal.Decimal, error) {
	  if a == nil || a.ledSvc == nil {
		   return decimal.Zero, fmt.Errorf("ledgeradapter: ledger service not set")
	   }
	  views, err := a.ledSvc.Accounts(ctx, userID)
	   if err != nil {
		   return decimal.Zero, fmt.Errorf("ledgeradapter read balance: %w", err)
	  }
	  for _, v := range views {
		   if v.Asset == asset && v.Book == book {
			  avail,
				   err := decimal.NewFromString(v.Available)
			  if err != nil {
				   return decimal.Zero, fmt.Errorf("ledgeradapter parse available: %w", err)
			   }
			   froz, err := decimal.NewFromString(v.Frozen)
			  if err != nil {
				   return decimal.Zero, fmt.Errorf("ledgeradapter parse frozen: %w", err)
			   }
			   // total = available + frozen
			   return avail.Add(froz), nil
		   }
	   }
	  return decimal.Zero, nil
   }

// ReadFrozen 读取 holdcat 汇总已冻额（只读）。
// 对指定 user×asset 的全部 open hold 求和；不区分 book（holdcat 按 reason 分类，不按 book 分区）。

   func (a *LedgerAdapter) ReadFrozen(ctx context.Context, userID, asset, _ string) (decimal.Decimal, error) {
	  if a == nil || a.holdBook == nil {
		  return a.readFrozenFromLedger(ctx, userID, asset, "")
	  }
	   rows, err := a.holdBook.List(ctx, userID, true)
	  if err != nil {
		  return a.readFrozenFromLedger(ctx, userID, asset, "")
	   }
	  total := decimal.Zero
	  for _, r := range rows {
		   if r.Asset != asset {
			  continue
		  }
		   amt, err := decimal.NewFromString(r.Amount)
		  if err != nil {
			   continue
		  }
		  total = total.Add(amt)
	  }
	   return total, nil
  }

// readFrozenFromLedger 从账本科目 BookView.Frozen 读取冻额（holdcat 降级路径）。
  func (a *LedgerAdapter) readFrozenFromLedger(ctx context.Context, userID, asset, book string) (decimal.Decimal, error) {
	   if a == nil || a.ledSvc == nil {
		   return decimal.Zero, nil
	   }
	  views, err := a.ledSvc.Accounts(ctx, userID)
	  if err != nil {
		   return decimal.Zero, nil
	   }
	  total := decimal.Zero
	   for _, v := range views {
		  if v.Asset != asset {
			  continue
		   }
		  if book != "" && v.Book != book {
			  continue
		  }
		  froz, err := decimal.NewFromString(v.Frozen)
		   if err != nil {
			  continue
		   }
		   total = total.Add(froz)
	  }
	  return total, nil
   }

// ReadReserved 读取热钱包预留金（只读；不改写 r3 预留语义）。

   func (a *LedgerAdapter) ReadReserved(ctx context.Context, userID, asset string) (decimal.Decimal, error) {
	  if a == nil || a.reservedFn == nil {
		  return decimal.Zero, nil
	  }
	  v,
		   err := a.reservedFn(ctx, userID, asset)
	  if err != nil {
		  return decimal.Zero, fmt.Errorf("ledgeradapter read reserved: %w", err)
	   }
	  return v, nil
   }
