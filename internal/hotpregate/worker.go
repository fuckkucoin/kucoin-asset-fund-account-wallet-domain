package hotpregate

import (
	"context"
	"fmt"
	"log"
	"time"
)

// WorkerMetrics Worker 指标回调（注入 Prometheus；注入计数器）。
type WorkerMetrics struct {
	// LeaseExpiredTotal 租约被标记为 expired 的总次数。
	LeaseExpiredTotal func()
	// DriftDetectedTotal 巡检发现漂移的总次数。
	DriftDetectedTotal func()
	// PatrolledTotal 巡检轮次总次数（每次 Run 调用计一次）。
	PatrolledTotal func()
	// ExpiryRunTotal 过期扫描轮次总次数。
	ExpiryRunTotal func()
}

   func noopFn() {}

  func (m *WorkerMetrics) safeLeaseExpired() {
	  if m != nil && m.LeaseExpiredTotal != nil {
		   m.LeaseExpiredTotal()
	   }
   }

   func (m *WorkerMetrics) safeDriftDetected() {
	  if m != nil && m.DriftDetectedTotal != nil {
		   m.DriftDetectedTotal()
	   }
   }

   func (m *WorkerMetrics) safePatrolled() {
	  if m != nil && m.PatrolledTotal != nil {
		   m.PatrolledTotal()
	  }
   }

  func (m *WorkerMetrics) safeExpiryRun() {
	   if m != nil && m.ExpiryRunTotal != nil {
		  m.ExpiryRunTotal()
	   }
   }

// ── ExpiryWorker ──────────────────────────────────────────────────────────────
// ExpiryWorker 定期扫描超期租约并标记为 expired。

type ExpiryWorker struct {
	svc      *Service
	interval time.Duration
	batchSz  int
	metrics *WorkerMetrics
}

// NewExpiryWorker 构造 ExpiryWorker。
// interval: 扫描间隔；batchSz: 单次扫描上限（防单次处理量过大）。

   func NewExpiryWorker(svc *Service, interval time.Duration, batchSz int, m *WorkerMetrics) *ExpiryWorker {
	   if interval <= 0 {
		  interval = 30 * time.Second
	  }
	  if batchSz <= 0 {
		  batchSz = 100
	   }
	   return &ExpiryWorker{svc: svc,
		   interval: interval, batchSz: batchSz, metrics: m}
   }

// Run 阻塞直到 ctx 取消。调用方以 go Run(ctx) 启动。
   func (w *ExpiryWorker) Run(ctx context.Context) {
	   ticker := time.NewTicker(w.interval)
	   defer ticker.Stop()
	  for {
		  select {
		   case <-ctx.Done():
			   return
		  case <-ticker.C:
			   w.runOnce(ctx)
		  }
	   }
   }

  func (w *ExpiryWorker) runOnce(ctx context.Context) {
	  w.metrics.safeExpiryRun()
	  now := w.svc.nowFn()
	   leases,
		  err := w.svc.store.ListExpiredLeases(ctx, now, w.batchSz)
	   if err != nil {
		  log.Printf("[hotpregate expiry] list expired leases error: %v", err)
		   return
	   }
	   for _, lease := range leases {
		   if !lease.IsExpired(now) {
			  continue
		  }
		   err := w.svc.store.UpdateLeaseState(ctx, lease.LeaseID, lease.State, LeaseExpired, now,
			   "expired by worker")
		  if err != nil {
			  log.Printf("[hotpregate expiry] mark expired %s: %v", lease.LeaseID, err)
			   continue
		   }
		   w.metrics.safeLeaseExpired()
		  log.Printf("[hotpregate expiry] lease %s asset=%s window=%s expired", lease.LeaseID, lease.Asset, lease.Window)
	  }
   }

// ── DriftPatrolWorker ─────────────────────────────────────────────────────────
// DriftPatrolWorker 对活跃租约主动巡检；发现漂移后升级状态并落 InvDriftProof。
// 巡检不依赖提交方触发，是预检租约漂移发现的主动路径。
// 与 ExpiryWorker 互不干扰：ExpiryWorker 按时间判断，DriftPatrolWorker 按 Observe 判断。
type DriftPatrolWorker struct {
	svc *Service
	interval time.Duration
	batchSz  int
	assets   []string // 巡检资产白名单；空表示全部
	metrics *WorkerMetrics
}

