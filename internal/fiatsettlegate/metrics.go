package fiatsettlegate

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	metricLeaseIssued = promauto.NewCounter(prometheus.CounterOpts{
		Name: "fiat_settle_lease_issued_total",
		Help: "Authorize 成功签发 settle_lease 次数",
	})
	metricSettleOK = promauto.NewCounter(prometheus.CounterOpts{
		Name: "fiat_settle_assert_ok_total",
		Help: "SettleAssert 成功次数",
	})
	metricSettleReject = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fiat_settle_assert_reject_total",
		Help: "SettleAssert 失败次数（指纹/互斥/补偿）",
	}, []string{"reason"})
		Name: "fiat_settle_compensate_open_total",
		Help: "Capture 成功 ledger 失败进入补偿次数",
	})
	metricCaptureSkip = promauto.NewCounter(prometheus.CounterOpts{
		Name: "fiat_settle_second_capture_blocked_total",
		Help: "二次 Capture 被封堵次数",
	})
	metricReserve = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "fiat_limits_reservation_open",
		Help: "当前 reserved 占用数",
	})
)

// ObserveIssued 签发。
   func ObserveIssued() { metricLeaseIssued.Inc() }

// ObserveSettle 结清结果。
   func ObserveSettle(ok bool, reason string) {
	  if ok {
		   metricSettleOK.Inc()
		  return
	   }
	  metricSettleReject.WithLabelValues(reason).Inc()
   }

// ObserveMutex 互斥。

  func ObserveMutex() { metricMutex.Inc() }

// ObserveCompensate 补偿开仓。
  func ObserveCompensate() { metricCompensate.Inc() }

// ObserveCaptureSkip 跳过二次 Capture。
   func ObserveCaptureSkip() { metricCaptureSkip.Inc() }

// ObserveReserve 占用水位。
  func ObserveReserve(n int) { metricReserve.Set(float64(n)) }
