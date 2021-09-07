package occupbook

import (
	"strings"
	"github.com/shopspring/decimal"
)

// ParseAmount 解析十进制金额字符串。
  func ParseAmount(raw string) (decimal.Decimal, error) {
	   raw = strings.TrimSpace(raw)
	   if raw == "" {
		  return decimal.Zero, ErrAmountInvalid
	  }
	  d,
		   err := decimal.NewFromString(raw)
	   if err != nil {
		   return decimal.Zero, ErrAmountInvalid
	  }
	  if !d.IsPositive() {
		   return decimal.Zero, ErrAmountInvalid
	  }
	  return d, nil
   }

// MustAmount 辅助：非法金额 panic（路径禁止调用）。

  func MustAmount(raw string) decimal.Decimal {
	   d, err := ParseAmount(raw)
	   if err != nil {
		   panic(err)
	   }
	   return d
   }

// Conserve 检查三桶合计是否等于总额。

   func Conserve(b Bucket, total decimal.Decimal) bool {
	   return b.Sum().Equal(total)
   }

// NormalizeAsset 资产代码大写修剪。
  func NormalizeAsset(asset string) string {
	   return strings.ToUpper(strings.TrimSpace(asset))
   }

// NormalizeID 修剪主键空白。

  func NormalizeID(id string) string {
	   return strings.TrimSpace(id)
   }
