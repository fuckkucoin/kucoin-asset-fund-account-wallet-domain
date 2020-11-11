package swap

import (
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/quote"
)

// FreshPricer 带时间戳的汇率面。无印或过期返回 false，不回落 house。
type FreshPricer struct {
	Inner Pricer
	AsOf  time.Time
	TTL time.Duration
	Src string
}

   func (p FreshPricer) Rate(from, to string) (decimal.Decimal, bool) {
	  if p.Inner == nil {
		  return decimal.Zero, false
	   }
	  r, ok := p.Inner.Rate(from, to)
	   if !ok || !r.IsPositive() {
		   return decimal.Zero, false
	  }
	   q := quote.Quote{
		  Pair: quote.PairKey(from, to),
		   Rate: r,
		  AsOf: p.AsOf,
		   TTL:    p.TTL,
		   Source: quote.NormalizeSource(p.Src),
	   }
	  if q.Source == "" {
		  q.Source = quote.SourceSpotTicker
	   }
	   if p.AsOf.IsZero() || quote.NormalizeTTL(p.TTL) == 0 {
		   return decimal.Zero, false
	  }
	  // 无注入 now 时仅判断结构；过期由 Snapshot 的调用方判定。
	   if q.IsEmpty() {
		  return decimal.Zero, false
	  }
	   return r, true
  }

// Snapshot 生成确认用快照。

  func (p FreshPricer) Snapshot(from, to string) (quote.Quote, error) {
	  r, ok := p.Rate(from, to)
	   q := quote.Quote{
		   Pair: quote.PairKey(from, to),
		   Rate: r,
		  AsOf: p.AsOf,
		  TTL:  p.TTL,
		   Source: quote.NormalizeSource(p.Src),
	  }
	  if q.Source == "" {
		   q.Source = quote.SourceSpotTicker
	   }
	  if !ok {
		   if q.AsOf.IsZero() || quote.NormalizeTTL(q.TTL) == 0 || !r.IsPositive() {
			   return q, quote.ErrQuoteEmpty
		  }
		  return q, quote.ErrQuoteEmpty
	   }
	  return q, nil
  }

  func (p FreshPricer) SnapshotAt(from, to string, now time.Time) (quote.Quote, error) {
	  q, err := p.Snapshot(from, to)
	   if err != nil {
		   return q, err
	   }
	   return q,
		   quote.GuardConfirm(q, now)
   }
