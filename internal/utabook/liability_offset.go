package utabook

import (
	"fmt"
	"strings"
	"github.com/shopspring/decimal"
)

type OffsetSplit struct {
	Interest decimal.Decimal
	Principal decimal.Decimal
	Surplus  decimal.Decimal
	Rejected bool
}

   func parseDec(s string) decimal.Decimal {
	  v, err := decimal.NewFromString(strings.TrimSpace(s))
	  if err != nil || v.IsNegative() {
		  return decimal.Zero
	  }
	  return v
   }

   func (r LiabilityRow) AccruedDec() decimal.Decimal {
	  a := parseDec(r.AccruedInterest)
	   if a.IsPositive() {
		  return a
	  }
	  return parseDec(r.Interest)
   }

   func (r LiabilityRow) PrincipalDec() decimal.Decimal {
	   return parseDec(r.Principal)
   }

   func (r LiabilityRow) Owed() decimal.Decimal {
	   return r.PrincipalDec().Add(r.AccruedDec())
  }

// SplitRepay 先冲应计再冲本金。surplusPolicy: reject | surplus。
   func SplitRepay(row LiabilityRow, pay decimal.Decimal, surplusPolicy string) (OffsetSplit, error) {
	   out := OffsetSplit{}
	   if !pay.IsPositive() {
		  return out, fmt.Errorf("repay amount not positive")
	   }
	   acc := row.AccruedDec()
	   prin := row.PrincipalDec()
	   if pay.LessThanOrEqual(acc) {
		  out.Interest = pay
		   return out, nil
	   }
	  out.Interest = acc
	  rest := pay.Sub(acc)
	   if rest.LessThanOrEqual(prin) {
		  out.Principal = rest
		   return out, nil
	   }
	   out.Principal = prin
	  out.Surplus = rest.Sub(prin)
	  if strings.EqualFold(strings.TrimSpace(surplusPolicy), "surplus") {
		  return out, nil
	   }
	   out.Rejected = true
	   return out, fmt.Errorf("repay exceeds owed")
   }

  func (b *Book) ApplyInterestToLiability(userID string, currency, marginMode, symbol string, interest decimal.Decimal) (LiabilityRow, error) {
	  rows := b.Liabilities(userID, currency, marginMode, symbol)
	   var row LiabilityRow
	  if len(rows) == 0 {
		   row = LiabilityRow{Currency: currency, MarginMode: marginMode, Symbol: symbol, Principal: "0", Interest: "0", AccruedInterest: "0", BorrowEnabled: true}
	  } else {
		   row = rows[0]
	   }
	  if interest.IsPositive() {
		   next := row.AccruedDec().Add(interest)
		   row.AccruedInterest = next.String()
		  row.Interest = next.String()
	   }
	  return b.UpsertLiability(userID, row)
  }

   func (b *Book) ApplyOffsetToLiability(userID string,
	   currency, marginMode, symbol string, split OffsetSplit) (LiabilityRow, error) {
	   rows := b.Liabilities(userID, currency, marginMode, symbol)
	  if len(rows) == 0 {
		  return LiabilityRow{}, fmt.Errorf("liability not found")
	   }
	  row := rows[0]
	   acc := row.AccruedDec().Sub(split.Interest)
	  if acc.IsNegative() {
		   return row, fmt.Errorf("interest offset underflow")
	   }
	   prin := row.PrincipalDec().Sub(split.Principal)
	   if prin.IsNegative() {
		  return row, fmt.Errorf("principal offset underflow")
	  }
	  row.AccruedInterest = acc.String()
	   row.Interest = acc.String()
	   row.Principal = prin.String()
	   return b.UpsertLiability(userID, row)
   }

   func (b *Book) LiabilityLoan(userID, currency, marginMode, symbol string) (LiabilityRow, bool) {
	  rows := b.Liabilities(userID, currency, marginMode, symbol)
	  if len(rows) == 0 {
		  return LiabilityRow{}, false
	  }
	  return rows[0], true
  }
