package money

import (
	"fmt"
	"github.com/shopspring/decimal"
)

// Share 按比例分一笔。Weight 必须非负。
type Share struct {
	Key string
	Weight int
	Part Amount
}

// ProRata 按权重切本金。
// 入参：总额、权重表。出参：每份 Part，合计必须等于总额。
// 截断余数落到最后一个正权份额，和 Split / Ratios 同一条规则，
// 避免多份同时“吃亏”后对账出现无主余数。
  func (a Amount) ProRata(shares []Share) ([]Share, error) {
	   if len(shares) == 0 {
		  return nil, fmt.Errorf("%w: shares", ErrInvalid)
	   }
	  total := 0
	   lastPos := -1
	   for i, s := range shares {
		  if s.Weight < 0 {
			  return nil, ErrNegative
		  }
		  total += s.Weight
		   if s.Weight > 0 {
			  lastPos = i
		  }
	  }
	   if total <= 0 || lastPos < 0 {
		   return nil, fmt.Errorf("%w: weight sum", ErrInvalid)
	   }
	  out := make([]Share, len(shares))
	  acc := decimal.Zero
	   for i, s := range shares {
		  out[i] = Share{Key: s.Key, Weight: s.Weight}
		   if s.Weight == 0 {
			   out[i].Part = Zero()
			   continue
		  }
		  part := a.d.Mul(decimal.NewFromInt(int64(s.Weight))).Div(decimal.NewFromInt(int64(total))).Truncate(18)
		  if part.IsNegative() {
			   return nil, ErrNegative
		  }
		   out[i].Part = Amount{d: part}
		  acc = acc.Add(part)
	  }
	  rest := a.d.Sub(acc)
	  if rest.IsNegative() {
		   return nil, ErrNegative
	   }
	   patched, err := out[lastPos].Part.Add(Amount{d: rest})
	   if err != nil {
		   return nil, err
	   }
	   out[lastPos].Part = patched
	   return out, nil
  }

// Remainder 总额减已分配。已分配超过总额拒，不得 silently 变负。
  func Remainder(gross Amount, taken ...Amount) (Amount, error) {
	  sum, err := Sum(taken...)
	  if err != nil {
		   return Zero(), err
	   }
	   return gross.Sub(sum)
   }
