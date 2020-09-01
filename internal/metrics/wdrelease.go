package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	WDReleaseAge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_wd_release_age_seconds",
		Help: "Max age of withdraw release tickets by chain state",
	}, []string{"state"})
	WDReleaseResidual = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_wd_release_residual",
		Help: "Authorized amount not yet frozen, summed by asset",
	}, []string{"asset"})
	WDReleasePrivilege = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_wd_release_privilege_total",
		Help: "Rejected privilege attempts on withdraw release",
	}, []string{"reason"})
	WDReleaseDrift = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_wd_release_policy_drift",
		Help: "Open withdraw release tickets whose policy version differs from the active table",
	})
	WDReleaseEvents = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_wd_release_events_total",
		Help: "Withdraw release chain events",
	}, []string{"event"})
)
