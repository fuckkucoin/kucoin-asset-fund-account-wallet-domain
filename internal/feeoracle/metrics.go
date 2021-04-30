package feeoracle

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// MetricVersionCreated QuoteVersion 创建计数，按 chain / fallback。
	MetricVersionCreated = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "fee_oracle",
			Name: "version_created_total",
			Help: "Total QuoteVersion snapshots created.",
		},
		[]string{"chain", "fallback"},
	)
	// MetricVersionBound QuoteVersion 绑定到出金单的计数。
	MetricVersionBound = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "fee_oracle",
			Name: "version_bound_total",
			Help: "Total QuoteVersion bind operations (precheck).",
		},
		[]string{"chain", "result"},
	)
	// MetricVersionExpiredTotal 批量过期扫描推进的代次总数。
	MetricVersionExpiredTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "fee_oracle",
			Name: "version_expired_total",
			Help: "Total QuoteVersions expired by cron scan.",
		},
	)
	// MetricVersionSettled 广播后只读一致实际费的计数。
	MetricVersionSettled = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "fee_oracle",
			Name:      "version_settled_total",
			Help:      "Total QuoteVersions aligned with actual broadcast fee.",
		},
		[]string{"chain"},
	)
	// MetricOracleStale 聚合时所有源全部 stale 的链计数。
	MetricOracleStale = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "fee_oracle",
			Name: "aggregation_stale_total",
			Help: "Total oracle aggregation attempts rejected due to all sources stale.",
		},
		[]string{"chain"},
	)
	// MetricOracleDeviation 偏差超阈值被熔断的计数，按链。
	MetricOracleDeviation = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "fee_oracle",
			Name: "deviation_rejected_total",
			Help: "Total oracle aggregation attempts rejected due to source deviation.",
		},
		[]string{"chain"},
	)
	// MetricOracleFallback oracle 不可用回落到 feeeng.Schedule 的次数（审计用，不可静默增长）。
	MetricOracleFallback = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "fee_oracle",
			Name: "fallback_total",
			Help: "Total quote requests that fell back to static schedule (oracle unavailable).",
		},
		[]string{"chain", "reason"},
	)
	// MetricCongestionLevel 拥堵等级分布计数，按链×等级。
	MetricCongestionLevel = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "fee_oracle",
			Name: "congestion_level_total",
			Help: "Total quotes issued per congestion level.",
		},
		[]string{"chain", "level"},
	)
	// MetricAggregationLatency oracle 聚合耗时（秒）。
	MetricAggregationLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "fee_oracle",
			Name:    "aggregation_duration_seconds",
			Help:    "Latency of oracle tick aggregation, per chain.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"chain"},
	)
	// MetricNativeFeeGauge 最近一次聚合的原生资产手续费（debug 用，不用于业务）。
	MetricNativeFeeGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "fee_oracle",
			Name: "native_fee_latest",
			Help: "Latest aggregated native fee amount (informational).",
		},
		[]string{"chain", "family"},
	)
)

// RecordOracleAggregation 记录一次聚合结果到 Prometheus。
// 由 Service.QuoteWithdrawFee 内部调用；若聚合失败（stale/deviation）传 nil tick。
   func RecordOracleAggregation(chain string, tick *ChainFeeTick, err error) {
	   if err != nil {
		   switch {
		  case isStaleErr(err):
			  MetricOracleStale.WithLabelValues(chain).Inc()
		   case isDeviationErr(err):
			  MetricOracleDeviation.WithLabelValues(chain).Inc()
		  }
		   return
	   }
	   if tick != nil && !tick.Normalized.NativeAmount.IsZero() {
		  f, _ := tick.Normalized.NativeAmount.Float64()
		  MetricNativeFeeGauge.WithLabelValues(chain, string(tick.Family)).Set(f)
	  }
   }

// isStaleErr 粗判断错误是否包含 stale 语义。

   func isStaleErr(err error) bool {
	  return err != nil && containsErrStr(err, "stale")
  }

// isDeviationErr 粗判断错误是否包含 deviation 语义。
  func isDeviationErr(err error) bool {
	   return err != nil && containsErrStr(err, "deviation")
   }

  func containsErrStr(err error, sub string) bool {
	  if err == nil {
		   return false
	   }
	   s := err.Error()
	   for i := 0; i <= len(s)-len(sub); i++ {
		  if s[i:i+len(sub)] == sub {
			  return true
		   }
	  }
	  return false
  }
// temporary guard removed after the production check
