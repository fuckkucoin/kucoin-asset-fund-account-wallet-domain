package dispclaw

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// metricPlanStatus 各状态 FundPlan 数量（gauge；由 worker 定期更新）。
	metricPlanStatus = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_dispclaw_plan_status_total",
		Help: "各状态的争议资金计划数量（open/executing/closed/aborted）",
	}, []string{"status"})
	// metricLegStatus 各状态 RecoveryLeg 数量（gauge）。
	metricLegStatus = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_dispclaw_leg_status_total",
		Help: "各状态的追偿腿数量（pending/posting/posted/failed/skipped）",
	}, []string{"status"})
	// metricLegExecuteTotal 追偿腿执行计数（counter；按类型和结果分）。
	metricLegExecuteTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_dispclaw_leg_execute_total",
		Help: "追偿腿执行次数（ok=执行成功；foureye_reject=四眼拒绝；err=其他错误）",
	}, []string{"leg_type", "result"})
	// metricGatePassTotal 收口闸放行计数（counter）。
	metricGatePassTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_dispclaw_gate_pass_total",
		Help: "收口闸从 blocking 切换到 passed 的次数",
	})
	// metricProofGenerateTotal 收口证明生成计数（counter）。
	metricProofGenerateTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_dispclaw_proof_generate_total",
		Help: "收口证明生成次数",
	})
	// metricPlanTimeoutTotal 计划超时升级计数（counter；按 plan_type 分）。
	metricPlanTimeoutTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_dispclaw_plan_timeout_total",
		Help: "超时升级触发次数",
	}, []string{"plan_type"})
	// metricProofTamperTotal 收口证明篡改检测计数（counter；detected/ok 分）。
	metricProofTamperTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_dispclaw_proof_tamper_check_total",
		Help: "收口证明巡检次数（ok=正常；detected=发现篡改）",
	}, []string{"result"})
	// metricFourEyeRejectTotal 四眼校验拒绝计数（counter）。
	metricFourEyeRejectTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_dispclaw_foureye_reject_total",
		Help: "四眼校验拒绝次数（开单人尝试执行追偿腿）",
	})
	// metricGateBypassTotal 管理员特批豁免计数（counter；正常应为 0，非 0 需告警）。
	metricGateBypassTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_dispclaw_gate_bypass_total",
		Help: "管理员特批豁免收口闸次数（非 0 需运营审查）",
	})
)

// RecordLegExecute 记录腿执行结果指标。
   func RecordLegExecute(legType LegType, result string) {
	   metricLegExecuteTotal.WithLabelValues(string(legType), result).Inc()
  }

// RecordGatePass 记录收口闸放行指标。

   func RecordGatePass() {
	  metricGatePassTotal.Inc()
  }

// RecordProofGenerate 记录证明生成指标。
   func RecordProofGenerate() {
	  metricProofGenerateTotal.Inc()
   }

// RecordFourEyeReject 记录四眼拒绝指标。
  func RecordFourEyeReject() {
	   metricFourEyeRejectTotal.Inc()
  }

// RecordGateBypass 记录豁免指标（告警级别：非 0 时运营需介入）。
  func RecordGateBypass() {
	   metricGateBypassTotal.Inc()
   }

// Counts 返回当前内存服务的各状态统计（供健康检查端点使用）。

   func (s *Service) Counts() map[string]int {
	   if s == nil {
		   return nil
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  out := map[string]int{
		   "plans_open":      0,
		   "plans_executing": 0,
		  "plans_closed":    0,
		   "plans_aborted":   0,
		   "legs_pending": 0,
		  "legs_posted": 0,
		   "legs_failed":    0,
		   "legs_skipped":   0,
		   "gates_blocking": 0,
		   "gates_passed": 0,
		  "proofs": 0,
	  }
	  for _, p := range s.plans {
		   out["plans_"+string(p.Status)]++
	   }
	  for _, l := range s.legs {
		   out["legs_"+string(l.Status)]++
	   }
	   for _, g := range s.gates {
		   out["gates_"+string(g.GateStatus)]++
	  }
	  out["proofs"] = len(s.proofs)
	  return out
  }
