package clearing

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	metricDayCloseStatus = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "clearing_dayclose_status",
		Help: "当前营业日 DayClose 状态（1=open,2=freezing,3=closed,4=reopened_limited）",
	}, []string{"business_day"})
	metricSuspenseOpenCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "clearing_suspense_open_count",
		Help: "当前 open 状态的挂账分录数",
	})
	metricSuspenseEscalatedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "clearing_suspense_escalated_total",
		Help: "累计 SLA 升级次数",
	})
	metricExceptLinkCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "clearing_exceptlink_count",
		Help: "清算异常与差异单关联记录数",
	})
	metricFenceBlockedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "clearing_fence_blocked_total",
		Help: "封锁窗拒绝的跨日推进尝试次数",
	}, []string{"business_day"})
)

// dayCloseStatusNum 将状态枚举转为数字便于 Gauge 展示。
   func dayCloseStatusNum(s DayCloseStatus) float64 {
	  switch s {
	   case DayCloseOpen:
		   return 1
	   case DayCloseFreeze:
		  return 2
	  case DayCloseClosed:
		  return 3
	   case DayCloseReopened:
		   return 4
	  }
	  return 0
  }

// ObserveDayCloseStatus 更新日切状态 Gauge。

   func ObserveDayCloseStatus(dc DayClose) {
	  metricDayCloseStatus.WithLabelValues(dc.BusinessDay).Set(dayCloseStatusNum(dc.Status))
   }

// ObserveSuspenseOpen 更新 open 挂账分录数。

  func ObserveSuspenseOpen(count int) {
	   metricSuspenseOpenCount.Set(float64(count))
   }

// ObserveSuspenseEscalated 升级事件发生时递增计数器。
  func ObserveSuspenseEscalated() {
	  metricSuspenseEscalatedTotal.Inc()
  }

// ObserveExceptLinkCount 更新关联记录 Gauge。

  func ObserveExceptLinkCount(count int) {
	  metricExceptLinkCount.Set(float64(count))
  }

// ObserveFenceBlocked 记录封锁窗拒绝次数。

  func ObserveFenceBlocked(businessDay string) {
	  metricFenceBlockedTotal.WithLabelValues(businessDay).Inc()
   }
