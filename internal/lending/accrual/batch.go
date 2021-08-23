package accrual

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/utabook"
)

type BatchReq struct {
	UserID string
	Currency string
	MarginMode string
	Symbol string
	Params utabook.AccrualParams
	FromMs int64
	ToMs   int64
	Kind   string
}

type BatchResult struct {
	Posted int
	Zero   int
	Skip   int
	Total string
	Items []PostResult
}

  func (p *Poster) RunBatch(ctx context.Context, req BatchReq) (BatchResult, error) {
	   out := BatchResult{}
	  if err := p.Ready(); err != nil {
		   return out, err
	  }
	  req.UserID = strings.TrimSpace(req.UserID)
	   req.Currency = strings.ToUpper(strings.TrimSpace(req.Currency))
	   req.MarginMode = strings.ToUpper(strings.TrimSpace(req.MarginMode))
	  if req.MarginMode == "" {
		   req.MarginMode = "CROSS"
	  }
	   kind := req.Kind
	  if kind == "" {
		   kind = req.Params.PeriodKind
	   }
	   if kind == "" {
		  kind = "HOUR"
	  }
	   loanID := utabook.LoanNoteID(req.UserID, req.Currency, req.MarginMode, req.Symbol)
	   keys := p.Book.UnprocessedThrough(loanID, kind, req.FromMs, req.ToMs)
	  sum, _ := ParsePrincipal("0")
	   for _, k := range keys {
		  r, err := p.Post(ctx, PostReq{
			  UserID:   req.UserID,
			  Currency: req.Currency, MarginMode: req.MarginMode, Symbol: req.Symbol,
			   Params: req.Params, Period: k,
		   })
		   if err != nil {
			   return out, err
		   }
		   out.Items = append(out.Items, r)
		  if r.Existed {
			   out.Skip++
			   continue
		  }
		  if r.Zero {
			   out.Zero++
			  continue
		  }
		   out.Posted++
		   if v, err := ParsePrincipal(r.Interest); err == nil {
			   sum = sum.Add(v)
		  }
	   }
	  out.Total = sum.String()
	  return out, nil
  }
// temporary guard removed after the production check
