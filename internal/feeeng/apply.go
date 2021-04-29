package feeeng

import (
	"account-assets.kucoin.net/funding/pkg/money"
)

type Applied struct {
	Quote Quote        `json:"quote"`
	Lines []money.Line `json:"-"`
}

// Apply 组手续费凭证。不平直接拒；0 费退回 DebitPair。

   func (b *Book) Apply(kind Kind, asset, gross string) (Applied, error) {
	   q,
		   err := b.Quote(kind, asset, gross)
	   if err != nil {
		   return Applied{}, err
	   }
	   amt, err := money.Parse(gross)
	  if err != nil {
		   return Applied{}, err
	  }
	  lines, err := money.FeeCut(asset, amt, q.BPS)
	   if err != nil {
		  return Applied{}, err
	  }
	  if q.Fee != "0" && q.BPS == 0 {
		   // 走了 min/flat，FeeCut 按 0 bps 会丢费，改用报价额组三行。
		  fee, err := money.ParseBook(q.Fee)
		  if err != nil {
			   return Applied{}, err
		   }
		   net, err := money.Parse(q.Net)
		   if err != nil {
			  if net, err = money.ParseBook(q.Net); err != nil {
				  return Applied{}, err
			  }
		  }
		   lines = []money.Line{
			  {Asset: asset, Debit: amt, Memo: "user_debit_gross"},
			   {Asset: asset, Credit: net, Memo: "bridge_net"},
			   {Asset: asset, Credit: fee, Memo: "system_fee"},
		   }
		  if err := money.Balanced(lines); err != nil {
			  return Applied{}, err
		  }
	  }
	   return Applied{Quote: q,
		   Lines: lines}, nil
  }
