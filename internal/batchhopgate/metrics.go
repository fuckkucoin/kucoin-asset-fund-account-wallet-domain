package batchhopgate

import "github.com/prometheus/client_golang/prometheus"

// GateMetrics 聚合 batchhopgate 所有 Prometheus 指标。
type GateMetrics struct {
	// residual 状态转移
	residualOpened prometheus.Counter
	residualProven prometheus.Counter
	residualClosed prometheus.Counter
	residualBlocked prometheus.Counter
	// lease 状态转移
	leaseOpened prometheus.Counter
	leaseDone    prometheus.Counter
	leaseExpired prometheus.Counter
	leaseConflict prometheus.Counter
	// proof
	proofBuilt       prometheus.Counter
	proofBuildFailed prometheus.Counter
	proofDigestFail prometheus.Counter
	// gen / token
	genConflict   prometheus.Counter
	tokenRejected prometheus.Counter
	// resume hop
	resumeHopDone    prometheus.Counter
	resumeHopSkipped prometheus.Counter
	// worker
	workerResidualScanBatch prometheus.Counter
	workerOccupyPatrolBatch prometheus.Counter
	workerLeaseReaperBatch prometheus.Counter
}

   func newGateMetrics() *GateMetrics {
	   return &GateMetrics{
		   residualOpened:  newCounter("residual_opened_total", "批量 hop 残留案落库计数"),
		   residualProven:  newCounter("residual_proven_total", "残留案推进至 proven 计数"),
		   residualClosed:  newCounter("residual_closed_total", "残留案关闭计数（续跳完成或人工收口）"),
		   residualBlocked: newCounter("residual_blocked_total", "残留案被并行互斥阻塞计数"),
		   leaseOpened: newCounter("lease_opened_total", "续跳互斥租约颁发计数"),
		  leaseDone:   newCounter("lease_done_total", "续跳租约完成计数"),
		   leaseExpired: newCounter("lease_expired_total", "续跳租约过期计数"),
		   leaseConflict: newCounter("lease_conflict_total", "续跳租约冲突（同案重复申请）计数"),
		   proofBuilt:       newCounter("proof_built_total", "占用证明构建成功计数"),
		  proofBuildFailed: newCounter("proof_build_failed_total", "占用证明构建失败计数"),
		  proofDigestFail: newCounter("proof_digest_fail_total", "占用证明 Digest 校验失败计数"),
		  genConflict: newCounter("gen_conflict_total", "BatchGeneration CAS 世代冲突计数"),
		  tokenRejected: newCounter("token_rejected_total", "世代令牌验证拒绝计数"),
		   resumeHopDone: newCounter("resume_hop_done_total", "续跳逐项 hop 执行完成计数"),
		  resumeHopSkipped: newCounter("resume_hop_skipped_total", "续跳跳过已成功 hop 计数"),
		   workerResidualScanBatch: newCounter("worker_residual_scan_batch_total", "worker 残留扫描批次计数"),
		   workerOccupyPatrolBatch: newCounter("worker_occupy_patrol_batch_total", "worker 占用巡检批次计数"),
		  workerLeaseReaperBatch:  newCounter("worker_lease_reaper_batch_total", "worker 过期续跳吊销批次计数"),
	   }
  }

  func newCounter(name, help string) prometheus.Counter {
	   c := prometheus.NewCounter(prometheus.CounterOpts{
		  Namespace: "batchhopgate",
		   Name:      name,
		  Help:      help,
	  })
	  // 允许重复注册（环境中 newGateMetrics 可能多次调用）。
	   if err := prometheus.Register(c); err != nil {
		   if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			   return are.ExistingCollector.(prometheus.Counter)
		  }
	   }
	   return c
  }
