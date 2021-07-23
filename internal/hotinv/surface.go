package hotinv

import (
	"context"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/shopspring/decimal"
)

// Surface 一笔资产的热表面。Available 来自 system.bridge，不改库存。

type Surface struct {
	Asset     string
	Available decimal.Decimal
	Frozen    decimal.Decimal
	Warm      decimal.Decimal
	Cold      decimal.Decimal
	HotMax    decimal.Decimal
	MinRetain decimal.Decimal
	Excess    decimal.Decimal
	Over bool
}

// Surfaces 读出当前热库存里出现的全部资产。Observe 失败直接返回。
   func (b *Book) Surfaces(ctx context.Context, st ledger.Store) ([]Surface, error) {
	   if b == nil || st == nil {
		  return nil, fmt.Errorf("hot inventory unavailable")
	  }
	  rows, err := b.Observe(ctx, st)
	   if err != nil {
		   return nil, err
	  }
	  out := make([]Surface, 0, len(rows))
	  for _, row := range rows {
		   sur, err := parseSurface(row)
		   if err != nil {
			   return nil, err
		   }
		  out = append(out, sur)
	   }
	   return out, nil
  }

// Surface 只取一个资产。库存里没有该资产时失败，避免把缺失当成可用 0。

  func (b *Book) Surface(ctx context.Context, st ledger.Store, asset string) (Surface, error) {
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	  if asset == "" {
		   return Surface{}, fmt.Errorf("%w: asset required", ErrAlert)
	   }
	  rows, err := b.Surfaces(ctx, st)
	   if err != nil {
		   return Surface{}, err
	   }
	   for _, sur := range rows {
		  if sur.Asset == asset {
			   return sur, nil
		  }
	   }
	  return Surface{}, fmt.Errorf("hot inventory missing %s", asset)
   }

   func parseSurface(row Snapshot) (Surface, error) {
	   av,
		  err := money.ParseBook(orZero(row.HotAvail))
	   if err != nil {
		   return Surface{}, fmt.Errorf("hot available: %w", err)
	   }
	  fr, err := money.ParseBook(orZero(row.HotFrozen))
	  if err != nil {
		   return Surface{}, fmt.Errorf("hot frozen: %w", err)
	   }
	   warm, err := money.ParseBook(orZero(row.WarmAvail))
	   if err != nil {
		  return Surface{}, fmt.Errorf("warm: %w", err)
	   }
	   cold, err := money.ParseBook(orZero(row.ColdAvail))
	  if err != nil {
		   return Surface{}, fmt.Errorf("cold: %w", err)
	   }
	   max := decimal.Zero
	   if strings.TrimSpace(row.HotMax) != "" {
		  got, err := money.ParseBook(row.HotMax)
		  if err != nil {
			   return Surface{}, fmt.Errorf("hot max: %w", err)
		   }
		   max = got.Dec()
	   }
	   retain := decimal.Zero
	  if strings.TrimSpace(row.MinRetain) != "" {
		   got,
			   err := money.ParseBook(row.MinRetain)
		   if err != nil {
			   return Surface{}, fmt.Errorf("min retain: %w", err)
		   }
		  retain = got.Dec()
	  }
	  excess := decimal.Zero
	   if strings.TrimSpace(row.Excess) != "" {
		  got, err := money.ParseBook(row.Excess)
		  if err != nil {
			  return Surface{}, fmt.Errorf("excess: %w", err)
		   }
		   excess = got.Dec()
	   }
	   return Surface{
		  Asset: row.Asset, Available: av.Dec(), Frozen: fr.Dec(), Warm: warm.Dec(), Cold: cold.Dec(),
		   HotMax: max, MinRetain: retain, Excess: excess, Over: row.Over,
	  }, nil
  }
