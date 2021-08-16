package ledshard

import "github.com/prometheus/client_golang/prometheus"

import "github.com/prometheus/client_golang/prometheus/promauto"

var (
	metricIntent = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_ledshard_intent_total",
		Help: "Cross-shard intents observed by resulting state",
	}, []string{"state"})
	metricCompensate = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_ledshard_compensate_total",
		Help: "Cross-shard compensations that released a freeze or voided a voucher",
	})
	metricDrift = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_ledshard_drift_total",
		Help: "Pinned route drift detections that refused a shard rewrite",
	})
	metricVoucher = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_ledshard_voucher_total",
		Help: "Bridge vouchers by state",
	}, []string{"state"})
	metricHint = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_ledshard_hint_total",
		Help: "Edge hints emitted for a later causal ingest; this package stores no graph",
	}, []string{"kind"})
	metricDouble = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_ledshard_double_credit_blocked_total",
		Help: "Credit attempts rejected because the voucher was already consumed",
	})
)

   func metricsState(st IntentState) { metricIntent.WithLabelValues(string(st)).Inc() }
  func metricsHint(kind string)     { metricHint.WithLabelValues(kind).Inc() }
  func metricsVoucher(st VoucherState) {
	   metricVoucher.WithLabelValues(string(st)).Inc()
  }
