package dustleak

import "github.com/prometheus/client_golang/prometheus"

var (
	// DustLeakDetected 已发现的 LeakCase 计数，按资产和泄漏类型分组。
	DustLeakDetected = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "funding",
		Subsystem: "dustleak",
		Name: "detected_total",
		Help: "已发现的粉尘兑换泄漏 LeakCase 总数",
	}, []string{"asset", "kind"})
	// DustLeakLeakedUSDT 累计泄漏 USDT 量（按资产和泄漏类型）。
	DustLeakLeakedUSDT = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "funding",
		Subsystem: "dustleak",
		Name:      "leaked_usdt_total",
		Help:      "累计泄漏 USDT（未经回滚的原始值）",
	}, []string{"asset", "kind"})
	// DustLeakDetectedDriftBps 漂移帽触发时的漂移幅度分布（基点）。
	DustLeakDetectedDriftBps = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "funding",
		Subsystem: "dustleak",
		Name:      "drift_bps",
		Help:      "触发漂移帽时的牌价漂移幅度（基点）",
		Buckets: []float64{10, 25, 50, 100, 200, 500},
	}, []string{"asset"})
	// DustRollbackPosted 已执行（posted）的回滚腿计数。
	DustRollbackPosted = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "funding",
		Subsystem: "dustleak",
		Name: "rollback_posted_total",
		Help: "已执行账务逆记的回滚腿总数",
	}, []string{"asset"})
	// DustRollbackFailed 回滚失败计数（进入 dispute）。
	DustRollbackFailed = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "funding",
		Subsystem: "dustleak",
		Name: "rollback_failed_total",
		Help: "回滚失败（进入 dispute）的腿总数",
	}, []string{"asset"})
	// DustGateBlocked 挡兑闸触发计数（同用户同资产被挡）。
	DustGateBlocked = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "funding",
		Subsystem: "dustleak",
		Name: "gate_blocked_total",
		Help: "因 LeakCase 触发挡兑闸的用户+资产组合数",
	}, []string{"asset"})
	// DustGateReleased 挡兑闸解除计数。
	DustGateReleased = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "funding",
		Subsystem: "dustleak",
		Name: "gate_released_total",
		Help: "LeakCase 收口后解除挡兑闸的总数",
	}, []string{"asset"})
	// DustPolicyGenUpdated 策略世代变更计数。
	DustPolicyGenUpdated = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "funding",
		Subsystem: "dustleak",
		Name:      "policy_gen_updated_total",
		Help: "策略世代递增次数（对应 PutPolicyGen 调用）",
	}, []string{"asset"})
	// DustConvertGuardRejected Floor Guard / Drift Cap 守卫拦截的 Convert 请求数。
	DustConvertGuardRejected = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "funding",
		Subsystem: "dustleak",
		Name: "convert_guard_rejected_total",
		Help: "被 Floor Guard 或 Drift Cap 守卫拦截的 Convert 请求数",
	}, []string{"asset", "reason"})
	// DustDetectorScanDuration 扫描器单次扫描耗时。
	DustDetectorScanDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "funding",
		Subsystem: "dustleak",
		Name:      "scan_duration_seconds",
		Help: "LeakDetector 单次扫描耗时（秒）",
		Buckets: prometheus.DefBuckets,
	}, []string{"source"})
)

// RegisterMetrics 注册所有指标到默认 registry。
// 在进程启动时调用一次。
   func RegisterMetrics() {
	  prometheus.MustRegister(
		  DustLeakDetected,
		   DustLeakLeakedUSDT,
		  DustLeakDetectedDriftBps,
		   DustRollbackPosted,
		  DustRollbackFailed,
		   DustGateBlocked,
		  DustGateReleased,
		   DustPolicyGenUpdated,
		  DustConvertGuardRejected,
		   DustDetectorScanDuration,
	  )
  }
