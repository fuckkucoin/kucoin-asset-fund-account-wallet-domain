package solvency

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	attestTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_solvency_attestations_total",
		Help: "Solvency attestations by resulting status",
	}, []string{"status"})
	driftTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_solvency_drifts_total",
		Help: "Opened solvency drift cases by severity",
	}, []string{"severity"})
	coverageTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_solvency_coverage_total",
		Help: "Reserve comparison outcomes",
	}, []string{"coverage"})
	patrolTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_solvency_patrol_findings_total",
		Help: "Integrity patrol findings",
	}, []string{"kind"})
	publishRefuse = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_solvency_publish_refused_total",
		Help: "Publish attempts refused before a root becomes published",
	}, []string{"reason"})
)

  func noteCollect(status Status) { attestTotal.WithLabelValues(string(status)).Inc() }

   func noteDrift(sev string) {
	  if sev == "" {
		   sev = "unknown"
	  }
	   driftTotal.WithLabelValues(sev).Inc()
   }

  func noteCoverage(c string) {
	   if c == "" {
		  c = "unknown"
	  }
	   coverageTotal.WithLabelValues(c).Inc()
   }

   func notePatrol(kind string) { patrolTotal.WithLabelValues(kind).Inc() }

   func noteRefuse(reason string) { publishRefuse.WithLabelValues(reason).Inc() }
