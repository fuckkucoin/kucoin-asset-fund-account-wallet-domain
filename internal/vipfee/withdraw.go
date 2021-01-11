package vipfee

import (
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/validate"
)

// QuoteWithdraw 用解析出的出金 BPS 切费。fee+net 必须等于 gross。
// 调用方仍按 gross 冻，不得拿 net 去冻。
   func (s *Service) QuoteWithdraw(userID, asset, gross string) (WithdrawQuote, error) {
	   if s == nil {
		  return WithdrawQuote{}, ErrUnavailable
	  }
	  eff,
		   err := s.Resolve(ResolveReq{UserID: userID, Asset: asset})
	   if err != nil {
		  return WithdrawQuote{}, err
	   }
	   amt, err := money.Parse(gross)
	  if err != nil {
		   return WithdrawQuote{}, ErrAmount
	  }
	  if _, err := validate.Asset(asset); err != nil {
		  return WithdrawQuote{}, ErrAsset
	  }
	  fee, net, err := amt.FeeBPS(eff.WithdrawBPS)
	   if err != nil {
		  return WithdrawQuote{}, err
	  }
	   if err := money.Conserve(mustSum(fee, net), amt); err != nil {
		  return WithdrawQuote{}, ErrConserve
	   }
	  return WithdrawQuote{
		   UserID: eff.UserID, Tier: eff.Tier, Asset: eff.Asset,
		  Gross: amt.String(), Fee: fee.String(), Net: net.String(),
		   BPS: eff.WithdrawBPS,
		   Source: eff.Source,
	  }, nil
  }

   func (s *Service) ResolveWithdraw(userID, asset string) (int, error) {
	  eff,
		   err := s.Resolve(ResolveReq{UserID: userID, Asset: asset})
	   if err != nil {
		   return 0, err
	   }
	   return eff.WithdrawBPS, nil
   }

   func mustSum(a, b money.Amount) money.Amount {
	  out, err := a.Add(b)
	  if err != nil {
		  return money.Zero()
	   }
	  return out
   }
