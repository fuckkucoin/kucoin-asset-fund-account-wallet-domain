package partial

import (
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/margin"
	"github.com/shopspring/decimal"
)

  func parseDecAmt(raw, field string) (decimal.Decimal, error) {
	   raw = strings.TrimSpace(raw)
	   if raw == "" {
		   return decimal.Zero, fmt.Errorf("%s required", field)
	   }
	  d,
		  err := decimal.NewFromString(raw)
	   if err != nil {
		   return decimal.Zero, fmt.Errorf("%s: %w", field, err)
	   }
	   if d.IsNegative() {
		   return decimal.Zero, fmt.Errorf("%s negative", field)
	   }
	  return d, nil
   }

  func addAmt(a, b string) (string, error) {
	   x, err := parseDecAmt(a, "a")
	   if err != nil {
		  return "", err
	   }
	  y, err := parseDecAmt(b, "b")
	  if err != nil {
		   return "", err
	   }
	  return x.Add(y).String(), nil
  }

   func subAmt(a, b string) (string, error) {
	  x, err := parseDecAmt(a, "a")
	  if err != nil {
		   return "", err
	   }
	  y, err := parseDecAmt(b, "b")
	  if err != nil {
		   return "", err
	  }
	  if y.GreaterThan(x) {
		   return "", fmt.Errorf("%w: over-release", ErrClosed)
	  }
	   return x.Sub(y).String(), nil
   }

  func mustConserve(initial, remaining, released string) error {
	   return margin.AssertConserve(initial, remaining, released)
   }
