package p2pabandon

import (
	"sync"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// 指标名称常量（供 worker.go / http.go 使用）。
const (
	MetricPlanArmed     = "plan_armed"
	MetricPlanReleased  = "plan_released"
	MetricPlanFilled    = "plan_filled"
	MetricConflict      = "conflict"
	MetricHoldEscalated = "hold_escalated"
	MetricProofTamper = "proof_tamper"
	MetricWorkerError = "worker_error"
	MetricAPIRequest  = "api_request"
	MetricAPIError = "api_error"
)

// Metrics 弃单收口指标集合（可注入 nil 则静默）。

type Metrics struct {
	eventCounter *prometheus.CounterVec
	holdGauge *prometheus.GaugeVec
}

var (
	globalOnce sync.Once
	globalMetrics *Metrics
)

// DefaultMetrics 返回全局共享 Metrics 实例（使用 promauto 自动注册）。
  func DefaultMetrics() *Metrics {
	  globalOnce.Do(func() {
		  globalMetrics = &Metrics{
			  eventCounter: promauto.NewCounterVec(prometheus.CounterOpts{
				   Namespace: "p2p_abandon",
				  Name:      "events_total",
				  Help:      "P2P 弃单收口事件计数（armed/released/filled/conflict/error 等）",
			   }, []string{"event", "label"}),
			  holdGauge: promauto.NewGaugeVec(prometheus.GaugeOpts{
				   Namespace: "p2p_abandon",
				  Name:      "hold_gauge",
				  Help:      "P2P 弃单收口瞬时指标（escalated hold 数量等）",
			   }, []string{"kind"}),
		  }
	   })
	   return globalMetrics
  }

// NoopMetrics 返回静默 Metrics（用）。

   func NoopMetrics() *Metrics {
	   return &Metrics{
		   eventCounter: prometheus.NewCounterVec(prometheus.CounterOpts{
			   Namespace: "p2p_abandon_noop",
			   Name: "events_total_noop",
		   }, []string{"event", "label"}),
		   holdGauge: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			  Namespace: "p2p_abandon_noop",
			   Name: "hold_gauge_noop",
		  }, []string{"kind"}),
	  }
  }

// metricsInc 计数器自增（m 为 nil 时静默）。

   func metricsInc(m *Metrics, event, label string) {
	  if m == nil || m.eventCounter == nil {
		   return
	   }
	  m.eventCounter.WithLabelValues(event, label).Inc()
   }

// metricsAdd 计数器累加（m 为 nil 时静默）。

  func metricsAdd(m *Metrics, event string, v float64) {
	  if m == nil || m.holdGauge == nil {
		   return
	   }
	  m.holdGauge.WithLabelValues(event).Add(v)
   }
