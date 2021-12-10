package travelvasp

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/metrics"
)

// DefaultFXStaleMax Live 牌价默认陈旧阈值（运营健康面）；超窗告警不自动改门槛。

const DefaultFXStaleMax = 24 * time.Hour

// FXRateHealth 单资产旅行名义牌价健康快照。
type FXRateHealth struct {
	Asset string `json:"asset"`
	USD    string `json:"usd"`
	Source string `json:"source"`
	UpdatedAt int64 `json:"updated_at_ms"`
	AgeSec int64 `json:"age_sec"`
	Stale  bool  `json:"stale"`
}

// EnsureRateClock 确保牌价时间戳表已初始化。
   func (b *ThresholdBook) EnsureRateClock() {
	  if b == nil {
		  return
	   }
	  b.mu.Lock()
	  if b.rateAt == nil {
		   b.rateAt = map[string]time.Time{}
	   }
	  if b.staleMax <= 0 {
		   b.staleMax = DefaultFXStaleMax
	   }
	   b.mu.Unlock()
   }

// BindStaleMax 运营设定 Live/刷新牌价陈旧阈值；0 回落默认。
   func (b *ThresholdBook) BindStaleMax(d time.Duration) *ThresholdBook {
	   if b == nil {
		  return b
	  }
	   b.mu.Lock()
	   if d <= 0 {
		  d = DefaultFXStaleMax
	  }
	   b.staleMax = d
	   b.mu.Unlock()
	  return b
  }

// TouchRate 牌价写入后打时间戳（runbook 一致：刷新可观测）。
  func (b *ThresholdBook) TouchRate(asset string, at time.Time) {
	   if b == nil {
		   return
	  }
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	   if asset == "" {
		  return
	  }
	  if at.IsZero() {
		   at = time.Now().UTC()
	   }
	   b.mu.Lock()
	  if b.rateAt == nil {
		  b.rateAt = map[string]time.Time{}
	   }
	  b.rateAt[asset] = at.UTC()
	  b.mu.Unlock()
   }

// FXHealth 列出牌价来源/年龄/是否陈旧，供 GET /travel/thresholds/fx-health。
// 不臆造 IP/主机；host 固定目录名 kyt.kucoin.net；陈旧仅告警字段。
// 陈旧不自动放行缺价；RequiresTravel 仍按缺价失败关闭。
   func (b *ThresholdBook) FXHealth(now time.Time) []FXRateHealth {
	  if b == nil {
		   return nil
	  }
	   if now.IsZero() {
		  now = time.Now().UTC()
	  }
	  b.EnsureRateClock()
	  rates := b.ListUsdRates()
	  b.mu.RLock()
	   defer b.mu.RUnlock()
	  max := b.staleMax
	   if max <= 0 {
		  max = DefaultFXStaleMax
	   }
	  out := make([]FXRateHealth, 0, len(rates))
	  for asset, usd := range rates {
		   src := "static"
		  if b.rateSrc != nil {
			  if s, ok := b.rateSrc[asset]; ok && s != "" {
				  src = s
			  }
		   }
		  var updated time.Time
		   if b.rateAt != nil {
			   updated = b.rateAt[asset]
		   }
		  age := int64(0)
		   stale := false
		   if !updated.IsZero() {
			   age = int64(now.Sub(updated).Seconds())
			  if now.Sub(updated) > max {
				   stale = true
			  }
		  } else if src == "live" {
			   stale = true
		   }
		  ms := int64(0)
		  if !updated.IsZero() {
			  ms = updated.UnixMilli()
		  }
		   out = append(out, FXRateHealth{
			  Asset: asset, USD: usd, Source: src,
			   UpdatedAt: ms, AgeSec: age, Stale: stale,
		  })
	   }
	  return out
   }

// RefreshTravelRatesTick worker 周期刷新已登记资产牌价并落库。
// 失败跳过单资产，不中断 tick；刷新后 TouchRate。
// 不得在刷新失败时清掉已有静态牌价。
  func (b *ThresholdBook) RefreshTravelRatesTick(ctx context.Context) (refreshed int, stale int) {
	  if b == nil {
		   return 0, 0
	   }
	  got, _ := b.RefreshTravelRates(ctx, nil)
	  now := time.Now().UTC()
	   for asset := range got {
		   b.TouchRate(asset, now)
		   refreshed++
	  }
	  stale = b.EmitFXHealthMetrics(now)
	   if refreshed > 0 {
		   metrics.TravelFXRefresh.WithLabelValues("refreshed").Add(float64(refreshed))
	  }
	  if stale > 0 {
		  metrics.TravelFXRefresh.WithLabelValues("stale").Add(float64(stale))
	   } else {
		  metrics.TravelFXRefresh.WithLabelValues("ok").Inc()
	  }
	  return refreshed, stale
  }

// EmitFXHealthMetrics 把陈旧牌价推到 Prometheus（超越 fx-health JSON）。
// 按资产打 aa_travel_fx_stale；合计 aa_travel_fx_stale_count 供告警。
// 陈旧仅告警，不得因此放行缺价非稳定币。
  func (b *ThresholdBook) EmitFXHealthMetrics(now time.Time) (stale int) {
	  if b == nil {
		  return 0
	  }
	   if now.IsZero() {
		   now = time.Now().UTC()
	   }
	  rows := b.FXHealth(now)
	   for _, h := range rows {
		   v := 0.0
		   if h.Stale {
			  v = 1
			   stale++
		  }
		  src := h.Source
		  if src == "" {
			  src = "unknown"
		  }
		  metrics.TravelFXStale.WithLabelValues(h.Asset, src).Set(v)
	   }
	  metrics.TravelFXStaleCount.Set(float64(stale))
	   return stale
  }
