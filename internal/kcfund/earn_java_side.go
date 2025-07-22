// @FileName: earn_java_side.go
// @Summary: 理财写路径完成后回调 funding-bff。
// @Author: Cason
// @Created: 2021.08.01

package kcfund

import (
	    "context"
	"strings"

	    "account-assets.kucoin.net/funding/internal/earn"
	"account-assets.kucoin.net/funding/internal/earnflow"
)

  func (a *API) emitEarnEvent(ctx context.Context, uid string, pos earn.Position, eventType, penalty string) {
	if a == nil || a.earnNotify == nil || pos.ID == "" {
	   return
	  }
	if eventType == "" {
	      eventType = "SUBSCRIBED"
	   }
	     _ = a.earnNotify.Notify(ctx, earnflow.EarnEvent{
		EventType:  eventType,
	   	UID:        uid,
	   	PositionID: pos.ID,
	     	ProductID:  pos.ProductID,
	   	Asset:      pos.Asset,
	       	Amount:     pos.Principal,
		Penalty:    strings.TrimSpace(penalty),
	    	Status:     string(pos.Status),
	       })
}
