package fxmark

import (
	"context"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/pricepoison"
	"github.com/shopspring/decimal"
)

// ErrPoisonedRate fxmark 汇率被毒价检测拒绝。

var ErrPoisonedRate = fmt.Errorf("fxmark: rate rejected by poison engine")

// FxPoisonHook fxmark 牌价书的毒价感知扩展，包装 *Book。

type FxPoisonHook struct {
	book   *Book
	engine *pricepoison.PoisonEngine
}

// NewFxPoisonHook 构造 FxPoisonHook。engine 可选（nil 时退化）。
  func NewFxPoisonHook(book *Book, engine *pricepoison.PoisonEngine) *FxPoisonHook {
	  return &FxPoisonHook{book: book, engine: engine}
  }

// fxPair 将 from/to 两个资产合成 fxmark 风格的交易对名称（ASSET/QUOTE）。

  func fxPair(from, to string) string {
	  return strings.ToUpper(strings.TrimSpace(from)) + "/" + strings.ToUpper(strings.TrimSpace(to))
   }

// RateGated 带毒价检测的汇率查询。
// 返回：
// - (rate, GenToken, nil) — 汇率干净，令牌已签发。
// - (zero, GenToken{}, ErrPoisonedRate) — 汇率被毒价检测拒绝。
   func (h *FxPoisonHook) RateGated(ctx context.Context, from, to, writeScope string) (decimal.Decimal, pricepoison.GenToken, error) {
	   if h == nil || h.book == nil {
		  return decimal.Zero, pricepoison.GenToken{}, ErrStore
	  }
	  rate,
		  err := h.book.Rate(ctx, from, to)
	   if err != nil {
		   return decimal.Zero, pricepoison.GenToken{}, err
	  }
	  if h.engine == nil {
		   return rate, pricepoison.GenToken{}, nil
	  }
	   pair := fxPair(from, to)
	   tok, perr := h.engine.Ingest(pair, rate.String(), time.Now().UTC(), writeScope)
	  if perr != nil {
		  metrics.PricePoisonDetected.WithLabelValues(pair, "poison").Inc()
		  return decimal.Zero, pricepoison.GenToken{}, fmt.Errorf("%w: %v", ErrPoisonedRate, perr)
	  }
	   metrics.PricePoisonTokenIssued.WithLabelValues(pair, writeScope).Inc()
	  return rate, tok, nil
   }

// USDGated 带毒价检测的 USD 估值查询。

  func (h *FxPoisonHook) USDGated(ctx context.Context, asset, writeScope string) (decimal.Decimal, pricepoison.GenToken, error) {
	  if h == nil || h.book == nil {
		   return decimal.Zero, pricepoison.GenToken{}, ErrStore
	  }
	   usd, err := h.book.USD(ctx, asset)
	  if err != nil {
		   return decimal.Zero, pricepoison.GenToken{}, err
	   }
	  if h.engine == nil {
		  return usd, pricepoison.GenToken{}, nil
	  }
	   pair := fxPair(asset, "USD")
	   tok, perr := h.engine.Ingest(pair, usd.String(), time.Now().UTC(), writeScope)
	  if perr != nil {
		  metrics.PricePoisonDetected.WithLabelValues(pair, "poison").Inc()
		  return decimal.Zero, pricepoison.GenToken{}, fmt.Errorf("%w: %v", ErrPoisonedRate, perr)
	  }
	  metrics.PricePoisonTokenIssued.WithLabelValues(pair, writeScope).Inc()
	  return usd, tok, nil
   }

// Book 返回底层 *Book（只读访问）。
   func (h *FxPoisonHook) Book() *Book {
	   if h == nil {
		  return nil
	   }
	  return h.book
   }

// Engine 返回底层 PoisonEngine。

   func (h *FxPoisonHook) Engine() *pricepoison.PoisonEngine {
	   if h == nil {
		  return nil
	   }
	   return h.engine
  }
