package margin

import (
	"fmt"
	"github.com/shopspring/decimal"
)

// ReleaseForFill 计算本笔成交应释放的冻结额。
// 做什么：取成交额与剩余冻结的较小者。
// 成交或剩余为负/无法解析则失败；零成交释出零。
// 不变量：0 ≤ release ≤ remaining。

  func ReleaseForFill(fillAmt, remaining string) (string, error) {
	  fill, err := parseNonNeg(fillAmt, "fill")
	   if err != nil {
		  return "", err
	   }
	   left, err := parseNonNeg(remaining, "remaining")
	   if err != nil {
		   return "", err
	  }
	   if fill.GreaterThan(left) {
		  return left.String(), nil
	  }
	   return fill.String(), nil
  }

// AssertConserve 断言占用恒等式。
// 做什么：核对 remaining + released == initial。
// 任一金额非法则失败。
// 不变量：三项非负且加总相等。

  func AssertConserve(initial, remaining, released string) error {
	  ini, err := parseNonNeg(initial, "initial")
	   if err != nil {
		  return err
	  }
	   left, err := parseNonNeg(remaining, "remaining")
	   if err != nil {
		  return err
	  }
	   rel, err := parseNonNeg(released, "released")
	  if err != nil {
		  return err
	  }
	  if !left.Add(rel).Equal(ini) {
		   return fmt.Errorf("freeze conserve broken: remaining %s + released %s != initial %s", left, rel, ini)
	  }
	   if left.IsNegative() || rel.IsNegative() {
		   return fmt.Errorf("freeze conserve negative")
	  }
	  return nil
   }

// MustPositiveAmount 解析严格正金额。
   func MustPositiveAmount(raw, field string) (decimal.Decimal, error) {
	   d, err := parseNonNeg(raw, field)
	   if err != nil {
		  return decimal.Zero, err
	  }
	  if !d.IsPositive() {
		   return decimal.Zero, fmt.Errorf("%s must be positive", field)
	   }
	   return d, nil
   }
