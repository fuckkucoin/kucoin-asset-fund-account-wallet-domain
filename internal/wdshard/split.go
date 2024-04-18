package wdshard

import (
	"fmt"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/shopspring/decimal"
)

// SplitResult 拆分结果。

type SplitResult struct {
	Amounts  []money.Amount
	LegCount int
	Shrank   bool // 热库不足导致分片数缩小
	Warning string
}

// SplitAmount 按策略拆分总额。
// total > 0；policy 已过 ValidatePolicy；hotAvail 可为空（忽略热库约束）。
  func SplitAmount(total money.Amount, policy PlanPolicy, hotAvail string) (SplitResult, error) {
	  if !total.IsPositive() {
		   return SplitResult{},
			   ErrAmountZero
	  }
	  minAmt, err := money.Parse(policy.MinLegAmount)
	  if err != nil {
		   return SplitResult{}, err
	   }
	   effectiveMax,
		  err := EffectiveMaxLeg(policy, hotAvail)
	  if err != nil {
		   return SplitResult{}, err
	   }
	  switch policy.SplitMode {
	   case SplitModeEqual:
		   return splitEqual(total, minAmt, effectiveMax, policy.MaxLegs)
	   case SplitModeCapFirst:
		  return splitCapFirst(total, minAmt, effectiveMax, policy.MaxLegs)
	   case SplitModeDrain:
		  return splitDrain(total, minAmt, effectiveMax, policy.MaxLegs)
	   default:
		   return SplitResult{}, fmt.Errorf("%w: %s", ErrPolicyInvalid, policy.SplitMode)
	   }
   }

// splitEqual 等额切分。若每片超过 effectiveMax，则增加片数（不超过 maxLegs）。
// 若总额无法在 maxLegs 内满足 minAmt 约束，返回 ErrLegAmount。
  func splitEqual(total, minAmt, effectiveMax money.Amount, maxLegs int) (SplitResult, error) {
	  // 至少需要几片才能让每片 <= effectiveMax？
	  minLegs := 1
	  for {
		  legAmt := money.FromDecimal(total.Dec().Div(decimal.NewFromInt(int64(minLegs))).Truncate(18))
		  if legAmt.Cmp(effectiveMax) <= 0 {
			  break
		  }
		   minLegs++
		  if minLegs > maxLegs {
			   return SplitResult{}, fmt.Errorf("%w: total %s cannot fit in %d legs of max %s each",
				   ErrTooManyLegs, total, maxLegs, effectiveMax)
		   }
	   }
	  // 最多切几片才能让每片 >= minAmt？
	   maxFit := maxLegs
	   for maxFit > 1 {
		   legAmt := money.FromDecimal(total.Dec().Div(decimal.NewFromInt(int64(maxFit))).Truncate(18))
		   if legAmt.Cmp(minAmt) >= 0 {
			   break
		  }
		  maxFit--
	  }
	  n := maxFit
	   if n < minLegs {
		   n = minLegs
	  }
	   if n > maxLegs {
		   n = maxLegs
	  }
	  parts, err := total.Split(n)
	   if err != nil {
		  return SplitResult{}, err
	  }
	  // 核对最后一片不为尘（尘并入末片已由 Split 保证，但末片须 >= minAmt）。
	  if parts[n-1].Cmp(minAmt) < 0 && n > 1 {
		   // 末片太小，合并到前一片。
		  merged, aerr := parts[n-2].Add(parts[n-1])
		   if aerr != nil {
			  return SplitResult{}, aerr
		   }
		   parts[n-2] = merged
		  parts = parts[:n-1]
		  n--
	   }
	  shrank := n < maxLegs && minLegs > 1
	   warn := ""
	   if shrank {
		   warn = fmt.Sprintf("hot cap reduced to %d legs", n)
	  }
	  return SplitResult{Amounts: parts, LegCount: n, Shrank: shrank, Warning: warn}, nil
  }

// splitCapFirst 尽量用 effectiveMax 填满每片，余数落最后一片。

   func splitCapFirst(total, minAmt, effectiveMax money.Amount, maxLegs int) (SplitResult, error) {
	   var parts []money.Amount
	  remain := total
	   for remain.IsPositive() {
		   if len(parts) >= maxLegs {
			   return SplitResult{}, fmt.Errorf("%w: cap_first overflow %d legs", ErrTooManyLegs, maxLegs)
		  }
		   piece := money.Min(remain,
			   effectiveMax)
		  sub, err := remain.Sub(piece)
		  if err != nil {
			   return SplitResult{}, err
		   }
		   // 最后剩余太小，并入本片。
		   if sub.IsPositive() && sub.Cmp(minAmt) < 0 {
			  piece, err = piece.Add(sub)
			  if err != nil {
				   return SplitResult{}, err
			   }
			   sub = money.Zero()
		   }
		   parts = append(parts, piece)
		  remain = sub
	   }
	   if len(parts) == 0 {
		   return SplitResult{},
			   ErrAmountZero
	  }
	  return SplitResult{Amounts: parts, LegCount: len(parts)}, nil
  }

// splitDrain 从剩余里每次取 min(remain, effectiveMax)，直到 maxLegs 内耗尽。
// 与 cap_first 类似但语义更明确。
  func splitDrain(total, minAmt, effectiveMax money.Amount, maxLegs int) (SplitResult, error) {
	   return splitCapFirst(total, minAmt, effectiveMax, maxLegs)
   }

// VerifyConservation 断言拆分结果之和等于原始总额。

  func VerifyConservation(total money.Amount, parts []money.Amount) error {
	   sum, err := money.Sum(parts...)
	   if err != nil {
		  return err
	  }
	  return money.Conserve(total, sum)
   }
