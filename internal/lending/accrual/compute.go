package accrual

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/utabook"
	"github.com/shopspring/decimal"
)

type ComputeIn struct {
	Principal decimal.Decimal
	Params    utabook.AccrualParams
	Factor    decimal.Decimal // 时间因子，缺省 1
}

type ComputeOut struct {
	Base decimal.Decimal
	Rate decimal.Decimal
	Factor decimal.Decimal
	Interest decimal.Decimal
	Zero     bool
	Reason   string
}

   func Compute(in ComputeIn) ComputeOut {
	  out := ComputeOut{Factor: in.Factor}
	  if !out.Factor.IsPositive() {
		   out.Factor = decimal.NewFromInt(1)
	  }
	   p := in.Params.Normalize()
	  rate, ok := p.RateForPeriod()
	  if !ok {
		  out.Zero = true
		   out.Reason = "rate_not_positive"
		   return out
	   }
	  out.Rate = rate
	   base := in.Principal
	  if !base.IsPositive() {
		   out.Zero = true
		   out.Reason = "principal_not_positive"
		  return out
	   }
	   free := p.InterestFreeDec()
	  if free.IsPositive() {
		  base = base.Sub(free)
	  }
	  if !base.IsPositive() {
		   out.Zero = true
		  out.Reason = "inside_interest_free"
		  return out
	  }
	   out.Base = base
	   out.Interest = base.Mul(rate).Mul(out.Factor)
	  if !out.Interest.IsPositive() {
		  out.Zero = true
		   out.Reason = "rounded_zero"
	  }
	  return out
  }

  func ParsePrincipal(s string) (decimal.Decimal, error) {
	  v, err := decimal.NewFromString(strings.TrimSpace(s))
	  if err != nil || v.IsNegative() {
		  return decimal.Zero,
			   ErrPrincipal
	   }
	   return v, nil
  }
