package swap

import (
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/quote"
)

// HouseDebugRate 返回 house_debug 快照，仅运营。
  func HouseDebugRate(from, to string, mid decimal.Decimal, asOf time.Time) quote.Quote {
	  return quote.HouseDebugMid(quote.PairKey(from,
		  to), mid, asOf)
  }
