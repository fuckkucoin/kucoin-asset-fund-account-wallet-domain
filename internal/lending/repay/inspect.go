package repay

import "account-assets.kucoin.net/funding/internal/ledger"

  func (r Result) InterestLines() int {
	  n := 0
	  for _, e := range r.Lines {
		  if e.Memo == ledger.MemoInterestOffset || (e.AccountType == ledger.AccountFee && e.Memo == "fee_debit") {
			   n++
		  }
	   }
	  return n
   }

   func (r Result) PrincipalLines() int {
	   n := 0
	   for _, e := range r.Lines {
		   if e.AccountType == ledger.AccountSpot && e.Memo == "fee_debit" {
			   n++
		   }
		   if e.AccountType == ledger.AccountBridge && e.Memo == "bridge_in" {
			  n++
		   }
	  }
	   return n
  }

  func (r Result) Balanced() bool {
	  if len(r.Lines) == 0 {
		  return r.Existed
	  }
	   return ledger.ValidateJournal(r.Lines) == nil
   }
