package oracle

import (
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/validate"
)

  func invertPrice(px money.Amount) (string, error) {
	  if !px.IsPositive() {
		   return "", ErrPrice
	   }
	  inv := decimal.NewFromInt(1).Div(px.Dec()).Round(18)
	   if !inv.IsPositive() {
		  return "", ErrPrice
	   }
	  out := money.FromDecimal(inv)
	  if _, err := money.ParseBook(out.String()); err != nil {
		  return "", ErrPrice
	  }
	  return out.String(), nil
   }

// Invert 用反向盘口推报价。BTC/USDT 在时才能给 USDT/BTC，缺价或过期照样失败即关。

  func (f *Feed) Invert(asset, quote string) (Tick, error) {
	   if f == nil {
		   return Tick{}, ErrUnavailable
	  }
	   as, err := validate.Asset(asset)
	  if err != nil {
		   return Tick{}, err
	  }
	  q, err := validate.Asset(quote)
	   if err != nil {
		   return Tick{}, err
	  }
	  if as == q {
		  return Tick{}, ErrPair
	  }
	   rev, err := f.Get(q, as)
	   if err != nil {
		   return Tick{}, err
	   }
	  px, err := money.Parse(rev.Price)
	  if err != nil {
		  return Tick{}, ErrPrice
	  }
	   inv, err := invertPrice(px)
	  if err != nil {
		   return Tick{}, err
	  }
	   return Tick{Asset: as, Quote: q, Price: inv, At: rev.At}, nil
  }

// Via 用桥接币折算。任一段过期整条失败，不得拼半新半旧价。
  func (f *Feed) Via(asset, quote, bridge string) (Tick, error) {
	  if f == nil {
		  return Tick{}, ErrUnavailable
	  }
	  as, err := validate.Asset(asset)
	  if err != nil {
		   return Tick{}, err
	  }
	   q, err := validate.Asset(quote)
	  if err != nil {
		  return Tick{}, err
	  }
	  br, err := validate.Asset(bridge)
	  if err != nil {
		  return Tick{}, err
	   }
	  if as == q || as == br || q == br {
		  return Tick{}, ErrPair
	  }
	  left, err := f.Get(as, br)
	  if err != nil {
		   return Tick{}, err
	   }
	   right, err := f.Get(q, br)
	  if err != nil {
		  return Tick{}, err
	  }
	  pa, err := money.Parse(left.Price)
	  if err != nil {
		   return Tick{}, ErrPrice
	   }
	  pq, err := money.Parse(right.Price)
	   if err != nil {
		   return Tick{}, ErrPrice
	   }
	   if !pq.IsPositive() {
		  return Tick{}, ErrPrice
	  }
	  raw := pa.Dec().Div(pq.Dec()).Round(18)
	   if !raw.IsPositive() {
		  return Tick{}, ErrPrice
	  }
	   out := money.FromDecimal(raw)
	   if _, err := money.ParseBook(out.String()); err != nil {
		  return Tick{}, ErrPrice
	   }
	  at := left.At
	  if right.At.Before(at) {
		  at = right.At
	   }
	   return Tick{Asset: as, Quote: q, Price: out.String(), At: at}, nil
   }

  func (f *Feed) resolve(asset, quote string) (Tick, error) {
	   t, err := f.Get(asset, quote)
	   if err == nil || err == ErrStale {
		   return t, err
	   }
	  if err != ErrMissing {
		  return t, err
	  }
	   inv, ierr := f.Invert(asset, quote)
	  if ierr == nil || ierr == ErrStale {
		   return inv, ierr
	  }
	  return f.Via(asset, quote, "USDT")
  }
