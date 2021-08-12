package ledseal

import (
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/shopspring/decimal"
)

// 绝对值阈值（单位：账本计量单位，即 decimal 精度）。
var (
	// critAbsThreshold 绝对差值超过此量即为 critical（资产最小显示单位 * 10000）。
	critAbsThreshold = decimal.NewFromInt(10000)
	// majorAbsThreshold 绝对差值超过此量即为 major。
	majorAbsThreshold = decimal.NewFromInt(1)
)

// 相核对率阈值（delta / max(before_total, after_total)）。
var (
	critRelThreshold = decimal.NewFromFloat(0.01) // 1%
	majorRelThreshold = decimal.NewFromFloat(0.0001) // 0.01%
)

// gradeSeverity 对单条漂移行计算严重度。
// 只读金额，不写任何账本行。

   func gradeSeverity(aBefore, aAfter, fBefore, fAfter money.Amount) DriftSeverity {
	   // 合计 before/after
	   totalBefore := aBefore.Dec().Add(fBefore.Dec())
	   totalAfter := aAfter.Dec().Add(fAfter.Dec())
	  // 净 delta（可用 + 冻结合计变化）
	   netDelta := totalAfter.Sub(totalBefore).Abs()
	  // 分母取 before/after 中较大者，防止除零。
	   denom := totalBefore
	  if totalAfter.GreaterThan(totalBefore) {
		   denom = totalAfter
	   }
	   if netDelta.IsZero() {
		  return SeverityNone
	  }
	   // 绝对值先判断。
	   if netDelta.GreaterThanOrEqual(critAbsThreshold) {
		  return SeverityCritical
	   }
	  // 相核对率判断（denom 为零则退化为绝对值判断）。
	  if !denom.IsZero() {
		  rel := netDelta.Div(denom)
		  if rel.GreaterThanOrEqual(critRelThreshold) {
			  return SeverityCritical
		  }
		  if rel.GreaterThanOrEqual(majorRelThreshold) {
			  if netDelta.GreaterThanOrEqual(majorAbsThreshold) {
				   return SeverityMajor
			   }
		  }
	   } else {
		  // denom == 0 说明 before/after 都是 0，但 delta != 0，不可能；走 minor 兜底。
		   return SeverityMinor
	   }
	   return SeverityMinor
   }

// signedDecStr 给 decimal 加符号前缀。

   func signedDecStr(d decimal.Decimal) string {
	  if d.IsZero() {
		  return "0"
	   }
	   if d.IsNegative() {
		  return d.String()
	   }
	  return "+" + d.String()
   }
