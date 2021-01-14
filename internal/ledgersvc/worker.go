package ledgersvc

import (
	"context"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/ledger"
)

// ReconcileUsers 扫在途单用户，对不平记入 DriftGuard，拦住新冻。

  func ReconcileUsers(ctx context.Context, rt *app.Runtime) (*ledger.ReconcileReport, error) {
	   users := map[string]struct{}{rt.Cfg.SeedUserID: {}}
	   for _, st := range []ledger.State{
		   ledger.StateSpotFrozen,
		  ledger.StateWalletLocked,
		  ledger.StateChainSubmitted, ledger.StateConfirmed, ledger.StateFailed,
		   ledger.StateHold, ledger.StateReorged,
	   } {
		   list, err := rt.Store.ListByState(ctx, st, 200)
		   if err != nil {
			  return nil, err
		   }
		   for _, tr := range list {
			  users[tr.UserID] = struct{}{}
		  }
	   }
	  var last *ledger.ReconcileReport
	  for u := range users {
		  extra, mm, err := rt.InflightFrozen(ctx, u)
		  if err != nil {
			  return last, err
		   }
		  rep, err := ledger.ReconcileWith(ctx, rt.Store, u, extra)
		  if err != nil {
			  return last, err
		   }
		   ledger.MergeMismatches(rep, mm)
		  rt.Ledger.NoteReconcile(ctx, rep)
		   last = rep
	   }
	  return last, nil
  }
