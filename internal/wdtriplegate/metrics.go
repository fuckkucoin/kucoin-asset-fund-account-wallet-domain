package wdtriplegate

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// MetricLeaseIssued 签发三联租约次数。
	MetricLeaseIssued = promauto.NewCounter(prometheus.CounterOpts{
		Name: "wdtriplegate_lease_issued_total",
		Help: "Number of triple leases successfully issued after preflight",
	})
	// MetricLeaseBound 广播前复验通过次数。
	MetricLeaseBound = promauto.NewCounter(prometheus.CounterOpts{
		Name: "wdtriplegate_lease_bound_total",
		Help: "Number of triple leases bound at pre-broadcast verification",
	})
	// MetricLeaseConsumed 广播成功消费租约次数。
	MetricLeaseConsumed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "wdtriplegate_lease_consumed_total",
		Help: "Number of triple leases consumed after successful broadcast",
	})
	// MetricLeaseExpired 租约过期次数（需重检）。
	MetricLeaseExpired = promauto.NewCounter(prometheus.CounterOpts{
		Name: "wdtriplegate_lease_expired_total",
		Help: "Number of triple leases expired without bind; require re-check",
	})
	// MetricLeaseBlocked 租约因漂移被挡写次数。
	MetricLeaseBlocked = promauto.NewCounter(prometheus.CounterOpts{
		Name: "wdtriplegate_lease_blocked_total",
		Help: "Number of triple leases blocked due to drift detection",
	})
	// MetricBindAllowed 广播前复验通过次数。
	MetricBindAllowed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "wdtriplegate_bind_allowed_total",
		Help: "Number of pre-broadcast binds that allowed broadcast",
	})
	// MetricBindRefused 广播前复验拒绝次数（含漂移）。
	MetricBindRefused = promauto.NewCounter(prometheus.CounterOpts{
		Name: "wdtriplegate_bind_refused_total",
		Help: "Number of pre-broadcast binds that refused broadcast (including drift)",
	})
	// MetricDriftCaseOpen 漂移案件产出数。
	MetricDriftCaseOpen = promauto.NewCounter(prometheus.CounterOpts{
		Name: "wdtriplegate_drift_case_open_total",
		Help: "Number of triple drift cases opened",
	})
	// MetricDriftCaseClosed 漂移案件关闭数（双人确认）。
	MetricDriftCaseClosed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "wdtriplegate_drift_case_closed_total",
		Help: "Number of triple drift cases closed by dual confirmation",
	})
	// MetricWorkerExpiryCycles 租约过期巡检周期数。
	MetricWorkerExpiryCycles = promauto.NewCounter(prometheus.CounterOpts{
		Name: "wdtriplegate_worker_expiry_cycles_total",
		Help: "Number of lease expiry worker scan cycles completed",
	})
	// MetricWorkerDriftScanCycles 漂移巡检周期数。
	MetricWorkerDriftScanCycles = promauto.NewCounter(prometheus.CounterOpts{
		Name: "wdtriplegate_worker_drift_scan_cycles_total",
		Help: "Number of drift scan worker cycles completed",
	})
	// MetricWorkerScanDuration Worker 扫描耗时分布。
	MetricWorkerScanDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name: "wdtriplegate_worker_scan_duration_seconds",
		Help: "Duration of wdtriplegate worker scan cycle",
		Buckets: prometheus.DefBuckets,
	})
	// MetricReCheckRequired 要求重检次数（租约过期或漂移后）。
	MetricReCheckRequired = promauto.NewCounter(prometheus.CounterOpts{
		Name: "wdtriplegate_recheck_required_total",
		Help: "Number of movements requiring re-check of CheckTriple due to lease expiry or drift",
	})
)
