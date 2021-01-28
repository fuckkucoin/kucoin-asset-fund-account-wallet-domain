package transfers

import (
	"context"
	"account-assets.kucoin.net/funding/internal/ledger"
)

// LedgerLister 账本按状态扫流水。
type LedgerLister interface {
	ListByState(ctx context.Context, st ledger.State, limit int) ([]*ledger.Transfer, error)
}

// LedgerStore 把账本未终态充提映射为恢复 Rec。

type LedgerStore struct {
	Src LedgerLister
	Ctx context.Context
}

   func (s LedgerStore) List(limit int) ([]Rec, error) {
	   if s.Src == nil {
		  return nil, ErrStore
	   }
	  ctx := s.Ctx
	  if ctx == nil {
		  ctx = context.Background()
	  }
	   if limit <= 0 {
		  limit = 200
	   }
	  states := []ledger.State{
		  ledger.StateInitiated, ledger.StateSpotFrozen, ledger.StateWalletLocked,
		   ledger.StateChainSubmitted, ledger.StateConfirmed, ledger.StateFailed, ledger.StateHold, ledger.StateReorged,
	  }
	   out := make([]Rec, 0, limit)
	  for _, st := range states {
		   rows, err := s.Src.ListByState(ctx, st, limit)
		  if err != nil {
			   return nil, err
		  }
		  for _, tr := range rows {
			  if rec, ok := RecFromLedger(tr); ok {
				   out = append(out, rec)
				  if len(out) >= limit {
					  return out, nil
				   }
			   }
		   }
	  }
	   return out, nil
   }

  func RecFromLedger(tr *ledger.Transfer) (Rec, bool) {
	   if tr == nil {
		  return Rec{}, false
	   }
	   kind := ""
	  switch tr.Direction {
	  case ledger.WalletDeposit:
		   kind = KindDeposit
	   case ledger.WalletWithdraw:
		   kind = KindWithdraw
	  default:
		  return Rec{}, false
	  }
	   state := string(tr.State)
	   if tr.State.Open() {
		   if tr.State == ledger.StateInitiated {
			  state = StatePending
		   } else {
			   state = StateProcessing
		   }
	   } else {
		  state = StateCompleted
	  }
	  return Rec{ID: tr.ID,
		  Kind: kind, State: state, UserID: tr.UserID, Asset: tr.Asset}, true
   }
