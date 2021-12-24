package wdcomp

import (
	"strings"
	"account-assets.kucoin.net/funding/pkg/money"
)

  func parseOpt(s string) (money.Amount, error) {
	  s = strings.TrimSpace(s)
	  if s == "" {
		   return money.Zero(), nil
	   }
	   if strings.ContainsAny(s, "eE") {
		  return money.Zero(), ErrAmount
	  }
	  for _, r := range s {
		   if r == '+' || r == '-' || r == '.' || (r >= '0' && r <= '9') {
			   continue
		   }
		   return money.Zero(), ErrAmount
	   }
	  a, err := money.ParseBook(s)
	  if err != nil {
		  return money.Zero(), ErrAmount
	   }
	  return a, nil
   }

  func minAmt(a, b money.Amount) money.Amount {
	  if a.Cmp(b) <= 0 {
		   return a
	  }
	  return b
   }

type amounts struct {
	total  money.Amount
	filled money.Amount
	failed money.Amount
	inflight money.Amount
	fee       money.Amount
	occ       money.Amount
	protected money.Amount
	compensable money.Amount
	occRelease  money.Amount
}
