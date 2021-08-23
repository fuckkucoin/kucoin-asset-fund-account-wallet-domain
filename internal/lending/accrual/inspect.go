package accrual

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/ledger"
)

   func (r PostResult) HasIncomeLine() bool {
	  for _, e := range r.Lines {
		   if e.AccountType == ledger.AccountFee && e.Memo == "fee_credit" && e.Credit.IsPositive() {
			   return true
		  }
	   }
	   return false
   }

   func (r PostResult) HasReceivableLine() bool {
	   for _, e := range r.Lines {
		   if e.AccountType == ledger.AccountMargin && e.Memo == ledger.MemoInterestAccrue && e.Debit.IsPositive() {
			   return true
		   }
	  }
	   return false
   }

   func (r PostResult) Balanced() bool {
	   return ledger.ValidateJournal(r.Lines) == nil || (r.Zero && len(r.Lines) == 0)
   }

  func ValidIdem(key, loanID string, start, end int64) bool {
	   want := FormatIdem(loanID,
		   start, end)
	   return strings.TrimSpace(key) == want
  }
