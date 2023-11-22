package fxmark

import (
	"context"
	"strings"
	"sync"
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/quote"
	"account-assets.kucoin.net/funding/internal/quotemultigate"
)

// ConvertLedger 闪兑入账面。

type ConvertLedger interface {
	WriteConvert(ctx context.Context, userID, from, to, idem string, fromAmt, toAmt decimal.Decimal) error
}

// RecConvertLedger 记录写入。
type RecConvertLedger struct {
	mu     sync.Mutex
	Writes int
	Last string
}

  func (r *RecConvertLedger) WriteConvert(_ context.Context,
	  userID, from, to, idem string, _, _ decimal.Decimal) error {
	   if r == nil {
		   return ErrLedger
	  }
	  r.mu.Lock()
	  defer r.mu.Unlock()
	  r.Writes++
	  r.Last = userID + "|" + from + "|" + to + "|" + idem
	  return nil
  }

   func (r *RecConvertLedger) Count() int {
	   if r == nil {
		   return 0
	  }
	   r.mu.Lock()
	  defer r.mu.Unlock()
	   return r.Writes
   }

// ConvertReq 闪兑确认请求。
type ConvertReq struct {
	UserID string
	From string
	To     string
	Amount decimal.Decimal
	Idem   string
	Now    time.Time
	Multi  quotemultigate.Bundle
}

// ConvertResult 成功结果。
type ConvertResult struct {
	UserID string `json:"user_id"`
	From string `json:"from"`
	To      string          `json:"to"`
	FromAmt decimal.Decimal `json:"from_amt"`
	ToAmt  decimal.Decimal `json:"to_amt"`
	Rate   decimal.Decimal `json:"rate"`
	Source string          `json:"source"`
	Applied bool `json:"applied"`
}

// ConfirmConvert 校验标记价后入账。空/过期失败且 Writes 不变。
  func (f *FreshBook) ConfirmConvert(ctx context.Context, ld ConvertLedger, req ConvertReq) (ConvertResult, error) {
	  var zero ConvertResult
	   if f == nil {
		   return zero, ErrQuoteEmpty
	   }
	   if strings.TrimSpace(req.UserID) == "" || !req.Amount.IsPositive() {
		   return zero, ErrAmount
	  }
	   now, err := quote.ResolveNow(f.clock, req.Now)
	  if err != nil {
		  return zero, err
	   }
	  var m Mark
	   var dec quotemultigate.Decision
	   usedMulti := req.Multi.Active()
	  if usedMulti {
		   req.Multi.Idem = quotemultigate.Coalesce(req.Multi.Idem, req.Idem)
		   if req.Multi.Key == "" {
			   req.Multi.Key = quote.PairKey(req.From, req.To)
		  }
		   dec,
			  err = quotemultigate.OrPanel(f.multi).Decide(req.Multi, now)
		   if err != nil {
			   return zero, err
		   }
		   m = Mark{
			   From: req.From,
			   To: req.To, Rate: dec.Quote.Rate,
			  AsOf: dec.Quote.AsOf, TTL: dec.Quote.TTL, Source: dec.Quote.Source,
		  }
	   } else if f.multi != nil && f.multi.NeedBundle() {
		   return zero, ErrQuoteTimeout
	   } else {
		  m,
			  err = f.MarkOf(req.From, req.To, now)
		  if err != nil {
			  return zero, err
		  }
	  }
	  if ld == nil {
		  return zero, ErrLedger
	  }
	  toAmt := req.Amount.Mul(m.Rate)
	  idem := strings.TrimSpace(req.Idem)
	   if idem == "" {
		   idem = req.UserID + "|" + m.Key()
	   }
	   if usedMulti && dec.Reentry {
		   return ConvertResult{
			   UserID: req.UserID,
			  From: quote.AssetKey(req.From),
			  To:   quote.AssetKey(req.To),
			  FromAmt: req.Amount,
			  ToAmt: toAmt,
			   Rate:  m.Rate,
			   Source: m.Source,
			   Applied: true,
		  }, nil
	   }
	   if err := ld.WriteConvert(ctx, req.UserID, quote.AssetKey(req.From), quote.AssetKey(req.To), idem, req.Amount, toAmt); err != nil {
		  return zero, err
	   }
	   if usedMulti {
		  quotemultigate.OrPanel(f.multi).Commit(idem, m.Key(), m.Rate.String(), dec.Fingerprint)
	  }
	  return ConvertResult{
		   UserID: req.UserID,
		   From: quote.AssetKey(req.From),
		  To:   quote.AssetKey(req.To),
		   FromAmt: req.Amount,
		  ToAmt:   toAmt,
		   Rate:   m.Rate,
		   Source: m.Source,
		  Applied: true,
	  }, nil
   }
