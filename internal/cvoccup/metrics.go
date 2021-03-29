package cvoccup

import (
	"sync"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	metricsOnce sync.Once
	occCreatedTotal *prometheus.CounterVec
	occSettledTotal    *prometheus.CounterVec
	occRolledBackTotal *prometheus.CounterVec
	breachOpenedTotal *prometheus.CounterVec
)

   func initMetrics() {
	   metricsOnce.Do(func() {
		   occCreatedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
			   Namespace: "aa",
			   Subsystem: "cvoccup",
			   Name: "occ_created_total",
			  Help: "路径占用创建计数（按路径类型与资产分组）",
		  }, []string{"path_kind", "asset"})
		   occSettledTotal = promauto.NewCounterVec(prometheus.CounterOpts{
			   Namespace: "aa",
			  Subsystem: "cvoccup",
			   Name: "occ_settled_total",
			   Help: "路径占用结算计数",
		   }, []string{"path_kind", "asset"})
		  occRolledBackTotal = promauto.NewCounterVec(prometheus.CounterOpts{
			  Namespace: "aa",
			   Subsystem: "cvoccup",
			   Name:      "occ_rolled_back_total",
			  Help: "路径占用回滚计数",
		   }, []string{"path_kind", "asset"})
		  breachOpenedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
			  Namespace: "aa",
			   Subsystem: "cvoccup",
			  Name:      "breach_opened_total",
			  Help: "穿仓事件开单计数（按类型与资产）",
		   }, []string{"breach_kind", "asset"})
	  })
   }

// PrometheusMetrics Prometheus 实现的指标记录器。
type PrometheusMetrics struct{}

// NewPrometheusMetrics 构造并注册指标。

   func NewPrometheusMetrics() *PrometheusMetrics {
	  initMetrics()
	   return &PrometheusMetrics{}
  }

  func (p *PrometheusMetrics) IncOccCreated(pathKind, asset string) {
	  if occCreatedTotal != nil {
		   occCreatedTotal.WithLabelValues(pathKind, asset).Inc()
	  }
   }

  func (p *PrometheusMetrics) IncOccSettled(pathKind, asset string) {
	   if occSettledTotal != nil {
		  occSettledTotal.WithLabelValues(pathKind, asset).Inc()
	   }
  }

   func (p *PrometheusMetrics) IncOccRolledBack(pathKind, asset string) {
	   if occRolledBackTotal != nil {
		  occRolledBackTotal.WithLabelValues(pathKind, asset).Inc()
	  }
  }

   func (p *PrometheusMetrics) IncBreachOpened(breachKind, asset string) {
	   if breachOpenedTotal != nil {
		   breachOpenedTotal.WithLabelValues(breachKind, asset).Inc()
	   }
  }
