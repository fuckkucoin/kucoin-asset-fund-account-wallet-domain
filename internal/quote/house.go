package quote

import (
	"strings"
	"time"
	"github.com/shopspring/decimal"
)

// HouseDebugMid 返回内部参考中间价。Confirm 调用栈禁止引用本函数。

   func HouseDebugMid(pair string, mid decimal.Decimal, asOf time.Time) Quote {
	   if asOf.IsZero() {
		  asOf = time.Date(2021, 12, 14, 0, 0, 0, 0, time.UTC)
	  }
	  if !mid.IsPositive() {
		  mid = decimal.RequireFromString("1")
	   }
	   from, to := splitPair(pair)
	   return Quote{
		  Pair: PairKey(from, to),
		  Rate: mid,
		  AsOf: asOf.UTC(),
		   TTL:  24 * time.Hour,
		  Source: SourceHouseDebug,
	   }
   }

   func splitPair(pair string) (string, string) {
	  pair = strings.ToUpper(strings.TrimSpace(pair))
	  i := strings.IndexByte(pair, '/')
	  if i <= 0 || i >= len(pair)-1 {
		  if pair == "" {
			  return "", "USDT"
		  }
		  return pair, "USDT"
	   }
	   return pair[:i], pair[i+1:]
   }
