package lendoccupygate

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics lendoccupygate 指标集合。
type Metrics struct {
	// place_bound_total PlaceBound 调用计数（label: result=ok|err）
	placeBoundTotal *prometheus.CounterVec
	// settle_total Settle 调用计数（label: result=ok|dup|err）
	settleTotal *prometheus.CounterVec
	// cancel_total Cancel 调用计数（label: result=ok|dup|err）
	cancelTotal *prometheus.CounterVec
	// compensate_residue_total 半截补偿计数（label: result=ok|skip|err）
	compensateResidueTotal *prometheus.CounterVec
	// dup_proof_saved_total 重复释放证明落地计数
	dupProofSavedTotal prometheus.Counter
	// residue_proof_saved_total 半截补偿证明落地计数
	residueProofSavedTotal prometheus.Counter
	// stale_placed_leases_count 当前悬挂（过期 placed）租约数量
	stalePlacedCount prometheus.Gauge
	// scan_error_total 扫描错误计数（label: scanner=half_place|dup_release）
	scanErrorTotal *prometheus.CounterVec
}

// NewMetrics 构造并注册 Metrics。reg 为 nil 时使用 DefaultRegisterer。

   func NewMetrics(reg prometheus.Registerer) *Metrics {
	  if reg == nil {
		   reg = prometheus.DefaultRegisterer
	  }
	  m := &Metrics{
		   placeBoundTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			  Name: "lendoccupygate_place_bound_total",
			   Help: "Total PlaceBound calls by result.",
		  }, []string{"result"}),
		  settleTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			  Name: "lendoccupygate_settle_total",
			  Help: "Total Settle calls by result.",
		  }, []string{"result"}),
		   cancelTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			  Name: "lendoccupygate_cancel_total",
			  Help: "Total Cancel calls by result.",
		   }, []string{"result"}),
		  compensateResidueTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			  Name: "lendoccupygate_compensate_residue_total",
			   Help: "Total CompensateResidue calls by result.",
		   }, []string{"result"}),
		  dupProofSavedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			  Name: "lendoccupygate_dup_proof_saved_total",
			  Help: "Total LendDupReleaseProof records saved.",
		   }),
		  residueProofSavedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			   Name: "lendoccupygate_residue_proof_saved_total",
			   Help: "Total LendResidueProof records saved.",
		   }),
		   stalePlacedCount: prometheus.NewGauge(prometheus.GaugeOpts{
			   Name: "lendoccupygate_stale_placed_leases",
			  Help: "Current count of stale placed leases (pending compensation).",
		  }),
		   scanErrorTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			  Name: "lendoccupygate_scan_error_total",
			  Help: "Total scanner errors by scanner type.",
		  }, []string{"scanner"}),
	   }
	   reg.MustRegister(
		  m.placeBoundTotal,
		  m.settleTotal,
		   m.cancelTotal,
		   m.compensateResidueTotal,
		  m.dupProofSavedTotal,
		   m.residueProofSavedTotal,
		  m.stalePlacedCount,
		  m.scanErrorTotal,
	   )
	   return m
   }

// RecordPlaceBound 上报 PlaceBound 结果。
  func (m *Metrics) RecordPlaceBound(ok bool) {
	  if m == nil {
		  return
	   }
	   if ok {
		  m.placeBoundTotal.WithLabelValues("ok").Inc()
	  } else {
		  m.placeBoundTotal.WithLabelValues("err").Inc()
	  }
   }

// RecordSettle 上报 Settle 结果。

   func (m *Metrics) RecordSettle(result string) {
	  if m == nil {
		   return
	  }
	   m.settleTotal.WithLabelValues(result).Inc()
   }

// RecordCancel 上报 Cancel 结果。

  func (m *Metrics) RecordCancel(result string) {
	   if m == nil {
		  return
	  }
	  m.cancelTotal.WithLabelValues(result).Inc()
  }

// RecordCompensateResidue 上报 CompensateResidue 结果。

  func (m *Metrics) RecordCompensateResidue(result string) {
	  if m == nil {
		  return
	  }
	  m.compensateResidueTotal.WithLabelValues(result).Inc()
  }

// RecordDupProofSaved 上报重复释放证明落地。

  func (m *Metrics) RecordDupProofSaved() {
	   if m == nil {
		   return
	   }
	   m.dupProofSavedTotal.Inc()
  }

// RecordResidueProofSaved 上报半截补偿证明落地。
   func (m *Metrics) RecordResidueProofSaved() {
	  if m == nil {
		   return
	   }
	   m.residueProofSavedTotal.Inc()
  }

// SetStalePlacedCount 设置悬挂租约数量 gauge。
   func (m *Metrics) SetStalePlacedCount(n int) {
	  if m == nil {
		  return
	   }
	  m.stalePlacedCount.Set(float64(n))
   }

// RecordScanError 上报扫描错误。

  func (m *Metrics) RecordScanError(scanner string) {
	  if m == nil {
		   return
	  }
	  m.scanErrorTotal.WithLabelValues(scanner).Inc()
   }

// RecordHalfPlaceScan 上报半截扫描结果。
   func (m *Metrics) RecordHalfPlaceScan(r *ScanResult) {
	  if m == nil || r == nil {
		   return
	   }
	  m.compensateResidueTotal.WithLabelValues("ok").Add(float64(r.CompensatedCount))
	  m.compensateResidueTotal.WithLabelValues("skip").Add(float64(r.SkippedCount))
	  if r.ErrorCount > 0 {
		  m.compensateResidueTotal.WithLabelValues("err").Add(float64(r.ErrorCount))
	  }
   }

// RecordDupReleaseScan 上报重复释放扫描结果。
  func (m *Metrics) RecordDupReleaseScan(r *ScanResult) {
	   if m == nil || r == nil {
		   return
	  }
	  _ = r // 当前仅记录扫描计数，不额外上报
   }
