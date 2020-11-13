package kcfund

import (
	   "context"
	"strings"

	        "account-assets.kucoin.net/funding/internal/accounttransfers"
	    "account-assets.kucoin.net/funding/internal/ledger"
	  "account-assets.kucoin.net/funding/internal/subacct"
)

// emitAccountTransferEvent 划转成功后通知 Java，失败不影响主路径。
func (a *API) emitAccountTransferEvent(ctx context.Context, tr *ledger.Transfer, transferType, fromKC, toKC, clientOid string) {
	   if a == nil || a.xferNotify == nil || tr == nil {
	   	return
	}
	_ = a.xferNotify.Notify(ctx, accounttransfers.TransferEvent{
	  	OrderID:         tr.ID,
		UID:             tr.UserID,
		Currency:        tr.Asset,
	      	Amount:          tr.Amount.String(),
		FromAccountType: fromKC,
	  	ToAccountType:   toKC,
	   	TransferType:    transferType,
		ClientOid:       clientOid,
	})
}

   func (a *API) emitAccountTransferHop(ctx context.Context, hop subacct.HopRec, uid, kind, clientOid, currency, amount string) {
	if a == nil || a.xferNotify == nil {
	    	return
	}
	fromKC, toKC := "MAIN", "SUB"
	  if strings.EqualFold(kind, "SUB_TO_PARENT") {
	  	fromKC, toKC = "SUB", "MAIN"
	}
	if a.rt != nil && a.rt.Ledger != nil && strings.TrimSpace(hop.TransferID) != "" {
	    	tr, err := a.rt.Ledger.Get(ctx, hop.TransferID)
	   	if err == nil && tr != nil {
			a.emitAccountTransferEvent(ctx, tr, kind, fromKC, toKC, clientOid)
	  		return
	    	}
	}
	_ = a.xferNotify.Notify(ctx, accounttransfers.TransferEvent{
	   	OrderID:         hop.ID,
	   	UID:             uid,
	    	Currency:        currency,
		Amount:          amount,
	  	FromAccountType: fromKC,
	  	ToAccountType:   toKC,
		TransferType:    kind,
	    	ClientOid:       clientOid,
	   })
}
