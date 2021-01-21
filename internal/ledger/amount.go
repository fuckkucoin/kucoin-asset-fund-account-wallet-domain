// 金额精度和流水号。
// 作者: King
// 日期: 2021.01.05
package ledger

import (
	"fmt"
	"strings"
	"unicode"
	"github.com/shopspring/decimal"
)

// maxLedgerInt DECIMAL(36,18) 整数部分上限，再大落库会溢出。

var maxLedgerInt = decimal.RequireFromString("999999999999999999")

// ExceedsLedgerMagnitude 超出 DECIMAL(36,18) 就落不进去。

   func ExceedsLedgerMagnitude(d decimal.Decimal) bool {
	   if !d.IsPositive() {
		  return false
	  }
	   return d.GreaterThan(maxLedgerInt)
  }

// ParseAmount 用户数量唯一入口。科学计数、NaN/Inf、超过 18 位小数、超量级一律拒，避免流水字符串和 DECIMAL(36,18) 对不上。

  func ParseAmount(s string) (decimal.Decimal, error) {
	  s = strings.TrimSpace(s)
	  if s == "" {
		   return decimal.Zero, fmt.Errorf("%w: empty", ErrInvalidAmount)
	  }
	  for _, r := range s {
		   if r == 'e' || r == 'E' {
			   return decimal.Zero, ErrScientific
		  }
		   if unicode.IsLetter(r) {
			   return decimal.Zero, fmt.Errorf("%w", ErrInvalidAmount)
		   }
	  }
	   d, err := decimal.NewFromString(s)
	  if err != nil {
		  return decimal.Zero, fmt.Errorf("%w", ErrInvalidAmount)
	  }
	  if !d.IsPositive() {
		   return decimal.Zero, fmt.Errorf("%w: must be positive", ErrInvalidAmount)
	  }
	  if d.Exponent() < -18 {
		  return decimal.Zero, ErrPrecision
	  }
	  if ExceedsLedgerMagnitude(d) {
		  return decimal.Zero, ErrAmountOverflow
	  }
	   return d, nil
  }

// RoundLedger 过账金额收口到 DECIMAL(36,18)，禁止乘法残留超精度。
  func RoundLedger(d decimal.Decimal) (decimal.Decimal, error) {
	  if d.Exponent() < -18 {
		   d = d.Truncate(18)
	   }
	  if !d.IsPositive() {
		   return decimal.Zero, fmt.Errorf("%w: must be positive", ErrInvalidAmount)
	   }
	  if ExceedsLedgerMagnitude(d) {
		   return decimal.Zero, ErrAmountOverflow
	  }
	   return d, nil
  }

// ParseRatio 成交比例入口：区间 [0, 1]，不允许科学计数。空串按 1 处理。
  func ParseRatio(s string) (decimal.Decimal, error) {
	   s = strings.TrimSpace(s)
	   if s == "" {
		  return decimal.NewFromInt(1), nil
	   }
	   for _, r := range s {
		  if r == 'e' || r == 'E' || unicode.IsLetter(r) {
			   return decimal.Zero, fmt.Errorf("%w: fill_ratio", ErrInvalidAmount)
		   }
	   }
	   d,
		  err := decimal.NewFromString(s)
	  if err != nil {
		   return decimal.Zero, fmt.Errorf("%w: fill_ratio", ErrInvalidAmount)
	  }
	   if d.IsNegative() || d.GreaterThan(decimal.NewFromInt(1)) {
		   return decimal.Zero, fmt.Errorf("%w: fill_ratio out of [0,1]", ErrInvalidAmount)
	   }
	  return d, nil
  }

// ParseBook 读我们自己落库的 DECIMAL 字符串。不再套用户入口规则（StringFixed(18) 不会写出科学计数）。
   func ParseBook(s string) (decimal.Decimal, error) {
	   d, err := decimal.NewFromString(s)
	  if err != nil {
		  return decimal.Zero, fmt.Errorf("%w: %s", ErrInvalidAmount, s)
	   }
	   return d, nil
   }

// NextJournalSeq 冻结后（seq 1–2）接着编流水号，入账/回滚不得和冻结那对撞号。

   func NextJournalSeq(existing []JournalEntry) int {
	   max := 0
	  for _, e := range existing {
		  if e.Seq > max {
			   max = e.Seq
		  }
	  }
	  return max + 1
  }
