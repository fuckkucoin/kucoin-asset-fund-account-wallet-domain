package utabook

import (
	"strings"
)

type BalanceEnrichment struct {
	Currency  string `json:"currency"`
	Liability string `json:"liability"`
	PotentialBorrow  string `json:"potentialBorrow"`
	CollateralStatus string `json:"collateralStatus"`
}

  func (b *Book) EnrichCurrency(userID, currency, equity, available string) BalanceEnrichment {
	  if b == nil {
		   return BalanceEnrichment{Currency: currency, Liability: "0", PotentialBorrow: "0", CollateralStatus: "0"}
	  }
	  currency = strings.ToUpper(strings.TrimSpace(currency))
	  liab := b.LiabilityByCurrency(userID, currency)
	  eq := strings.TrimSpace(equity)
	   if eq == "" {
		   eq = strings.TrimSpace(available)
	  }
	  pot := b.RateCatalog().PotentialBorrow(userID, currency, eq)
	   return BalanceEnrichment{
		  Currency: currency, Liability: liab, PotentialBorrow: pot,
		  CollateralStatus: b.CollateralStatus(currency),
	   }
   }

  func (b *Book) EnrichBalances(userID string, rows []map[string]string) []map[string]any {
	   out := make([]map[string]any,
		  0, len(rows))
	   for _, r := range rows {
		   cur := strings.ToUpper(strings.TrimSpace(r["currency"]))
		   eq := r["equity"]
		  if eq == "" {
			   eq = r["balance"]
		  }
		   en := b.EnrichCurrency(userID, cur, eq, r["available"])
		  row := map[string]any{
			   "currency":  cur,
			  "equity":    r["equity"],
			  "hold":      r["hold"],
			  "balance":   r["balance"],
			   "available": r["available"],
			   "liability": en.Liability,
			   "potentialBorrow":  en.PotentialBorrow,
			  "collateralStatus": en.CollateralStatus,
		  }
		  if row["equity"] == nil || row["equity"] == "" {
			   row["equity"] = r["balance"]
		   }
		  out = append(out, row)
	  }
	  return out
   }
