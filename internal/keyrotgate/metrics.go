package keyrotgate

import "github.com/prometheus/client_golang/prometheus"

var (
	// 租约生命周期
	MetricLeaseOpened = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "keyrotgate_lease_opened_total",
		Help: "轮换租约签发次数。",
	})
	MetricLeaseApproved = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "keyrotgate_lease_approved_total",
		Help: "轮换租约批准次数。",
	})
	MetricLeaseExecuting = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "keyrotgate_lease_executing_total",
		Help: "轮换租约进入执行次数。",
	})
	MetricLeaseDone = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "keyrotgate_lease_done_total",
		Help: "轮换租约完成次数。",
	})
	MetricLeaseBlocked = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "keyrotgate_lease_blocked_total",
		Help: "轮换租约被阻塞次数。",
	})
	MetricLeaseExpired = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "keyrotgate_lease_expired_total",
		Help: "轮换租约 TTL 过期次数。",
	})
	// 执行阻塞原因
	MetricExecuteBlockedInflight = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "keyrotgate_execute_blocked_inflight_total",
		Help: "因在途签名存在被阻塞的执行次数。",
	})
	MetricExecuteBlockedSlotOp = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "keyrotgate_execute_blocked_slot_op_total",
		Help: "因槽操作失败被阻塞的执行次数。",
	})
	// 在途签名绑定
	MetricBindActive = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "keyrotgate_sign_bind_active_total",
		Help: "在途签名绑定创建次数。",
	})
	MetricBindReleased = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "keyrotgate_sign_bind_released_total",
		Help: "在途签名绑定主动释放次数。",
	})
	MetricBindExpired = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "keyrotgate_sign_bind_expired_total",
		Help: "在途签名绑定 TTL 过期次数。",
	})
	MetricBindRejected = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "keyrotgate_sign_bind_rejected_total",
		Help: "在途签名绑定因槽 Retired 被拒绝次数。",
	})
	// 漂移检测
	MetricDriftDetected = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "keyrotgate_drift_detected_total",
		Help: "指纹漂移案件产出次数。",
	})
	MetricDriftProven = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "keyrotgate_drift_proven_total",
		Help: "漂移案件证明提交次数。",
	})
	MetricDriftClosed = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "keyrotgate_drift_closed_total",
		Help: "漂移案件关闭次数。",
	})
	// 指纹证明
	MetricProofIssued = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "keyrotgate_fingerprint_proof_issued_total",
		Help: "指纹证明签发次数。",
	})
	// Worker 运行状态
	MetricWorkerExpiryCycles = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "keyrotgate_worker_expiry_cycles_total",
		Help: "租约过期巡检周期数。",
	})
	MetricWorkerBindExpiryCycles = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "keyrotgate_worker_bind_expiry_cycles_total",
		Help: "签名绑定过期巡检周期数。",
	})
	MetricWorkerDriftScanCycles = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "keyrotgate_worker_drift_scan_cycles_total",
		Help: "漂移巡检周期数。",
	})
	MetricWorkerScanDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: "keyrotgate_worker_scan_duration_seconds",
		Help: "Worker 单次巡检耗时（秒）。",
		Buckets: prometheus.DefBuckets,
	})
)

  func init() {
	   prometheus.MustRegister(
		  MetricLeaseOpened,
		   MetricLeaseApproved,
		  MetricLeaseExecuting,
		  MetricLeaseDone,
		   MetricLeaseBlocked,
		   MetricLeaseExpired,
		   MetricExecuteBlockedInflight,
		   MetricExecuteBlockedSlotOp,
		   MetricBindActive,
		   MetricBindReleased,
		   MetricBindExpired,
		  MetricBindRejected,
		  MetricDriftDetected,
		   MetricDriftProven,
		   MetricDriftClosed,
		  MetricProofIssued,
		  MetricWorkerExpiryCycles,
		   MetricWorkerBindExpiryCycles,
		  MetricWorkerDriftScanCycles,
		  MetricWorkerScanDuration,
	  )
  }
