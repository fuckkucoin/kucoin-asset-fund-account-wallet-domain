// 功能：cvfund 模块 partial_fill。
// 作者：MilesFord
// 日期：2021.03.28

package cvfund

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/ledger"
	"github.com/shopspring/decimal"
)

// PartialFill 限价闪兑部分成交，按成交量贷记目标币并保留剩余冻结。
// 累计成交不得超过 FromSize；幂等键含 fillSeq；足额成交才释放 holdcat。
// 部分成交后取消只应退剩余；重复 fillSeq 不得二次贷记。

   func (b *Book) PartialFill(ctx context.Context, userID, orderID, fillSize, fillSeq string) (LimitOrder, error) {
	   o, err := b.loadLimit(ctx, userID, orderID, "")
	   if err != nil {
		   return LimitOrder{}, err
	   }
	   if o.Status != "OPEN" && o.Status != "PARTIAL" {
		   return o, ErrOpen
	   }
	  if time.Now().UTC().UnixMilli() > o.ExpiryTime {
		   return b.expireLimit(ctx, o)
	  }
	   fill, err := decimal.NewFromString(strings.TrimSpace(fillSize))
	   if err != nil || !fill.IsPositive() {
		   return LimitOrder{}, ErrAmount
	  }
	   src, err := decimal.NewFromString(o.FromSize)
	  if err != nil {
		  return LimitOrder{}, err
	  }
	  filled := decimal.Zero
	  if strings.TrimSpace(o.FilledSize) != "" {
		   filled,
			  err = decimal.NewFromString(o.FilledSize)
		   if err != nil {
			  return LimitOrder{}, err
		  }
	   }
	   remain := src.Sub(filled)
	   if fill.GreaterThan(remain) {
		   return LimitOrder{}, ErrAmount
	  }
	  price, err := decimal.NewFromString(o.Price)
	   if err != nil || !price.IsPositive() {
		   return LimitOrder{}, ErrLimit
	  }
	   got := fill.Mul(price)
	   got, err = ledger.RoundLedger(got)
	  if err != nil {
		  return LimitOrder{}, err
	  }
	   seq := strings.TrimSpace(fillSeq)
	   if seq == "" {
		   seq = "1"
	  }
	  idem := "cv-lim-partial:" + o.ClientOrderID + ":" + seq
	  if b.led != nil {
		   tr, err := b.led.Create(ctx, ledger.CreateReq{
			   UserID: o.UserID, Direction: ledger.ConvertCredit, Asset: o.ToCurrency, Amount: got,
			   IdempotencyKey: idem,
		   })
		  if err != nil {
			  return LimitOrder{}, err
		   }
		   if tr != nil && o.CreditID == "" {
			   o.CreditID = tr.ID
		   }
		   if convertBook(o.AccountType) == "trading" {
			  if _, err := b.led.Create(ctx, ledger.CreateReq{
				  UserID: o.UserID, Direction: ledger.FundingToTrading, Asset: o.ToCurrency, Amount: got,
				  IdempotencyKey: idem + ":hop",
			   }); err != nil {
				  return LimitOrder{}, err
			   }
		  }
	  }
	   if o.HoldID != "" && b.kv != nil && fill.Equal(remain) {
		   _, _ = holdcat.New(b.kv).Release(ctx, o.UserID, o.HoldID)
		   o.HoldID = ""
	  }
	  o.FilledSize = filled.Add(fill).String()
	  o.FilledTime = time.Now().UTC().UnixMilli()
	   prevTo := decimal.Zero
	   if strings.TrimSpace(o.FilledToSize) != "" {
		   prevTo, _ = decimal.NewFromString(o.FilledToSize)
	   }
	  o.FilledToSize = prevTo.Add(got).String()
	  if filled.Add(fill).Equal(src) {
		   o.Status = "FILLED"
		   o.ToSize = o.FilledToSize
	  } else {
		  o.Status = "PARTIAL"
	  }
	   if err := b.saveLimit(ctx, o); err != nil {
		  return LimitOrder{}, err
	   }
	  return o, nil
   }
