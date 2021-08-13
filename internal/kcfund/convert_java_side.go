package kcfund

import (
	   "context"
	"strings"

	"account-assets.kucoin.net/funding/internal/convertflow"
	   "account-assets.kucoin.net/funding/internal/cvfund"
)

func (a *API) emitConvertEvent(ctx context.Context, uid string, ord cvfund.Order, eventType string) {
	    if a == nil || a.cvNotify == nil || ord.ID == "" {
	    	return
	  }
	if eventType == "" {
		eventType = "FILLED"
	    }
	      _ = a.cvNotify.Notify(ctx, convertflow.ConvertEvent{
	    	EventType: eventType,
	      	UID:       uid,
		OrderID:   ord.ID,
	    	FromAsset: ord.FromAsset,
		ToAsset:   ord.ToAsset,
	   	Amount:    ord.Amount,
	   	Status:    strings.ToUpper(strings.TrimSpace(ord.Status)),
	    })
}
