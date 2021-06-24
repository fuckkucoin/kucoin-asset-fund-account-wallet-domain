package fxmark

import (
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/metrics"
)

// VendorSLOTarget Live FX 厂商可用性目标（错误预算 0.5%）。
const VendorSLOTarget = 0.995

// VendorBurnHook 燃烧告警回调（severity 由调谐钩子决定）。
type VendorBurnHook func(alert VendorBurnAlert)

// VendorBurnAlert Live FX 厂商错误预算燃烧快照（单窗）。
type VendorBurnAlert struct {
	Name string `json:"name"`
	Target float64 `json:"target"`
	Requests int64 `json:"requests"`
	Errors int64 `json:"errors"`
	Budget    float64 `json:"budget"`
	Remaining float64 `json:"remaining"`
	BurnRate float64 `json:"burn_rate"`
	Alert    bool    `json:"alert"`
	Severity string  `json:"severity"`
	Window string `json:"window"`
}

// VendorMultiBurn 多窗燃烧面（5m + 1h）。
type VendorMultiBurn struct {
	Target float64 `json:"target"`
	Windows map[string]VendorBurnAlert `json:"windows"` // "5m" / "1h"
	Primary VendorBurnAlert            `json:"primary"` // 1h 主窗（兼容旧面）
	DualFire bool `json:"dual_fire"` // 5m∧1h 同时告警
	PageSev string `json:"page_severity,omitempty"` // dual-fire 时 page
}

type vendorSample struct {
	at time.Time
	failed bool
}

// VendorSLO Live FX 厂商 SLO 燃烧预算与严重度调谐（多滚动时间窗）。
// 默认同时计算 5m 与 1h burn rate；样本保留最长窗。
// 短窗放大瞬时错误率；调窗不得放行缺价。

type VendorSLO struct {
	mu      sync.Mutex
	target  float64
	windows []time.Duration // 多窗；末窗为最长保留
	samples []vendorSample
	pageBurn float64
	ticketBurn   float64
	minSamples   int64
	severityHook func(burn float64, remaining float64) string
	onAlert VendorBurnHook
	clock   func() time.Time
}

var (
	defaultVendorWindows = []time.Duration{5 * time.Minute, time.Hour}
)

// DefaultVendorSLO 默认 Live FX 厂商燃烧面（5m+1h 多窗）。

  func DefaultVendorSLO() *VendorSLO {
	  return &VendorSLO{
		   target: VendorSLOTarget,
		  windows: append([]time.Duration(nil),
			   defaultVendorWindows...),
		   pageBurn:   2.0,
		   ticketBurn: 1.0,
		   minSamples: 20,
		  clock:      func() time.Time { return time.Now().UTC() },
	  }
  }

var globalVendorSLO = DefaultVendorSLO()

// GlobalVendorSLO 进程内 Live FX 厂商 SLO 单例。
  func GlobalVendorSLO() *VendorSLO { return globalVendorSLO }

// BindWindows 调谐多滚动时间窗；空忽略；自动按升序保留最长样本。
  func (s *VendorSLO) BindWindows(ds ...time.Duration) *VendorSLO {
	  if s == nil || len(ds) == 0 {
		  return s
	   }
	  clean := make([]time.Duration, 0, len(ds))
	  for _, d := range ds {
		   if d > 0 {
			   clean = append(clean, d)
		   }
	  }
	   if len(clean) == 0 {
		  return s
	  }
	   // 升序
	  for i := 0; i < len(clean); i++ {
		  for j := i + 1; j < len(clean); j++ {
			   if clean[j] < clean[i] {
				  clean[i], clean[j] = clean[j], clean[i]
			  }
		  }
	  }
	  s.mu.Lock()
	   s.windows = clean
	  s.pruneLocked(s.nowLocked())
	   s.mu.Unlock()
	   return s
  }

// BindWindow 兼容旧单窗 API；设为 [d] 或保留 5m+d。

  func (s *VendorSLO) BindWindow(d time.Duration) *VendorSLO {
	   if s == nil || d <= 0 {
		  return s
	  }
	   if d >= time.Hour {
		   return s.BindWindows(5*time.Minute, d)
	  }
	   return s.BindWindows(d, time.Hour)
   }

