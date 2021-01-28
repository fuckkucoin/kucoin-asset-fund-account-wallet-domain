package subfreezecas

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	metricCASOpen = promauto.NewCounter(prometheus.CounterOpts{
		Name: "subacct_freeze_cas_open_total",
		Help: "Freeze CAS documents opened.",
	})
	metricFreeze = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "subacct_freeze_cas_freeze_total",
		Help: "Freeze operations that advanced CAS.",
	}, []string{"scope"})
	metricThaw = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "subacct_freeze_cas_thaw_total",
		Help: "Thaw operations that advanced CAS.",
	}, []string{"scope"})
	metricDebitOK = promauto.NewCounter(prometheus.CounterOpts{
		Name: "subacct_freeze_cas_debit_ok_total",
		Help: "Debits that passed CAS assert.",
	})
	metricDebitDenied = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "subacct_freeze_cas_debit_denied_total",
		Help: "Debits blocked by CAS assert.",
	}, []string{"reason"})
	metricPunch = promauto.NewCounter(prometheus.CounterOpts{
		Name: "subacct_freeze_cas_punch_total",
		Help: "Frozen spend punches recorded.",
	})
	metricCompensate = promauto.NewCounter(prometheus.CounterOpts{
		Name: "subacct_freeze_cas_compensate_total",
		Help: "Punch compensations posted.",
	})
	metricAck = promauto.NewCounter(prometheus.CounterOpts{
		Name: "subacct_freeze_cas_proof_ack_total",
		Help: "Spend proofs acked by ops.",
	})
	metricRevoke = promauto.NewCounter(prometheus.CounterOpts{
		Name: "subacct_freeze_cas_vel_revoke_total",
		Help: "Velocity window revocations.",
	})
	metricUnfreezeDenied = promauto.NewCounter(prometheus.CounterOpts{
		Name: "subacct_freeze_cas_unfreeze_child_denied_total",
		Help: "UnfreezeChild refused while parent frozen.",
	})
	metricExport = promauto.NewCounter(prometheus.CounterOpts{
		Name: "subacct_freeze_cas_export_total",
		Help: "Proof export pump rows.",
	})
)

  func recordCASOpen()            { metricCASOpen.Inc() }
  func recordFreeze(scope string) { metricFreeze.WithLabelValues(normScope(scope)).Inc() }

   func recordThaw(scope string) { metricThaw.WithLabelValues(normScope(scope)).Inc() }
   func recordDebitOK()          { metricDebitOK.Inc() }

  func recordDebitDenied(reason string) {
	  metricDebitDenied.WithLabelValues(normScope(reason)).Inc()
  }
  func recordPunch()      { metricPunch.Inc() }
  func recordCompensate() { metricCompensate.Inc() }

  func recordAck() { metricAck.Inc() }

  func recordRevoke() { metricRevoke.Inc() }

   func recordUnfreezeDenied() { metricUnfreezeDenied.Inc() }
  func recordExport(n int) {
	   if n > 0 {
		   metricExport.Add(float64(n))
	  }
   }

   func normScope(s string) string {
	   if s == "" {
		   return "unknown"
	   }
	   if len(s) > 24 {
		  return s[:24]
	  }
	   return s
   }
