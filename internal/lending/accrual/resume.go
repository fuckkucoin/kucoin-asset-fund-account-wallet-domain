package accrual

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/utabook"
)

type ResumeReq struct {
	UserID   string
	Currency string
	MarginMode string
	Symbol string
	Params utabook.AccrualParams
	   if err := p.Ready(); err != nil {
		  return BatchResult{}, err
	  }
	   req.UserID = strings.TrimSpace(req.UserID)
	  req.Currency = strings.ToUpper(strings.TrimSpace(req.Currency))
	   req.MarginMode = strings.ToUpper(strings.TrimSpace(req.MarginMode))
	   if req.MarginMode == "" {
		  req.MarginMode = "CROSS"
	  }
	   kind := req.Params.PeriodKind
	   if kind == "" {
		   kind = "HOUR"
	  }
	  loanID := utabook.LoanNoteID(req.UserID, req.Currency, req.MarginMode, req.Symbol)
	   from := req.NowMs
	   if last, ok := p.Book.LastProcessed(loanID); ok {
		  from = last.Key.PeriodStart
	  } else if req.Lookback > 0 {
		   from = req.NowMs - int64(req.Lookback)*utabook.PeriodSpanMs(kind)
	   }
	  return p.RunBatch(ctx, BatchReq{
		  UserID: req.UserID, Currency: req.Currency, MarginMode: req.MarginMode, Symbol: req.Symbol,
		   Params: req.Params, FromMs: from, ToMs: req.NowMs, Kind: kind,
	  })
  }