// BindSeverityTune 运营调谐严重度钩子；返回 ticket/page/none。
// 未绑时用 pageBurn/ticketBurn 阈值；钩子不得放行缺价。

   func (s *VendorSLO) BindSeverityTune(fn func(burn, remaining float64) string) *VendorSLO {
	   if s != nil {
		   s.mu.Lock()
		  s.severityHook = fn
		   s.mu.Unlock()
	  }
	  return s
   }

   // BindBurnThresholds 调谐 ticket/page 燃烧阈值与最小样本。

  func (s *VendorSLO) BindBurnThresholds(ticket, page float64, minSamples int64) *VendorSLO {
	  if s == nil {
		   return s
	   }
	   s.mu.Lock()
	  if ticket > 0 {
		  s.ticketBurn = ticket
	  }
	  if page > 0 {
		   s.pageBurn = page
	   }
	  if minSamples > 0 {
		   s.minSamples = minSamples
	  }
	   s.mu.Unlock()
	   return s
  }

// OnAlert 燃烧告警钩子（推 slo / ops）。
   func (s *VendorSLO) OnAlert(fn VendorBurnHook) *VendorSLO {
	   if s != nil {
		   s.mu.Lock()
		  s.onAlert = fn
		  s.mu.Unlock()
	  }
	  return s
  }

   func (s *VendorSLO) nowLocked() time.Time {
	  if s.clock != nil {
		   return s.clock()
	  }
	   return time.Now().UTC()
  }

   func (s *VendorSLO) maxWindowLocked() time.Duration {
	   if len(s.windows) == 0 {
		   return time.Hour
	   }
	  return s.windows[len(s.windows)-1]
  }

// pruneLocked 剔除最长窗外样本；须持锁。

  func (s *VendorSLO) pruneLocked(now time.Time) {
	  maxW := s.maxWindowLocked()
	  if maxW <= 0 || len(s.samples) == 0 {
		   return
	  }
	  cut := now.Add(-maxW)
	  i := 0
	   for i < len(s.samples) && s.samples[i].at.Before(cut) {
		  i++
	  }
	  if i > 0 {
		   s.samples = append([]vendorSample(nil), s.samples[i:]...)
	   }
  }

   func (s *VendorSLO) countsInLocked(now time.Time, window time.Duration) (reqs, errs int64) {
	   cut := now.Add(-window)
	   for _, sm := range s.samples {
		  if sm.at.Before(cut) {
			   continue
		   }
		   reqs++
		  if sm.failed {
			   errs++
		  }
	   }
	  return reqs, errs
   }

   func windowLabel(d time.Duration) string {
	  if d == 5*time.Minute {
		  return "5m"
	   }
	  if d == time.Hour {
		  return "1h"
	   }
	  if d%time.Hour == 0 {
		   return strings.TrimSuffix((d).String(), "0s") // fallback
	  }
	   return d.String()
  }

   func (s *VendorSLO) alertForLocked(now time.Time, window time.Duration) VendorBurnAlert {
	   budget := 1 - s.target
	  reqs, errs := s.countsInLocked(now, window)
	  label := windowLabel(window)
	  out := VendorBurnAlert{
		   Name: "live_fx_vendor", Target: s.target, Requests: reqs, Errors: errs,
		   Budget: budget, Window: label, Severity: "none",
	   }
	  if reqs == 0 {
		   out.Remaining = budget
		   return out
	   }
	  errRate := float64(errs) / float64(reqs)
	  out.Remaining = budget - errRate
	  if budget > 0 {
		   out.BurnRate = errRate / budget
	  }
	   minN := s.minSamples
	   // 5m 窗样本阈值按比例收紧，避免短窗永远不告警
	   if window < time.Hour && minN > 5 {
		  minN = minN / 4
		   if minN < 5 {
			  minN = 5
		  }
	   }
	  if reqs < minN {
		  return out
	   }
	   sev := ""
	  if s.severityHook != nil {
		   sev = strings.TrimSpace(s.severityHook(out.BurnRate, out.Remaining))
	   }
	   if sev == "" {
		   switch {
		   case out.Remaining <= 0 || out.BurnRate >= s.pageBurn:
			  sev = "page"
		  case out.BurnRate >= s.ticketBurn:
			   sev = "ticket"
		   default:
			   sev = "none"
		  }
	  }
	  out.Severity = sev
	   out.Alert = sev == "ticket" || sev == "page"
	   return out
   }

