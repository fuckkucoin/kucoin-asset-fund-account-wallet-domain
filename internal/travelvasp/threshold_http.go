package travelvasp

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/fxmark"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/slo"
	"github.com/gin-gonic/gin"
)

// RegisterThresholds 走廊门槛运营面（列表/写入/牌价），热更新共享 ThresholdBook。
// 与入金 Observe 绑定同一 book 指针；PUT 先写 SQL 再热更新内存。
// 写路径仅 AML/运维角色；损坏门槛不得绕过核验闸。
  func RegisterThresholds(v1 *gin.RouterGroup, rt *app.Runtime) {
	   book := thresholdBookFrom(rt)
	  // Live FX 厂商燃烧严重度默认走 slo.SeverityFor；运营 PUT fx-severity 可覆盖。
	  fxmark.GlobalVendorSLO().BindSeverityTune(func(burn, remaining float64) string {
		   return slo.SeverityFor("AATravelFXVendorBurn", burn, remaining)
	   })
	   if rt != nil && rt.TravelFXRefresh == nil {
		  rt.TravelFXRefresh = func(ctx context.Context) {
			   n,
				   stale := book.RefreshTravelRatesTick(ctx)
			  if n > 0 || stale > 0 {
				   _ = n
				   _ = stale
			  }
		   }
	   }
	  v1.GET("/travel/thresholds", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   if err2 := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err2 != nil {
				  c.JSON(403, gin.H{"error": err.Error()})
				   return
			   }
		   }
		  c.JSON(200, gin.H{
			   "default_threshold": book.DefaultMin(),
			   "corridors":         book.List(),
			  "usd_rates":         book.ListUsdRates(),
			  "host": "kyt.kucoin.net",
		   })
	  })
	  v1.PUT("/travel/thresholds", func(c *gin.Context) {
		   actor := perm.FromRequest(c.Request, "")
		  if err := actor.Check(perm.ActAMLOpen); err != nil {
			  if err2 := actor.Check(perm.ActOpsAdmin); err2 != nil {
				   c.JSON(403, gin.H{"error": err.Error()})
				  return
			   }
		  }
		  var body struct {
			  DefaultThreshold string `json:"default_threshold"`
			  Corridors []CorridorThreshold `json:"corridors"`
			  UsdRates map[string]string `json:"usd_rates"`
			  Replace  bool              `json:"replace"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  if body.Replace || len(body.Corridors) > 0 && strings.TrimSpace(body.DefaultThreshold) != "" {
			  book.ReplaceAll(body.DefaultThreshold, body.Corridors)
		   } else {
			   if strings.TrimSpace(body.DefaultThreshold) != "" {
				   book.SetDefault(body.DefaultThreshold)
			   }
			   for _, row := range body.Corridors {
				   book.Put(row)
			   }
		   }
		   if len(body.UsdRates) > 0 {
			   book.BindUsdRates(body.UsdRates)
			   for asset := range body.UsdRates {
				   book.MarkRateSource(asset, "static")
			  }
		   }
		  // 先落库再回读确认热表；SQL 不可用时仅内存热更新。
		  if err := book.PersistAll(c.Request.Context()); err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, gin.H{
			  "default_threshold": book.DefaultMin(),
			  "corridors": book.List(),
			   "usd_rates": book.ListUsdRates(),
			   "persisted": book.sqlOn(),
		   })
	  })
	  v1.PUT("/travel/thresholds/corridor", func(c *gin.Context) {
		   actor := perm.FromRequest(c.Request, "")
		   if err := actor.Check(perm.ActAMLOpen); err != nil {
			  if err2 := actor.Check(perm.ActOpsAdmin); err2 != nil {
				  c.JSON(403, gin.H{"error": err.Error()})
				   return
			  }
		   }
		   var body CorridorThreshold
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  if strings.TrimSpace(body.Corridor) == "" {
			  c.JSON(400, gin.H{"error": "corridor required"})
			  return
		   }
		  book.Put(body)
		   if err := book.PersistCorridor(c.Request.Context(), body); err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, body)
	   })
	  v1.PUT("/travel/thresholds/usd-rate", func(c *gin.Context) {
		   actor := perm.FromRequest(c.Request, "")
		   if err := actor.Check(perm.ActAMLOpen); err != nil {
			  if err2 := actor.Check(perm.ActOpsAdmin); err2 != nil {
				   c.JSON(403, gin.H{"error": err.Error()})
				  return
			  }
		   }
		   var body struct {
			   Asset string `json:"asset"`
			  USD    string `json:"usd"`
			  Source string `json:"source"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		  if err := book.PutUsdRate(body.Asset, body.USD); err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  src := strings.TrimSpace(body.Source)
		  if src == "" {
			   src = "static"
		   }
		  book.MarkRateSource(body.Asset, src)
		   if err := book.PersistUsdRate(c.Request.Context(), body.Asset, body.USD, src); err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, gin.H{"asset": strings.ToUpper(strings.TrimSpace(body.Asset)), "usd": body.USD, "source": src})
	  })
	  v1.POST("/travel/thresholds/refresh-fx", func(c *gin.Context) {
		   actor := perm.FromRequest(c.Request, "")
		   if err := actor.Check(perm.ActAMLOpen); err != nil {
			   if err2 := actor.Check(perm.ActOpsAdmin); err2 != nil {
				   c.JSON(403, gin.H{"error": err.Error()})
				   return
			   }
		   }
		  var body struct {
			   Assets []string `json:"assets"`
		  }
		  _ = c.ShouldBindJSON(&body)
		   got, err := book.RefreshTravelRates(c.Request.Context(), body.Assets)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   now := time.Now().UTC()
		  for asset := range got {
			   book.TouchRate(asset, now)
		  }
		  c.JSON(200, gin.H{"usd_rates": got, "all": book.ListUsdRates(), "health": book.FXHealth(now)})
	   })
	  // 旅行 FX 健康面（年龄/来源/陈旧）；runbook 一致，无伪造区域 IP。
	  v1.GET("/travel/thresholds/fx-health", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  if err2 := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err2 != nil {
				   if err3 := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err3 != nil {
					  c.JSON(403, gin.H{"error": err.Error()})
					   return
				  }
			   }
		   }
		  now := time.Now().UTC()
		   rates := book.FXHealth(now)
		  stale := book.EmitFXHealthMetrics(now)
		  staleAssets := make([]string, 0)
		  for _, h := range rates {
			   if h.Stale {
				  staleAssets = append(staleAssets, h.Asset)
			   }
		  }
		  multi := fxmark.GlobalVendorSLO().SnapshotMulti()
		  c.JSON(200, gin.H{
			   "host":          "kyt.kucoin.net",
			   "stale_max_sec": int((book.staleMaxOrDefault()).Seconds()),
			   "rates":         rates,
			   "stale_count": stale,
			  "stale_assets":  staleAssets,
			  "vendor_slo":    multi.Primary,
			  "dual_fire":     multi.DualFire,
			   "page_severity": multi.PageSev,
			   "checked_at_ms": now.UnixMilli(),
		   })
	  })
	  // Live FX 厂商 SLO 燃烧预算快照。
	  v1.GET("/travel/thresholds/fx-vendor-slo", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   if err2 := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err2 != nil {
				   c.JSON(403, gin.H{"error": err.Error()})
				  return
			  }
		  }
		   multi := fxmark.GlobalVendorSLO().SnapshotMulti()
		   c.JSON(200, gin.H{
			   "vendor_slo": multi.Primary, "windows": multi.Windows,
			  "dual_fire": multi.DualFire, "page_severity": multi.PageSev,
			   "host": "kyt.kucoin.net", "target": fxmark.VendorSLOTarget,
		  })
	   })
	   // 严重度调谐钩子（ticket/page/none）；覆盖 AATravelFXVendorBurn / AATravelFXStale。
	   v1.PUT("/travel/thresholds/fx-severity", func(c *gin.Context) {
		   actor := perm.FromRequest(c.Request, "")
		   if err := actor.Check(perm.ActOpsAdmin); err != nil {
			   if err2 := actor.Check(perm.ActAMLOpen); err2 != nil {
				  c.JSON(403, gin.H{"error": err.Error()})
				  return
			   }
		   }
		   var body struct {
			   Alert string `json:"alert"`
			  Severity   string  `json:"severity"`
			  TicketBurn float64 `json:"ticket_burn"`
			   PageBurn   float64 `json:"page_burn"`
			   MinSamples int64 `json:"min_samples"`
			  WindowSec int64 `json:"window_sec"`
		  }
		   if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(400, gin.H{"error": err.Error()})
			   return
		  }
		   if body.Alert != "" {
			   slo.SetSeverityOverride(body.Alert, body.Severity)
		   }
		  if body.TicketBurn > 0 || body.PageBurn > 0 || body.MinSamples > 0 {
			   fxmark.GlobalVendorSLO().BindBurnThresholds(body.TicketBurn, body.PageBurn, body.MinSamples)
		   }
		  if body.WindowSec > 0 {
			  fxmark.GlobalVendorSLO().BindWindow(time.Duration(body.WindowSec) * time.Second)
		   }
		   multi := fxmark.GlobalVendorSLO().SnapshotMulti()
		   c.JSON(200, gin.H{
			   "overrides":  slo.SeverityOverrides(),
			  "vendor_slo": multi.Primary,
			  "windows":    multi.Windows,
			   "dual_fire":  multi.DualFire,
		  })
	   })
  }

  func (b *ThresholdBook) staleMaxOrDefault() time.Duration {
	  if b == nil {
		   return DefaultFXStaleMax
	  }
	   b.mu.RLock()
	  d := b.staleMax
	   b.mu.RUnlock()
	   if d <= 0 {
		  return DefaultFXStaleMax
	  }
	  return d
   }

  func thresholdBookFrom(rt *app.Runtime) *ThresholdBook {
	  if rt != nil {
		  if book, ok := rt.TravelThresh.(*ThresholdBook); ok && book != nil {
			   if rt.SQL != nil && !book.sqlOn() {
				   book.BindSQL(rt.SQL)
				  _ = book.Hydrate(context.Background())
			  }
			  bindFXFromRuntime(book, rt)
			  return book
		   }
	  }
	  book := NewThresholdBook("0", nil)
	  if rt != nil {
		  if rt.Cfg != nil {
			   rows := make([]CorridorThreshold, 0, len(rt.Cfg.Travel.Corridors))
			   for _, c := range rt.Cfg.Travel.Corridors {
				   rows = append(rows,
					  CorridorThreshold{Corridor: c.Corridor, Asset: c.Asset, MinAmount: c.MinAmount})
			   }
			   book.ReplaceAll(rt.Cfg.Travel.DefaultThreshold, rows)
			   if len(rt.Cfg.Travel.UsdRates) > 0 {
				   book.BindUsdRates(rt.Cfg.Travel.UsdRates)
				  for asset := range rt.Cfg.Travel.UsdRates {
					   book.MarkRateSource(asset, "static")
				   }
			   }
		  }
		   if rt.SQL != nil {
			   book.BindSQL(rt.SQL)
			   _ = book.Hydrate(context.Background())
		   }
		  bindFXFromRuntime(book, rt)
		   rt.TravelThresh = book
	  }
	  return book
   }

   func bindFXFromRuntime(book *ThresholdBook, rt *app.Runtime) {
	   if book == nil || rt == nil || rt.FXMarks == nil {
		   return
	   }
	   if marks, ok := rt.FXMarks.(*fxmark.Book); ok && marks != nil {
		   book.BindFXMark(marks)
	  }
   }
