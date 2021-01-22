package accrual

import (
	"time"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/shopspring/decimal"
)

var (
	metricBatchTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "accrual_batch_total",
		Help: "跨日应计批次累计数量",
	}, []string{"status"})
	metricLineAmountTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "accrual_line_amount_total",
		Help: "累计应计分录金额（以 USDT 计，按 asset 标签区分）",
	}, []string{"asset", "kind"})
	metricPostDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "accrual_batch_post_duration_seconds",
		Help:    "应计批次过账耗时分布",
		Buckets: prometheus.DefBuckets,
	}, []string{"business_day"})
	metricReversalTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "accrual_reversal_total",
		Help: "应计批次冲正累计次数",
	}, []string{"business_day"})
)

// ObserveBatchCreated 批次创建时记录指标。
  func ObserveBatchCreated() {
	  metricBatchTotal.WithLabelValues("draft").Inc()
   }

// ObserveBatchPosted 批次过账时记录指标。

   func ObserveBatchPosted(bat AccrualBatch, start time.Time) {
	  metricBatchTotal.WithLabelValues("posted").Inc()
	   metricPostDuration.WithLabelValues(bat.BusinessDay).Observe(time.Since(start).Seconds())
   }

// ObserveBatchReversed 批次冲正时记录指标。
  func ObserveBatchReversed(businessDay string) {
	  metricReversalTotal.WithLabelValues(businessDay).Inc()
	  metricBatchTotal.WithLabelValues("reversed").Inc()
   }

// ObserveLineAmounts 记录分录金额累计。
// lines 为已追加的分录列表；只统计 debit 侧避免双计。

   func ObserveLineAmounts(lines []AccrualLine) {
	  for _, l := range lines {
		  if l.Direction != "debit" {
			   continue
		  }
		   amt, err := decimal.NewFromString(l.Amount)
		   if err != nil {
			  continue
		   }
		   f, _ := amt.Float64()
		  metricLineAmountTotal.WithLabelValues(l.Asset, string(l.Kind)).Add(f)
	   }
  }
