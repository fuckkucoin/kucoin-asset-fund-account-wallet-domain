package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// ReclaimCasesOpened 已开立回收案件计数（按 kind/chain/asset）。
	ReclaimCasesOpened = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_reclaim_cases_opened_total",
		Help: "Reclaim cases opened by kind, chain, and asset",
	}, []string{"kind", "chain", "asset"})
	// ReclaimStateTransitions 案件状态变更计数（from/to/chain/asset 四维）。
	// 告警：to=escalated 出现时触发 P2 告警。
	ReclaimStateTransitions = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_reclaim_state_transitions_total",
		Name: "aa_reclaim_sla_breaches_total",
		Help: "Reclaim SLA breach events triggering escalation",
	}, []string{"event", "chain", "asset"})
	// ReclaimOccupancyHeld 当前活跃预占数量（Gauge）。
	ReclaimOccupancyHeld = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_reclaim_occupancy_held",
		Help: "Current number of output keys held in ReclaimOccupancy",
	})
	// ReclaimLateCreditBlocked 迟贷记路径被阻塞计数（按 chain/asset）。
	// 告警：此计数 > 0 时运营须人工核查竞态事件。
	ReclaimLateCreditBlocked = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_reclaim_late_credit_blocked_total",
		Help: "Deposit credit attempts blocked due to active reclaim occupancy",
	}, []string{"chain", "asset"})
	// ReclaimRaceEventDropped 竞态事件队列溢出丢弃计数。
	// 告警：> 0 表示事件消费速度落后，需调查消费方。
	ReclaimRaceEventDropped = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_reclaim_race_event_dropped_total",
		Help: "LateCreditRaceEvent queue overflow drops; each > 0 needs investigation",
	})
	// ReclaimSweepLegsCreated Sweep leg 目录创建计数（按 chain/asset）。
	ReclaimSweepLegsCreated = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_reclaim_sweep_legs_created_total",
		Help: "Sweep leg catalogs created after successful reclaim",
	}, []string{"chain", "asset"})
	// ReclaimActiveCases 当前活跃（非终态）案件数量（Gauge）。
	ReclaimActiveCases = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_reclaim_active_cases",
		Help: "Current number of non-terminal reclaim cases in memory",
	})
)
