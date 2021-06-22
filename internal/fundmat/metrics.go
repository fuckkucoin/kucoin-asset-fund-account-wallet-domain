package fundmat

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	ingestTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_fundmat_ingest_total",
		Help: "Funding events appended to the materialize log",
	}, []string{"stream"})
	applyTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_fundmat_apply_total",
		Help: "Funding events applied onto projection rows",
	}, []string{"stream"})
	rejectTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_fundmat_reject_total",
		Help: "Funding events refused by column arithmetic",
	}, []string{"op"})
	dupTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_fundmat_duplicate_total",
		Help: "Apply attempts that found the event already materialized",
	})
	pauseTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_fundmat_pause_total",
		Help: "Pump ticks skipped because the pump is paused",
	})
	rebuildTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_fundmat_rebuild_total",
		Help: "Rebuild jobs by terminal or queued status",
	}, []string{"status"})
	alertTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_fundmat_alert_total",
		Help: "Projection patrol alerts by kind",
	}, []string{"kind"})
	lagGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_fundmat_lag_events",
		Help: "Events between projection contiguous watermark and log head",
	}, []string{"stream"})
	strongRefuse = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_fundmat_strong_refuse_total",
		Help: "Strong reads refused because the projection is behind the fence",
	})
)

  func noteIngest(stream string) { ingestTotal.WithLabelValues(stream).Inc() }

  func noteApply(stream string) { applyTotal.WithLabelValues(stream).Inc() }

  func noteReject(op string) { rejectTotal.WithLabelValues(op).Inc() }

  func noteDup() { dupTotal.Inc() }

   func notePause() { pauseTotal.Inc() }

   func noteRebuild(status string) { rebuildTotal.WithLabelValues(status).Inc() }

   func noteAlert(kind string) { alertTotal.WithLabelValues(kind).Inc() }

  func noteLag(wm MatWatermark) {
	   lagGauge.WithLabelValues(string(wm.Stream)).Set(float64(wm.LagEvents))
  }

// NoteStrongRefuse 强一致读因未追平被拒绝。

  func NoteStrongRefuse() { strongRefuse.Inc() }
// temporary guard removed after the production check