// NewDriftPatrolWorker 构造 DriftPatrolWorker。
// assets: 巡检资产列表（空表示通过 ObserveAll 获取全部资产）。

  func NewDriftPatrolWorker(svc *Service, interval time.Duration, batchSz int, assets []string, m *WorkerMetrics) *DriftPatrolWorker {
	  if interval <= 0 {
		   interval = 60 * time.Second
	  }
	  if batchSz <= 0 {
		   batchSz = 50
	  }
	   return &DriftPatrolWorker{svc: svc, interval: interval, batchSz: batchSz, assets: assets, metrics: m}
   }

// Run 阻塞直到 ctx 取消。

  func (w *DriftPatrolWorker) Run(ctx context.Context) {
	   ticker := time.NewTicker(w.interval)
	   defer ticker.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			  return
		  case <-ticker.C:
			   w.patrolOnce(ctx)
		   }
	   }
  }

  func (w *DriftPatrolWorker) patrolOnce(ctx context.Context) {
	  w.metrics.safePatrolled()
	  assets := w.assets
	  if len(assets) == 0 {
		  // 通过 ObserveAll 获取当前有快照的全部资产。
		   snaps, err := w.svc.observer.ObserveAll(ctx)
		   if err != nil {
			  log.Printf("[hotpregate patrol] observe all error: %v", err)
			   return
		   }
		  for _, s := range snaps {
			  assets = append(assets, s.Asset)
		   }
	   }
	   for _, asset := range assets {
		  w.patrolAsset(ctx, asset)
	   }
  }

  func (w *DriftPatrolWorker) patrolAsset(ctx context.Context, asset string) {
	  leases,
		  err := w.svc.store.ListActiveLeasesByAsset(ctx, asset)
	  if err != nil {
		  log.Printf("[hotpregate patrol] list active leases asset=%s: %v", asset, err)
		   return
	   }
	  if len(leases) == 0 {
		   return
	   }
	   // 只 Observe 一次，复用于该资产所有活跃租约。
	  now := w.svc.nowFn()
	   snap, err := w.svc.observer.Observe(ctx, asset)
	  if err != nil {
		  log.Printf("[hotpregate patrol] observe asset=%s: %v", asset, err)
		  return
	  }
	  for i := range leases {
		   lease := leases[i]
		   if lease.IsExpired(now) {
			  continue
		  }
		   recheckDigest := ComputeDigest(snap, lease.BandSnapshot, now)
		   if DigestMatch(lease.Digest, recheckDigest) {
			  continue // 无漂移
		   }
		  // 漂移：签发证明并升级租约。
		   severity := ClassifyDrift(lease.Digest, recheckDigest, lease.BandSnapshot)
		   reason := fmt.Sprintf("[patrol] %s", buildDriftReason(lease.Digest, recheckDigest, severity))
		   proof := InvDriftProof{
			  ProofID: newProofID(),
			   LeaseID: lease.LeaseID,
			  Asset:  lease.Asset,
			  Window: lease.Window,
			  SnapshotDigestAtIssue: lease.Digest,
			   SnapshotDigestAtRecheck: recheckDigest,
			  HotAvailDelta:           HotAvailDelta(lease.Digest, recheckDigest),
			   WarmAvailDelta: WarmAvailDelta(lease.Digest, recheckDigest),
			   DigestMismatch: true,
			  Severity: severity,
			   Reason:    reason,
			  CreatedAt: now,
		   }
		   if err := w.svc.store.InsertProof(ctx, proof); err != nil {
			  log.Printf("[hotpregate patrol] insert proof lease=%s: %v", lease.LeaseID, err)
			   continue
		   }
		   if err := w.svc.store.SetLeaseDriftProof(ctx, lease.LeaseID, proof.ProofID, now); err != nil {
			  log.Printf("[hotpregate patrol] set drift proof lease=%s: %v", lease.LeaseID, err)
			   continue
		   }
		   w.metrics.safeDriftDetected()
		   log.Printf("[hotpregate patrol] drift detected lease=%s asset=%s severity=%s delta=%s",
			  lease.LeaseID, asset, severity, proof.HotAvailDelta)
	   }
   }

// noopFn 防止 metrics 回调为 nil 时 panic。

var _ = noopFn
