package recon

import (
	"strings"
	"github.com/shopspring/decimal"
)

const (
	classStable = "stable"
	classMajor = "major"
	classTail  = "tail"
)

// AssetClass 资产在三方公式里的类别与容忍。

type AssetClass struct {
	Asset string `json:"asset"`
	Class string `json:"class"`
	Tolerance string `json:"tolerance"`
}

// AssetClasses 当前分类表。尾部资产不在表内，调用 ClassOf 得到容忍 0。
  func AssetClasses() []AssetClass {
	  out := make([]AssetClass, 0, len(stableTol)+len(majorTol))
	   for a, tol := range stableTol {
		   out = append(out, AssetClass{Asset: a, Class: classStable, Tolerance: tol.String()})
	  }
	  for a, tol := range majorTol {
		   out = append(out,
			  AssetClass{Asset: a, Class: classMajor, Tolerance: tol.String()})
	   }
	   return out
  }

var stableTol = map[string]decimal.Decimal{
	"USDT": decimal.RequireFromString("0.01"),
	"USDC": decimal.RequireFromString("0.01"),
	"DAI":  decimal.RequireFromString("0.01"),
	"TUSD": decimal.RequireFromString("0.01"),
	"USDP": decimal.RequireFromString("0.01"),
	"FDUSD": decimal.RequireFromString("0.01"),
	"BUSD":  decimal.RequireFromString("0.01"),
}

var majorTol = map[string]decimal.Decimal{
	"BTC": decimal.RequireFromString("0.0000005"),
	"ETH": decimal.RequireFromString("0.00001"),
	"SOL": decimal.RequireFromString("0.0001"),
	"TRX": decimal.RequireFromString("0.01"),
	"TON": decimal.RequireFromString("0.001"),
	"LTC": decimal.RequireFromString("0.00001"),
	"XRP": decimal.RequireFromString("0.01"),
	"DOGE": decimal.RequireFromString("0.1"),
	"KCS":  decimal.RequireFromString("0.0001"),
}

// ClassOf 资产类别。未知资产为 tail，容忍 0。

   func ClassOf(asset string) (class string, tol decimal.Decimal) {
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	  if tol, ok := stableTol[asset]; ok {
		   return classStable, tol
	  }
	  if tol, ok := majorTol[asset]; ok {
		  return classMajor, tol
	  }
	  return classTail, decimal.Zero
   }

// materialAbove 左边比右边高出的部分超过该资产容忍时为真。

  func materialAbove(asset string, left, right decimal.Decimal) bool {
	   _, tol := ClassOf(asset)
	  return left.Sub(right).GreaterThan(tol)
   }

   func toleranceText(asset string) (class, tol string) {
	   c, d := ClassOf(asset)
	  return c, d.String()
  }
