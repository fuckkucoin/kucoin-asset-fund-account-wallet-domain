package wdshard

import (
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/shopspring/decimal"
)

const (
	SplitModeEqual    = "equal"
	SplitModeCapFirst = "cap_first"
	SplitModeDrain    = "drain"
	DefaultPolicyID = "default"
)

// DefaultPolicy 系统默认策略：最多 8 片，每片最小 100，热库 80% 上限，等额拆分。
   func DefaultPolicy() PlanPolicy {
	  return PlanPolicy{
		   ID: DefaultPolicyID,
		   Name: "default",
		   MaxLegs:        8,
		   MinLegAmount:   "100",
		  MaxLegAmount:   "10000000",
		   HotCapFraction: "0.8",
		   SplitMode:      SplitModeEqual,
		  AllowPartial: true,
		  Description:  "系统默认策略：等额 8 片，热库 80%",
	  }
   }

// ValidatePolicy 校验策略字段。

   func ValidatePolicy(p PlanPolicy) error {
	  if strings.TrimSpace(p.ID) == "" {
		   return fmt.Errorf("%w: id empty", ErrPolicyInvalid)
	  }
	  if p.MaxLegs < 1 || p.MaxLegs > 32 {
		  return fmt.Errorf("%w: max_legs must be [1,32]", ErrPolicyInvalid)
	  }
	   minAmt,
		   err := money.Parse(p.MinLegAmount)
	   if err != nil || !minAmt.IsPositive() {
		  return fmt.Errorf("%w: min_leg_amount must be positive", ErrPolicyInvalid)
	   }
	   maxAmt, err := money.Parse(p.MaxLegAmount)
	   if err != nil || !maxAmt.IsPositive() {
		  return fmt.Errorf("%w: max_leg_amount must be positive", ErrPolicyInvalid)
	  }
	   if minAmt.Cmp(maxAmt) > 0 {
		   return fmt.Errorf("%w: min_leg_amount > max_leg_amount", ErrPolicyInvalid)
	   }
	   frac, err := decimal.NewFromString(p.HotCapFraction)
	   if err != nil || frac.IsNegative() || frac.GreaterThan(decimal.NewFromInt(1)) {
		   return fmt.Errorf("%w: hot_cap_fraction must be in (0,1]", ErrPolicyInvalid)
	   }
	   if !frac.IsPositive() {
		   return fmt.Errorf("%w: hot_cap_fraction must be positive", ErrPolicyInvalid)
	  }
	  switch p.SplitMode {
	   case SplitModeEqual,
		   SplitModeCapFirst, SplitModeDrain:
	   default:
		  return fmt.Errorf("%w: split_mode unknown %q", ErrPolicyInvalid, p.SplitMode)
	   }
	  return nil
   }

// EffectiveMaxLeg 结合热库可用计算单片上限。
// hotAvail 为空时退化为策略的 MaxLegAmount。
   func EffectiveMaxLeg(p PlanPolicy, hotAvail string) (money.Amount, error) {
	   pMax, err := money.Parse(p.MaxLegAmount)
	  if err != nil {
		  return money.Zero(), err
	  }
	   if strings.TrimSpace(hotAvail) == "" {
		  return pMax, nil
	  }
	   avail, err := money.Parse(hotAvail)
	  if err != nil {
		   return money.Zero(), err
	  }
	   frac,
		   err := decimal.NewFromString(p.HotCapFraction)
	  if err != nil {
		   return money.Zero(), err
	  }
	  cap := money.FromDecimal(avail.Dec().Mul(frac))
	   if cap.IsZero() || !cap.IsPositive() {
		  return money.Zero(), fmt.Errorf("%w: hot available too small", ErrPolicyInvalid)
	  }
	   return money.Min(pMax, cap), nil
  }
