package travelvasp

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

// BindSQL 挂 SQL 作为门槛与牌价权威源。
// Open 后须 Hydrate；OpenMem 不绑。
// 未绑定时 PUT 仅热更新内存，重启丢失。
   func (b *ThresholdBook) BindSQL(db *persist.SQLDB) *ThresholdBook {
	   if b != nil {
		   b.sql = db
	   }
	  return b
   }

  func (b *ThresholdBook) sqlOn() bool {
	   return b != nil && b.sql != nil && b.sql.DB != nil
   }

// Hydrate 启动时装回 SQL 门槛与牌价到内存表。
// 覆盖配置默认；无行时保留现有内存/配置。
// Hydrate 失败不得静默空表放行大额。
   func (b *ThresholdBook) Hydrate(ctx context.Context) error {
	  if !b.sqlOn() {
		  return nil
	  }
	  if ctx == nil {
		   ctx = context.Background()
	  }
	  store := sqlrepo.NewTravelThresh(b.sql)
	  def, err := store.GetMeta(ctx, "default_threshold")
	  if err != nil && err != sqlrepo.ErrNotFound {
		  return err
	  }
	   rows, err := store.ListThresholds(ctx)
	  if err != nil {
		   return err
	   }
	  rates,
		   err := store.ListUSDRates(ctx)
	  if err != nil {
		   return err
	  }
	  corridors := make([]CorridorThreshold, 0, len(rows))
	   for _, r := range rows {
		  corridors = append(corridors, CorridorThreshold{Corridor: r.Corridor, Asset: r.Asset, MinAmount: r.MinAmount})
	   }
	  if strings.TrimSpace(def) != "" || len(corridors) > 0 {
		  if strings.TrimSpace(def) == "" {
			  def = b.DefaultMin()
		   }
		  b.ReplaceAll(def, corridors)
	   }
	  if len(rates) > 0 {
		  m := make(map[string]string, len(rates))
		   for _, r := range rates {
			   m[r.Asset] = r.USD
		  }
		   b.BindUsdRates(m)
	  }
	  return nil
   }

// PersistAll 把当前内存门槛与牌价写回 SQL。
  func (b *ThresholdBook) PersistAll(ctx context.Context) error {
	  if !b.sqlOn() {
		   return nil
	   }
	   if ctx == nil {
		  ctx = context.Background()
	   }
	  store := sqlrepo.NewTravelThresh(b.sql)
	   if err := store.PutMeta(ctx, "default_threshold", b.DefaultMin()); err != nil {
		  return err
	   }
	   list := b.List()
	   rows := make([]sqlrepo.TravelThreshold, 0, len(list))
	   now := time.Now().UTC()
	   for _, r := range list {
		  if strings.EqualFold(r.Corridor, "DEFAULT") && strings.TrimSpace(r.Asset) == "" {
			  continue
		   }
		   rows = append(rows,
			  sqlrepo.TravelThreshold{Corridor: r.Corridor, Asset: r.Asset, MinAmount: r.MinAmount, UpdatedAt: now})
	  }
	  if err := store.ReplaceThresholds(ctx, rows); err != nil {
		  return err
	  }
	  rates := b.ListUsdRates()
	   usdRows := make([]sqlrepo.TravelUSDRate,
		   0, len(rates))
	   for asset, usd := range rates {
		  src := "static"
		  if b.rateSourceOf(asset) == "live" {
			  src = "live"
		   }
		   usdRows = append(usdRows, sqlrepo.TravelUSDRate{Asset: asset, USD: usd, Source: src, UpdatedAt: now})
	  }
	  return store.ReplaceUSDRates(ctx, usdRows)
  }

// PersistCorridor 单走廊写入 SQL。

  func (b *ThresholdBook) PersistCorridor(ctx context.Context, row CorridorThreshold) error {
	   if !b.sqlOn() {
		  return nil
	   }
	  return sqlrepo.NewTravelThresh(b.sql).PutThreshold(ctx, sqlrepo.TravelThreshold{
		   Corridor: row.Corridor, Asset: row.Asset, MinAmount: row.MinAmount, UpdatedAt: time.Now().UTC(),
	   })
   }

// PersistUsdRate 单资产牌价写入 SQL。

  func (b *ThresholdBook) PersistUsdRate(ctx context.Context, asset, usd, source string) error {
	   if !b.sqlOn() {
		   return nil
	   }
	   if strings.TrimSpace(source) == "" {
		   source = "static"
	  }
	  return sqlrepo.NewTravelThresh(b.sql).PutUSDRate(ctx, sqlrepo.TravelUSDRate{
		  Asset: asset, USD: usd, Source: source, UpdatedAt: time.Now().UTC(),
	  })
   }

  func (b *ThresholdBook) rateSourceOf(asset string) string {
	   if b == nil {
		  return "static"
	  }
	   b.mu.RLock()
	   defer b.mu.RUnlock()
	  if b.rateSrc == nil {
		   return "static"
	   }
	   if s, ok := b.rateSrc[strings.ToUpper(strings.TrimSpace(asset))]; ok {
		   return s
	  }
	   return "static"
  }

// MarkRateSource 标记牌价来源（static|live）。
  func (b *ThresholdBook) MarkRateSource(asset, source string) {
	  if b == nil {
		  return
	  }
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	  source = strings.TrimSpace(source)
	   if asset == "" {
		  return
	   }
	   b.mu.Lock()
	   if b.rateSrc == nil {
		  b.rateSrc = map[string]string{}
	  }
	   b.rateSrc[asset] = source
	  if b.rateAt == nil {
		  b.rateAt = map[string]time.Time{}
	  }
	  b.rateAt[asset] = time.Now().UTC()
	   b.mu.Unlock()
  }
