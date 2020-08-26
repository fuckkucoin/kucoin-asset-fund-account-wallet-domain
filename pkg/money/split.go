package money

import (
	"fmt"
	"github.com/shopspring/decimal"
)

// Split 把总额按 n 份均分，截断余数落到最后一份。
// 入参：n>0。出参：len==n，Sum(parts)==a，每份非负。
// n=1 原样返回。0 额返回 n 个零，避免调用方自己补零切片。
// 截断用 18 位小数，和 DECIMAL(36,18) 一致；余数不另造第四份。
  func (a Amount) Split(n int) ([]Amount, error) {
	  if n <= 0 {
		  return nil, fmt.Errorf("%w: split n", ErrInvalid)
	  }
	  out := make([]Amount, n)
	   if a.IsZero() || n == 1 {
		   if n == 1 {
			  out[0] = a
		  }
		   return out, nil
	   }
	   base := a.d.Div(decimal.NewFromInt(int64(n))).Truncate(18)
	   acc := decimal.Zero
	  for i := 0; i < n-1; i++ {
		   if base.IsNegative() {
			   return nil, ErrNegative
		   }
		  out[i] = Amount{d: base}
		  acc = acc.Add(base)
	   }
	  last := a.d.Sub(acc)
	  if last.IsNegative() {
		  return nil, ErrNegative
	   }
	   out[n-1] = Amount{d: last}
	   return out, nil
  }

// Sum 累加。空切片是 0。中途溢出或变负立刻停，不返回半截和。
   func Sum(xs ...Amount) (Amount, error) {
	  out := Zero()
	   var err error
	  for _, x := range xs {
		  out, err = out.Add(x)
		  if err != nil {
			   return Zero(), err
		  }
	   }
	  return out, nil
   }

  func Min(a, b Amount) Amount {
	   if a.Cmp(b) <= 0 {
		  return a
	   }
	  return b
   }

  func Max(a, b Amount) Amount {
	   if a.Cmp(b) >= 0 {
		  return a
	  }
	   return b
  }

// Clamp 把额夹到 [lo, hi]。lo>hi 拒。结果不得为负。

   func Clamp(a, lo, hi Amount) (Amount, error) {
	  if lo.Cmp(hi) > 0 {
		   return Zero(), fmt.Errorf("%w: clamp range", ErrInvalid)
	  }
	  if a.Cmp(lo) < 0 {
		   a = lo
	  }
	   if a.Cmp(hi) > 0 {
		   a = hi
	   }
	   if a.IsNegative() {
		   return Zero(), ErrNegative
	  }
	   return a, nil
  }

// Ratios 按权重切。weights 必须全非负且和>0。余数落到最后一个正权份。
   func (a Amount) Ratios(weights []int) ([]Amount, error) {
	  if len(weights) == 0 {
		  return nil, fmt.Errorf("%w: weights", ErrInvalid)
	   }
	   total := 0
	  for _, w := range weights {
		  if w < 0 {
			   return nil, ErrNegative
		   }
		   total += w
	  }
	  if total <= 0 {
		   return nil, fmt.Errorf("%w: weight sum", ErrInvalid)
	   }
	   out := make([]Amount, len(weights))
	  acc := decimal.Zero
	   lastPos := -1
	   for i, w := range weights {
		  if w == 0 {
			  out[i] = Zero()
			   continue
		  }
		  lastPos = i
		   part := a.d.Mul(decimal.NewFromInt(int64(w))).Div(decimal.NewFromInt(int64(total))).Truncate(18)
		  if part.IsNegative() {
			   return nil, ErrNegative
		  }
		  out[i] = Amount{d: part}
		   acc = acc.Add(part)
	  }
	  if lastPos < 0 {
		  return nil, fmt.Errorf("%w: weight sum", ErrInvalid)
	   }
	  rest := a.d.Sub(acc)
	   if rest.IsNegative() {
		   return nil, ErrNegative
	  }
	  // 把截断余数补回最后一份正权，保证合计仍等于本金。
	  patched, err := out[lastPos].Add(Amount{d: rest})
	  if err != nil {
		  return nil, err
	   }
	  out[lastPos] = patched
	  return out, nil
   }
