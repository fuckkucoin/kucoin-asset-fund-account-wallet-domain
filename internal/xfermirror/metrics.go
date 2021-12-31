package xfermirror

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// MirrorTransitionsTotal 镜像状态变更总数（按目标状态）。
	MirrorTransitionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_xfermirror_transitions_total",
		Help: "Mirror state transitions, by to_state",
	}, []string{"to_state"})
	// MirrorAlignConflicts 状态一致冲突次数。
	MirrorAlignConflicts = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_xfermirror_align_conflicts_total",
		Help: "Initiator vs mirror state alignment conflicts detected",
	})
	// PumpMaterializeTotal 泵物化次数（按结果）。
	PumpMaterializeTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_xfermirror_pump_materialize_total",
		Help: "Mirror pump materialization attempts, by result",
	}, []string{"result"})
	// ActiveMirrorGauge 当前 processing 状态镜像数。
	ActiveMirrorGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_xfermirror_processing_count",
		Help: "Current count of mirrors in processing state",
	})
	// ManualOverrideTotal 人工强制推进到 manual 的次数。
	ManualOverrideTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_xfermirror_manual_overrides_total",
		Help: "Count of manual overrides applied to mirror views",
	})
)
