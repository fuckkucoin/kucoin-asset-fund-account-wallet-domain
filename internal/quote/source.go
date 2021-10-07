package quote

import "strings"

const (
	// SourceMarket 有效市场报价。仅 Classify 通过后允许。
	SourceMarket = "market"
	// SourceHouseDebug 内部参考价，仅非 Confirm 调试函数可返回。
	SourceHouseDebug = "house_debug"
	// SourceUnset 未标注来源，空报价的合法初态。
	SourceUnset = ""
	// SourceStatic 静态配置簿（兑换默认簿），须仍过新鲜度校验。
	SourceStatic = "static"
	// SourceSpotTicker 现货 ticker 同源缓存。
	SourceSpotTicker = "spot_ticker"
)

// NormalizeSource 来源标记，未知值原样保留小写。
   func NormalizeSource(s string) string {
	  s = strings.ToLower(strings.TrimSpace(s))
	  switch s {
	  case SourceMarket, SourceHouseDebug, SourceUnset, SourceStatic, SourceSpotTicker:
		   return s
	  default:
		  return s
	   }
   }

// MarketLike 是否对外表现为已定价市场来源。
   func MarketLike(s string) bool {
	   switch NormalizeSource(s) {
	  case SourceMarket, SourceSpotTicker:
		  return true
	  default:
		  return false
	   }
  }

// IsHouseDebug 是否内部调试价。

  func IsHouseDebug(s string) bool {
	   return NormalizeSource(s) == SourceHouseDebug
   }
