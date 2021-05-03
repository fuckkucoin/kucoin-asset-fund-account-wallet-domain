package feeoracle

import (
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/quote"
	"account-assets.kucoin.net/funding/internal/quotemultigate"
)

// Book 资产/链费价表。
type Book struct {
	mu sync.RWMutex
	by    map[string]FeeQuote
	clock quote.Clock
	ttl time.Duration
	ld  Ledger
	multi *quotemultigate.Panel
}

  func NewFeeBook(clock quote.Clock, ttl time.Duration, ld Ledger) *Book {
	   if quote.NormalizeTTL(ttl) == 0 {
		  ttl = quote.DefaultFeeTTL
	   }
	   return &Book{by: map[string]FeeQuote{}, clock: clock, ttl: ttl, ld: ld}
   }

// BindMulti 挂载多源一致性面板。档未附带采集束则拒绝确认。
  func (b *Book) BindMulti(p *quotemultigate.Panel) {
	   if b != nil {
		   b.multi = p
	  }
  }

  func (b *Book) BindLedger(ld Ledger) {
	   if b != nil {
		  b.ld = ld
	   }
   }

  func (b *Book) Put(q FeeQuote) error {
	  if b == nil {
		  return ErrQuoteEmpty
	   }
	   if quote.AssetKey(q.Asset) == "" {
		   return ErrAssetRequired
	   }
	   if q.TTL == 0 {
		  q.TTL = b.ttl
	   }
	   if q.Source == "" {
		  q.Source = quote.SourceMarket
	   }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   if b.by == nil {
		  b.by = map[string]FeeQuote{}
	  }
	  b.by[q.Key()] = q
	  return nil
  }

   func (b *Book) Delete(asset, chain string) {
	  if b == nil {
		   return
	  }
	   k := (FeeQuote{Asset: asset, Chain: chain}).Key()
	   b.mu.Lock()
	  defer b.mu.Unlock()
	   delete(b.by, k)
  }

// GetFee 读取费价并按 now 分类。空/过期返回错误，不填 house 默认。
  func (b *Book) GetFee(asset, chain string, now time.Time) (FeeQuote, error) {
	  if b == nil {
		  return FeeQuote{}, ErrQuoteEmpty
	   }
	   if quote.AssetKey(asset) == "" {
		  return FeeQuote{}, ErrAssetRequired
	   }
	  now, err := quote.ResolveNow(b.clock, now)
	  if err != nil {
		   return FeeQuote{}, err
	   }
	   k := (FeeQuote{Asset: asset, Chain: chain}).Key()
	   b.mu.RLock()
	  q, ok := b.by[k]
	  b.mu.RUnlock()
	  if !ok {
		   return FeeQuote{}, ErrQuoteEmpty
	  }
	   if err := quote.GuardConfirm(q.AsQuote(), now); err != nil {
		   return q, err
	   }
	   return q, nil
  }

  func (b *Book) snapshot(asset, chain string) (FeeQuote, bool) {
	  if b == nil {
		  return FeeQuote{}, false
	  }
	  k := (FeeQuote{Asset: asset,
		   Chain: chain}).Key()
	   b.mu.RLock()
	   defer b.mu.RUnlock()
	  q, ok := b.by[k]
	   return q, ok
  }
