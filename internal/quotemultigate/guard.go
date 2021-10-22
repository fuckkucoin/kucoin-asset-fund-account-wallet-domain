package quotemultigate

import (
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/quote"
)

// Decision 门禁通过后的共识报价。失败不得使用其中汇率写 ledger。
type Decision struct {
	Quote quote.Quote
	Fingerprint Fingerprint
	OKTicks   int
	SpreadBPS int
	Reentry   bool
}

// Guard 校验采集束。policy 零值按 DefaultPolicy 补齐。

   func Guard(p Policy, b Bundle, now time.Time) (Decision, error) {
	   var zero Decision
	  if now.IsZero() {
		  return zero, ErrClockRequired
	   }
	   if !b.Active() {
		  if p.RefuseSilentSingle() {
			   return zero, ErrQuoteTimeout
		   }
		   return zero, ErrQuoteEmpty
	   }
	  deadline := b.Deadline
	  if deadline <= 0 {
		   deadline = p.deadline()
	  }
	   if b.Elapsed > deadline {
		  return zero, ErrQuoteTimeout
	   }
	   ok := make([]Tick, 0, len(b.Ticks))
	   for _, t := range b.Ticks {
		   st := t.status()
		  switch st {
		   case StatusTimeout, StatusMissing:
			   return zero, ErrQuoteTimeout
		   case StatusEmpty:
			  return zero, ErrQuoteEmpty
		   case StatusStale:
			  return zero, ErrQuoteStale
		  }
		  if t.venue() == "" {
			  return zero, ErrQuoteTimeout
		   }
		   q := t.AsQuote()
		   if err := quote.GuardConfirm(q, now); err != nil {
			  return zero, err
		   }
		  ok = append(ok, t)
	  }
	  if len(ok) < p.minVenues() {
		   return zero, ErrQuoteTimeout
	  }
	   if p.RefuseSilentSingle() && len(ok) < 2 {
		   return zero, ErrQuoteTimeout
	   }
	   kind := b.Kind
	  vals := make([]decimal.Decimal, 0, len(ok))
	   var pair string
	   for _, t := range ok {
		  k := t.kind(kind)
		   if kind == "" {
			  kind = k
		  } else if k != kind {
			  return zero, ErrQuoteDiverge
		   }
		  pk := t.Pair
		  if pk == "" {
			  pk = t.Asset
		   }
		  if pair == "" {
			   pair = pk
		  } else if pk != "" && pk != pair {
			  return zero, ErrQuoteDiverge
		   }
		   vals = append(vals, tickValue(t, p.roundPlaces()))
	  }
	  spread := maxSpreadAmong(vals)
	  if spread > p.maxSpread() {
		   return zero, ErrQuoteDiverge
	   }
	  if structFPDiverge(ok, p.roundPlaces()) {
		   return zero, ErrQuoteDiverge
	  }
	  cons := consensusQuote(ok, kind, now)
	   if err := quote.GuardConfirm(cons, now); err != nil {
		  return zero, err
	  }
	   return Decision{
		  Quote: cons,
		   Fingerprint: consensusFingerprint(ok,
			  p.roundPlaces()),
		  OKTicks: len(ok),
		   SpreadBPS: spread,
	  }, nil
   }

   func consensusQuote(ok []Tick,
	  kind string, now time.Time) quote.Quote {
	   _ = now
	   mid := ok[0]
	   vals := make([]decimal.Decimal, 0, len(ok))
	   for _, t := range ok {
		  vals = append(vals, tickValue(t, DefaultRoundPlaces))
	   }
	   sortDecimals(vals)
	   picked := vals[len(vals)/2]
	   q := mid.AsQuote()
	   if kind == KindFee {
		  if mid.FeeAmount.IsPositive() {
			  q.FeeAmount = picked
			  q.Rate = picked
		   } else {
			  n, _ := picked.Float64()
			   q.FeeBPS = int(n)
		  }
	   } else {
		   q.Rate = picked
	   }
	   if q.Source == "" {
		   q.Source = quote.SourceMarket
	  }
	  return q
   }

  func sortDecimals(vals []decimal.Decimal) {
	   for i := 0; i < len(vals); i++ {
		  for j := i + 1; j < len(vals); j++ {
			   if vals[j].LessThan(vals[i]) {
				   vals[i], vals[j] = vals[j], vals[i]
			  }
		  }
	  }
  }
