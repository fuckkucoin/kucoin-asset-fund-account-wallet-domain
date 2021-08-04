package isobook

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

type persistSQL = persist.SQLDB

  func (b *Book) BindSQL(db *persist.SQLDB) *Book {
	   if b != nil {
		   b.sql = db
		  b.loadSQL()
	   }
	  return b
   }

  func (b *Book) persistPair(p Pair) {
	   if b == nil || b.sql == nil || b.sql.DB == nil {
		   return
	   }
	  _ = sqlrepo.NewFundingSubjects(b.sql).PutIsolated(context.Background(), sqlrepo.IsolatedPair{
		  UserID: p.UserID, Symbol: p.Symbol, Status: p.Status, DebtRatio: p.DebtRatio,
		   BaseCurrency: p.Base.Currency, BaseTotal: p.Base.Total, BaseAvailable: p.Base.Available,
		  BaseHold:      p.Base.Hold,
		  BaseLiability: p.Base.Liability,
		   QuoteCurrency: p.Quote.Currency, QuoteTotal: p.Quote.Total, QuoteAvailable: p.Quote.Available,
		  QuoteHold: p.Quote.Hold, QuoteLiability: p.Quote.Liability,
		  UpdatedAt: time.Now().UTC(),
	  })
  }

   func (b *Book) loadSQL() {
	  if b == nil || b.sql == nil || b.sql.DB == nil {
		  return
	  }
	   rows, err := sqlrepo.NewFundingSubjects(b.sql).ListIsolated(context.Background())
	  if err != nil || len(rows) == 0 {
		   return
	  }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   for _, r := range rows {
		   p := &Pair{
			   UserID: r.UserID, Symbol: r.Symbol, Status: r.Status, DebtRatio: r.DebtRatio,
			   Base: Side{
				   Currency: r.BaseCurrency, Total: r.BaseTotal, Available: r.BaseAvailable,
				  Hold: r.BaseHold, Liability: r.BaseLiability, LiabilityPrincipal: "0", LiabilityInterest: "0",
				  MaxBorrowSize: "0", BorrowEnabled: false, TransferInEnabled: true,
			  },
			   Quote: Side{
				   Currency: r.QuoteCurrency,
				  Total: r.QuoteTotal, Available: r.QuoteAvailable,
				  Hold: r.QuoteHold, Liability: r.QuoteLiability, LiabilityPrincipal: "0", LiabilityInterest: "0",
				  MaxBorrowSize: "0", BorrowEnabled: false, TransferInEnabled: true,
			   },
		   }
		   b.rows[r.UserID+"|"+r.Symbol] = p
	  }
   }
