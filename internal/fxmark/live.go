package fxmark

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/metrics"
	"github.com/shopspring/decimal"
)

// LiveFeed 实时 FX 牌价拉取面；失败须回落静态表，不得臆造。
type LiveFeed interface {
	// QuoteUSD 返回 asset→USD；不可用返回错误。
	QuoteUSD(ctx context.Context, asset string) (decimal.Decimal, error)
}

// HTTPFeed 按配置询价地址拉实时牌价。
type HTTPFeed struct {
	url    string
	client *http.Client
}

   func NewHTTPFeed(raw string) *HTTPFeed {
	  return &HTTPFeed{
		  url: strings.TrimSpace(raw),
		   client: &http.Client{Timeout: 2 * time.Second},
	   }
   }

   func (f *HTTPFeed) QuoteUSD(ctx context.Context, asset string) (decimal.Decimal, error) {
	   start := time.Now()
	  fail := func(err error) (decimal.Decimal, error) {
		  metrics.TravelFXVendor.WithLabelValues("fail").Inc()
		   metrics.TravelFXVendorLatency.Observe(time.Since(start).Seconds())
		   GlobalVendorSLO().Observe(true)
		   return decimal.Zero, err
	  }
	   if f == nil || f.url == "" {
		  return fail(ErrRate)
	   }
	   if !config.AllowOutbound(f.url) {
		  return fail(ErrDest)
	   }
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	  if asset == "" {
		   return fail(ErrAsset)
	  }
	  u, err := url.Parse(f.url)
	   if err != nil {
		   return fail(ErrDest)
	   }
	   q := u.Query()
	  q.Set("from", asset)
	   q.Set("to", "USD")
	   u.RawQuery = q.Encode()
	  req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	   if err != nil {
		   metrics.TravelFXVendor.WithLabelValues("fail").Inc()
		   metrics.TravelFXVendorLatency.Observe(time.Since(start).Seconds())
		   GlobalVendorSLO().Observe(true)
		   return errDecimal(err)
	  }
	  resp, err := f.client.Do(req)
	  if err != nil {
		  return fail(ErrHTTP)
	  }
	  defer resp.Body.Close()
	   if resp.StatusCode >= 300 {
		   return fail(ErrHTTP)
	   }
	   raw, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	   if err != nil {
		   return fail(ErrHTTP)
	  }
	  var body struct {
		   Rate string `json:"rate"`
		   USD string `json:"usd"`
	   }
	  if json.Unmarshal(raw, &body) != nil {
		   return fail(ErrHTTP)
	  }
	  s := strings.TrimSpace(body.Rate)
	  if s == "" {
		  s = strings.TrimSpace(body.USD)
	  }
	  n, err := decimal.NewFromString(s)
	   if err != nil || !n.IsPositive() {
		   return fail(ErrRate)
	  }
	  metrics.TravelFXVendor.WithLabelValues("ok").Inc()
	  metrics.TravelFXVendorLatency.Observe(time.Since(start).Seconds())
	  GlobalVendorSLO().Observe(false)
	  return n, nil
   }

  func errDecimal(err error) (decimal.Decimal, error) {
	  return decimal.Zero, err
   }

// StaticFallback Live 失败时回落静态 map（配置/运营写入）。
type StaticFallback struct {
	mu sync.RWMutex
	rates map[string]string
}

   func NewStaticFallback(rates map[string]string) *StaticFallback {
	   s := &StaticFallback{rates: map[string]string{}}
	  for k, v := range rates {
		   asset := strings.ToUpper(strings.TrimSpace(k))
		  rate := strings.TrimSpace(v)
		   if asset != "" && rate != "" {
			   s.rates[asset] = rate
		  }
	   }
	  return s
   }

  func (s *StaticFallback) Put(asset, usd string) {
	   if s == nil {
		   return
	  }
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	   usd = strings.TrimSpace(usd)
	   if asset == "" || usd == "" {
		   return
	   }
	   s.mu.Lock()
	   if s.rates == nil {
		  s.rates = map[string]string{}
	   }
	  s.rates[asset] = usd
	  s.mu.Unlock()
   }

   func (s *StaticFallback) QuoteUSD(_ context.Context, asset string) (decimal.Decimal, error) {
	  if s == nil {
		  return decimal.Zero, ErrRate
	   }
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	   s.mu.RLock()
	  raw, ok := s.rates[asset]
	  s.mu.RUnlock()
	  if !ok || strings.TrimSpace(raw) == "" {
		  return decimal.Zero, ErrRate
	   }
	   n, err := decimal.NewFromString(raw)
	  if err != nil || !n.IsPositive() {
		  return decimal.Zero, ErrRate
	  }
	  return n, nil
  }

  func (s *StaticFallback) Snapshot() map[string]string {
	  if s == nil {
		  return nil
	   }
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	   out := make(map[string]string, len(s.rates))
	   for k, v := range s.rates {
		  out[k] = v
	  }
	   return out
  }

// Composite 先 Live 后 Static；供旅行规则门槛名义与估值共用。

type Composite struct {
	Live LiveFeed
	Static *StaticFallback
}

   func (c *Composite) QuoteUSD(ctx context.Context, asset string) (decimal.Decimal, string, error) {
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	  if asset == "USD" || asset == "USDT" || asset == "USDC" || asset == "DAI" {
		  return decimal.NewFromInt(1), "stable", nil
	  }
	  if c != nil && c.Live != nil {
		  if n, err := c.Live.QuoteUSD(ctx, asset); err == nil && n.IsPositive() {
			  return n, "live", nil
		   }
		   // Live 已在 HTTPFeed 记 fail；此处记 fallback 供燃烧面区分。
		  metrics.TravelFXVendor.WithLabelValues("fallback").Inc()
	   }
	   if c != nil && c.Static != nil {
		   if n, err := c.Static.QuoteUSD(ctx, asset); err == nil && n.IsPositive() {
			   return n, "static", nil
		   }
	  }
	  return decimal.Zero, "", ErrRate
  }

// BindLive 给牌价簿挂实时源（估值 Rate 先走 HTTP）。

   func (b *Book) BindLive(feed LiveFeed) *Book {
	  if b != nil {
		  b.live = feed
	  }
	  return b
  }

// BindStaticTravel 挂旅行规则静态回落表。

  func (b *Book) BindStaticTravel(fb *StaticFallback) *Book {
	   if b != nil {
		  b.travelStatic = fb
	  }
	   return b
   }

// TravelUSD 旅行规则名义用牌价；Live→Static→KV；失败返回错误供失败关闭。

  func (b *Book) TravelUSD(ctx context.Context, asset string) (decimal.Decimal, string, error) {
	   comp := &Composite{}
	  if b != nil {
		   comp.Live = b.live
		  comp.Static = b.travelStatic
	   }
	   if n, src, err := comp.QuoteUSD(ctx, asset); err == nil {
		   return n, src, nil
	   }
	   if b != nil {
		  if n, err := b.USD(ctx, asset); err == nil {
			   return n, "kv", nil
		  }
	   }
	   return decimal.Zero, "", ErrRate
  }
