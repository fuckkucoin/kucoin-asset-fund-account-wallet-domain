package fxmark

import (
	"sync"
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/quote"
	"account-assets.kucoin.net/funding/internal/quotemultigate"
)

// Mark 单币记价。
type Mark struct {
	From string `json:"from"`
	To string `json:"to"`
	Rate   decimal.Decimal `json:"rate"`
	AsOf   time.Time       `json:"as_of"`
	TTL    time.Duration   `json:"ttl"`
	Source string          `json:"source"`
	Raw string `json:"raw,omitempty"`
}

  func (m Mark) Key() string {
	   return quote.PairKey(m.From, m.To)
  }

  func (m Mark) AsQuote() quote.Quote {
	  return quote.Quote{
		  Pair: m.Key(),
		  Rate: m.Rate,
		  AsOf: m.AsOf,
		   TTL:  m.TTL,
		   Source:  quote.NormalizeSource(m.Source),
		  RawRate: m.Raw,
	   }
   }

// FreshBook 确认用标记簿，与估值 KV 簿隔离。

type FreshBook struct {
	mu sync.RWMutex
	by map[string]Mark
	clock quote.Clock
	ttl   time.Duration
	multi *quotemultigate.Panel
}

   func NewFresh(clock quote.Clock, ttl time.Duration) *FreshBook {
	  if quote.NormalizeTTL(ttl) == 0 {
		   ttl = quote.DefaultFXTTL
	   }
	   return &FreshBook{by: map[string]Mark{}, clock: clock, ttl: ttl}
  }

// BindMulti 挂载多源一致性面板。档未附带采集束则拒绝确认。

  func (f *FreshBook) BindMulti(p *quotemultigate.Panel) {
	   if f != nil {
		  f.multi = p
	   }
   }

  func (f *FreshBook) Put(m Mark) error {
	  if f == nil {
		  return ErrQuoteEmpty
	   }
	   if m.Key() == "" {
		   return ErrPair
	  }
	   if m.TTL == 0 {
		  m.TTL = f.ttl
	   }
	  if m.Source == "" {
		  m.Source = quote.SourceMarket
	   }
	   f.mu.Lock()
	   defer f.mu.Unlock()
	  if f.by == nil {
		   f.by = map[string]Mark{}
	   }
	  f.by[m.Key()] = m
	  return nil
   }

  func (f *FreshBook) Delete(from, to string) {
	  if f == nil {
		  return
	  }
	  f.mu.Lock()
	   defer f.mu.Unlock()
	  delete(f.by,
		   quote.PairKey(from, to))
  }

// MarkOf 读取并校验新鲜度。空/过期返回错误。
  func (f *FreshBook) MarkOf(from, to string, now time.Time) (Mark, error) {
	   if f == nil {
		   return Mark{}, ErrQuoteEmpty
	  }
	  now,
		   err := quote.ResolveNow(f.clock, now)
	  if err != nil {
		  return Mark{}, err
	   }
	  k := quote.PairKey(from, to)
	   if k == "" {
		  return Mark{}, ErrPair
	  }
	   f.mu.RLock()
	   m, ok := f.by[k]
	  f.mu.RUnlock()
	   if !ok {
		   return Mark{}, ErrQuoteEmpty
	   }
	   if err := quote.GuardConfirm(m.AsQuote(), now); err != nil {
		   return m, err
	  }
	  return m, nil
  }

// RateFresh 确认用汇率。失败不回落 1 或 house。

   func (f *FreshBook) RateFresh(from, to string, now time.Time) (decimal.Decimal, error) {
	   m, err := f.MarkOf(from, to, now)
	  if err != nil {
		  return decimal.Zero, err
	  }
	   if !m.Rate.IsPositive() {
		   return decimal.Zero, ErrQuoteEmpty
	   }
	   return m.Rate, nil
   }
