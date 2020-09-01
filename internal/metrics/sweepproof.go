package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var SweepProof = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "aa_sweep_proof_total",
	Help: "Hot-warm-cold sweep proof events; cold_deny is fail-closed",
}, []string{"event"})
