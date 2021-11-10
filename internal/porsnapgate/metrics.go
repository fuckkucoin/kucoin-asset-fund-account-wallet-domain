package porsnapgate

import "github.com/prometheus/client_golang/prometheus"

// Metrics 收集 porsnapgate 领域指标。

type Metrics struct {
	BuildWithGenTotal  prometheus.Counter
	AttestTotal        prometheus.Counter
	PublishAttestTotal prometheus.Counter
	RevokeAttestTotal  prometheus.Counter
	DriftProofsTotal   *prometheus.CounterVec
	ProveAttestedTotal prometheus.Counter
	ProveAttestedFail prometheus.Counter
	GenExpiredTotal   prometheus.Counter
	ReVerifyFailTotal prometheus.Counter
}

// NewMetrics 注册并返回 Metrics；registry 为 nil 时使用 DefaultRegisterer。

   func NewMetrics(registry prometheus.Registerer) *Metrics {
	   if registry == nil {
		  registry = prometheus.DefaultRegisterer
	  }
	   m := &Metrics{
		  BuildWithGenTotal: prometheus.NewCounter(prometheus.CounterOpts{
			  Name: "porsnapgate_build_with_gen_total",
			   Help: "Total number of BuildWithGen calls (snap_gen sealed).",
		  }),
		  AttestTotal: prometheus.NewCounter(prometheus.CounterOpts{
			   Name: "porsnapgate_attest_total",
			   Help: "Total number of Attest calls (attestation created).",
		   }),
		   PublishAttestTotal: prometheus.NewCounter(prometheus.CounterOpts{
			  Name: "porsnapgate_publish_attest_total",
			   Help: "Total number of PublishAttest calls.",
		  }),
		   RevokeAttestTotal: prometheus.NewCounter(prometheus.CounterOpts{
			  Name: "porsnapgate_revoke_attest_total",
			   Help: "Total number of RevokeAttest calls.",
		  }),
		  DriftProofsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			  Name: "porsnapgate_drift_proofs_total",
			  Help: "Total number of drift proofs created, by kind.",
		   }, []string{"kind"}),
		   ProveAttestedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			  Name: "porsnapgate_prove_attested_total",
			   Help: "Total number of ProveAttested calls.",
		  }),
		   ProveAttestedFail: prometheus.NewCounter(prometheus.CounterOpts{
			   Name: "porsnapgate_prove_attested_fail_total",
			  Help: "Total number of ProveAttested failures (root mismatch).",
		  }),
		   GenExpiredTotal: prometheus.NewCounter(prometheus.CounterOpts{
			  Name: "porsnapgate_gen_expired_total",
			   Help: "Total number of snap_gens expired by archive worker.",
		  }),
		   ReVerifyFailTotal: prometheus.NewCounter(prometheus.CounterOpts{
			   Name: "porsnapgate_reverify_fail_total",
			  Help: "Total number of attestation re-verify failures.",
		   }),
	   }
	  for _, c := range []prometheus.Collector{
		  m.BuildWithGenTotal, m.AttestTotal, m.PublishAttestTotal, m.RevokeAttestTotal,
		  m.DriftProofsTotal,
		   m.ProveAttestedTotal, m.ProveAttestedFail,
		  m.GenExpiredTotal, m.ReVerifyFailTotal,
	   } {
		   _ = registry.Register(c)
	   }
	   return m
   }
