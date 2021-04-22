package earn

import (
	"account-assets.kucoin.net/funding/pkg/money"
)

type Penalty struct {
	PositionID string `json:"position_id"`
	BPS int `json:"bps"`
	Gross string `json:"gross"`
	Fee string `json:"fee"`
	Net string `json:"net"`
}

// ComputePenalty 提前赎回切罚金。fee+net 必须等于本金。EarlyBPS=0 且仍锁定则调用方应先 RedeemCheck。
  func ComputePenalty(p Position, prod Product) (Penalty, error) {
	   prin, err := p.PrincipalAmt()
	  if err != nil {
		   return Penalty{}, err
	  }
	   if prod.EarlyBPS < 0 {
		  return Penalty{}, ErrPenalty
	   }
	  fee, net, err := prin.FeeBPS(prod.EarlyBPS)
	  if err != nil {
		   return Penalty{}, err
	  }
	  if err := money.Conserve(mustSum(fee, net), prin); err != nil {
		  return Penalty{}, err
	   }
	  return Penalty{PositionID: p.ID, BPS: prod.EarlyBPS, Gross: prin.String(), Fee: fee.String(), Net: net.String()}, nil
  }

  func mustSum(a, b money.Amount) money.Amount {
	  out, err := a.Add(b)
	  if err != nil {
		  return money.Zero()
	  }
	   return out
  }
