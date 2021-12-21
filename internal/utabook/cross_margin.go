package utabook

import (
	"strings"
	"time"
)

type CrossAsset struct {
	Currency string `json:"currency"`
	Total string `json:"total"`
	Available string `json:"available"`
	Hold string `json:"hold"`
	Liability     string `json:"liability"`
	MaxBorrow     string `json:"maxBorrow"`
	BorrowEnabled bool   `json:"borrowEnabled"`
	TransferEnabled bool  `json:"transferEnabled"`
	UpdatedAt       int64 `json:"updatedAt"`
}

   func (b *Book) UpsertCross(userID string, row CrossAsset) (CrossAsset, error) {
	   if b == nil {
		   return row, nil
	   }
	  userID = strings.TrimSpace(userID)
	  row.Currency = strings.ToUpper(strings.TrimSpace(row.Currency))
	   if row.Currency == "" {
		  return row, nil
	   }
	   for _, field := range []*string{&row.Total, &row.Available, &row.Hold, &row.Liability, &row.MaxBorrow} {
		  if strings.TrimSpace(*field) == "" {
			   *field = "0"
		  }
	  }
	   if row.UpdatedAt == 0 {
		   row.UpdatedAt = time.Now().UTC().UnixMilli()
	  }
	  if err := b.persistCross(userID, row); err != nil {
		  return CrossAsset{}, err
	  }
	   b.mu.Lock()
	  list := b.cross[userID]
	  replaced := false
	   for i, r := range list {
		  if r.Currency == row.Currency {
			   list[i] = row
			   replaced = true
			   break
		  }
	  }
	   if !replaced {
		  list = append(list, row)
	   }
	   b.cross[userID] = list
	   b.mu.Unlock()
	   return row, nil
   }

   func (b *Book) CrossMargin(userID, currency string) []CrossAsset {
	   if b == nil {
		  return []CrossAsset{}
	  }
	  userID = strings.TrimSpace(userID)
	  currency = strings.ToUpper(strings.TrimSpace(currency))
	  b.mu.Lock()
	  list := b.cross[userID]
	  b.mu.Unlock()
	  out := make([]CrossAsset, 0)
	   for _, r := range list {
		   if currency != "" && r.Currency != currency {
			  continue
		   }
		  out = append(out, r)
	  }
	  return out
  }

  func (b *Book) CrossMarginView(userID, currency string) map[string]any {
	  rows := b.CrossMargin(userID, currency)
	   return map[string]any{
		  "userId": userID, "marginMode": "CROSS", "assets": rows,
	   }
   }

   func (b *Book) MergeCrossFromBalance(userID,
	  currency, balance, available, hold string) (CrossAsset, error) {
	   liab := b.LiabilityByCurrency(userID, currency)
	  maxBorrow := b.RateCatalog().PotentialBorrow(userID, currency, balance)
	   return b.UpsertCross(userID, CrossAsset{
		   Currency: currency, Total: balance, Available: available, Hold: hold,
		   Liability: liab, MaxBorrow: maxBorrow, BorrowEnabled: true, TransferEnabled: true,
	   })
   }
