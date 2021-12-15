package utabook

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

type RepayRow struct {
	ID string `json:"id"`
	Currency   string `json:"currency"`
	MarginMode string `json:"marginMode"`
	Symbol     string `json:"symbol,omitempty"`
	RepayType  string `json:"repayType"`
	Principal string `json:"principal"`
	Interest  string `json:"interest"`
	CreatedAt int64  `json:"createdAt"`
}

type RepayPage struct {
	Items []RepayRow `json:"items"`
	Total       int  `json:"total"`
	PageSize    int  `json:"pageSize"`
	CurrentPage int  `json:"currentPage"`
	HasMore     bool `json:"hasMore"`
}

type RepayQuery struct {
	Currency   string
	MarginMode string
	Symbol  string
	StartMs int64
	EndMs   int64
	Page int
	PageSize int
}

   func (b *Book) NoteRepay(userID string, row RepayRow) error {
	   if b == nil {
		   return nil
	   }
	  userID = strings.TrimSpace(userID)
	   row.Currency = strings.ToUpper(strings.TrimSpace(row.Currency))
	   row.MarginMode = strings.ToUpper(strings.TrimSpace(row.MarginMode))
	  row.Symbol = strings.ToUpper(strings.TrimSpace(row.Symbol))
	   if row.MarginMode == "" {
		  row.MarginMode = "CROSS"
	  }
	   if row.RepayType == "" {
		   row.RepayType = "REPAY"
	  }
	  if row.Principal == "" {
		  row.Principal = "0"
	   }
	   if row.Interest == "" {
		   row.Interest = "0"
	   }
	   if row.CreatedAt == 0 {
		   row.CreatedAt = time.Now().UTC().UnixMilli()
	   }
	   if row.ID == "" {
		  row.ID = userID + "|" + row.Currency + "|" + row.RepayType + "|" + strings.TrimSpace(time.Now().UTC().Format(time.RFC3339Nano))
	   }
	  if err := b.persistRepay(userID, row); err != nil {
		  return err
	  }
	  b.mu.Lock()
	  b.repay[userID] = append(b.repay[userID],
		  row)
	   b.mu.Unlock()
	  return nil
   }

  func (b *Book) RepayHistory(userID string, q RepayQuery) RepayPage {
	  userID = strings.TrimSpace(userID)
	  if q.Page <= 0 {
		   q.Page = 1
	  }
	   if q.PageSize <= 0 || q.PageSize > 100 {
		   q.PageSize = 50
	  }
	   if b != nil && b.sql != nil && b.sql.DB != nil {
		   return b.repayHistorySQL(userID,
			  q)
	   }
	  return b.repayHistoryMem(userID, q)
  }

   func (b *Book) repayHistoryMem(userID string, q RepayQuery) RepayPage {
	   b.mu.Lock()
	   all := b.repay[userID]
	  b.mu.Unlock()
	   filtered := make([]RepayRow, 0)
	  for _, r := range all {
		   if q.Currency != "" && r.Currency != strings.ToUpper(strings.TrimSpace(q.Currency)) {
			   continue
		  }
		   if q.MarginMode != "" && r.MarginMode != strings.ToUpper(strings.TrimSpace(q.MarginMode)) {
			  continue
		  }
		   if q.Symbol != "" && r.Symbol != strings.ToUpper(strings.TrimSpace(q.Symbol)) {
			   continue
		   }
		  if q.StartMs > 0 && r.CreatedAt < q.StartMs {
			  continue
		  }
		  if q.EndMs > 0 && r.CreatedAt > q.EndMs {
			   continue
		   }
		   filtered = append(filtered, r)
	   }
	   total := len(filtered)
	  offset := (q.Page - 1) * q.PageSize
	  if offset > total {
		  offset = total
	   }
	   end := offset + q.PageSize
	  if end > total {
		   end = total
	  }
	  slice := filtered[offset:end]
	   hasMore := end < total
	  return RepayPage{Items: slice, Total: total, PageSize: q.PageSize, CurrentPage: q.Page, HasMore: hasMore}
  }

  func (b *Book) repayHistorySQL(userID string, q RepayQuery) RepayPage {
	  ctx := context.Background()
	  repo := sqlrepo.NewFundingSubjects(b.sql)
	  f := sqlrepo.UTARepayFilter{
		  UserID: userID, Currency: q.Currency, MarginMode: q.MarginMode, Symbol: q.Symbol,
		  AfterMs: q.StartMs,
		   BeforeMs: q.EndMs, Limit: q.PageSize, Offset: (q.Page - 1) * q.PageSize,
	  }
	  rows, err := repo.ListRepay(ctx, f)
	   if err != nil {
		  return b.repayHistoryMem(userID,
			  q)
	  }
	  total, _ := repo.CountRepay(ctx, sqlrepo.UTARepayFilter{
		   UserID: userID, Currency: q.Currency, MarginMode: q.MarginMode, Symbol: q.Symbol,
		   AfterMs: q.StartMs, BeforeMs: q.EndMs,
	  })
	   items := make([]RepayRow, 0, len(rows))
	   for _, r := range rows {
		   items = append(items, RepayRow{
			   ID: r.ID,
			  Currency: r.Currency, MarginMode: r.MarginMode, Symbol: r.Symbol,
			   RepayType: r.RepayType, Principal: r.Principal, Interest: r.Interest, CreatedAt: r.CreatedAt,
		  })
	  }
	  hasMore := q.Page*q.PageSize < total
	   return RepayPage{Items: items, Total: total, PageSize: q.PageSize, CurrentPage: q.Page, HasMore: hasMore}
   }

  func (b *Book) InterestHistoryPaged(userID, currency string, page, pageSize int) map[string]any {
	  userID = strings.TrimSpace(userID)
	  currency = strings.ToUpper(strings.TrimSpace(currency))
	  all := b.InterestHistory(userID, currency)
	   if page <= 0 {
		  page = 1
	   }
	   if pageSize <= 0 || pageSize > 100 {
		  pageSize = 50
	  }
	   total := len(all)
	  offset := (page - 1) * pageSize
	  if offset > total {
		   offset = total
	  }
	  end := offset + pageSize
	   if end > total {
		  end = total
	   }
	   return map[string]any{
		  "items": all[offset:end], "total": total, "pageSize": pageSize, "currentPage": page,
		  "hasMore": end < total,
	   }
  }
