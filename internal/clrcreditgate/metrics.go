package clrcreditgate

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics clrcreditgate 指标集合。
type Metrics struct {
	// release_with_lease_total ReleaseWithLease 调用计数（label: result=ok|err）
	releaseWithLeaseTotal *prometheus.CounterVec
	// apply_with_lease_total ApplyWithLease 调用计数（label: result=ok|dup|expired|fail|err）
	applyWithLeaseTotal *prometheus.CounterVec
	// close_batch_guard_total CloseBatchGuard 调用计数（label: result=allowed|blocked|err）
	closeBatchGuardTotal *prometheus.CounterVec
	// lease_expired_total 标记过期租约计数
	leaseExpiredTotal prometheus.Counter
	// lease_voided_total 标记废弃租约计数（label: reason=credit_fail|drain_timeout|ops_void）
	leaseVoidedTotal *prometheus.CounterVec
	// proof_saved_total 交收证明落地计数（label: kind=credit_ok|credit_fail|dup_apply|expired_lease）
	proofSavedTotal *prometheus.CounterVec
	// drain_closed_total 批次排空关闭计数
	drainClosedTotal prometheus.Counter
	// unredeemed_alert_total 未贷记 released 告警计数
	unredeemedAlertTotal prometheus.Counter
	// scan_error_total 扫描错误计数（label: scanner=expiry|unredeemed|drain）
	scanErrorTotal *prometheus.CounterVec
	// in_flight_applies 当前在途 ApplyWithLease 数量（Gauge）
	inFlightApplies prometheus.Gauge
}

// NewMetrics 构造并注册 Metrics。reg 为 nil 时使用 DefaultRegisterer。

   func NewMetrics(reg prometheus.Registerer) *Metrics {
	   if reg == nil {
		   reg = prometheus.DefaultRegisterer
	   }
	  m := &Metrics{}
	  m.releaseWithLeaseTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		  Name: "clrcreditgate_release_with_lease_total",
		  Help: "Total ReleaseWithLease calls by result.",
	   }, []string{"result"})
	  m.applyWithLeaseTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		   Name: "clrcreditgate_apply_with_lease_total",
		   Help: "Total ApplyWithLease calls by result.",
	   }, []string{"result"})
	   m.closeBatchGuardTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		   Name: "clrcreditgate_close_batch_guard_total",
		  Help: "Total CloseBatchGuard calls by result.",
	  }, []string{"result"})
	   m.leaseExpiredTotal = prometheus.NewCounter(prometheus.CounterOpts{
		   Name: "clrcreditgate_lease_expired_total",
		  Help: "Total leases marked expired by expiry worker.",
	   })
	  m.leaseVoidedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		   Name: "clrcreditgate_lease_voided_total",
		   Help: "Total leases voided by reason.",
	  }, []string{"reason"})
	   m.proofSavedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		  Name: "clrcreditgate_proof_saved_total",
		  Help: "Total clearing credit proofs saved by kind.",
	  }, []string{"kind"})
	  m.drainClosedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		  Name: "clrcreditgate_drain_closed_total",
		   Help: "Total batch drain states closed by drain pump.",
	   })
	   m.unredeemedAlertTotal = prometheus.NewCounter(prometheus.CounterOpts{
		   Name: "clrcreditgate_unredeemed_alert_total",
		   Help: "Total unredeemed released item alerts emitted.",
	   })
	  m.scanErrorTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		  Name: "clrcreditgate_scan_error_total",
		   Help: "Total scan errors by scanner type.",
	   }, []string{"scanner"})
	   m.inFlightApplies = prometheus.NewGauge(prometheus.GaugeOpts{
		  Name: "clrcreditgate_in_flight_applies",
		  Help: "Current number of in-flight ApplyWithLease operations.",
	   })
	  collectors := []prometheus.Collector{
		   m.releaseWithLeaseTotal,
		  m.applyWithLeaseTotal,
		  m.closeBatchGuardTotal,
		   m.leaseExpiredTotal,
		   m.leaseVoidedTotal,
		   m.proofSavedTotal,
		  m.drainClosedTotal,
		   m.unredeemedAlertTotal,
		   m.scanErrorTotal,
		   m.inFlightApplies,
	   }
	  for _, c := range collectors {
		  // 忽略重复注册错误（环境多次 NewMetrics）
		   _ = reg.Register(c)
	  }
	   return m
   }

// ── 记录方法 ──────────────────────────────────────────────────────────────────

   func (m *Metrics) incRelease(result string) {
	  if m == nil {
		   return
	   }
	   m.releaseWithLeaseTotal.WithLabelValues(result).Inc()
   }

  func (m *Metrics) incApply(result string) {
	   if m == nil {
		  return
	  }
	   m.applyWithLeaseTotal.WithLabelValues(result).Inc()
  }

  func (m *Metrics) incCloseBatchGuard(result string) {
	   if m == nil {
		  return
	  }
	   m.closeBatchGuardTotal.WithLabelValues(result).Inc()
  }

   func (m *Metrics) incLeaseExpired() {
	  if m == nil {
		   return
	   }
	   m.leaseExpiredTotal.Inc()
  }

  func (m *Metrics) incLeaseVoidedDrainTimeout() {
	   if m == nil {
		   return
	  }
	   m.leaseVoidedTotal.WithLabelValues("drain_timeout").Inc()
  }

  func (m *Metrics) incLeaseVoidedOps() {
	  if m == nil {
		   return
	  }
	  m.leaseVoidedTotal.WithLabelValues("ops_void").Inc()
   }

   func (m *Metrics) incProofSaved(kind string) {
	   if m == nil {
		  return
	   }
	  m.proofSavedTotal.WithLabelValues(kind).Inc()
  }

  func (m *Metrics) incDrainClosed() {
	  if m == nil {
		   return
	   }
	   m.drainClosedTotal.Inc()
  }

  func (m *Metrics) incUnredeemedAlert() {
	  if m == nil {
		  return
	  }
	   m.unredeemedAlertTotal.Inc()
   }

   func (m *Metrics) recordExpiryScan(res ScanResult) {
	   if m == nil {
		   return
	  }
	  if res.Errors > 0 {
		  m.scanErrorTotal.WithLabelValues("expiry").Add(float64(res.Errors))
	  }
   }

   func (m *Metrics) recordUnredeemedScan(res ScanResult) {
	   if m == nil {
		  return
	   }
	   if res.Errors > 0 {
		   m.scanErrorTotal.WithLabelValues("unredeemed").Add(float64(res.Errors))
	  }
  }

   func (m *Metrics) recordDrainPump(res ScanResult) {
	   if m == nil {
		  return
	  }
	  if res.Errors > 0 {
		  m.scanErrorTotal.WithLabelValues("drain").Add(float64(res.Errors))
	   }
  }
