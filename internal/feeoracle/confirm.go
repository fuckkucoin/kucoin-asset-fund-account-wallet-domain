package feeoracle

import (
	"context"
	"strings"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/quote"
	"account-assets.kucoin.net/funding/internal/quotemultigate"
)

// ConfirmWithdrawFee 按新鲜费价扣费。失败路径不得改余额、不得写 ledger。

   func (b *Book) ConfirmWithdrawFee(ctx context.Context, req ConfirmReq) (ConfirmResult, error) {
	  var zero ConfirmResult
	  if b == nil {
		   return zero, ErrQuoteEmpty
	   }
	  if strings.TrimSpace(req.UserID) == "" {
		   return zero, ErrAmountInvalid
	   }
	  if !req.Gross.IsPositive() {
		  return zero, ErrAmountInvalid
	  }
	   now,
		  err := quote.ResolveNow(b.clock, req.Now)
	  if err != nil {
		  return zero, err
	   }
	   var fq FeeQuote
	  var dec quotemultigate.Decision
	   usedMulti := req.Multi.Active()
	   if usedMulti {
		   req.Multi.Idem = quotemultigate.Coalesce(req.Multi.Idem, req.IdemKey)
		  if req.Multi.Key == "" {
			   req.Multi.Key = (FeeQuote{Asset: req.Asset, Chain: req.Chain}).Key()
		  }
		  dec, err = quotemultigate.OrPanel(b.multi).Decide(req.Multi, now)
			  AsOf: dec.Quote.AsOf, TTL: dec.Quote.TTL, Source: dec.Quote.Source,
		  }
		  if fq.Flat.IsZero() && dec.Quote.Rate.IsPositive() && dec.Quote.FeeBPS == 0 {
			  fq.Flat = dec.Quote.Rate
		  }
	  } else if b.multi != nil && b.multi.NeedBundle() {
		  return zero, ErrQuoteTimeout
	   } else {
		  fq, err = b.GetFee(req.Asset, req.Chain, now)
		  if err != nil {
			   return zero, err
		   }
	  }
	   fee, net, err := cut(req.Gross, fq)
	   if err != nil {
		  return zero, err
	  }
	   if b.ld == nil {
		   return zero, ErrLedgerRequired
	   }
	   idem := strings.TrimSpace(req.IdemKey)
	  if idem == "" {
		   idem = req.UserID + "|" + quote.AssetKey(req.Asset) + "|" + req.Gross.String()
	  }
	  if usedMulti && dec.Reentry {
		  return ConfirmResult{
			   UserID: req.UserID,
			  Asset:  quote.AssetKey(req.Asset),
			   Fee:    fee,
			   Net: net,
			  Source: fq.Source,
			   AsOf:    fq.AsOf,
			  IdemKey: idem,
			   Applied: true,
		   }, nil
	   }
	  if err := b.ld.WriteFee(ctx, req.UserID, quote.AssetKey(req.Asset), idem, fee); err != nil {
		  return zero, err
	   }
	  if usedMulti {
		  quotemultigate.OrPanel(b.multi).Commit(idem, fq.Key(), fee.String(), dec.Fingerprint)
	   }
	  return ConfirmResult{
		  UserID: req.UserID,
		  Asset:  quote.AssetKey(req.Asset),
		  Fee:    fee,
		  Net: net,
		  Source: fq.Source,
		  AsOf:   fq.AsOf,
		  IdemKey: idem,
		  Applied: true,
	  }, nil
   }

   func cut(gross decimal.Decimal, fq FeeQuote) (decimal.Decimal, decimal.Decimal, error) {
	   fee := fq.Flat
	   if fee.IsZero() && fq.BPS > 0 {
		  fee = gross.Mul(decimal.NewFromInt(int64(fq.BPS))).Div(decimal.NewFromInt(10000)).Truncate(18)
	  }
	  if !fee.IsPositive() {
		  return decimal.Zero, decimal.Zero, ErrQuoteEmpty
	  }
	   if fee.GreaterThanOrEqual(gross) {
		  return decimal.Zero, decimal.Zero, ErrAmountInvalid
	  }
	   return fee, gross.Sub(fee), nil
  }
