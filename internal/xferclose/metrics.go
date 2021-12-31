// 功能：Prometheus 指标：半截 hop 检测、收口执行、SLA 违约。
// 作者：Bronson
// 日期：2021.12.30

package xferclose

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// HopsDetectedTotal 检测到的半截工单总数。
	HopsDetectedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_xferclose_hops_detected_total",
		Help: "Half-hop tickets detected, by kind",
	}, []string{"kind"})
	// HopsSLABreachedTotal SLA 超限工单累计数。
	HopsSLABreachedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_xferclose_hops_sla_breached_total",
		Help: "Half-hop tickets that breached SLA deadline, by asset",
	}, []string{"asset"})
	// CloseoutActionsTotal 收口动作执行次数（按动作和结果）。
	CloseoutActionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_xferclose_closeout_actions_total",
		Help: "Closeout actions executed, by action and result",
	}, []string{"action", "result"})
	// CloseoutLegsTotal 收口腿执行总数（按动作和状态）。
	CloseoutLegsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_xferclose_legs_total",
		Help: "Closeout legs executed, by action and state",
	}, []string{"action", "state"})
	// ActiveHopsGauge 当前活跃（非终态）半截工单数。
	ActiveHopsGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_xferclose_active_hops",
		Help: "Current count of non-terminal half-hop tickets",
	})
	// BreachedHopsGauge 当前 SLA 已超限且未收口的工单数。
	BreachedHopsGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_xferclose_breached_hops",
		Help: "Half-hop tickets currently SLA-breached and not yet closed",
	})
	// CloseoutDurationSeconds 收口动作耗时分布。
	CloseoutDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "aa_xferclose_closeout_duration_seconds",
		Help: "Duration of closeout action execution",
		Buckets: []float64{0.05,
			0.1, 0.25, 0.5, 1, 2, 5},
	}, []string{"action"})
	// PlanStateGauge 各状态收口计划数。
	PlanStateGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_xferclose_plan_state",
		Help: "Count of closeout plans by state",
	}, []string{"state"})
	// SLARuleMatchTotal SLA 规则命中次数（按规则 ID）。
	SLARuleMatchTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_xferclose_sla_rule_matches_total",
		Help: "SLA rule match count by rule ID",
	}, []string{"rule_id"})
	// DetectScanTotal 检测扫描轮次计数。
	DetectScanTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_xferclose_detect_scans_total",
		Help: "Total detect worker scan rounds",
	})
	// BackfillIngressTotal 补入腿执行次数（成功/失败）。
	BackfillIngressTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_xferclose_backfill_ingress_total",
		Help: "Backfill ingress executions, by result (success/duplicate/fail)",
	}, []string{"result"})
	// RollbackEgressTotal 回滚出腿执行次数。
	RollbackEgressTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_xferclose_rollback_egress_total",
		Help: "Rollback egress executions, by result",
	}, []string{"result"})
)

// RecordDetect 从 DetectResult 更新指标。
  func RecordDetect(r DetectResult) {
	   DetectScanTotal.Inc()
	  if r.NewHops > 0 {
		   HopsDetectedTotal.WithLabelValues("open").Add(float64(r.NewHops))
	  }
	  if r.Breached > 0 {
		  HopsSLABreachedTotal.WithLabelValues("unknown").Add(float64(r.Breached))
	   }
   }

// RecordCloseoutResult 从 CloseoutResult 更新指标。
   func RecordCloseoutResult(r CloseoutResult) {
	   resultStr := "fail"
	  if r.Success {
		  resultStr = "success"
	   }
	  CloseoutActionsTotal.WithLabelValues(string(r.Action), resultStr).Inc()
   }
