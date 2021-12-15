package utabook

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/isobook"
)

// IsolatedReader 由运行时注入 isobook，避免 utabook 与 isobook 循环依赖写路径。
type IsolatedReader func(userID, symbol string) []isobook.Pair

var isolatedHook IsolatedReader

   func SetIsolatedReader(fn IsolatedReader) {
	   isolatedHook = fn
   }

  func (b *Book) IsolatedMargin(userID, symbol string) map[string]any {
	   userID = strings.TrimSpace(userID)
	  symbol = strings.ToUpper(strings.TrimSpace(symbol))
	  pairs := []isobook.Pair{}
	   if isolatedHook != nil {
		  pairs = isolatedHook(userID, symbol)
	  }
	   items := make([]map[string]any, 0, len(pairs))
	  for _, p := range pairs {
		  items = append(items, map[string]any{
			  "symbol": p.Symbol, "status": p.Status, "debtRatio": p.DebtRatio,
			   "baseAsset": p.Base, "quoteAsset": p.Quote,
		  })
	   }
	   return map[string]any{
		   "userId": userID, "marginMode": "ISOLATED", "accounts": items,
	   }
  }

   func (b *Book) IsolatedLiabilityTotal(userID, symbol string) string {
	   if isolatedHook == nil {
		   return "0"
	   }
	  sum := "0"
	   for _, p := range isolatedHook(userID, symbol) {
		  sum = sumDecimal(sum, p.Base.Liability)
		  sum = sumDecimal(sum, p.Quote.Liability)
	   }
	   return sum
   }

   func (b *Book) MarginRead(userID, marginMode, symbol, currency string) map[string]any {
	  marginMode = strings.ToUpper(strings.TrimSpace(marginMode))
	  switch marginMode {
	   case "ISOLATED", "ISOLATED_MARGIN":
		  return b.IsolatedMargin(userID, symbol)
	  case "CROSS", "CROSS_MARGIN", "":
		  return b.CrossMarginView(userID, currency)
	   default:
		  return map[string]any{"userId": userID, "marginMode": marginMode, "error": "unsupported_margin_mode"}
	  }
  }
