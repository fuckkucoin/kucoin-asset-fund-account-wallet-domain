package daycut

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/lending/repay"
	"account-assets.kucoin.net/funding/internal/utabook"
	"github.com/shopspring/decimal"
)

type ReconReq struct {
	UserID   string
	Currency string
	MarginMode string
	Symbol string
	Period utabook.PeriodKey
	RepayIdem string
	OffsetAmt string
}

type ReconResult struct {
	Key ReconKey
	Batch BatchRec
	Matched  bool
	AccrueOK bool
	OffsetOK bool
}

   func (e *Engine) ReconcileOffset(ctx context.Context, req ReconReq) (ReconResult, error) {
	   out := ReconResult{}
	  if err := e.Ready(); err != nil {
		   return out, err
	   }
	  req.UserID = strings.TrimSpace(req.UserID)
	   req.Currency = strings.ToUpper(strings.TrimSpace(req.Currency))
	  req.MarginMode = strings.ToUpper(strings.TrimSpace(req.MarginMode))
	  if req.MarginMode == "" {
		   req.MarginMode = "CROSS"
	   }
	  loanID := utabook.LoanNoteID(req.UserID, req.Currency, req.MarginMode, req.Symbol)
	  if !req.Period.Valid() {
		   req.Period.LoanID = loanID
	   }
	   if !req.Period.Valid() {
		   return out, ErrWindow
	   }
	  cutMs := req.Period.PeriodStart
	   key := KeyOf(loanID, req.Period.Kind, cutMs)
	  batch, err := e.RequireBatch(key)
	   if err != nil {
		  return out, err
	  }
	  if batch.Status != StatusPosted {
		   return out, ErrMissPost
	   }
	   out.Batch = batch
	   accrueCol := batch.AccruedTotal
	  if mark, ok := e.Book.PeriodMark(req.Period); ok {
		   accrueCol = mark.Amount
		   out.AccrueOK = true
	   } else {
		  return out, e.Cfg.RefuseMissPost()
	  }
	   rk := BuildReconKey(loanID,
		   req.Period, accrueCol, req.OffsetAmt, req.RepayIdem)
	  out.Key = rk
	  if !rk.Thick() {
		   return out, ErrReconcile
	  }
	   if e.Led != nil && strings.TrimSpace(rk.AccrueIdem) != "" {
		  tr, err := e.Led.LoanTransferByIdem(ctx, rk.AccrueIdem)
		  if err != nil || tr == nil {
			   return out, e.Cfg.RefuseMissPost()
		  }
		  if !ledger.DayCutAccrueAligned(tr, loanID, req.Period.PeriodStart, req.Period.PeriodEnd) {
			   return out, ErrReconcile
		   }
	   }
	  if e.Led != nil && strings.TrimSpace(req.RepayIdem) != "" {
		   tr, err := e.Led.LoanTransferByIdem(ctx, req.RepayIdem)
		   if err == nil && tr != nil {
			  out.OffsetOK = ledger.DayCutOffsetAligned(tr,
				  loanID)
			   if !out.OffsetOK {
				  return out, ErrReconcile
			   }
			  if want, err := decimal.NewFromString(strings.TrimSpace(req.OffsetAmt)); err == nil && want.IsPositive() {
				   if !tr.Amount.Equal(want) && !tr.Amount.GreaterThanOrEqual(want) {
					   return out, ErrReconcile
				  }
			  }
		   }
	   }
	  out.Matched = out.AccrueOK && rk.Thick()
	   return out, nil
  }

   func (e *Engine) OffsetThenReconcile(ctx context.Context, cut RunReq, off repay.Req, period utabook.PeriodKey) (ReconResult, error) {
	   if _, err := e.Run(ctx, cut); err != nil {
		   return ReconResult{}, err
	  }
	  if e.Rep == nil {
		   return ReconResult{},
			  ErrUnavailable
	  }
	  res, err := e.Rep.Offset(ctx, off)
	   if err != nil {
		  return ReconResult{}, err
	  }
	  return e.ReconcileOffset(ctx, ReconReq{
		   UserID: cut.UserID, Currency: cut.Currency, MarginMode: cut.MarginMode, Symbol: cut.Symbol,
		   Period: period, RepayIdem: res.IdemKey, OffsetAmt: res.Split.Interest.String(),
	   })
   }
