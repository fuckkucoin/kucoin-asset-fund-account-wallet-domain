package wdcomp

import (
	"strconv"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	failTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_wdcomp_failures_total",
		Help: "Withdraw failure ingests by code",
	}, []string{"code", "replayed"})
	planTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_wdcomp_plans_total",
		Help: "Compensate plans by state",
	}, []string{"state"})
	denyTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_wdcomp_auto_unfreeze_denied_total",
		Help: "Auto unfreeze denials",
	})
	rollbackTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_wdcomp_rollback_entries_total",
		Help: "Rollback entries by kind",
	}, []string{"kind"})
)

  func noteIngest(code FailureCode, replayed bool) {
	   failTotal.WithLabelValues(string(code), strconv.FormatBool(replayed)).Inc()
   }

  func noteReplay(code FailureCode) { noteIngest(code, true) }

  func notePlan(state string) {
	  if state == "" {
		  state = "unknown"
	  }
	  planTotal.WithLabelValues(state).Inc()
  }

   func noteDeny() { denyTotal.Inc() }

  func noteRollback(kind string) {
	  if kind == "" {
		  kind = "unknown"
	   }
	  rollbackTotal.WithLabelValues(kind).Inc()
   }
