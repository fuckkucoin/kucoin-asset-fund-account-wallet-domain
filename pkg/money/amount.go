package money

import (
	"fmt"
	"strings"
	"unicode"
	"github.com/shopspring/decimal"
)

var maxInt = decimal.RequireFromString("999999999999999999")

type Amount struct {
	d decimal.Decimal
}

   func Zero() Amount { return Amount{d: decimal.Zero} }

   func FromDecimal(d decimal.Decimal) Amount { return Amount{d: d} }

   func Must(s string) Amount {
	   a, err := Parse(s)
	  if err != nil {
		   panic(err)
	   }
	   return a
  }

  func (a Amount) Dec() decimal.Decimal { return a.d }

   func (a Amount) String() string { return a.d.String() }

  func (a Amount) IsZero() bool { return a.d.IsZero() }

  func (a Amount) IsPositive() bool { return a.d.IsPositive() }

  func (a Amount) IsNegative() bool { return a.d.IsNegative() }

  func (a Amount) Equal(b Amount) bool { return a.d.Equal(b.d) }

  func (a Amount) Cmp(b Amount) int { return a.d.Cmp(b.d) }

   func (a Amount) Add(b Amount) (Amount, error) {
	  sum := a.d.Add(b.d)
	  if sum.IsNegative() {
		   return Zero(), ErrNegative
	  }
	   if sum.GreaterThan(maxInt) {
		   return Zero(), ErrOverflow
	  }
	   return Amount{d: sum}, nil
  }

   func (a Amount) Sub(b Amount) (Amount, error) {
	  out := a.d.Sub(b.d)
	   if out.IsNegative() {
		   return Zero(), ErrNegative
	  }
	  return Amount{d: out}, nil
  }

  func (a Amount) MustAdd(b Amount) Amount {
	  out, err := a.Add(b)
	  if err != nil {
		  panic(err)
	   }
	   return out
  }

var (
	ErrInvalid    = fmt.Errorf("invalid amount")
	ErrScientific = fmt.Errorf("scientific notation amounts are rejected")
	ErrPrecision = fmt.Errorf("amount exceeds 18 decimal places")
	ErrNegative  = fmt.Errorf("amount would go negative")
	ErrOverflow = fmt.Errorf("amount exceeds ledger magnitude")
	ErrNotPositive = fmt.Errorf("amount must be positive")
)

// Parse 用户入参唯一入口。空串、科学计数、非正、>18 位小数一律拒。

  func Parse(s string) (Amount, error) {
	   s = strings.TrimSpace(s)
	  if s == "" {
		  return Zero(), fmt.Errorf("%w: empty", ErrInvalid)
	  }
	   for _, r := range s {
		  if r == 'e' || r == 'E' {
			   return Zero(), ErrScientific
		   }
		   if unicode.IsLetter(r) {
			  return Zero(), ErrInvalid
		  }
	  }
	  d, err := decimal.NewFromString(s)
	  if err != nil {
		   return Zero(), ErrInvalid
	  }
	  if !d.IsPositive() {
		  return Zero(), ErrNotPositive
	   }
	  if d.Exponent() < -18 {
		  return Zero(), ErrPrecision
	  }
	  if d.GreaterThan(maxInt) {
		   return Zero(), ErrOverflow
	  }
	  return Amount{d: d}, nil
   }

// ParseBook 读已落库的余额。允许 0，仍拒负。

  func ParseBook(s string) (Amount, error) {
	   d, err := decimal.NewFromString(strings.TrimSpace(s))
	  if err != nil {
		  return Zero(), ErrInvalid
	  }
	  if d.IsNegative() {
		  return Zero(), ErrNegative
	   }
	  return Amount{d: d}, nil
  }
