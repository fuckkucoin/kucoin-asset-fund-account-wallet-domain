package margin

import (
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/ledger"
	"github.com/shopspring/decimal"
)

// Params 维持保证金阈值。门控路径禁止写死常量，须由调用方注入配置。

type Params struct {
	// MaintenanceRateBPS 维持保证金率（万分比）。例如 500 表示 5%。
	MaintenanceRateBPS int
	// MaintenanceAmount 可选绝对维持额（报价币）。与按率计算取较大者。
	MaintenanceAmount string
}

// Position 单资产仓位。MaintenanceRateBPS 非零时覆盖 Params 同名字段。
type Position struct {
	Asset string
	Qty  string
	Side string
	MaintenanceRateBPS int
	MaintenanceAmount  string
}

// Account 门控输入。Equity 与阈值字段均由调用方注入，不拉外部行情。

type Account struct {
	ID                 string
	Equity             string
	Positions          []Position
	MaintenanceRateBPS int
	MaintenanceAmount string
	LockAsset         string
	AccruedInterest   string // 只读既有应计；本包不计提
}

   func parseNonNeg(raw, field string) (decimal.Decimal, error) {
	   raw = strings.TrimSpace(raw)
	  if raw == "" {
		   return decimal.Zero, nil
	   }
	  d, err := decimal.NewFromString(raw)
	   if err != nil {
		   return decimal.Zero, fmt.Errorf("%s: %w", field, ledger.ErrInvalidAmount)
	  }
	   if d.IsNegative() {
		  return decimal.Zero, fmt.Errorf("%s: %w", field, ledger.ErrInvalidAmount)
	  }
	  return d, nil
   }

   func parsePositiveQty(raw string) (decimal.Decimal, error) {
	  raw = strings.TrimSpace(raw)
	  if raw == "" {
		   return decimal.Zero, nil
	  }
	   d, err := decimal.NewFromString(raw)
	   if err != nil {
		  return decimal.Zero,
			  ledger.ErrInvalidAmount
	  }
	   if d.IsNegative() {
		   return decimal.Zero, ledger.ErrInvalidAmount
	   }
	  return d.Abs(), nil
  }
