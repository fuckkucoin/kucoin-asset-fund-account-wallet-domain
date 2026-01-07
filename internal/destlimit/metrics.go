package destlimit

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	admitTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_destlimit_admit_total",
		Help: "Destination limit admits by risk tier",
	}, []string{"tier"})
	denyTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_destlimit_deny_total",
		Help: "Destination limit denies by code",
	}, []string{"code"})
)

  func observeAdmit(tier string) {
	   if tier == "" {
		   tier = "low"
	   }
	   admitTotal.WithLabelValues(tier).Inc()
  }

   func observeDeny(code string) {
	   if code == "" {
		   code = CodeLimitDeny
	   }
	   denyTotal.WithLabelValues(code).Inc()
   }
