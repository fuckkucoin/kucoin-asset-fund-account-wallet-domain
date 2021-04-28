package destentrygate

import "github.com/prometheus/client_golang/prometheus"

// Metrics Prometheus 计数器组。

type Metrics struct {
	issued   *prometheus.CounterVec
	bound    prometheus.Counter
	consumed prometheus.Counter
	expired prometheus.Counter
	revoked *prometheus.CounterVec
	proofs  *prometheus.CounterVec
	genBump *prometheus.CounterVec
	patrol prometheus.Counter
}

// NewMetrics 注册到指定 Registerer；reg 为空则用默认注册表。

   func NewMetrics(reg prometheus.Registerer) *Metrics {
	  if reg == nil {
		  reg = prometheus.DefaultRegisterer
	   }
	   m := &Metrics{
		  issued: prometheus.NewCounterVec(prometheus.CounterOpts{
			  Name: "destentrygate_lease_issued_total",
			   Help: "dest_entry_lease issued after destallow precheck",
		   }, []string{"mode"}),
		   bound: prometheus.NewCounter(prometheus.CounterOpts{
			  Name: "destentrygate_lease_bound_total",
			   Help: "leases bound at withdraw assert",
		   }),
		   consumed: prometheus.NewCounter(prometheus.CounterOpts{
			   Name: "destentrygate_lease_consumed_total",
			  Help: "leases consumed after withdraw submit",
		   }),
		   expired: prometheus.NewCounter(prometheus.CounterOpts{
			  Name: "destentrygate_lease_expired_total",
			  Help: "leases archived by TTL expire worker",
		   }),
		   revoked: prometheus.NewCounterVec(prometheus.CounterOpts{
			   Name: "destentrygate_lease_revoked_total",
			  Help: "leases revoked by lifecycle events",
		   }, []string{"reason"}),
		   proofs: prometheus.NewCounterVec(prometheus.CounterOpts{
			  Name: "destentrygate_drift_proofs_total",
			   Help: "DestEntryDriftProof opened",
		  }, []string{"kind"}),
		   genBump: prometheus.NewCounterVec(prometheus.CounterOpts{
			  Name: "destentrygate_entry_gen_bump_total",
			  Help: "entry_gen bumps",
		   }, []string{"reason"}),
		  patrol: prometheus.NewCounter(prometheus.CounterOpts{
			  Name: "destentrygate_patrol_revoked_total",
			  Help: "unconsumed leases revoked by patrol",
		  }),
	  }
	  _ = reg.Register(m.issued)
	   _ = reg.Register(m.bound)
	  _ = reg.Register(m.consumed)
	  _ = reg.Register(m.expired)
	  _ = reg.Register(m.revoked)
	  _ = reg.Register(m.proofs)
	  _ = reg.Register(m.genBump)
	  _ = reg.Register(m.patrol)
	  return m
  }

  func (m *Metrics) IncIssued(mode string) {
	   if m != nil && m.issued != nil {
		  m.issued.WithLabelValues(mode).Inc()
	   }
   }

   func (m *Metrics) IncBound() {
	   if m != nil && m.bound != nil {
		  m.bound.Inc()
	   }
  }

   func (m *Metrics) IncConsumed() {
	   if m != nil && m.consumed != nil {
		  m.consumed.Inc()
	  }
   }

  func (m *Metrics) IncExpired(n int) {
	  if m != nil && m.expired != nil && n > 0 {
		  m.expired.Add(float64(n))
	   }
  }

  func (m *Metrics) IncRevoked(reason string, n int) {
	   if m != nil && m.revoked != nil && n > 0 {
		  m.revoked.WithLabelValues(reason).Add(float64(n))
	   }
  }

   func (m *Metrics) IncProof(kind string) {
	   if m != nil && m.proofs != nil {
		   m.proofs.WithLabelValues(kind).Inc()
	   }
  }

  func (m *Metrics) IncGenBump(reason string) {
	   if m != nil && m.genBump != nil {
		   m.genBump.WithLabelValues(reason).Inc()
	  }
  }

  func (m *Metrics) IncPatrol(n int) {
	   if m != nil && m.patrol != nil && n > 0 {
		  m.patrol.Add(float64(n))
	  }
  }
