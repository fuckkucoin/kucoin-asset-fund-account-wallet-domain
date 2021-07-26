package hotreserve

import (
	"strings"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/shopspring/decimal"
)

   func parseNonNeg(s string) (decimal.Decimal, error) {
	   s = strings.TrimSpace(s)
	  if s == "" || strings.ContainsAny(s, "eE") {
		   return decimal.Zero, ErrAmount
	   }
	  for _, r := range s {
		  if r == '+' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			   return decimal.Zero, ErrAmount
		  }
	  }
	  a, err := money.ParseBook(s)
	   if err != nil {
		  return decimal.Zero, ErrAmount
	   }
	   return a.Dec(), nil
   }

   func parsePos(s string) (decimal.Decimal, error) {
	  a, err := money.Parse(strings.TrimSpace(s))
	   if err != nil {
		  return decimal.Zero, ErrAmount
	  }
	  return a.Dec(), nil
  }

   func sumDec(vals []decimal.Decimal) decimal.Decimal {
	   out := decimal.Zero
	   for _, v := range vals {
		   out = out.Add(v)
	   }
	   return out
  }

   func maxDec(a, b decimal.Decimal) decimal.Decimal {
	  if a.GreaterThan(b) {
		  return a
	   }
	  return b
  }

   func posDelta(live, snap decimal.Decimal) decimal.Decimal {
	   d := live.Sub(snap)
	   if d.IsNegative() {
		  return decimal.Zero
	   }
	   return d
  }
