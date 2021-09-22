package p2pgate

import (
	"strings"
	"github.com/shopspring/decimal"
)

type decAcc struct {
	v     decimal.Decimal
	valid bool
	bad   bool
}

  func (a *decAcc) add(raw string) {
	  if a == nil {
		   return
	   }
	   n, err := decimal.NewFromString(strings.TrimSpace(raw))
	   if err != nil {
		   a.bad = true
		  return
	  }
	   if !a.valid {
		   a.v = n
		  a.valid = true
		   return
	   }
	  a.v = a.v.Add(n)
   }

   func (a decAcc) sub(b decAcc) decAcc {
	  if !a.valid && !b.valid {
		  return decAcc{}
	   }
	  left, right := a.v, b.v
	   if !a.valid {
		   left = decimal.Zero
	  }
	  if !b.valid {
		  right = decimal.Zero
	  }
	   return decAcc{v: left.Sub(right), valid: true}
  }

   func (a decAcc) addAcc(b decAcc) decAcc {
	   if !a.valid {
		   return b
	  }
	  if !b.valid {
		   return a
	  }
	  return decAcc{v: a.v.Add(b.v), valid: true}
   }

  func (a decAcc) string() string {
	   if !a.valid {
		   return "0"
	   }
	   return a.v.String()
   }

   func (a decAcc) failed() bool { return a.bad }

  func (a decAcc) isPositive() bool {
	   return a.valid && a.v.IsPositive()
   }

   func parseDec(raw string) (decimal.Decimal, bool) {
	  n, err := decimal.NewFromString(strings.TrimSpace(raw))
	  if err != nil {
		  return decimal.Zero, false
	   }
	   return n, true
   }

  func decEqual(a, b string) bool {
	  x, okx := parseDec(a)
	  y, oky := parseDec(b)
	  if !okx || !oky {
		   return false
	  }
	   return x.Equal(y)
  }

   func sumAmounts(vals []string) string {
	   var acc decAcc
	   for _, v := range vals {
		   acc.add(v)
	  }
	  return acc.string()
   }

   func addAmountMap(dst map[string]string, asset, amount string) {
	  if dst == nil {
		  return
	   }
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	  if asset == "" {
		   return
	  }
	   var acc decAcc
	  acc.add(dst[asset])
	   acc.add(amount)
	   dst[asset] = acc.string()
  }
