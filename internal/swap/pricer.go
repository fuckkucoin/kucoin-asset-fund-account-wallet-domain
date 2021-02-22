// 兑换报价：读现货 ticker 缓存。
// 作者: Likeo
// 日期: 2021.01.13
package swap

import (
	"github.com/shopspring/decimal"
)

// Pricer 是兑换汇率面。必须与现货 ticker 同源，禁止另开一本价。
type Pricer interface {
	Rate(from, to string) (decimal.Decimal, bool)
}

type StaticPricer struct{ Rates map[string]decimal.Decimal }

  func (p StaticPricer) Rate(from, to string) (decimal.Decimal, bool) {
	   if from == to {
		   return decimal.NewFromInt(1), true
	  }
	   if r, ok := p.Rates[from+"/"+to]; ok {
		   return r, true
	  }
	  if r, ok := p.Rates[to+"/"+from]; ok && !r.IsZero() {
		   return decimal.NewFromInt(1).Div(r), true
	   }
	  return decimal.Zero, false
  }

  func DefaultPricer() StaticPricer {
	   return StaticPricer{Rates: map[string]decimal.Decimal{
		   "BTC/USDT": decimal.RequireFromString("64000"),
		   "ETH/USDT": decimal.RequireFromString("3200"),
		  "SOL/USDT": decimal.RequireFromString("140"),
		   "TRX/USDT": decimal.RequireFromString("0.12"),
		   "TON/USDT": decimal.RequireFromString("5.5"),
	   }}
  }

// CachePricer 读现货同一套 ticker；没有活印才回落到静态簿，避免钱包页和交易页两套价。
type CachePricer struct {
	Live func(from,
		  to string) (decimal.Decimal, bool)
	Has      func(from, to string) bool
	Fallback Pricer
}

  func (p CachePricer) Rate(from, to string) (decimal.Decimal, bool) {
	  if p.Live != nil {
		   if r, ok := p.Live(from, to); ok {
			   return r, true
		  }
		   // 挂了现货簿就不能换静态价，不然钱包页和成交两套数。
		   return decimal.Zero, false
	   }
	   if p.Fallback != nil {
		  return p.Fallback.Rate(from, to)
	   }
	   return decimal.Zero, false
   }
