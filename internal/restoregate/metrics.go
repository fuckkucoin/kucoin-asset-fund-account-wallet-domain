package restoregate

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics 灾备回灌世代闸 Prometheus 指标。
var Metrics = struct {
	PlansCreatedTotal prometheus.Counter
	PlansAbortedTotal prometheus.Counter
	PlansSealedTotal  prometheus.Counter
	GenConflictTotal prometheus.Counter
	GenLagTotal          prometheus.Counter
	WriteFrozenTotal     prometheus.Counter
	HalfSealTotal        prometheus.Counter
	SplitBrainOpenTotal  prometheus.Counter
	SplitBrainClearTotal prometheus.Counter
	ActiveMutex          prometheus.Gauge
	OpenProofs           prometheus.Gauge
}{
	PlansCreatedTotal: promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "restoregate",
		Name: "plans_created_total",
		Help: "灾备回灌计划创建总数。",
	}),
	PlansAbortedTotal: promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "restoregate",
		Name: "plans_aborted_total",
		Help: "灾备回灌计划中止总数。",
	}),
	PlansSealedTotal: promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "restoregate",
		Name:      "plans_sealed_total",
		Help: "灾备回灌计划成功封印总数。",
	}),
	GenConflictTotal: promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "restoregate",
		Name: "gen_conflict_total",
		Help: "世代冲突（活账水位 > 计划目标）总次数。",
	}),
	GenLagTotal: promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "restoregate",
		Name: "gen_lag_total",
		Help: "世代落后（计划目标 < 系统世代）总次数。",
	}),
	WriteFrozenTotal: promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "restoregate",
		Name:      "write_frozen_total",
		Help:      "写路径被冻结拒绝的意图总数。",
	}),
	HalfSealTotal: promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "restoregate",
		Name:      "half_seal_total",
		Help: "半恢复封印创建总数。",
	}),
	SplitBrainOpenTotal: promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "restoregate",
		Name:      "splitbrain_open_total",
		Help: "脑裂证明打开总数。",
	}),
	SplitBrainClearTotal: promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "restoregate",
		Name:      "splitbrain_clear_total",
		Help:      "脑裂证明四眼解除总数。",
	}),
	ActiveMutex: promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "restoregate",
		Name:      "active_mutex",
		Help:      "当前活跃写互斥锁数量（正常最多 1）。",
	}),
	OpenProofs: promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "restoregate",
		Name:      "open_proofs",
		Help:      "当前 open/clearing 脑裂证明数量。",
	}),
}
