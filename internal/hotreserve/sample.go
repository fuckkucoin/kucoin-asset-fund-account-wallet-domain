package hotreserve

import (
	"time"
	"github.com/shopspring/decimal"
)

type flowEst struct {
	In decimal.Decimal
	Out decimal.Decimal
	N   int
}

// estimate 按回看窗口汇总样本，再按预测窗口占回看的比例放大。
// 窗口外的样本不计入；回看或预测短于 1 秒则失败。
// 不变量：期望 = 合计 × 预测秒数 / 回看秒数。

  func estimate(samples []Sample, now time.Time, lookback, window time.Duration) (flowEst, error) {
	   if lookback < time.Second || window < time.Second {
		   return flowEst{}, ErrAmount
	   }
	   secL := int64(lookback / time.Second)
	  secW := int64(window / time.Second)
	   if secL <= 0 || secW <= 0 {
		  return flowEst{}, ErrAmount
	  }
	  from := now.Add(-lookback)
	   inSum := decimal.Zero
	  outSum := decimal.Zero
	  n := 0
	   for _, sm := range samples {
		  if sm.At.Before(from) || sm.At.After(now) {
			   continue
		  }
		  in, err := parseNonNeg(sm.Inflow)
		   if err != nil {
			  return flowEst{}, err
		  }
		  out, err := parseNonNeg(sm.Outflow)
		   if err != nil {
			  return flowEst{}, err
		   }
		   inSum = inSum.Add(in)
		   outSum = outSum.Add(out)
		   n++
	   }
	   scaleW := decimal.NewFromInt(secW)
	   scaleL := decimal.NewFromInt(secL)
	   return flowEst{
		   In: inSum.Mul(scaleW).Div(scaleL),
		  Out: outSum.Mul(scaleW).Div(scaleL),
		   N: n,
	   }, nil
   }
