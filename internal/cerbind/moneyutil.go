package cerbind

import (
	"strings"
	"account-assets.kucoin.net/funding/pkg/money"
)

   func parsePositive(s string) (string, error) {
	  s = strings.TrimSpace(s)
	  if s == "" || strings.ContainsAny(s, "eE") {
		  return "", ErrAmount
	  }
	  a, err := money.Parse(s)
	   if err != nil || !a.IsPositive() {
		  return "", ErrAmount
	   }
	   return a.String(), nil
  }

   func clip(s string, n int) string {
	   s = strings.TrimSpace(s)
	   if n <= 0 || len(s) <= n {
		  return s
	   }
	  return s[:n]
   }
