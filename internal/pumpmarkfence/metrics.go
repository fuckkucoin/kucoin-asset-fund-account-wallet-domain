package pumpmarkfence

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	metricIssue = promauto.NewCounter(prometheus.CounterOpts{
		Name: "outbox_pump_mark_fence_gen_issue_total",
		Help: "Deliver effect gens issued.",
	})
	metricDeliver = promauto.NewCounter(prometheus.CounterOpts{
		Name: "outbox_pump_mark_fence_deliver_total",
		Help: "Handler deliveries that advanced gen to delivered.",
	})
	metricMark = promauto.NewCounter(prometheus.CounterOpts{
		Name: "outbox_pump_mark_fence_mark_total",
		Help: "Marks that applied effect keys.",
	})
	metricResume = promauto.NewCounter(prometheus.CounterOpts{
		Name: "outbox_pump_mark_fence_resume_total",
		Help: "Half-mark resumes.",
	})
	metricHalfOpen = promauto.NewCounter(prometheus.CounterOpts{
		Name: "outbox_pump_mark_fence_half_open_total",
		Help: "Half-mark cases left open after publish.",
	})
	metricDup = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "outbox_pump_mark_fence_dup_proof_total",
		Help: "Outbox duplicate-effect proofs.",
	}, []string{"kind"})
	metricExpire = promauto.NewCounter(prometheus.CounterOpts{
		Name: "outbox_pump_mark_fence_expire_total",
		Help: "Gens expired by TTL.",
	})
	metricAbandon = promauto.NewCounter(prometheus.CounterOpts{
		Name: "outbox_pump_mark_fence_half_abandon_total",
		Help: "Half-mark cases abandoned.",
	})
	metricPoisonOK = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "outbox_pump_mark_fence_poison_ok_total",
		Help: "Poison replays allowed.",
	}, []string{"mode"})
	metricPoisonDeny = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "outbox_pump_mark_fence_poison_deny_total",
		Help: "Poison replays denied.",
	}, []string{"reason"})
	metricHandlerFail = promauto.NewCounter(prometheus.CounterOpts{
		Name: "outbox_pump_mark_fence_handler_fail_total",
		Help: "Handler failures before mark.",
	})
	metricExport = promauto.NewCounter(prometheus.CounterOpts{
		Name: "outbox_pump_mark_fence_export_total",
		Help: "Dup proofs exported.",
	})
)

  func recordIssue() { metricIssue.Inc() }

  func recordDeliver() { metricDeliver.Inc() }

  func recordMark()     { metricMark.Inc() }
   func recordResume()   { metricResume.Inc() }
  func recordHalfOpen() { metricHalfOpen.Inc() }

  func recordExpire()  { metricExpire.Inc() }
  func recordAbandon() { metricAbandon.Inc() }

  func recordHandlerFail() { metricHandlerFail.Inc() }

   func recordDup(kind string) {
	  metricDup.WithLabelValues(normLabel(kind)).Inc()
  }

  func recordPoisonOK(mode string) {
	   metricPoisonOK.WithLabelValues(normLabel(mode)).Inc()
  }

   func recordPoisonDeny(reason string) {
	   metricPoisonDeny.WithLabelValues(normLabel(reason)).Inc()
   }

  func recordExport(n int) {
	   if n > 0 {
		   metricExport.Add(float64(n))
	   }
  }

  func normLabel(s string) string {
	  if s == "" {
		   return "unknown"
	   }
	   if len(s) > 32 {
		   return s[:32]
	  }
	  return s
   }
