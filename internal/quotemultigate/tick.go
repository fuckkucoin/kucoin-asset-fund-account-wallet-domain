package quotemultigate

import (
	"strings"
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/quote"
)

const (
	StatusOK = "ok"
	StatusTimeout = "timeout"
	StatusMissing = "missing"
	StatusEmpty   = "empty"
	StatusStale = "stale"
	KindRate = "rate"
	KindFee  = "fee"
)

// Tick 单源观测。Status 非 ok 时不得当作有效价。

type Tick struct {
	Venue string `json:"venue"`
	Kind  string `json:"kind,omitempty"`
	Pair string `json:"pair,omitempty"`
	Asset     string          `json:"asset,omitempty"`
	Rate      decimal.Decimal `json:"rate"`
	FeeBPS    int             `json:"fee_bps,omitempty"`
	FeeAmount decimal.Decimal `json:"fee_amount,omitempty"`
	AsOf      time.Time       `json:"as_of"`
	TTL       time.Duration   `json:"ttl"`
	Source    string          `json:"source"`
	Status string `json:"status,omitempty"`
	RawRate string `json:"raw_rate,omitempty"`
}

// Bundle 一次确认的多源采集结果。

type Bundle struct {
	Key string `json:"key"`
	Kind     string        `json:"kind,omitempty"`
	Ticks    []Tick        `json:"ticks"`
	Elapsed  time.Duration `json:"elapsed"`
	Deadline time.Duration `json:"deadline,omitempty"`
	Idem     string        `json:"idem,omitempty"`
}

// Active 是否携带多源采集信息。仅 Elapsed>0 也视为一次采集尝试。
   func (b Bundle) Active() bool {
	   return len(b.Ticks) > 0 || b.Elapsed > 0
  }

   func (t Tick) venue() string {
	  return strings.ToLower(strings.TrimSpace(t.Venue))
   }

   func (t Tick) status() string {
	  s := strings.ToLower(strings.TrimSpace(t.Status))
	  if s == "" {
		  return StatusOK
	  }
	  return s
   }

  func (t Tick) kind(fallback string) string {
	  k := strings.ToLower(strings.TrimSpace(t.Kind))
	  if k == "" {
		  k = strings.ToLower(strings.TrimSpace(fallback))
	  }
	   if k == "" {
		   if t.FeeBPS > 0 || t.FeeAmount.IsPositive() {
			  return KindFee
		  }
		   return KindRate
	  }
	  return k
   }

   func (t Tick) AsQuote() quote.Quote {
	  pair := t.Pair
	   if pair == "" {
		  pair = t.Asset
	  }
	  return quote.Quote{
		   Pair: pair,
		  Asset: t.Asset,
		   Rate:      t.Rate,
		   FeeBPS:    t.FeeBPS,
		   FeeAmount: t.FeeAmount,
		  AsOf: t.AsOf,
		  TTL: t.TTL,
		  Source:  quote.NormalizeSource(t.Source),
		   RawRate: t.RawRate,
	  }
   }

   func TickFromQuote(venue string, q quote.Quote) Tick {
	  return Tick{
		  Venue: venue,
		  Pair:  q.Pair,
		  Asset: q.Asset,
		   Rate:  q.Rate,
		  FeeBPS:    q.FeeBPS,
		   FeeAmount: q.FeeAmount,
		   AsOf: q.AsOf,
		   TTL: q.TTL,
		   Source: q.Source,
		   Status:  StatusOK,
		   RawRate: q.RawRate,
		  Kind: kindOfQuote(q),
	   }
  }

  func kindOfQuote(q quote.Quote) string {
	   if q.FeeBPS > 0 || q.FeeAmount.IsPositive() {
		   if q.Rate.IsZero() {
			   return KindFee
		  }
	   }
	   return KindRate
  }
