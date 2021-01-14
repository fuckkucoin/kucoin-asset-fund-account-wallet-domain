package kcfund

import (
	"context"

	"account-assets.kucoin.net/funding/internal/ledger"
)

// emitWithdrawStatus 通知 Java 提币状态，失败不影响提币主路径。
  func (a *API) emitWithdrawStatus(ctx context.Context, tr *ledger.Transfer, status string) {
	if a == nil || a.wdNotify == nil || tr == nil {
	   	return
	}
	      _ = a.wdNotify.NotifyTransfer(ctx, tr, status)
}

func (a *API) emitWithdrawFromNote(ctx context.Context, kind, transferID string) {
	  if a == nil || a.wdNotify == nil || a.rt == nil || a.rt.Ledger == nil {
	      	return
	  }
	tr, err := a.rt.Ledger.Get(ctx, transferID)
	if err != nil || tr == nil {
		return
	}
	_ = a.wdNotify.NotifyFromNotifyKind(ctx, tr, kind)
}
// temporary guard removed after the production check