// Observe 记录一次 Live 询价成败并按多窗刷新燃烧指标。
// 同时推 5m/1h burn_rate；窗外自动剔除。
// 燃烧告警不放行缺价非稳定币。

   func (s *VendorSLO) Observe(failed bool) VendorBurnAlert {
	  if s == nil {
		  s = GlobalVendorSLO()
	  }
	   s.mu.Lock()
	  now := s.nowLocked()
	   s.samples = append(s.samples, vendorSample{at: now, failed: failed})
	  s.pruneLocked(now)
	   multi := s.multiLocked(now)
	   hook := s.onAlert
	  s.mu.Unlock()
	  s.emitMetrics(multi)
	   primary := multi.Primary
	  if multi.DualFire {
		  primary.Severity = "page"
		   primary.Alert = true
	   }
	   if primary.Alert && hook != nil {
		   hook(primary)
	   }
	  return primary
   }

  func (s *VendorSLO) emitMetrics(multi VendorMultiBurn) {
	   for label, alert := range multi.Windows {
		  metrics.TravelFXBurnRateWindow.WithLabelValues(label).Set(alert.BurnRate)
		   metrics.TravelFXBurnRemainingWindow.WithLabelValues(label).Set(alert.Remaining)
	   }
	  // 兼容旧无 label 序列：主窗 1h
	   metrics.TravelFXBurnRemaining.Set(multi.Primary.Remaining)
	   metrics.TravelFXBurnRate.Set(multi.Primary.BurnRate)
   }

   func (s *VendorSLO) multiLocked(now time.Time) VendorMultiBurn {
	  wins := s.windows
	  if len(wins) == 0 {
		  wins = defaultVendorWindows
	   }
	   out := VendorMultiBurn{
		   Target:  s.target,
		   Windows: make(map[string]VendorBurnAlert, len(wins)),
	   }
	   var primary VendorBurnAlert
	   for _, w := range wins {
		  a := s.alertForLocked(now, w)
		   out.Windows[a.Window] = a
		  primary = a // 末窗（最长）作主窗
	  }
	   out.Primary = primary
	  // 5m∧1h 双窗同时 ticket+ → page 严重度（双火升级）。
	   w5, ok5 := out.Windows["5m"]
	  w1, ok1 := out.Windows["1h"]
	   if !ok1 {
		  w1 = primary
		   ok1 = primary.Window == "1h" || primary.Window != ""
	  }
	   if ok5 && ok1 && w5.Alert && w1.Alert {
		   out.DualFire = true
		   out.PageSev = "page"
		  // 主窗升 page；短窗保持原 severity 供面观察
		   if primary.Severity != "page" {
			  primary.Severity = "page"
			   primary.Alert = true
			   out.Primary = primary
			   out.Windows[primary.Window] = primary
		   }
		  if a, ok := out.Windows["1h"]; ok && a.Severity != "page" {
			   a.Severity = "page"
			   a.Alert = true
			  out.Windows["1h"] = a
		  }
	  }
	  return out
   }

// Snapshot 只读主窗（1h）燃烧快照（兼容旧面）。
   func (s *VendorSLO) Snapshot() VendorBurnAlert {
	  return s.SnapshotMulti().Primary
  }

// SnapshotMulti 只读多窗燃烧快照。

   func (s *VendorSLO) SnapshotMulti() VendorMultiBurn {
	   if s == nil {
		   s = GlobalVendorSLO()
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  now := s.nowLocked()
	   s.pruneLocked(now)
	  return s.multiLocked(now)
  }
