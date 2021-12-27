package svcdial

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var dialOps = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "aa_svcdial_ops_total",
	Help: "Inter-service dial attempts by target service and result",
}, []string{"service", "result"})

  func observeDial(service, result string) {
	  if service == "" {
		  service = "unknown"
	  }
	  if result == "" {
		   result = "unknown"
	   }
	   dialOps.WithLabelValues(service,
		   result).Inc()
   }
