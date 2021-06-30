package fxonce

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics 持有本包所有 Prometheus 指标。

type Metrics struct {
	// EffectsProposed 累计 proposed 次数（含重建）。
	EffectsProposed prometheus.Counter
	// EffectsApplied 累计 applied 次数（CAS 通过，账本意图可产生）。
	EffectsApplied prometheus.Counter
	// EffectsRejected 累计 rejected 次数（stale_gen / dup_gen / poisoned）。
	EffectsRejected *prometheus.CounterVec
	// EffectsPoisoned 当前 poisoned effect 数量（Gauge，Worker 周期更新）。
	PoisonedEffects prometheus.Gauge
	// DupProofsCreated 累计产生的双花证明数量。
	DupProofsCreated prometheus.Counter
	// UnreviewedDups 当前未审阅的 DupProof 数量（Gauge，Worker 周期更新）。
	UnreviewedDups prometheus.Gauge
	// CASLatencyMs CAS Apply 延迟直方图（毫秒）。
	CASLatencyMs prometheus.Histogram
	// WorkerRuns 累计 Worker.Run 执行次数。
	WorkerRuns prometheus.Counter
	// ForceReplays 累计强制重放次数（需关注此值异常升高）。
	ForceReplays prometheus.Counter
	// KindGateDecisions 各 kind 闸裁决计数（by kind, decision）。
	KindGateDecisions *prometheus.CounterVec
}

// NewMetrics 注册并返回指标集合。subsystem 建议传入 "fxonce"。
   func NewMetrics(subsystem string) *Metrics {
	  if subsystem == "" {
		  subsystem = "fxonce"
	  }
	   return &Metrics{
		  EffectsProposed: promauto.NewCounter(prometheus.CounterOpts{
			  Namespace: "aa",
			  Subsystem: subsystem,
			  Name:      "effects_proposed_total",
			  Help:      "Total number of fund effects proposed (including re-proposals after force-replay).",
		  }),
		  EffectsApplied: promauto.NewCounter(prometheus.CounterOpts{
			   Namespace: "aa",
			  Subsystem: subsystem,
			  Name: "effects_applied_total",
			   Help: "Total number of fund effects successfully applied (CAS passed, ledger intent may be produced).",
		  }),
		  EffectsRejected: promauto.NewCounterVec(prometheus.CounterOpts{
			   Namespace: "aa",
			  Subsystem: subsystem,
			  Name: "effects_rejected_total",
			   Help: "Total number of fund effects rejected by CAS, by reject_code.",
		  }, []string{"reject_code"}),
		   PoisonedEffects: promauto.NewGauge(prometheus.GaugeOpts{
			  Namespace: "aa",
			   Subsystem: subsystem,
			  Name:      "poisoned_effects_current",
			  Help:      "Current number of fund effects in poisoned state (require manual dual-approval to release).",
		   }),
		  DupProofsCreated: promauto.NewCounter(prometheus.CounterOpts{
			   Namespace: "aa",
			   Subsystem: subsystem,
			  Name:      "dup_proofs_created_total",
			  Help: "Total number of dup proofs created (each represents one double-spend attempt blocked).",
		  }),
		   UnreviewedDups: promauto.NewGauge(prometheus.GaugeOpts{
			   Namespace: "aa",
			  Subsystem: subsystem,
			  Name: "unreviewed_dup_proofs_current",
			  Help: "Current number of dup proofs pending desk review.",
		   }),
		   CASLatencyMs: promauto.NewHistogram(prometheus.HistogramOpts{
			   Namespace: "aa",
			  Subsystem: subsystem,
			   Name:      "cas_apply_latency_ms",
			   Help:      "Latency of CASEngine.Apply in milliseconds.",
			   Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500},
		   }),
		  WorkerRuns: promauto.NewCounter(prometheus.CounterOpts{
			  Namespace: "aa",
			   Subsystem: subsystem,
			   Name:      "worker_runs_total",
			   Help: "Total number of Worker.Run executions.",
		   }),
		  ForceReplays: promauto.NewCounter(prometheus.CounterOpts{
			   Namespace: "aa",
			   Subsystem: subsystem,
			   Name: "force_replays_total",
			   Help: "Total number of force-replay operations (dual-approval releases from poisoned state).",
		  }),
		  KindGateDecisions: promauto.NewCounterVec(prometheus.CounterOpts{
			   Namespace: "aa",
			   Subsystem: subsystem,
			   Name: "kind_gate_decisions_total",
			  Help: "Total number of KindGate.Decide calls, by kind and decision (must_gate / bypass / blocked).",
		   }, []string{"kind", "decision"}),
	  }
   }

// RecordApply 记录一次 CAS Apply 结果。

   func (m *Metrics) RecordApply(result MarkResult, latencyMs float64) {
	   if m == nil {
		  return
	   }
	  m.CASLatencyMs.Observe(latencyMs)
	  if result.Idempotent {
		   return
	  }
	   if result.Rejected {
		   m.EffectsRejected.WithLabelValues(string(result.RejectCode)).Add(1)
		  if result.Dup != nil {
			  m.DupProofsCreated.Add(1)
		   }
	   } else {
		  m.EffectsApplied.Add(1)
	  }
  }

// RecordGateDecision 记录一次 KindGate 裁决。

  func (m *Metrics) RecordGateDecision(kind string, decision string) {
	  if m == nil {
		  return
	  }
	   m.KindGateDecisions.WithLabelValues(kind, decision).Add(1)
  }
