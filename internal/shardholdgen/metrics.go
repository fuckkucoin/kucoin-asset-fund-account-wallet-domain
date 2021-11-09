package shardholdgen

import "github.com/prometheus/client_golang/prometheus"

// Metrics Prometheus 计数。
type Metrics struct {
	issued prometheus.Counter
	held       prometheus.Counter
	released   prometheus.Counter
	expired    prometheus.Counter
	broadcast  prometheus.Counter
	conflict   prometheus.Counter
	doubleHold prometheus.Counter
	missRel    prometheus.Counter
	liveHash   prometheus.Counter
	failClosed prometheus.Counter
	proofs     *prometheus.CounterVec
	half       *prometheus.CounterVec
	patrol prometheus.Counter
	export prometheus.Counter
	resume prometheus.Counter
}

   func NewMetrics(reg prometheus.Registerer) *Metrics {
	  if reg == nil {
		   reg = prometheus.NewRegistry()
	   }
	   m := &Metrics{
		   issued: prometheus.NewCounter(prometheus.CounterOpts{
			  Name: "shardholdgen_issued_total",
			  Help: "leg_hold_gen issued",
		  }),
		   held: prometheus.NewCounter(prometheus.CounterOpts{
			  Name: "shardholdgen_held_total",
			   Help: "generations moved to held",
		   }),
		  released: prometheus.NewCounter(prometheus.CounterOpts{
			  Name: "shardholdgen_released_total",
			   Help: "generations released",
		   }),
		  expired: prometheus.NewCounter(prometheus.CounterOpts{
			   Name: "shardholdgen_expired_total",
			   Help: "generations expired by patrol",
		  }),
		  broadcast: prometheus.NewCounter(prometheus.CounterOpts{
			   Name: "shardholdgen_broadcast_total",
			  Help: "broadcast submits bound to generation",
		   }),
		  conflict: prometheus.NewCounter(prometheus.CounterOpts{
			   Name: "shardholdgen_conflict_total",
			   Help: "advance vs release mutex conflicts",
		  }),
		  doubleHold: prometheus.NewCounter(prometheus.CounterOpts{
			   Name: "shardholdgen_double_hold_total",
			   Help: "double-hold proofs",
		  }),
		   missRel: prometheus.NewCounter(prometheus.CounterOpts{
			  Name: "shardholdgen_miss_release_total",
			  Help: "miss-release proofs",
		   }),
		   liveHash: prometheus.NewCounter(prometheus.CounterOpts{
			  Name: "shardholdgen_live_hash_guard_total",
			  Help: "broadcasting release rejects",
		  }),
		   failClosed: prometheus.NewCounter(prometheus.CounterOpts{
			   Name: "shardholdgen_fail_closed_total",
			  Help: "operations rejected for missing generation",
		   }),
		   proofs: prometheus.NewCounterVec(prometheus.CounterOpts{
			   Name: "shardholdgen_proofs_total",
			   Help: "ShardHoldProof opened",
		   }, []string{"kind"}),
		   half: prometheus.NewCounterVec(prometheus.CounterOpts{
			   Name: "shardholdgen_half_cases_total",
			   Help: "HalfLegCase opened",
		  }, []string{"kind"}),
		   patrol: prometheus.NewCounter(prometheus.CounterOpts{
			  Name: "shardholdgen_patrol_total",
			   Help: "half-leg patrol ticks",
		  }),
		   export: prometheus.NewCounter(prometheus.CounterOpts{
			  Name: "shardholdgen_export_total",
			  Help: "proofs exported",
		  }),
		   resume: prometheus.NewCounter(prometheus.CounterOpts{
			  Name: "shardholdgen_resume_total",
			  Help: "half-leg resumes",
		  }),
	  }
	   reg.MustRegister(m.issued, m.held, m.released, m.expired, m.broadcast, m.conflict,
		   m.doubleHold, m.missRel, m.liveHash, m.failClosed, m.proofs, m.half, m.patrol, m.export, m.resume)
	  return m
  }

  func (m *Metrics) IncIssued() {
	   if m != nil {
		   m.issued.Inc()
	  }
  }
   func (m *Metrics) IncHeld() {
	   if m != nil {
		  m.held.Inc()
	   }
   }

  func (m *Metrics) IncReleased() {
	  if m != nil {
		   m.released.Inc()
	   }
   }

  func (m *Metrics) IncExpired() {
	   if m != nil {
		   m.expired.Inc()
	  }
  }
   func (m *Metrics) IncBroadcast() {
	   if m != nil {
		   m.broadcast.Inc()
	   }
   }
  func (m *Metrics) IncConflict() {
	   if m != nil {
		   m.conflict.Inc()
	   }
  }

  func (m *Metrics) IncDoubleHold() {
	  if m != nil {
		  m.doubleHold.Inc()
	  }
  }
   func (m *Metrics) IncMissRelease() {
	   if m != nil {
		   m.missRel.Inc()
	  }
   }
   func (m *Metrics) IncLiveHash() {
	  if m != nil {
		   m.liveHash.Inc()
	   }
  }

   func (m *Metrics) IncFailClosed() {
	   if m != nil {
		   m.failClosed.Inc()
	   }
  }
  func (m *Metrics) IncProof(kind string) {
	   if m != nil {
		   m.proofs.WithLabelValues(kind).Inc()
	   }
   }
  func (m *Metrics) IncHalf(kind string) {
	   if m != nil {
		   m.half.WithLabelValues(kind).Inc()
	  }
  }
  func (m *Metrics) IncPatrol() {
	   if m != nil {
		  m.patrol.Inc()
	   }
   }
  func (m *Metrics) AddExport(n int) {
	  if m != nil && n > 0 {
		   m.export.Add(float64(n))
	  }
  }
  func (m *Metrics) IncResume() {
	  if m != nil {
		  m.resume.Inc()
	  }
  }
