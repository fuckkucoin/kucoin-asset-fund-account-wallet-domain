package creditoutgate

import "github.com/prometheus/client_golang/prometheus"

var (
	// metricEnqueueTotal 投递 Enqueue 计数（by board_key, replay）。
	metricEnqueueTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "creditoutgate",
		Name: "enqueue_total",
		Help: "Total number of Enqueue calls (credit deliver lease created or replayed).",
	}, []string{"board_key", "result"})
	// metricDeliverTotal Deliver 计数（by board_key, result）。
	metricDeliverTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "creditoutgate",
		Name: "deliver_total",
		Help: "Total number of Deliver calls by result (delivered, replay, gen_drift, revoked, epoch_mismatch, error).",
	}, []string{"board_key", "result"})
	// metricFenceFlipTotal 护栏翻转计数（by board_key, close）。
	metricFenceFlipTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "creditoutgate",
		Name: "fence_flip_total",
		Help: "Total number of FenceFlip calls.",
	}, []string{"board_key", "close"})
	// metricRevokedTotal 护栏翻转吊销租约计数。
	metricRevokedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "creditoutgate",
		Name:      "revoked_leases_total",
		Help:      "Total number of leases revoked by FenceFlip.",
	}, []string{"board_key"})
	// metricCasesTotal 事故证明追加计数（by board_key, kind）。
	metricCasesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "creditoutgate",
		Name: "fence_cases_total",
		Help: "Total number of CreditFenceCase records appended.",
	}, []string{"board_key", "kind"})
	// metricActiveLeases 当前活跃租约数（by board_key）。
	metricActiveLeases = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "creditoutgate",
		Name: "active_leases",
		Help: "Current number of active (enqueued or delivering) leases.",
	}, []string{"board_key"})
	// metricWorldGen 当前板卡世代（by board_key）。
	metricWorldGen = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "creditoutgate",
		Name:      "board_world_gen",
		Help: "Current world generation of each board.",
	}, []string{"board_key"})
)

// RegisterMetrics 注册全部 Prometheus 指标（幂等）。

   func RegisterMetrics(reg prometheus.Registerer) {
	  if reg == nil {
		  reg = prometheus.DefaultRegisterer
	  }
	  for _, c := range []prometheus.Collector{
		   metricEnqueueTotal,
		   metricDeliverTotal,
		  metricFenceFlipTotal,
		   metricRevokedTotal,
		   metricCasesTotal,
		   metricActiveLeases,
		  metricWorldGen,
	   } {
		  _ = reg.Register(c)
	   }
   }

// recordEnqueue 记录 Enqueue 指标。
  func recordEnqueue(boardKey, result string) {
	  metricEnqueueTotal.WithLabelValues(boardKey, result).Inc()
   }

// recordDeliver 记录 Deliver 指标。

  func recordDeliver(boardKey, result string) {
	  metricDeliverTotal.WithLabelValues(boardKey, result).Inc()
  }

// recordFenceFlip 记录 FenceFlip 指标。
  func recordFenceFlip(boardKey string, close bool, revokedCount int) {
	   closeStr := "false"
	   if close {
		  closeStr = "true"
	   }
	   metricFenceFlipTotal.WithLabelValues(boardKey,
		  closeStr).Inc()
	  if revokedCount > 0 {
		  metricRevokedTotal.WithLabelValues(boardKey).Add(float64(revokedCount))
	   }
   }

// recordCase 记录事故证明指标。

   func recordCase(boardKey string, kind CreditFenceCaseKind) {
	   metricCasesTotal.WithLabelValues(boardKey, string(kind)).Inc()
   }
