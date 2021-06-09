package fundintgate

import "github.com/prometheus/client_golang/prometheus"

// Metrics 领域指标。
type Metrics struct {
	LeaseIssued   prometheus.Counter
	SendBoundOK   prometheus.Counter
	LeaseExpired  prometheus.Counter
	LeaseRevoked  prometheus.Counter
	PatrolRevoked prometheus.Counter
	GenBump     prometheus.Counter
	DriftProofs *prometheus.CounterVec
}

// NewMetrics 注册指标；registry 为 nil 时使用 DefaultRegisterer。

  func NewMetrics(registry prometheus.Registerer) *Metrics {
	  if registry == nil {
		   registry = prometheus.DefaultRegisterer
	  }
	   m := &Metrics{
		  LeaseIssued: prometheus.NewCounter(prometheus.CounterOpts{
			  Name: "fundintgate_lease_issued_total",
			  Help: "CheckWithLease 签发租约次数。",
		   }),
		   SendBoundOK: prometheus.NewCounter(prometheus.CounterOpts{
			   Name: "fundintgate_send_bound_ok_total",
			  Help: "SendAssert 绑定并 SpotSend 成功次数。",
		   }),
		   LeaseExpired: prometheus.NewCounter(prometheus.CounterOpts{
			   Name: "fundintgate_lease_expired_total",
			   Help: "过期扫描归档租约次数。",
		   }),
		  LeaseRevoked: prometheus.NewCounter(prometheus.CounterOpts{
			   Name: "fundintgate_lease_revoked_total",
			   Help: "Halt/换闸/禁提/运维吊销租约次数。",
		  }),
		   PatrolRevoked: prometheus.NewCounter(prometheus.CounterOpts{
			  Name: "fundintgate_patrol_revoked_total",
			   Help: "未消费巡检吊销次数。",
		  }),
		   GenBump: prometheus.NewCounter(prometheus.CounterOpts{
			  Name: "fundintgate_generation_bump_total",
			   Help: "闸世代抬升次数。",
		   }),
		  DriftProofs: prometheus.NewCounterVec(prometheus.CounterOpts{
			  Name: "fundintgate_drift_proofs_total",
			  Help: "漂移证明条数，按 kind。",
		   }, []string{"kind"}),
	   }
	   for _, c := range []prometheus.Collector{
		  m.LeaseIssued, m.SendBoundOK, m.LeaseExpired, m.LeaseRevoked,
		  m.PatrolRevoked, m.GenBump, m.DriftProofs,
	   } {
		   _ = registry.Register(c)
	  }
	   return m
   }
