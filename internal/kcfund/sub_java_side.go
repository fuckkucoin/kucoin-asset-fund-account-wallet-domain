package kcfund

import (
	"context"

	  "account-assets.kucoin.net/funding/internal/subaccounts"
)

func (a *API) emitSubLifecycle(ctx context.Context, eventType, masterUID, subUserID, subName string, payload map[string]any) {
	if a == nil || a.subNotify == nil {
		return
	       }
	_ = a.subNotify.Notify(ctx, subaccounts.LifecycleEvent{
	  	EventType: eventType,
		MasterUID: masterUID,
	    	SubUserID: subUserID,
		SubName:   subName,
		Payload:   payload,
	   })
}
