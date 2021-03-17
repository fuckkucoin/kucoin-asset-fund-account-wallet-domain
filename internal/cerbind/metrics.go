package cerbind

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	transitions = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_cerbind_transition_total",
		Help: "Cold-sign ceremony transitions by target state",
	}, []string{"to"})
	gateTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_cerbind_gate_total",
		Help: "Sign-broadcast binding checks by result",
	}, []string{"result"})
	stuckGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_cerbind_stuck_sessions",
		Help: "Ceremony sessions alerted as stuck on the last patrol",
	})
	integrityTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_cerbind_integrity_total",
		Help: "Binding integrity patrol actions",
	}, []string{"kind"})
	timeoutTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_cerbind_timeout_total",
		Help: "Ceremony sessions aborted by deadline",
	})
)

   func countTo(st State) {
	  transitions.WithLabelValues(string(st)).Inc()
   }

  func countGate(result string) {
	  gateTotal.WithLabelValues(result).Inc()
   }
