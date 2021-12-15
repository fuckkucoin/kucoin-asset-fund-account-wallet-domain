package utabook

import (
	"strings"
	"github.com/shopspring/decimal"
)

   func (b *Book) PostedSum(loanID string) decimal.Decimal {
	   sum := decimal.Zero
	  for _, m := range b.ListMarks(loanID) {
		   if m.Kind != MarkPosted {
			  continue
		  }
		   v, err := decimal.NewFromString(strings.TrimSpace(m.Amount))
		  if err == nil {
			   sum = sum.Add(v)
		   }
	   }
	  return sum
  }

  func (b *Book) ZeroCount(loanID string) int {
	   n := 0
	   for _, m := range b.ListMarks(loanID) {
		   if m.Kind == MarkZero {
			   n++
		   }
	  }
	  return n
  }

  func (b *Book) PostedCount(loanID string) int {
	  n := 0
	  for _, m := range b.ListMarks(loanID) {
		  if m.Kind == MarkPosted {
			   n++
		   }
	  }
	   return n
  }

  func (b *Book) AccrualAgrees(userID, currency, mode, symbol string) bool {
	  row, ok := b.LiabilityLoan(userID, currency, mode, symbol)
	   if !ok {
		  return b.PostedSum(LoanNoteID(userID, currency, mode, symbol)).IsZero()
	  }
	   return row.AccruedDec().Equal(b.PostedSum(LoanNoteID(userID, currency, mode, symbol)))
   }

  func (b *Book) NextOpenPeriod(loanID, kind string, nowMs int64) PeriodKey {
	  cur := PeriodOf(loanID, kind, nowMs)
	  if last, ok := b.LastProcessed(loanID); ok && last.Key.PeriodEnd > cur.PeriodStart {
		   return NextPeriod(last.Key)
	   }
	   return cur
   }
