// cvfund 模块 both。

// Author: Cason
// Date: 2021.03.27

package cvfund

import (
	"context"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/ledger"
	"github.com/shopspring/decimal"
)

// PlaceBOTH 支付账户 BOTH（Funding+Trading 合计）市价闪兑。
// 借记 CoverTrading 先补资金再补交易；目标币入资金账，不自动拆回交易账。
// 服务端不得编造汇率；牌价须由调用方或已绑定 MarkSource 提供。
  func (b *Book) PlaceBOTH(ctx context.Context, userID, from, to, amount, rate string) (Order, error) {
	   if ctx == nil {
		  ctx = context.Background()
	  }
	  if strings.TrimSpace(rate) == "" && b != nil && b.marks != nil {
		  rt, err := b.marks.Rate(ctx, from, to)
		   if err != nil || !rt.IsPositive() {
			   return Order{}, ErrQuoteSrc
		   }
		   rate = rt.String()
	  }
	  return b.place(ctx, "", userID, from, to, amount, rate, "both")
  }

// SettleBOTHIntegrity 核对 BOTH 成交后资金账目标币已入、源币已扣。
// 只读核对，不得补铸；CoverTrading 补账幂等键可观测。
// 账本未绑定则跳过；漂移返回错误供运营面。

  func (b *Book) SettleBOTHIntegrity(ctx context.Context, o Order) error {
	   if b == nil || b.led == nil {
		  return nil
	  }
	  if !strings.EqualFold(o.Book, "both") {
		  return fmt.Errorf("%w: not both", ErrBook)
	   }
	  if ctx == nil {
		  ctx = context.Background()
	  }
	  amt, err := decimal.NewFromString(strings.TrimSpace(o.Amount))
	   if err != nil || !amt.IsPositive() {
		   return ErrAmount
	   }
	  rate, err := decimal.NewFromString(strings.TrimSpace(o.Rate))
	   if err != nil || !rate.IsPositive() {
		  return ErrRate
	   }
	  got := amt.Mul(rate)
	  got, err = ledger.RoundLedger(got)
	   if err != nil {
		  return err
	   }
	  if o.DebitID == "" || o.CreditID == "" {
		   return fmt.Errorf("convert both missing journal ids")
	  }
	  debit, err := b.led.Get(ctx, o.DebitID)
	   if err != nil || debit == nil {
		   return fmt.Errorf("convert both debit missing")
	   }
	  credit, err := b.led.Get(ctx, o.CreditID)
	  if err != nil || credit == nil {
		  return fmt.Errorf("convert both credit missing")
	   }
	  if debit.Direction != ledger.ConvertDebit || !strings.EqualFold(debit.Asset, o.FromAsset) {
		   return fmt.Errorf("convert both debit mismatch")
	  }
	   if credit.Direction != ledger.ConvertCredit || !strings.EqualFold(credit.Asset, o.ToAsset) {
		   return fmt.Errorf("convert both credit mismatch")
	   }
	  if !debit.Amount.Equal(amt) {
		   return fmt.Errorf("convert both debit amount drift")
	   }
	  if !credit.Amount.Equal(got) {
		  return fmt.Errorf("convert both credit amount drift")
	  }
	   return nil
   }

// PlaceMarket 按支付账户记账的市价闪兑；缺 rate 时取 MarkSource。
// Funding/Trading/BOTH 统一入口；无牌价失败关闭。
// 不得在缺价时编造汇率；与 PlaceQuoted(quoteID) 不同。

  func (b *Book) PlaceMarket(ctx context.Context, userID, from, to, amount, rate, book string) (Order, error) {
	   if ctx == nil {
		  ctx = context.Background()
	  }
	   book = strings.ToLower(strings.TrimSpace(book))
	  if book == "" {
		   book = "funding"
	  }
	   if strings.TrimSpace(rate) == "" && b != nil && b.marks != nil {
		   rt, err := b.marks.Rate(ctx, from, to)
		   if err != nil || !rt.IsPositive() {
			   return Order{}, ErrQuoteSrc
		   }
		   rate = rt.String()
	   }
	   return b.place(ctx, "", userID, from, to, amount, rate, book)
   }
