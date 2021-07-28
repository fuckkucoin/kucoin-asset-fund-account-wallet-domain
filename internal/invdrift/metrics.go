package invdrift

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// DriftBatchesTotal 扫描批次总数（按最终状态分桶）
	DriftBatchesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_invdrift_batches_total",
		Help: "Invdrift scan batches completed, by final state (graded/sealed/resolved)",
	}, []string{"state"})
	// DriftCritTotal crit 级偏离条目累计数（任一批次内计数）
	DriftCritTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_invdrift_crit_items_total",
		Help: "Cumulative crit-grade drift items; alert when rate > 0 per 5m",
	})
	// DriftWarnGauge 当前 sealed 批次内 warn 条目数
	DriftWarnGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_invdrift_warn_items_current",
		Help: "Warn-grade drift items in currently sealed batches",
	})
	// DriftScanErrors 扫描失败次数
	DriftScanErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_invdrift_scan_errors_total",
		Help: "Invdrift scan errors; alert on any increment",
	})
	// DriftAdvicesActive 当前有效（proposed）封堵建议数
	DriftAdvicesActive = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_invdrift_advices_active",
		Help: "Currently proposed seal advices; alert when > 0 for > 5m",
	})
	// DriftAdvicesExpired 自动过期的封堵建议累计数
	DriftAdvicesExpired = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_invdrift_advices_expired_total",
		Help: "Seal advices expired by TTL (drift converged)",
	})
	// DriftProofIntegrityFail 证明完整性巡检失败次数
	DriftProofIntegrityFail = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_invdrift_proof_integrity_fail_total",
		Help: "DriftProof integrity check failures; alert on any increment",
	})
	// DriftEscalateTotal 未收敛升级事件总数
	DriftEscalateTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_invdrift_escalate_total",
		Help: "Sealed batches escalated due to non-convergence; alert on any increment",
	})
	// DriftDeltaHot 各资产 hot 侧偏离绝对值（最近一批）
	DriftDeltaHot = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_invdrift_delta_hot",
		Help: "Latest drift delta (ledger_hot - cust_hot) per asset",
	}, []string{"asset", "data_center"})
	// DriftDeltaCold 各资产 cold 侧偏离绝对值（最近一批）
	DriftDeltaCold = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_invdrift_delta_cold",
		Help: "Latest drift delta (ledger_cold - cust_cold) per asset",
	}, []string{"asset", "data_center"})
)
