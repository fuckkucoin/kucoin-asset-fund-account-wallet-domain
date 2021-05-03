package feeeng

import (
	"fmt"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Quote struct {
	Kind  Kind   `json:"kind"`
	Asset string `json:"asset"`
	Gross string `json:"gross"`
	Fee   string `json:"fee"`
	Net string `json:"net"`
	BPS int `json:"bps"`
	Schedule string `json:"schedule_id"`
	Version int `json:"version"`
}

// Quote 只算不改账。fee+net 必须等于 gross；夹 min/max 后仍守恒。
// 出金调用方仍按 gross 冻，不得拿 net 去冻。

   func (b *Book) Quote(kind Kind, asset, gross string) (Quote, error) {
	  if b == nil {
		  return Quote{}, ErrUnavailable
	  }
	   amt, err := money.Parse(gross)
	  if err != nil {
		   return Quote{}, err
	  }
	   sch, err := b.Get(kind, asset)
	  if err != nil {
		  return Quote{}, err
	   }
	  fee, net, err := cut(amt, sch)
	   if err != nil {
		   return Quote{}, err
	   }
	   if err := money.Conserve(mustSum(fee, net), amt); err != nil {
		  return Quote{}, fmt.Errorf("%w: %v", ErrQuote, err)
	   }
	   return Quote{
		  Kind: kind, Asset: asset, Gross: amt.String(),
		   Fee: fee.String(), Net: net.String(), BPS: sch.BPS,
		  Schedule: sch.ID,
		   Version: sch.Version,
	   }, nil
   }

  func cut(gross money.Amount, sch Schedule) (fee money.Amount, net money.Amount, err error) {
	  fee, net, err = gross.FeeBPS(sch.BPS)
	  if err != nil {
		  return money.Zero(), money.Zero(), err
	   }
	   if sch.Flat != "" {
		  flat, err := money.ParseBook(sch.Flat)
		  if err != nil {
			   return money.Zero(), money.Zero(), err
		  }
		   if flat.IsPositive() {
			   fee = flat
			   net, err = gross.Sub(fee)
			   if err != nil {
				  return money.Zero(), money.Zero(), err
			   }
		   }
	  }
	   if sch.MinFee != "" {
		   min,
			  err := money.ParseBook(sch.MinFee)
		  if err != nil {
			   return money.Zero(), money.Zero(), err
		   }
		   if fee.Cmp(min) < 0 && min.Cmp(gross) < 0 {
			  fee = min
			  net, err = gross.Sub(fee)
			  if err != nil {
				  return money.Zero(), money.Zero(), err
			  }
		  }
	  }
	   if sch.MaxFee != "" {
		   max, err := money.ParseBook(sch.MaxFee)
		   if err != nil {
			   return money.Zero(), money.Zero(), err
		   }
		  if max.IsPositive() && fee.Cmp(max) > 0 {
			   fee = max
			  net, err = gross.Sub(fee)
			  if err != nil {
				   return money.Zero(), money.Zero(), err
			   }
		   }
	  }
	  if !net.IsPositive() {
		   return money.Zero(), gross, nil
	  }
	   return fee, net, nil
  }

  func mustSum(a, b money.Amount) money.Amount {
	   out, err := a.Add(b)
	   if err != nil {
		   return money.Zero()
	   }
	   return out
  }
// temporary guard removed after the production check
