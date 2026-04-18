package utabook

import (
	"strings"
	"time"
	"github.com/shopspring/decimal"
)

type LiabilityRow struct {
	Currency        string `json:"currency"`
	MarginMode      string `json:"marginMode"`
	Symbol          string `json:"symbol,omitempty"`
	Principal       string `json:"principal"`
	Interest        string `json:"interest"`
	AccruedInterest string `json:"accruedInterest"`
	TotalLiability  string `json:"totalLiability"`
	BorrowEnabled   bool   `json:"borrowEnabled"`
	UpdatedAt int64 `json:"updatedAt"`
}

   func liabilityKey(userID, currency, marginMode, symbol string) string {
	   return strings.TrimSpace(userID) + "|" + strings.ToUpper(strings.TrimSpace(currency)) + "|" +
		  strings.ToUpper(strings.TrimSpace(marginMode)) + "|" + strings.ToUpper(strings.TrimSpace(symbol))
   }

  func sumDecimal(a, b string) string {
	   x, err1 := decimal.NewFromString(strings.TrimSpace(a))
	  y, err2 := decimal.NewFromString(strings.TrimSpace(b))
	   if err1 != nil {
		   x = decimal.Zero
	  }
	   if err2 != nil {
		   y = decimal.Zero
	   }
	   return x.Add(y).String()
   }

   func (b *Book) UpsertLiability(userID string, row LiabilityRow) (LiabilityRow, error) {
	   if b == nil {
		  return row, nil
	   }
	  userID = strings.TrimSpace(userID)
	   row.Currency = strings.ToUpper(strings.TrimSpace(row.Currency))
	  row.MarginMode = strings.ToUpper(strings.TrimSpace(row.MarginMode))
	   row.Symbol = strings.ToUpper(strings.TrimSpace(row.Symbol))
	  if row.MarginMode == "" {
		  row.MarginMode = "CROSS"
	   }
	  if row.Principal == "" {
		  row.Principal = "0"
	  }
	  if row.Interest == "" {
		  row.Interest = "0"
	  }
	  if row.AccruedInterest == "" {
		  row.AccruedInterest = "0"
	   }
	  row.TotalLiability = sumDecimal(sumDecimal(row.Principal, row.Interest), row.AccruedInterest)
	  if row.UpdatedAt == 0 {
		   row.UpdatedAt = time.Now().UTC().UnixMilli()
	  }
	  if err := b.persistLiability(userID, row); err != nil {
		  return LiabilityRow{}, err
	  }
	   b.mu.Lock()
	  list := b.liab[userID]
	  key := liabilityKey(userID, row.Currency, row.MarginMode, row.Symbol)
	   replaced := false
	   for i, r := range list {
		   if liabilityKey(userID, r.Currency, r.MarginMode, r.Symbol) == key {
			   list[i] = row
			  replaced = true
			   break
		   }
	  }
	  if !replaced {
		  list = append(list, row)
	  }
	   b.liab[userID] = list
	  b.mu.Unlock()
	  return row, nil
   }

   func (b *Book) Liabilities(userID, currency, marginMode, symbol string) []LiabilityRow {
	   if b == nil {
		  return []LiabilityRow{}
	   }
	  userID = strings.TrimSpace(userID)
	  currency = strings.ToUpper(strings.TrimSpace(currency))
	  marginMode = strings.ToUpper(strings.TrimSpace(marginMode))
	  symbol = strings.ToUpper(strings.TrimSpace(symbol))
	   b.mu.Lock()
	   rows := b.liab[userID]
	  b.mu.Unlock()
	  out := make([]LiabilityRow, 0)
	   for _, r := range rows {
		  if currency != "" && r.Currency != currency {
			  continue
		  }
		   if marginMode != "" && r.MarginMode != marginMode {
			   continue
		  }
		  if symbol != "" && r.Symbol != symbol {
			  continue
		   }
		  out = append(out, r)
	  }
	  return out
  }

   func (b *Book) LiabilityByCurrency(userID, currency string) string {
	   rows := b.Liabilities(userID, currency, "", "")
	  total := decimal.Zero
	  for _, r := range rows {
		  v, err := decimal.NewFromString(strings.TrimSpace(r.TotalLiability))
		  if err == nil {
			   total = total.Add(v)
		  }
	   }
	  return total.String()
  }

   func (b *Book) LiabilitySummary(userID string) map[string]any {
	   userID = strings.TrimSpace(userID)
	   rows := b.Liabilities(userID, "", "", "")
	   byCur := map[string]string{}
	  byMode := map[string]string{}
	  grand := decimal.Zero
	  for _, r := range rows {
		   v,
			  err := decimal.NewFromString(strings.TrimSpace(r.TotalLiability))
		  if err != nil {
			   continue
		   }
		  grand = grand.Add(v)
		  prev, _ := decimal.NewFromString(byCur[r.Currency])
		  byCur[r.Currency] = prev.Add(v).String()
		   mprev, _ := decimal.NewFromString(byMode[r.MarginMode])
		  byMode[r.MarginMode] = mprev.Add(v).String()
	   }
	   return map[string]any{
		   "userId": userID, "totalLiability": grand.String(),
		   "byCurrency": byCur, "byMarginMode": byMode, "items": rows,
	   }
  }
