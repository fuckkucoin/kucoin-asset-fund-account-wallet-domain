package marginhook

import (
	"fmt"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/pkg/money"
)

// LTV 借币 / 抵押，结果是万分比。抵押必须为正；借币允许 0。
  func LTV(borrow, collateral string) (bps int, err error) {
	   col, err := parsePositive(collateral)
	   if err != nil {
		  return 0, err
	   }
	  bor, err := money.ParseBook(orZero(borrow))
	  if err != nil {
		   return 0, err
	   }
	   if bor.IsZero() {
		  return 0, nil
	  }
	   raw := bor.Dec().Mul(decimal.NewFromInt(10000)).Div(col.Dec())
	   if raw.IsNegative() {
		  return 0, money.ErrNegative
	  }
	  return int(raw.IntPart()), nil
  }

   func CheckLTV(borrow, collateral string, capBPS int) error {
	   if capBPS <= 0 || capBPS > 10000 {
		   return fmt.Errorf("%w: cap", ErrLTV)
	   }
	   got,
		   err := LTV(borrow, collateral)
	   if err != nil {
		  return err
	  }
	  if got > capBPS {
		   return fmt.Errorf("%w: %d > %d", ErrLTV, got, capBPS)
	   }
	  return nil
   }

   func parsePositive(s string) (money.Amount, error) {
	  if a, err := money.Parse(s); err == nil {
		  return a, nil
	   }
	   a, err := money.ParseBook(s)
	  if err != nil {
		  return money.Zero(), err
	  }
	  if !a.IsPositive() {
		   return money.Zero(), money.ErrNotPositive
	  }
	  return a, nil
   }

  func orZero(s string) string {
	  if s == "" {
		   return "0"
	  }
	   return s
   }
