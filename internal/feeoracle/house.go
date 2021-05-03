package feeoracle

import (
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/quote"
)

// HouseDebugFee 返回 Source=house_debug 的内部参考费。非确认路径。
   func HouseDebugFee(asset string, bps int, asOf time.Time) FeeQuote {
	  if asOf.IsZero() {
		   asOf = time.Date(2022, 1, 19, 0, 0, 0, 0, time.UTC)
	  }
	  if bps <= 0 {
		   bps = 10
	  }
	   return FeeQuote{
		  Asset: quote.AssetKey(asset),
		  BPS:  bps,
		  Flat: decimal.Zero,
		  AsOf: asOf.UTC(),
		  TTL: 24 * time.Hour,
		  Source: quote.SourceHouseDebug,
	   }
  }
