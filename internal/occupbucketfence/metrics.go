package occupbucketfence

import "github.com/prometheus/client_golang/prometheus"

// Metrics 指标集合。

type Metrics struct {
	holdTotal    *prometheus.CounterVec
	releaseTotal *prometheus.CounterVec
	reclassTotal *prometheus.CounterVec
	breakTotal  prometheus.Counter
	halfResumed prometheus.Counter
	expiredTotal prometheus.Counter
	scanTotal *prometheus.CounterVec
	halfOpen  prometheus.Gauge
	breakOpen prometheus.Gauge
}

// NewMetrics 注册指标。
   func NewMetrics(reg prometheus.Registerer) *Metrics {
	  if reg == nil {
		  reg = prometheus.DefaultRegisterer
	   }
	  m := &Metrics{
		  holdTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			  Name: "occupbucketfence_hold_total",
			   Help: "HoldWithFence calls by result.",
		   }, []string{"result"}),
		  releaseTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			   Name: "occupbucketfence_release_total",
			   Help: "ReleaseAssert calls by result.",
		  }, []string{"result"}),
		   reclassTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			  Name: "occupbucketfence_reclass_total",
			   Help: "ReclassAssert calls by result.",
		  }, []string{"result"}),
		  breakTotal: prometheus.NewCounter(prometheus.CounterOpts{
			  Name: "occupbucketfence_break_proof_total",
			   Help: "OccupancyBreakProof saved.",
		  }),
		   halfResumed: prometheus.NewCounter(prometheus.CounterOpts{
			   Name: "occupbucketfence_half_resumed_total",
			  Help: "Half bucket cases resumed.",
		  }),
		   expiredTotal: prometheus.NewCounter(prometheus.CounterOpts{
			   Name: "occupbucketfence_fence_expired_total",
			  Help: "Fences expired by worker.",
		   }),
		  scanTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			  Name: "occupbucketfence_scan_total",
			  Help: "Worker scans by name and result.",
		   }, []string{"name", "result"}),
		  halfOpen: prometheus.NewGauge(prometheus.GaugeOpts{
			   Name: "occupbucketfence_half_open",
			   Help: "Open half bucket cases.",
		   }),
		   breakOpen: prometheus.NewGauge(prometheus.GaugeOpts{
			  Name: "occupbucketfence_break_open",
			  Help: "Open or newly proven break proofs from last patrol.",
		   }),
	   }
	  reg.MustRegister(m.holdTotal, m.releaseTotal, m.reclassTotal, m.breakTotal, m.halfResumed, m.expiredTotal, m.scanTotal, m.halfOpen, m.breakOpen)
	  return m
  }

   func (m *Metrics) RecordHold(ok bool) {
	  if m == nil {
		  return
	  }
	  if ok {
		   m.holdTotal.WithLabelValues("ok").Inc()
	   } else {
		   m.holdTotal.WithLabelValues("err").Inc()
	   }
   }

  func (m *Metrics) RecordRelease(result string) {
	   if m == nil {
		   return
	   }
	  m.releaseTotal.WithLabelValues(result).Inc()
   }

  func (m *Metrics) RecordReclass(result string) {
	  if m == nil {
		   return
	  }
	   m.reclassTotal.WithLabelValues(result).Inc()
   }

   func (m *Metrics) RecordBreak() {
	  if m == nil {
		  return
	  }
	   m.breakTotal.Inc()
   }

   func (m *Metrics) AddHalfResumed(n int) {
	  if m == nil {
		   return
	  }
	   m.halfResumed.Add(float64(n))
   }

  func (m *Metrics) AddExpired(n int) {
	  if m == nil {
		   return
	   }
	  m.expiredTotal.Add(float64(n))
  }

   func (m *Metrics) RecordScan(name string, ok bool) {
	  if m == nil {
		  return
	  }
	  if ok {
		  m.scanTotal.WithLabelValues(name, "ok").Inc()
	  } else {
		  m.scanTotal.WithLabelValues(name, "err").Inc()
	  }
  }

  func (m *Metrics) SetHalfOpen(n int) {
	   if m == nil {
		   return
	  }
	   m.halfOpen.Set(float64(n))
   }

   func (m *Metrics) SetBreakOpen(n int) {
	  if m == nil {
		  return
	  }
	  m.breakOpen.Set(float64(n))
  }
