package utabook

import (
	"strings"
	"github.com/shopspring/decimal"
)

type ReadSnapshot struct {
	UserID      string         `json:"userId"`
	Mode        string         `json:"accountMode"`
	Risk        RiskView       `json:"risk"`
	Liabilities []LiabilityRow `json:"liabilities"`
	Cross      []CrossAsset `json:"crossAssets"`
	Borrowable []string     `json:"borrowable"`
}

  func (b *Book) Snapshot(userID string) ReadSnapshot {
	  userID = strings.TrimSpace(userID)
	  return ReadSnapshot{
		   UserID: userID, Mode: b.Mode(userID), Risk: b.Risk(userID),
		  Liabilities: b.Liabilities(userID, "", "", ""),
		  Cross:       b.CrossMargin(userID, ""), Borrowable: b.Borrowable(),
	  }
   }

  func (b *Book) GatewayOverview(userID string) map[string]any {
	   snap := b.Snapshot(userID)
	   return map[string]any{
		  "userId": snap.UserID, "accountMode": snap.Mode,
		   "riskGrade": snap.Risk.RiskGrade, "marginLevel": snap.Risk.MarginLevel,
		   "liabilityCount": len(snap.Liabilities), "crossAssetCount": len(snap.Cross),
		   "borrowable": snap.Borrowable,
	  }
   }

   func (b *Book) TotalCrossLiability(userID string) string {
	   sum := decimal.Zero
	  for _, row := range b.CrossMargin(userID, "") {
		  v, err := decimal.NewFromString(strings.TrimSpace(row.Liability))
		  if err == nil {
			  sum = sum.Add(v)
		   }
	  }
	   if sum.IsPositive() {
		  return sum.String()
	   }
	   return b.LiabilityByCurrency(userID, "")
   }

   func (b *Book) ValidateBorrowReadClosed(currency string) bool {
	   currency = strings.ToUpper(strings.TrimSpace(currency))
	   if currency == "" {
		  return false
	  }
	   lim := b.InterestLimits(currency)
	   rate, err := decimal.NewFromString(strings.TrimSpace(lim.CurrentRateHourly))
	  if err != nil {
		  return false
	  }
	  // 未配置非零利率且 borrow_limit 为 0 时，读面仍返回结构但 potentialBorrow 为 0（fail-closed 借入写路径在邻域）。
	   max, err := decimal.NewFromString(strings.TrimSpace(lim.BorrowLimitTotal))
	  if err != nil {
		  return false
	  }
	   return rate.IsZero() && max.IsZero()
  }

  func (b *Book) DescribeCurrency(userID,
	   currency string) map[string]any {
	   currency = strings.ToUpper(strings.TrimSpace(currency))
	   en := b.EnrichCurrency(userID, currency, "0", "0")
	  return map[string]any{
		  "currency": currency,
		   "limits":   b.InterestLimits(currency),
		  "tiers": b.ListRateTiers(currency),
		   "collateral": b.CollateralRule(currency),
		   "enrichment": en,
	   }
   }
