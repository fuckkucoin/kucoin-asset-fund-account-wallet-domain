package oracle

import (
	"fmt"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/validate"
)

// Convert 把数量标成报价币。缺价或过期失败即关，不得用过期价算 LTV。

   func (f *Feed) Convert(asset, amount, quote string) (money.Amount, Tick, error) {
	  if f == nil {
		   return money.Zero(), Tick{}, ErrUnavailable
	   }
	   as, err := validate.Asset(asset)
	   if err != nil {
		   return money.Zero(), Tick{}, err
	   }
	  q, err := validate.Asset(quote)
	   if err != nil {
		   return money.Zero(), Tick{}, err
	  }
	   amt, err := money.Parse(amount)
	  if err != nil {
		   return money.Zero(), Tick{}, err
	   }
	   if as == q {
		   return amt, Tick{Asset: as, Quote: q, Price: "1", At: f.clock()}, nil
	  }
	   t, err := f.resolve(as, q)
	   if err != nil {
		   return money.Zero(), t, err
	   }
	   px, err := money.Parse(t.Price)
	   if err != nil {
		   return money.Zero(), t, err
	  }
	  raw := amt.Dec().Mul(px.Dec())
	   out := money.FromDecimal(raw)
	  if out.IsNegative() {
		  return money.Zero(), t, ErrPrice
	  }
	   if _, err := money.ParseBook(out.String()); err != nil {
		   return money.Zero(), t, fmt.Errorf("%w: %v", ErrPrice, err)
	  }
	   return out, t, nil
  }
