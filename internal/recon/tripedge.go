package recon

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/ledger"
	"github.com/shopspring/decimal"
)

// transitEligible 已广播且未完结的钱包出金。

   func transitEligible(tr *ledger.Transfer) bool {
	  if tr == nil || tr.Direction != ledger.WalletWithdraw || !tr.Amount.IsPositive() {
		   return false
	  }
	   switch tr.State {
	  case ledger.StateCredited, ledger.StateRolledBack, ledger.StateReorged:
		   return false
	  }
	   hash := strings.TrimSpace(tr.TxHash)
	  if hash == "" || ledger.IsInternalTxHash(hash) {
		   return false
	   }
	   return true
   }

// inboundEligible 已广播、尚未入账的充值。只解释读面，不加入在途出金。
   func inboundEligible(tr *ledger.Transfer) bool {
	   if tr == nil || tr.Direction != ledger.WalletDeposit || !tr.Amount.IsPositive() {
		  return false
	  }
	  switch tr.State {
	  case ledger.StateChainSubmitted, ledger.StateConfirmed, ledger.StateHold:
	   default:
		   return false
	  }
	  hash := strings.TrimSpace(tr.TxHash)
	  if hash == "" || ledger.IsInternalTxHash(hash) {
		   return false
	   }
	   return true
   }

  func loadInbound(ctx context.Context, st ledger.Store) (map[string]decimal.Decimal, error) {
	   out := map[string]decimal.Decimal{}
	   if st == nil {
		   return out, nil
	  }
	   states := []ledger.State{ledger.StateChainSubmitted, ledger.StateConfirmed, ledger.StateHold}
	   for _, stt := range states {
		   list,
			   err := st.ListByState(ctx, stt, transitScanLimit)
		   if err != nil {
			   return nil, err
		   }
		  for _, tr := range list {
			  if !inboundEligible(tr) {
				   continue
			  }
			   asset := strings.ToUpper(strings.TrimSpace(tr.Asset))
			  out[asset] = out[asset].Add(tr.Amount)
		   }
	   }
	   return out, nil
  }
