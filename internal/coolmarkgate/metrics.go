package coolmarkgate

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// metricMarkTotal 冷却 Mark 签发次数。
	// 标签 asset：资产代码；reused：是否复用现有租约。
	metricMarkTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cool_mark_gate_mark_total",
		Help: "Total number of CoolMark operations.",
	}, []string{"asset", "reused"})
	// metricExecBlockedTotal Exec 路径拒绝次数。
	// 标签 asset：资产代码；reason：拒绝原因（gen_mismatch / window_active / proof_missing）。
	metricExecBlockedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cool_mark_gate_exec_blocked_total",
		Help: "Total number of exec verifications blocked by cool gate.",
	}, []string{"asset", "reason"})
	// metricExecAllowedTotal Exec 路径放行次数。
	metricExecAllowedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cool_mark_gate_exec_allowed_total",
		Help: "Total number of exec verifications allowed by cool gate.",
	}, []string{"asset"})
	// metricProofIssuedTotal CoolProof 签发次数。
	metricProofIssuedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cool_mark_gate_proof_issued_total",
		Help: "Total number of CoolProof issued by ClearCool.",
	}, []string{"asset"})
	// metricBreachTotal 违约案例落库次数。
	// 标签 asset：资产代码；decision：blocked 或 drifted。
	metricBreachTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cool_mark_gate_breach_total",
		Help: "Total number of CoolBreachCase recorded.",
	}, []string{"asset", "decision"})
	// metricPatrolClearedTotal worker 单轮清除的冷却租约数。
	metricPatrolClearedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "cool_mark_gate_patrol_cleared_total",
		Help: "Total number of cool leases cleared by patrol worker.",
	})
	// metricStalePlanDetectedTotal 漂移发现 worker 检测到的漂移事件数。
	metricStalePlanDetectedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cool_mark_gate_stale_plan_detected_total",
		Help: "Total number of stale plan drift events detected.",
	}, []string{"asset"})
	// metricActiveCoolLeases 当前活跃冷却租约数（Gauge）。
	metricActiveCoolLeases = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cool_mark_gate_active_cool_leases",
		Help: "Current number of active (non-cleared) cool leases.",
	}, []string{"asset"})
)

// ──────────────────────────────────────────────────────────────────────────────
// 指标上报辅助函数（供 Engine 方法内部调用）
// ──────────────────────────────────────────────────────────────────────────────

  func recordMarkMetric(asset string, reused bool) {
	  reusedLabel := "false"
	  if reused {
		   reusedLabel = "true"
	   }
	   metricMarkTotal.WithLabelValues(asset, reusedLabel).Inc()
	   if !reused {
		   metricActiveCoolLeases.WithLabelValues(asset).Inc()
	  }
  }

  func recordExecBlockedMetric(asset, reason string) {
	  metricExecBlockedTotal.WithLabelValues(asset, reason).Inc()
   }

   func recordExecAllowedMetric(asset string) {
	   metricExecAllowedTotal.WithLabelValues(asset).Inc()
   }

   func recordProofIssuedMetric(asset string) {
	   metricProofIssuedTotal.WithLabelValues(asset).Inc()
	   metricActiveCoolLeases.WithLabelValues(asset).Dec()
   }

  func recordBreachMetric(asset string, decision BreachDecision) {
	  metricBreachTotal.WithLabelValues(asset, string(decision)).Inc()
   }

  func recordPatrolClearedMetric(n int) {
	  metricPatrolClearedTotal.Add(float64(n))
   }

   func recordStalePlanDetectedMetric(asset string) {
	   metricStalePlanDetectedTotal.WithLabelValues(asset).Inc()
  }
