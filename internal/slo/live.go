// 运行中的 SLO 快照，供告警读取。
// 作者: Likeo
// 日期: 2021.11.11
package slo

import (
	"strconv"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// ProcessView 进程内就绪、节点和行情半边。
type ProcessView struct {
	ReadyOK     bool
	NodesDown   int
	MarketLagMs int64
	MarketStaleRate float64
	MarketApplied      uint64
	MarketStaleDropped uint64
}

// Gathered Prometheus 半边。可注入，HTTP 走 GatherPrometheus。

type Gathered struct {
	SubmitOK   float64
	SubmitFail float64
	ReconcileMismatchMax float64
	HTTP5xx float64
	HTTPTotal       float64
	QueryP99Seconds float64
	FundGateStore   float64
	ReconOpen float64
	AuditGapFail float64
	RegionSG     float64
	RegionTYO float64
	RegionSGSeen bool
	RegionTYOSeen    bool
	TravelFXStale    float64
	TravelFXBurnRate float64
	TravelFXBurnRemain float64
	TravelFXVendorFail float64
	TravelFXVendorOK   float64
	TravelFXBurnRate5m float64
	WDTrackOKMin       float64
	WDTrackOKSeen      bool
	WDTrackDriftMax float64
	WDTrackHydrateErrMax float64
	WDTrackPersistErrMax float64
	WDReorgSagaOpen      float64
	WDReorgSagaMaxAge   float64 // max across open phases
	WDReorgSagaTimeouts float64
	EarnAuditRows       float64
	EarnAuditMaxAge     float64
	EarnAuditWriteErr   float64
}

// Alert 一条已评估规则，名称与 deploy/alerts.yml 对应。

type Alert struct {
	Name string `json:"name"`
	Severity string  `json:"severity"`
	Firing   bool    `json:"firing"`
	Value    float64 `json:"value"`
	Threshold float64 `json:"threshold"`
	Hint      string  `json:"hint"`
}

// Snapshot /v1/slo 和 /v1/ops/alerts 用的现场快照。
type Snapshot struct {
	Disclaimer string `json:"disclaimer"`
	ReadyOK    bool   `json:"ready_ok"`
	NodesDown  int    `json:"nodes_down"`
	Submit map[string]any `json:"chain_submit"`
	Query  map[string]any `json:"wallet_query"`
	Market    map[string]any `json:"market_push"`
	Reconcile map[string]any `json:"reconcile"`
	HTTP      map[string]any `json:"http"`
	Fund map[string]any `json:"fund"`
	Region   map[string]any `json:"region"`
	Alerts   []Alert        `json:"alerts"`
	Localize []string       `json:"localize"`
}

   func GatherPrometheus() Gathered {
	  mfs, err := prometheus.DefaultGatherer.Gather()
	   if err != nil {
		   return Gathered{}
	   }
	  var g Gathered
	  var queryP99 float64
	   for _, mf := range mfs {
		   switch mf.GetName() {
		  case "aa_chain_submit_total":
			  for _, m := range mf.GetMetric() {
				  v := counterValue(m)
				   switch label(m, "result") {
				   case "ok":
					   g.SubmitOK += v
				   case "fail":
					   g.SubmitFail += v
				  }
			   }
		  case "aa_reconcile_mismatches":
			  for _, m := range mf.GetMetric() {
				  if m.Gauge != nil && m.Gauge.Value != nil && *m.Gauge.Value > g.ReconcileMismatchMax {
					   g.ReconcileMismatchMax = *m.Gauge.Value
				  }
			   }
		  case "aa_http_requests_total":
			  for _, m := range mf.GetMetric() {
				  v := counterValue(m)
				  g.HTTPTotal += v
				  if statusClass(label(m, "status")) == "5" {
					  g.HTTP5xx += v
				   }
			   }
		   case "aa_wallet_query_seconds":
			   for _, m := range mf.GetMetric() {
				   if m.Histogram == nil {
					   continue
				  }
				  q := histogramUpperBound(m.Histogram, 0.99)
				  if q > queryP99 {
					  queryP99 = q
				   }
			  }
		  case "aa_fund_gates_total":
			  for _, m := range mf.GetMetric() {
				  if label(m, "result") == "store" {
					   g.FundGateStore += counterValue(m)
				   }
			   }
		  case "aa_recon_backlog":
			  for _, m := range mf.GetMetric() {
				  if m.Gauge != nil && m.Gauge.Value != nil && *m.Gauge.Value > g.ReconOpen {
					   g.ReconOpen = *m.Gauge.Value
				  }
			  }
		   case "aa_audit_gaps_total":
			  for _, m := range mf.GetMetric() {
				   if label(m, "result") != "ok" {
					  g.AuditGapFail += counterValue(m)
				   }
			  }
		  case "aa_travel_fx_stale_count":
			  for _, m := range mf.GetMetric() {
				   if m.Gauge != nil && m.Gauge.Value != nil && *m.Gauge.Value > g.TravelFXStale {
					   g.TravelFXStale = *m.Gauge.Value
				  }
			   }
		  case "aa_travel_fx_burn_rate":
			  for _, m := range mf.GetMetric() {
				   if m.Gauge != nil && m.Gauge.Value != nil {
					  g.TravelFXBurnRate = *m.Gauge.Value
				   }
			   }
		  case "aa_travel_fx_burn_rate_window":
			   for _, m := range mf.GetMetric() {
				  if m.Gauge != nil && m.Gauge.Value != nil && label(m, "window") == "5m" {
					  g.TravelFXBurnRate5m = *m.Gauge.Value
				   }
			  }
		  case "aa_travel_fx_burn_remaining":
			  for _, m := range mf.GetMetric() {
				  if m.Gauge != nil && m.Gauge.Value != nil {
					   g.TravelFXBurnRemain = *m.Gauge.Value
				   }
			   }
		   case "aa_wd_track_ok":
			  for _, m := range mf.GetMetric() {
				  if m.Gauge == nil || m.Gauge.Value == nil {
					   continue
				   }
				   v := *m.Gauge.Value
				  if !g.WDTrackOKSeen {
					  g.WDTrackOKMin = v
					   g.WDTrackOKSeen = true
				  } else if v < g.WDTrackOKMin {
					   g.WDTrackOKMin = v
				   }
			   }
		   case "aa_wd_track_drift":
			   for _, m := range mf.GetMetric() {
				  if m.Gauge != nil && m.Gauge.Value != nil && *m.Gauge.Value > g.WDTrackDriftMax {
					  g.WDTrackDriftMax = *m.Gauge.Value
				  }
			   }
		  case "aa_wd_track_hydrate_err":
			  for _, m := range mf.GetMetric() {
				   if m.Gauge != nil && m.Gauge.Value != nil && *m.Gauge.Value > g.WDTrackHydrateErrMax {
					  g.WDTrackHydrateErrMax = *m.Gauge.Value
				   }
			  }
		  case "aa_wd_track_persist_err":
			   for _, m := range mf.GetMetric() {
				   if m.Gauge != nil && m.Gauge.Value != nil && *m.Gauge.Value > g.WDTrackPersistErrMax {
					   g.WDTrackPersistErrMax = *m.Gauge.Value
				  }
			  }
		   case "aa_wd_reorg_saga":
			  for _, m := range mf.GetMetric() {
				   if m.Gauge == nil || m.Gauge.Value == nil {
					  continue
				   }
				  ph := label(m, "phase")
				   if ph == "reorged" || ph == "resigning" || ph == "resubmitting" {
					  g.WDReorgSagaOpen += *m.Gauge.Value
				  }
			  }
		   case "aa_wd_reorg_saga_max_age_seconds":
			   for _, m := range mf.GetMetric() {
				   if m.Gauge == nil || m.Gauge.Value == nil {
					   continue
				  }
				   ph := label(m, "phase")
				  if ph == "reorged" || ph == "resigning" || ph == "resubmitting" {
					   if *m.Gauge.Value > g.WDReorgSagaMaxAge {
						   g.WDReorgSagaMaxAge = *m.Gauge.Value
					  }
				  }
			  }
		  case "aa_wd_reorg_saga_timeout_total":
			   for _, m := range mf.GetMetric() {
				  v := counterValue(m)
				  if v > g.WDReorgSagaTimeouts {
					  g.WDReorgSagaTimeouts = v
				  }
			   }
		   case "aa_earn_product_audit_rows":
			   for _, m := range mf.GetMetric() {
				  if m.Gauge != nil && m.Gauge.Value != nil {
					   g.EarnAuditRows = *m.Gauge.Value
				  }
			  }
		  case "aa_earn_product_audit_max_age_seconds":
			  for _, m := range mf.GetMetric() {
				   if m.Gauge != nil && m.Gauge.Value != nil && *m.Gauge.Value > g.EarnAuditMaxAge {
					  g.EarnAuditMaxAge = *m.Gauge.Value
				   }
			   }
		  case "aa_earn_product_audit_write_err":
			  for _, m := range mf.GetMetric() {
				   if m.Gauge != nil && m.Gauge.Value != nil {
					  g.EarnAuditWriteErr = *m.Gauge.Value
				   }
			   }
		  case "aa_travel_fx_vendor_total":
			  for _, m := range mf.GetMetric() {
				   v := counterValue(m)
				   switch label(m, "result") {
				   case "ok":
					  g.TravelFXVendorOK += v
				  case "fail":
					   g.TravelFXVendorFail += v
				   }
			  }
		  case "aa_region_write_ok":
			   for _, m := range mf.GetMetric() {
				   v := 0.0
				   if m.Gauge != nil && m.Gauge.Value != nil {
					   v = *m.Gauge.Value
				  }
				   switch label(m, "region") {
				  case "sg":
					  g.RegionSG = v
					  g.RegionSGSeen = true
				   case "tyo":
					   g.RegionTYO = v
					   g.RegionTYOSeen = true
				   }
			   }
		   }
	   }
	   g.QueryP99Seconds = queryP99
	   return g
  }

  func Evaluate(p ProcessView, g Gathered) []Alert {
	  submitN := g.SubmitOK + g.SubmitFail
	   submitRate := 1.0
	  if submitN > 0 {
		   submitRate = g.SubmitOK / submitN
	   }
	   httpErr := 0.0
	   if g.HTTPTotal > 0 {
		  httpErr = g.HTTP5xx / g.HTTPTotal
	   }
	   availBudget := 1.0 - 0.9995
	  queryP99Ms := g.QueryP99Seconds * 1000
	   marketSamples := p.MarketApplied + p.MarketStaleDropped
	  return []Alert{
		  {
			  Name: "ProcessReady", Severity: "page", Firing: !p.ReadyOK,
			   Value: boolToFloat(!p.ReadyOK), Threshold: 0,
			   Hint: "GET /readyz then mysql/redis ping; wallet-core stays up if only chain RPCs are degraded",
		   },
		  {
			   Name: "AAReconcileMismatch", Severity: "page", Firing: g.ReconcileMismatchMax > 0,
			  Value: g.ReconcileMismatchMax, Threshold: 0,
			  Hint: "GET /v1/ledger/statement and GET /v1/ledger/reconcile; GET /v1/ledger/audit?entity_type=transfer",
		  },
		  {
			   Name: "AAChainSubmitSuccess", Severity: "page",
			   Firing: submitN > 0 && submitRate < ChainSubmitTarget,
			   Value: submitRate, Threshold: ChainSubmitTarget,
			   Hint: "GET /v1/wallet/nodes/health; check aa_chain_rpc_total and circuit-open endpoints",
		  },
		   {
			   Name: "AAMarketPushLag", Severity: "ticket",
			  Firing: p.MarketLagMs > MarketPushTarget.Milliseconds(),
			  Value: float64(p.MarketLagMs), Threshold: float64(MarketPushTarget.Milliseconds()),
			  Hint: "GET /v1/market/lag; confirm marketd is producing and api is consuming market.ticks",
		   },
		   {
			   Name: "AAMarketStaleRate", Severity: "ticket",
			  Firing: marketSamples > 0 && p.MarketStaleRate > MarketStaleRateTarget,
			  Value:  p.MarketStaleRate, Threshold: MarketStaleRateTarget,
			  Hint: "GET /v1/market/lag; dropped seq/lag ticks — not a second ticker book",
		   },
		   {
			   Name: "AAWalletQueryP99", Severity: "ticket",
			  Firing: g.QueryP99Seconds > 0 && g.QueryP99Seconds > QueryP99.Seconds(),
			   Value:  queryP99Ms, Threshold: float64(QueryP99.Milliseconds()),
			   Hint: "GET /v1/wallet/nodes/health; L1/Redis cache and hedged reads sit under query_timeout",
		   },
		  {
			  Name: "AAHTTP5xxBudget", Severity: "page",
			  Firing: g.HTTPTotal >= 20 && httpErr > availBudget,
			   Value:  httpErr, Threshold: availBudget,
			  Hint: "GET /metrics aa_http_requests_total; X-Trace-Id on 5xx logs then GET /v1/ledger/audit",
		   },
		   {
			  Name: "AANodesDegraded", Severity: "ticket", Firing: p.NodesDown > 0,
			  Value: float64(p.NodesDown), Threshold: 0,
			   Hint: "GET /readyz (degraded=true is allowed) and GET /v1/wallet/nodes/health — failover must keep submit alive",
		  },
		  {
			   Name: "AAFundGateStore", Severity: "page", Firing: g.FundGateStore > 0,
			  Value: g.FundGateStore, Threshold: 0,
			   Hint: "GET /metrics aa_fund_gates_total{result=\"store\"}; SQL/allowlist faults fail closed",
		   },
		   {
			  Name: "AAReconBacklog", Severity: "ticket", Firing: g.ReconOpen > 0,
			   Value: g.ReconOpen, Threshold: 0,
			  Hint: "GET /metrics aa_recon_backlog; open or acked discrepancy tickets",
		   },
		  {
			  Name: "AAAuditGaps", Severity: "page", Firing: g.AuditGapFail > 0,
			   Value: g.AuditGapFail, Threshold: 0,
			   Hint: "GET /metrics aa_audit_gaps_total; completeness/cover/tamper is fail-closed",
		   },
		  {
			  Name: "AASGWriteCIDR", Severity: "page",
			  Firing: g.RegionSGSeen && g.RegionSG < 1,
			  Value: g.RegionSG, Threshold: 1,
			  Hint: "SG write IP must be 10.18.0.0/16; GET /metrics aa_region_write_ok{region=\"sg\"}",
		   },
		  {
			  Name: "AATYOStandbyCIDR", Severity: "page",
			   Firing: g.RegionTYOSeen && g.RegionTYO < 1,
			   Value:  g.RegionTYO, Threshold: 1,
			   Hint: "TYO standby IP must be 10.19.0.0/16; GET /metrics aa_region_write_ok{region=\"tyo\"}",
		   },
		   {
			  Name: "AATravelFXStale", Severity: "ticket",
			  Firing: g.TravelFXStale > 0,
			   Value:  g.TravelFXStale, Threshold: 0,
			  Hint: "GET /v1/travel/thresholds/fx-health; aa_travel_fx_stale_count — stale FX alert only, missing-rate still fail-closed",
		  },
		   travelFXVendorBurnAlert(g),
		   {
			  Name: "AATravelFXVendorBurn5m", Severity: "ticket",
			   Firing: g.TravelFXBurnRate5m >= 1,
			   Value: g.TravelFXBurnRate5m, Threshold: 1,
			   Hint: "GET /v1/travel/thresholds/fx-vendor-slo; aa_travel_fx_burn_rate_window{window=5m}",
		   },
		   {
			   Name: "AATravelFXVendorBurnDual", Severity: "page",
			   Firing: g.TravelFXBurnRate5m >= 1 && g.TravelFXBurnRate >= 1,
			   Value:  g.TravelFXBurnRate, Threshold: 1,
			  Hint: "GET /v1/travel/thresholds/fx-vendor-slo; 5m∧1h dual-fire → page",
		  },
		  {
			   Name: "AAWDTrackHealth", Severity: "page",
			   Firing: (g.WDTrackOKSeen && g.WDTrackOKMin < 1) || g.WDTrackDriftMax > 0 || g.WDTrackHydrateErrMax > 0 || g.WDTrackPersistErrMax > 0,
			   Value:  g.WDTrackOKMin, Threshold: 1,
			   Hint: "GET /v1/ops/admin/wd-track-health; aa_wd_track_* hydrate/persist/drift",
		   },
		   {
			  // 软阈 ticket：开放 saga 年龄超 SagaTicketAge(10m)，仍在 DefaultSagaTimeout 内。
			   Name: "AAWDReorgSagaStuck", Severity: "ticket",
			  Firing: g.WDReorgSagaMaxAge >= 600 || (g.WDReorgSagaOpen > 0 && g.WDReorgSagaMaxAge >= 600),
			   Value:  g.WDReorgSagaMaxAge, Threshold: 600,
			   Hint: "GET /v1/withdrawals/broadcast/sagas; aa_wd_reorg_saga_max_age_seconds — soft stuck ticket (<30m timeout)",
		   },
		  {
			  // 硬阈 page：年龄已过 DefaultSagaTimeout，说明 TickSagas 未推进/未超时失败。
			  Name: "AAWDReorgSagaOverTimeout", Severity: "page",
			  Firing: g.WDReorgSagaMaxAge >= 1800,
			  Value:  g.WDReorgSagaMaxAge, Threshold: 1800,
			   Hint: "GET /v1/withdrawals/broadcast/sagas; max age past DefaultSagaTimeout — worker stuck, not auto-failing",
		  },
		  {
			  // 超时失败累计：已 fail 的 saga 须人工重开（ticket，非 page）。
			  Name: "AAWDReorgSagaTimeouts", Severity: "ticket",
			  Firing: g.WDReorgSagaTimeouts > 0,
			   Value:  g.WDReorgSagaTimeouts,
			  Threshold: 0,
			   Hint:      "POST /v1/withdrawals/broadcast/:id/resubmit/reopen; GET .../sagas/timed-out — TickSagas timed out; ops reopen, old_tx_hash retained",
		  },
		  {
			   // 理财目录软删审计写失败或超保留窗未清扫。
			   Name: "AAEarnProductAudit", Severity: "ticket",
			   Firing: g.EarnAuditWriteErr >= 1 || g.EarnAuditMaxAge >= 365*24*3600,
			  Value:  g.EarnAuditMaxAge, Threshold: 365 * 24 * 3600,
			   Hint: "GET /v1/ops/earn/products/audit?format=csv; POST .../audit/archive; POST .../audit/purge — write_err or age past 365d; cold-archive before purge",
		   },
	  }
  }

// travelFXVendorBurnAlert Live FX 厂商错误预算燃烧告警；严重度可调谐。
// 默认 ticket@burn>=1、page@burn>=2 或 remaining<=0；SeverityTune 可覆盖。
// 燃烧告警不放行缺价非稳定币。
   func travelFXVendorBurnAlert(g Gathered) Alert {
	   sev := SeverityFor("AATravelFXVendorBurn", g.TravelFXBurnRate, g.TravelFXBurnRemain)
	  if sev == "" {
		   switch {
		  case g.TravelFXBurnRemain < 0 || g.TravelFXBurnRate >= 2:
			  sev = "page"
		   case g.TravelFXBurnRate >= 1:
			   sev = "ticket"
		  default:
			  sev = "none"
		  }
	   }
	  // 5m∧1h 双火强制 page
	   if g.TravelFXBurnRate5m >= 1 && g.TravelFXBurnRate >= 1 {
		   sev = "page"
	   }
	  firing := sev == "ticket" || sev == "page"
	  if g.TravelFXVendorOK+g.TravelFXVendorFail < 20 {
		  firing = false
		   sev = "none"
	   }
	  return Alert{
		   Name: "AATravelFXVendorBurn", Severity: sev, Firing: firing,
		   Value: g.TravelFXBurnRate, Threshold: 1,
		  Hint: "GET /v1/travel/thresholds/fx-vendor-slo; aa_travel_fx_burn_rate — Live FX vendor error budget; missing-rate still fail-closed",
	  }
  }

   func Localize(alerts []Alert) []string {
	  out := make([]string, 0, 4)
	  for _, a := range alerts {
		   if a.Firing {
			   out = append(out, a.Name+": "+a.Hint)
		  }
	   }
	   if len(out) == 0 {
		  out = append(out, "no firing alerts; start at GET /readyz, GET /v1/ledger/statement, GET /v1/market/lag, GET /v1/wallet/nodes/health")
	  }
	  return out
  }

// Live 汇总本进程就绪、节点、行情滞后、提交成功率，给 /v1/slo 和告警用。
  func Live(p ProcessView, g Gathered) Snapshot {
	  alerts := Evaluate(p, g)
	   submitN := g.SubmitOK + g.SubmitFail
	   submitRate := 1.0
	   if submitN > 0 {
		   submitRate = g.SubmitOK / submitN
	  }
	  httpErr := 0.0
	  if g.HTTPTotal > 0 {
		   httpErr = g.HTTP5xx / g.HTTPTotal
	  }
	   return Snapshot{
		   Disclaimer: ProcessDisclaimer,
		  ReadyOK:    p.ReadyOK,
		  NodesDown: p.NodesDown,
		  Submit: map[string]any{
			  "ok":           g.SubmitOK,
			  "fail":         g.SubmitFail,
			  "success_rate": submitRate,
			   "target":       ChainSubmitTarget,
			  "meets_target": submitN == 0 || submitRate >= ChainSubmitTarget,
			  "samples":      submitN,
		  },
		   Query: map[string]any{
			   "p99_ms": g.QueryP99Seconds * 1000,
			  "target_ms": QueryP99.Milliseconds(),
			   "timeout_ms":   QueryTimeout.Milliseconds(),
			  "meets_target": g.QueryP99Seconds == 0 || g.QueryP99Seconds <= QueryP99.Seconds(),
		  },
		  Market: map[string]any{
			  "last_lag_ms":         p.MarketLagMs,
			   "lag_target_ms":       MarketPushTarget.Milliseconds(),
			   "polling_baseline_ms": MarketPollingBaseline.Milliseconds(),
			  "stale_rate":          p.MarketStaleRate,
			  "stale_rate_target":   MarketStaleRateTarget,
			  "noisy_feed_baseline": MarketNoisyFeedBaseline,
			  "applied":       p.MarketApplied,
			   "stale_dropped": p.MarketStaleDropped,
			   "meets_lag_target":   p.MarketLagMs <= MarketPushTarget.Milliseconds(),
			   "meets_stale_target": p.MarketApplied+p.MarketStaleDropped == 0 || p.MarketStaleRate <= MarketStaleRateTarget,
		   },
		  Reconcile: map[string]any{
			   "mismatch_max": g.ReconcileMismatchMax,
			   "ok":      g.ReconcileMismatchMax == 0,
			  "backlog": g.ReconOpen,
		   },
		   HTTP: map[string]any{
			   "total":               g.HTTPTotal,
			  "status_5xx":          g.HTTP5xx,
			   "error_rate":          httpErr,
			  "availability_target": AvailabilityTarget,
		  },
		   Fund: map[string]any{
			  "gate_store": g.FundGateStore,
			  "recon_open":     g.ReconOpen,
			   "audit_gap_fail": g.AuditGapFail,
		  },
		  Region: map[string]any{
			  "sg": g.RegionSG,
			  "tyo":      g.RegionTYO,
			   "sg_seen":  g.RegionSGSeen,
			   "tyo_seen": g.RegionTYOSeen,
		   },
		  Alerts: alerts,
		  Localize: Localize(alerts),
	  }
   }

// Firing 当前触发的告警，配合 Localize 给分钟级定位。
  func Firing(alerts []Alert) []Alert {
	  out := make([]Alert, 0)
	  for _, a := range alerts {
		   if a.Firing {
			   out = append(out, a)
		   }
	  }
	  return out
  }

  func label(m *dto.Metric, name string) string {
	  for _, lp := range m.GetLabel() {
		   if lp.GetName() == name {
			  return lp.GetValue()
		   }
	  }
	  return ""
   }

  func counterValue(m *dto.Metric) float64 {
	   if m == nil || m.Counter == nil || m.Counter.Value == nil {
		  return 0
	   }
	  return *m.Counter.Value
   }

  func statusClass(status string) string {
	  if status == "" {
		  return ""
	   }
	   if _, err := strconv.Atoi(status); err != nil {
		  return ""
	  }
	   return status[:1]
  }

   func histogramUpperBound(h *dto.Histogram, q float64) float64 {
	  if h == nil || h.GetSampleCount() == 0 {
		   return 0
	   }
	   need := q * float64(h.GetSampleCount())
	   for _, b := range h.GetBucket() {
		  if float64(b.GetCumulativeCount()) >= need {
			   return b.GetUpperBound()
		  }
	   }
	  if h.SampleSum != nil && h.GetSampleCount() > 0 {
		  return h.GetSampleSum() / float64(h.GetSampleCount())
	   }
	  return 0
   }

   func boolToFloat(v bool) float64 {
	  if v {
		   return 1
	   }
	   return 0
  }
