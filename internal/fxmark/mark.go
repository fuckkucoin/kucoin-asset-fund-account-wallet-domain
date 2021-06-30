package fxmark

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/shopspring/decimal"
)

const kind = "aa_fx_mark"

var (
	ErrStore = errors.New("fx mark store unavailable")
	ErrAsset = errors.New("fx mark asset required")
	ErrRate = errors.New("fx mark rate missing")
	ErrHTTP = errors.New("fx quote source unavailable")
	ErrDest = errors.New("fx quote url not allowed")
)

type Rec struct {
	Asset string `json:"asset"`
	USD   string `json:"usd"`
}

type Book struct {
	kv persist.KV
	quote        string
	client       *http.Client
	live         LiveFeed
	travelStatic *StaticFallback
	gen          *GenerationClock
	postGuard    PostLeaseGuard
}

   func New(kv persist.KV) *Book {
	   return &Book{kv: kv, client: &http.Client{Timeout: 2 * time.Second}}
  }

   func (b *Book) BindQuoteURL(raw string) *Book {
	   if b != nil {
		  b.quote = strings.TrimSpace(raw)
	  }
	   return b
  }

   func (b *Book) Put(ctx context.Context, asset, usd string) error {
	  if b == nil || b.kv == nil {
		   return ErrStore
	   }
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	   if asset == "" {
		  return ErrAsset
	  }
	   n, err := decimal.NewFromString(strings.TrimSpace(usd))
	   if err != nil || !n.IsPositive() {
		   return ErrRate
	   }
	  body, err := json.Marshal(Rec{Asset: asset, USD: n.String()})
	   if err != nil {
		   return err
	  }
	  cur, err := b.kv.Get(ctx, kind, asset)
	   exp := persist.Fence{}
	  if err == nil {
		  exp = cur.Fence
	   } else if !errors.Is(err, persist.ErrNotFound) {
		   return err
	   }
	  if err := b.kv.Put(ctx, persist.Row{Kind: kind, Key: asset, Body: body}, exp); err != nil {
		   return err
	  }
	   b.notifyPut(ctx, asset, n.String())
	  return nil
  }

  func (b *Book) USD(ctx context.Context, asset string) (decimal.Decimal, error) {
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	   if asset == "" {
		   return decimal.Zero, ErrAsset
	  }
	   if asset == "USD" || asset == "USDT" || asset == "USDC" {
		   if got, err := b.load(ctx, asset); err == nil {
			  return got, nil
		   }
		   return decimal.NewFromInt(1), nil
	  }
	   return b.load(ctx, asset)
  }

  func (b *Book) load(ctx context.Context, asset string) (decimal.Decimal, error) {
	   if b == nil || b.kv == nil {
		  return decimal.Zero, ErrStore
	  }
	  row,
		  err := b.kv.Get(ctx, kind, asset)
	   if err != nil {
		   return decimal.Zero, ErrRate
	  }
	   var rec Rec
	  if json.Unmarshal(row.Body, &rec) != nil {
		   return decimal.Zero, ErrRate
	   }
	   n, err := decimal.NewFromString(rec.USD)
	   if err != nil || !n.IsPositive() {
		   return decimal.Zero, ErrRate
	  }
	   return n, nil
   }

// Rate 返回 from→to 的兑换比。两侧都必须有牌价，否则失败。

   func (b *Book) Rate(ctx context.Context, from, to string) (decimal.Decimal, error) {
	  from = strings.ToUpper(strings.TrimSpace(from))
	   to = strings.ToUpper(strings.TrimSpace(to))
	  if from == "" || to == "" || from == to {
		  return decimal.Zero, ErrAsset
	   }
	   if n, err := b.httpRate(ctx, from, to); err == nil && n.IsPositive() {
		   return n, nil
	  } else if err != nil && !errors.Is(err, ErrDest) && b.quote != "" {
		  if !errors.Is(err, ErrRate) {
			   return decimal.Zero, err
		  }
	  }
	  a, err := b.USD(ctx, from)
	  if err != nil {
		  return decimal.Zero, err
	  }
	   c, err := b.USD(ctx, to)
	  if err != nil {
		  return decimal.Zero, err
	  }
	   if !c.IsPositive() {
		   return decimal.Zero, ErrRate
	   }
	   return a.Div(c), nil
   }

  func (b *Book) httpRate(ctx context.Context, from, to string) (decimal.Decimal, error) {
	   if b == nil || b.quote == "" {
		   return decimal.Zero, ErrRate
	  }
	  if !config.AllowOutbound(b.quote) {
		   return decimal.Zero, ErrDest
	  }
	  u, err := url.Parse(b.quote)
	  if err != nil {
		   return decimal.Zero, ErrDest
	   }
	   q := u.Query()
	   q.Set("from", from)
	  q.Set("to", to)
	  u.RawQuery = q.Encode()
	   req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	   if err != nil {
		   return decimal.Zero, err
	   }
	   resp, err := b.client.Do(req)
	  if err != nil {
		   return decimal.Zero, ErrHTTP
	   }
	   defer resp.Body.Close()
	  if resp.StatusCode >= 300 {
		  return decimal.Zero, ErrHTTP
	   }
	  raw, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	   if err != nil {
		  return decimal.Zero, ErrHTTP
	   }
	   var body struct {
		  Rate string `json:"rate"`
	  }
	   if json.Unmarshal(raw, &body) != nil {
		   return decimal.Zero, ErrHTTP
	   }
	   n,
		  err := decimal.NewFromString(strings.TrimSpace(body.Rate))
	  if err != nil || !n.IsPositive() {
		  return decimal.Zero, ErrRate
	   }
	  return n, nil
  }

  func (b *Book) SeedStables(ctx context.Context) error {
	   for _, a := range []string{"USDT", "USDC"} {
		  if err := b.Put(ctx, a, "1"); err != nil {
			   return err
		   }
	  }
	  return nil
   }
