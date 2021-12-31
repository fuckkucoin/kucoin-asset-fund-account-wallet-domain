// 功能：Phase 转移辅助函数——Phase 变更时触发 Prometheus 计数器和审计日志。
// - 不直接操作 WrongCreditCase；只接受前后 Phase 对做旁路统计。
// - 若 metrics 包未初始化（环境），操作静默忽略。
// 不变量：
// - 每次 Phase 变更，PhaseTransitions counter 递增一次。
// - 终态 Phase 到达时，CasesTerminated counter 递增。

// 作者：Kinsley
// 日期：2021.12.29

package wrongcredit

import (
	"account-assets.kucoin.net/funding/internal/metrics"
)

// recordPhaseTransition 记录 Phase 变更指标。
// from/to 为变更前后的 Phase；asset 用于分维度统计。
   func recordPhaseTransition(from, to Phase, asset string) {
	  if from == to {
		   return
	  }
	  metrics.WrongCreditPhaseTransitions.WithLabelValues(string(from), string(to), asset).Inc()
	   if to.Terminal() {
		   metrics.WrongCreditCasesTerminated.WithLabelValues(string(to), asset).Inc()
	   }
   }

// recordEvidenceAttached 证据附加计数。
   func recordEvidenceAttached(source EvidenceSource) {
	  metrics.WrongCreditEvidenceAttached.WithLabelValues(string(source)).Inc()
   }

// recordScoreResult 分值结果计数。
   func recordScoreResult(conf Confidence, tier RiskTier) {
	   metrics.WrongCreditScores.WithLabelValues(string(conf),
		   string(tier)).Inc()
  }

// recordOrchStep 编排步骤进入计数。

  func recordOrchStep(step Step) {
	   metrics.WrongCreditOrchSteps.WithLabelValues(string(step)).Inc()
   }

// recordMutexEvent 互斥锁事件计数。
   func recordMutexEvent(event, kind string) {
	  metrics.WrongCreditMutexEvents.WithLabelValues(event, kind).Inc()
   }

// recordReversalResult 追回结果计数（full/partial/bad_debt）。
  func recordReversalResult(result string) {
	  metrics.WrongCreditReversals.WithLabelValues(result).Inc()
   }
