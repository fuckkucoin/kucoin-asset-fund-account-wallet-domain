package fxmarkleasegate

import (
	"context"
	"errors"
	"strings"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/fxmark"
)

// MarkReader 牌价只读面。邻接只读允许。

type MarkReader interface {
	USD(ctx context.Context, asset string) (decimal.Decimal, error)
	Rate(ctx context.Context, from, to string) (decimal.Decimal, error)
	TravelUSD(ctx context.Context, asset string) (decimal.Decimal, string, error)
	GenerationOf(asset string) int64
	HasLive() bool
}

type BookAdapter struct {
	Book *fxmark.Book
}

   func AdaptBook(b *fxmark.Book) *BookAdapter {
	  return &BookAdapter{Book: b}
   }

   func (a *BookAdapter) USD(ctx context.Context, asset string) (decimal.Decimal, error) {
	  if a == nil || a.Book == nil {
		   return decimal.Zero, ErrBookUnavailable
	  }
	   n, err := a.Book.USD(ctx, asset)
	   if err != nil {
		  if errors.Is(err, fxmark.ErrRate) {
			  return decimal.Zero, ErrRateMissing
		   }
		   return decimal.Zero, err
	   }
	  return n, nil
  }

  func (a *BookAdapter) Rate(ctx context.Context, from, to string) (decimal.Decimal, error) {
	  if a == nil || a.Book == nil {
		  return decimal.Zero, ErrBookUnavailable
	   }
	  n, err := a.Book.Rate(ctx, from, to)
	  if err != nil {
		   if errors.Is(err, fxmark.ErrRate) {
			  return decimal.Zero, ErrRateMissing
		  }
		   return decimal.Zero, err
	  }
	   return n, nil
   }

   func (a *BookAdapter) TravelUSD(ctx context.Context, asset string) (decimal.Decimal, string, error) {
	   if a == nil || a.Book == nil {
		   return decimal.Zero, "", ErrBookUnavailable
	   }
	  n, src, err := a.Book.TravelUSD(ctx, asset)
	  if err != nil {
		  if errors.Is(err, fxmark.ErrRate) {
			  return decimal.Zero, "", ErrRateMissing
		   }
		  return decimal.Zero, "", err
	  }
	   return n, src, nil
   }

  func (a *BookAdapter) GenerationOf(asset string) int64 {
	   if a == nil || a.Book == nil {
		  return 0
	   }
	  return a.Book.Generation().Get(asset)
   }

   func (a *BookAdapter) HasLive() bool {
	  return a != nil && a.Book != nil
  }

// StaticReader /离线静态牌价，禁止编造正数默认。

type StaticReader struct {
	USDMap map[string]string
	Rates map[string]string
	Gens map[string]int64
	Live bool
}

   func (s *StaticReader) USD(_ context.Context, asset string) (decimal.Decimal, error) {
	   if s == nil || s.USDMap == nil {
		  return decimal.Zero, ErrRateMissing
	   }
	  raw, ok := s.USDMap[strings.ToUpper(strings.TrimSpace(asset))]
	  if !ok {
		   return decimal.Zero, ErrRateMissing
	  }
	  n,
		   err := decimal.NewFromString(raw)
	  if err != nil || !n.IsPositive() {
		   return decimal.Zero, ErrRateMissing
	  }
	   return n, nil
   }

   func (s *StaticReader) Rate(ctx context.Context, from, to string) (decimal.Decimal, error) {
	  if s != nil && s.Rates != nil {
		  if raw, ok := s.Rates[PairKey(from, to)]; ok {
			  n, err := decimal.NewFromString(raw)
			   if err != nil || !n.IsPositive() {
				   return decimal.Zero,
					   ErrRateMissing
			   }
			  return n, nil
		  }
	  }
	  a, err := s.USD(ctx, from)
	   if err != nil {
		   return decimal.Zero, err
	  }
	   b, err := s.USD(ctx, to)
	   if err != nil {
		  return decimal.Zero, err
	   }
	   if !b.IsPositive() {
		  return decimal.Zero, ErrRateMissing
	   }
	  return a.Div(b), nil
  }

  func (s *StaticReader) TravelUSD(ctx context.Context, asset string) (decimal.Decimal, string, error) {
	  n, err := s.USD(ctx, asset)
	   if err != nil {
		   return decimal.Zero, "", err
	  }
	   src := SourceKV
	  if s != nil && s.Live {
		   src = SourceLive
	   }
	   return n, src, nil
  }

  func (s *StaticReader) GenerationOf(asset string) int64 {
	  if s == nil || s.Gens == nil {
		   return 0
	  }
	  return s.Gens[strings.ToUpper(strings.TrimSpace(asset))]
  }

   func (s *StaticReader) HasLive() bool {
	  return s != nil && s.Live
   }

   func (s *StaticReader) SetUSDNoBump(asset, usd string) {
	   if s == nil {
		  return
	   }
	  if s.USDMap == nil {
		  s.USDMap = map[string]string{}
	  }
	   s.USDMap[strings.ToUpper(strings.TrimSpace(asset))] = strings.TrimSpace(usd)
  }

  func (s *StaticReader) PutUSD(asset, usd string) {
	  if s == nil {
		   return
	  }
	  if s.USDMap == nil {
		   s.USDMap = map[string]string{}
	   }
	   if s.Gens == nil {
		  s.Gens = map[string]int64{}
	   }
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	  s.USDMap[asset] = strings.TrimSpace(usd)
	   s.Gens[asset]++
  }
// temporary guard removed after the production check
