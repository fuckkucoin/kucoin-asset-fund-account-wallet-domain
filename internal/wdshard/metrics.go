package wdshard

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// metricPlanTotal 计划按终态统计。state label = filled | partial_closed | aborted。
	metricPlanTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_wdshard_plan_total",
		Help: "Shard plans reaching a given terminal state",
	}, []string{"asset", "state"})
	// metricLegTotal 分片腿按终态统计。
	metricLegTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_wdshard_leg_total",
		Help: "Shard legs reaching a given terminal state",
	}, []string{"asset", "state"})
	// metricActivePlans 当前 running 计划数（gauge，按资产）。
	metricActivePlans = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_wdshard_active_plans",
		Help: "Shard plans currently in running state",
	}, []string{"asset"})
	// metricActiveLegs 当前 broadcasting legs 数量（在途）。
	metricActiveLegs = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_wdshard_inflight_legs",
		Help: "Shard legs currently broadcasting",
	}, []string{"asset"})
	// metricConserveViolation 守恒漂移告警计数。
	metricConserveViolation = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_wdshard_conserve_violation_total",
		Help: "Conservation violations detected; nonzero means accounting drift",
	}, []string{"plan_id"})
	// metricWorkerTick Worker 每次 tick 耗时。
	metricWorkerTick = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "aa_wdshard_worker_tick_seconds",
		Help: "Time spent per worker tick",
		Buckets: []float64{0.001,
			0.005, 0.01, 0.05, 0.1, 0.5, 1, 5},
	}, []string{"worker"})
	// metricHoldOpen partial-fill 层当前开立的 ShardHold 总数（per-asset gauge）。
	metricPartialHoldOpen = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_wdpartial_hold_open",
		Help: "ShardHold records currently in open state",
	}, []string{"asset"})
	// metricPartialFillConfirmed 累计已确认 PartialFill 数。
	metricPartialFillConfirmed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_wdpartial_fill_confirmed_total",
		Help: "PartialFill records reaching confirmed state",
	}, []string{"asset"})
)

// ObservePlanTerminal 在计划到达终态时调用，更新 counter 与 gauge。
   func ObservePlanTerminal(asset string, state PlanState) {
	  metricPlanTotal.WithLabelValues(asset, string(state)).Inc()
	  metricActivePlans.WithLabelValues(asset).Dec()
  }

// ObservePlanStart 在计划进入 running 时调用。

   func ObservePlanStart(asset string) {
	  metricActivePlans.WithLabelValues(asset).Inc()
  }

// ObserveLegTerminal 在腿到达终态时调用。
   func ObserveLegTerminal(asset string, state LegState) {
	   metricLegTotal.WithLabelValues(asset, string(state)).Inc()
	   if state == LegFilled {
		  metricActiveLegs.WithLabelValues(asset).Dec()
	   }
   }

// ObserveLegBroadcast 在腿进入 broadcasting 时调用。

  func ObserveLegBroadcast(asset string) {
	  metricActiveLegs.WithLabelValues(asset).Inc()
  }

// ObserveConserveViolation 记录守恒违反事件。
  func ObserveConserveViolation(planID string) {
	   metricConserveViolation.WithLabelValues(planID).Inc()
   }
