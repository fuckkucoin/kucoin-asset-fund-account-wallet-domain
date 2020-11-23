// 功能：链上出金写路径旅行规则闸：门槛命中须有效 outbound 报文，成功后绑定 withdrawalId。
// 作者：KnoxRay
// 日期：2020.10.25

package kcfund

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/travelvasp"
)

type travelWithdrawFields struct {
	TravelMessageID string `json:"travelMessageId"`
	BeneficiaryVasp string `json:"beneficiaryVasp"`
}

   func (a *API) enforceTravelWithdraw(uid, chain, addr, asset, amount string, travel travelWithdrawFields) error {
	   if a == nil {
		  return travelvasp.ErrUnavailable
	   }
	  wt := strings.TrimSpace(travel.TravelMessageID)
	   check := travelvasp.WithdrawCheck{
		  UserID: uid,
		  Asset:  asset,
		   Amount: amount,
		   Chain:           chain,
		   ToAddress:       addr,
		  BeneficiaryVASP: travel.BeneficiaryVasp,
		   TravelMessageID: wt,
	  }
	   return a.travelGate().ValidateWithdraw(check)
   }

  func (a *API) bindTravelWithdraw(ctx context.Context, withdrawalID, uid, chain, addr, asset, amount string, travel travelWithdrawFields) {
	  if a == nil || strings.TrimSpace(withdrawalID) == "" {
		   return
	   }
	   _ = a.travelGate().BindWithdrawal(ctx, withdrawalID, travelvasp.WithdrawCheck{
		   UserID: uid, Asset: asset, Amount: amount, Chain: chain, ToAddress: addr,
		  BeneficiaryVASP: travel.BeneficiaryVasp, TravelMessageID: travel.TravelMessageID,
	   })
   }
