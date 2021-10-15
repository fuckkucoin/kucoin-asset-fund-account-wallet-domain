package reclaim

import (
	"account-assets.kucoin.net/funding/internal/metrics"
)

// recordReclaimOpened 记录案件开立指标。
  func recordReclaimOpened(kind, chain, asset string) {
	   metrics.ReclaimCasesOpened.WithLabelValues(kind, chain, asset).Inc()
	   metrics.ReclaimActiveCases.Inc()
   }

// recordReclaimStateChange 记录状态变更指标。

  func recordReclaimStateChange(from, to, chain, asset string) {
	  metrics.ReclaimStateTransitions.WithLabelValues(from,
		   to, chain, asset).Inc()
	  // 终态时活跃数递减
	  switch State(to) {
	  case StateReclaimed, StateCancelled, StateAutoCredited:
		   metrics.ReclaimActiveCases.Dec()
	   }
  }

// recordReclaimSLABreach 记录 SLA 超时指标。
  func recordReclaimSLABreach(event, chain, asset string) {
	  metrics.ReclaimSLABreaches.WithLabelValues(event, chain, asset).Inc()
  }

// recordLateCreditBlocked 记录迟贷记阻塞指标。
  func recordLateCreditBlocked(chain, asset string) {
	   metrics.ReclaimLateCreditBlocked.WithLabelValues(chain, asset).Inc()
   }

// recordSweepLegsCreated 记录 Sweep leg 目录创建指标。

  func recordSweepLegsCreated(chain, asset string) {
	  metrics.ReclaimSweepLegsCreated.WithLabelValues(chain, asset).Inc()
  }
