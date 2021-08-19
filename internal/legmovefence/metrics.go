package legmovefence

import "github.com/prometheus/client_golang/prometheus"

// Metrics Prometheus 计数器。

type Metrics struct {
	issued   prometheus.Counter
	enqueued prometheus.Counter
	moved    prometheus.Counter
	expired  prometheus.Counter
	revoked *prometheus.CounterVec
	proofs  *prometheus.CounterVec
	half *prometheus.CounterVec
	patrol prometheus.Counter
	export prometheus.Counter
}

  func NewMetrics(reg prometheus.Registerer) *Metrics {
	  if reg == nil {
		   reg = prometheus.DefaultRegisterer
	   }
	  m := &Metrics{
		  issued: prometheus.NewCounter(prometheus.CounterOpts{
			   Name: "legmovefence_issued_total",
			  Help: "leg_move_fence issued",
		  }),
		  enqueued: prometheus.NewCounter(prometheus.CounterOpts{
			  Name: "legmovefence_enqueued_total",
			   Help: "fences marked enqueued after collect intent",
		   }),
		   moved: prometheus.NewCounter(prometheus.CounterOpts{
			   Name: "legmovefence_moved_total",
			  Help: "successful inventory transfers bound to fence",
		   }),
		  expired: prometheus.NewCounter(prometheus.CounterOpts{
			  Name: "legmovefence_expired_total",
			  Help: "fences expired by TTL worker",
		  }),
		  revoked: prometheus.NewCounterVec(prometheus.CounterOpts{
			   Name: "legmovefence_revoked_total",
			  Help: "fences revoked",
		   }, []string{"reason"}),
		  proofs: prometheus.NewCounterVec(prometheus.CounterOpts{
			  Name: "legmovefence_proofs_total",
			   Help: "GeoDoubleMoveProof opened",
		   }, []string{"kind"}),
		  half: prometheus.NewCounterVec(prometheus.CounterOpts{
			   Name: "legmovefence_half_cases_total",
			  Help: "HalfMoveCase opened",
		   }, []string{"kind"}),
		   patrol: prometheus.NewCounter(prometheus.CounterOpts{
			  Name: "legmovefence_patrol_resumed_total",
			   Help: "half-move cases resumed by patrol",
		   }),
		   export: prometheus.NewCounter(prometheus.CounterOpts{
			   Name: "legmovefence_proofs_exported_total",
			   Help: "proofs marked exported",
		   }),
	  }
	  _ = reg.Register(m.issued)
	   _ = reg.Register(m.enqueued)
	   _ = reg.Register(m.moved)
	   _ = reg.Register(m.expired)
	  _ = reg.Register(m.revoked)
	   _ = reg.Register(m.proofs)
	   _ = reg.Register(m.half)
	  _ = reg.Register(m.patrol)
	  _ = reg.Register(m.export)
	   return m
  }

  func (m *Metrics) IncIssued() {
	  if m != nil && m.issued != nil {
		   m.issued.Inc()
	  }
  }
   func (m *Metrics) IncEnqueued() {
	   if m != nil && m.enqueued != nil {
		   m.enqueued.Inc()
	  }
  }

  func (m *Metrics) IncMoved() {
	   if m != nil && m.moved != nil {
		  m.moved.Inc()
	   }
  }

   func (m *Metrics) IncExpired() {
	  if m != nil && m.expired != nil {
		   m.expired.Inc()
	   }
  }

  func (m *Metrics) IncRevoked(reason string) {
	   if m != nil && m.revoked != nil {
		   m.revoked.WithLabelValues(reason).Inc()
	  }
  }

   func (m *Metrics) IncProof(kind string) {
	  if m != nil && m.proofs != nil {
		   m.proofs.WithLabelValues(kind).Inc()
	   }
  }
   func (m *Metrics) IncHalf(kind string) {
	  if m != nil && m.half != nil {
		   m.half.WithLabelValues(kind).Inc()
	   }
   }
  func (m *Metrics) IncPatrol() {
	  if m != nil && m.patrol != nil {
		   m.patrol.Inc()
	   }
   }

   func (m *Metrics) IncExport(n int) {
	   if m != nil && m.export != nil {
		   m.export.Add(float64(n))
	  }
  }
