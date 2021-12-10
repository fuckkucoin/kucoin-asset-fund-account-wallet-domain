package svcreg

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var registryOps = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "aa_registry_ops_total",
	Help: "Service registry operations by op and result",
}, []string{"op", "result"})

  func observeRegistry(op, result string) {
	  if op == "" {
		   op = "unknown"
	  }
	   if result == "" {
		   result = "unknown"
	  }
	   registryOps.WithLabelValues(op, result).Inc()
   }
