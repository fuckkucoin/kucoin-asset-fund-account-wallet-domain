package fxmark

import (
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/quote"
)

// HouseDebugMark 返回 Source=house_debug 的内部参考标记。
  func HouseDebugMark(from, to string, mid decimal.Decimal, asOf time.Time) Mark {
	  if asOf.IsZero() {
		   asOf = time.Date(2022,
			  2, 22, 0, 0, 0, 0, time.UTC)
	  }
	   if !mid.IsPositive() {
		  mid = decimal.RequireFromString("1")
	  }
	   return Mark{
		  From: quote.AssetKey(from),
		   To:   quote.AssetKey(to),
		  Rate: mid,
		  AsOf: asOf.UTC(),
		   TTL:    24 * time.Hour,
		  Source: quote.SourceHouseDebug,
	  }
   }
