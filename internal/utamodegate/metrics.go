package utamodegate

import "sync/atomic"

// GateMetrics 保存所有引擎内部计数器。
// 使用 atomic.Int64 避免额外锁开销。
type GateMetrics struct {
	// ── ModeLease FSM ────────────────────────────────────────────────────────
	leaseAcquired atomic.Int64 // Acquire 成功（open 状态）
	leaseBound    atomic.Int64 // Bind 成功（bound 状态）
	leaseSwitched atomic.Int64 // Switch 成功（switched 终态）
	leaseBlocked   atomic.Int64 // Acquire 因在途负债被 blocked
	leaseUnblocked atomic.Int64 // worker Unblock 推进（blocked → open）
	// ── LiabilityBind ────────────────────────────────────────────────────────
	bindRegistered atomic.Int64 // Register 成功
	bindReleased   atomic.Int64 // Release 成功
	bindExpired    atomic.Int64 // TTL 过期标记 BindExpired
	bindDriftFound atomic.Int64 // 跨模式键漂移发现（worker 扫描）
	// ── ModeDriftCase ────────────────────────────────────────────────────────
	driftCaseOpened atomic.Int64 // DriftCase open
	driftCaseProven atomic.Int64 // DriftCase proven
	driftCaseClosed atomic.Int64 // DriftCase closed（四眼收口）
	driftBlocked    atomic.Int64 // SetMode/借出被 DriftBlocked
	// ── ModeDriftProof ───────────────────────────────────────────────────────
	proofBuilt atomic.Int64 // Build 成功
	proofVerified atomic.Int64 // Verify 通过
	proofMismatch atomic.Int64 // Verify 失败（摘要不符）
	// ── ModeGeneration CAS ───────────────────────────────────────────────────
	genBumped atomic.Int64 // CAS 递增成功
	genStale     atomic.Int64 // CAS 世代冲突（ErrStaleGen）
	genValidOk   atomic.Int64 // ValidateToken 通过
	genValidFail atomic.Int64 // ValidateToken 失败（令牌过期）
	// ── SetMode 请求 ──────────────────────────────────────────────────────────
	modeSwitchTotal   atomic.Int64 // SetMode 总请求数
	modeSwitchSuccess atomic.Int64 // SetMode 成功（模式已切换）
	modeSwitchPending atomic.Int64 // SetMode 进入待切换队列（blocked）
	modeSwitchDenied  atomic.Int64 // SetMode 被拒绝（DriftBlocked / StaleGen）
}

   func newGateMetrics() *GateMetrics {
	   return &GateMetrics{}
   }

// ── 快照导出（供 HTTP /stats 接口聚合）────────────────────────────────────────
// Snapshot 返回所有计数器的快照（map[string]int64），供 HTTP /stats 响应。

   func (m *GateMetrics) Snapshot() map[string]int64 {
	  return map[string]int64{
		  "lease_acquired": m.leaseAcquired.Load(),
		  "lease_bound":    m.leaseBound.Load(),
		  "lease_switched": m.leaseSwitched.Load(),
		  "lease_blocked":  m.leaseBlocked.Load(),
		   "lease_unblocked":  m.leaseUnblocked.Load(),
		  "bind_registered":  m.bindRegistered.Load(),
		  "bind_released":    m.bindReleased.Load(),
		   "bind_expired":     m.bindExpired.Load(),
		   "bind_drift_found": m.bindDriftFound.Load(),
		   "drift_case_opened": m.driftCaseOpened.Load(),
		  "drift_case_proven": m.driftCaseProven.Load(),
		   "drift_case_closed": m.driftCaseClosed.Load(),
		  "drift_blocked":     m.driftBlocked.Load(),
		  "proof_built":       m.proofBuilt.Load(),
		   "proof_verified":    m.proofVerified.Load(),
		  "proof_mismatch": m.proofMismatch.Load(),
		  "gen_bumped": m.genBumped.Load(),
		  "gen_stale":  m.genStale.Load(),
		   "gen_valid_ok":   m.genValidOk.Load(),
		   "gen_valid_fail": m.genValidFail.Load(),
		  "mode_switch_total":   m.modeSwitchTotal.Load(),
		  "mode_switch_success": m.modeSwitchSuccess.Load(),
		  "mode_switch_pending": m.modeSwitchPending.Load(),
		   "mode_switch_denied": m.modeSwitchDenied.Load(),
	  }
   }
