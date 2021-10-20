package reconflow

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// 包级指标变量，Engine/Worker 直接引用。

var (
	// ReconFlowRunsTotal FlowRun 生命周期计数（按 diff_kind 与终态分）。
	ReconFlowRunsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_reconflow_runs_total",
		Help: "FlowRun lifecycle events by diff_kind and outcome (alert on escalated spike)",
	}, []string{"diff_kind", "outcome"})
	// ReconFlowActionsTotal 动作执行结果计数。
	ReconFlowActionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_reconflow_actions_total",
		Help: "Action execution results by action_kind and result",
	}, []string{"action_kind", "result"})
	// ReconFlowSLABreached SLA 违约计数（按 diff_kind 分）。
	ReconFlowSLABreached = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_reconflow_sla_breached_total",
		Help: "FlowRun SLA breach count by diff_kind (alert on any breach)",
	}, []string{"diff_kind"})
	// ReconFlowActiveRuns 当前非终态 Run 数量（Gauge）。
	ReconFlowActiveRuns = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_reconflow_active_runs",
		Help: "Current count of non-terminal FlowRuns",
	})
	// ReconFlowGatedSteps 当前等待人工放行的步骤数（Gauge）。
	ReconFlowGatedSteps = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_reconflow_gated_steps",
		Help: "Current count of FlowSteps waiting for human gate",
	})
	// ReconFlowWorkerTick Worker 心跳计数（按 worker 名分）。
	ReconFlowWorkerTick = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_reconflow_worker_ticks_total",
		Help: "Worker tick count by worker name",
	}, []string{"worker"})
	// ReconFlowWorkerErrors Worker 错误计数。
	ReconFlowWorkerErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_reconflow_worker_errors_total",
		Help: "Worker error count by worker name",
	}, []string{"worker"})
	// ReconFlowClassifyLatency 分类耗时直方图。
	ReconFlowClassifyLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "aa_reconflow_classify_seconds",
		Help:    "Classify duration in seconds",
		Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5},
	})
	// ReconFlowActionLatency 动作执行耗时直方图。
	ReconFlowActionLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "aa_reconflow_action_seconds",
		Help:    "Action execution duration by action_kind",
		Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 5},
	}, []string{"action_kind"})
)
