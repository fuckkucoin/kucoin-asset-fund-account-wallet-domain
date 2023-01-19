package viptiergate

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	defaultFreezeScanInterval = 15 * time.Second
	defaultDriftScanInterval   = 30 * time.Second
	defaultQuoteExpiryInterval = 60 * time.Second
	defaultLeaseProgressInterval = 45 * time.Second
	defaultEscalateThreshold     = 10 * time.Minute
	defaultScanBatchSize = 200
)

// AlertHook 告警回调接口；对接 Kafka/Webhook；nil 时退化为日志输出。
type AlertHook interface {
	Fire(ctx context.Context, event AlertEvent) error
}

// AlertEvent 告警事件负载。
type AlertEvent struct {
	Kind   string `json:"kind"`
	UserID string `json:"user_id,omitempty"`
	CaseID  string `json:"drift_case_id,omitempty"`
	LeaseID string `json:"lease_id,omitempty"`
	Severity string    `json:"severity,omitempty"`
	At       time.Time `json:"at"`
	Detail   string    `json:"detail"`
}

// WorkerConfig 配置注入（零值使用内置默认值）。
type WorkerConfig struct {
	FreezeScanInterval  time.Duration
	DriftScanInterval   time.Duration
	QuoteExpiryInterval time.Duration
	LeaseProgressInterval time.Duration
	EscalateThreshold time.Duration
	ScanBatchSize     int
	Alert AlertHook
}

  func (c WorkerConfig) freezeScanInterval() time.Duration {
	   if c.FreezeScanInterval > 0 {
		   return c.FreezeScanInterval
	  }
	   return defaultFreezeScanInterval
  }

  func (c WorkerConfig) driftScanInterval() time.Duration {
	   if c.DriftScanInterval > 0 {
		  return c.DriftScanInterval
	   }
	  return defaultDriftScanInterval
   }

   func (c WorkerConfig) quoteExpiryInterval() time.Duration {
	  if c.QuoteExpiryInterval > 0 {
		  return c.QuoteExpiryInterval
	  }
	   return defaultQuoteExpiryInterval
  }

  func (c WorkerConfig) leaseProgressInterval() time.Duration {
	   if c.LeaseProgressInterval > 0 {
		  return c.LeaseProgressInterval
	  }
	  return defaultLeaseProgressInterval
  }

   func (c WorkerConfig) escalateThreshold() time.Duration {
	  if c.EscalateThreshold > 0 {
		  return c.EscalateThreshold
	   }
	   return defaultEscalateThreshold
   }

  func (c WorkerConfig) scanBatch() int {
	  if c.ScanBatchSize > 0 {
		  return c.ScanBatchSize
	  }
	   return defaultScanBatchSize
   }

// Worker 管理四个后台 goroutine 的生命周期。

type Worker struct {
	svc *Service
	cfg WorkerConfig
	log func(format string, args ...any)
	mu      sync.Mutex
	running bool
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewWorker 构造 Worker；log 为日志回调，nil 时静默。

  func NewWorker(svc *Service, cfg WorkerConfig, log func(string, ...any)) *Worker {
	  if log == nil {
		   log = func(string, ...any) {}
	}
	return &Worker{svc: svc, cfg: cfg, log: log}
}

// Start 启动四个后台 goroutine；幂等，重复调用无效。

   func (w *Worker) Start(ctx context.Context) {
	  w.mu.Lock()
	   defer w.mu.Unlock()
	   if w.running {
		  return
	   }
	   runCtx, cancel := context.WithCancel(ctx)
	  w.cancel = cancel
	  w.running = true
	  w.wg.Add(4)
	   go w.runFreezeScanner(runCtx)
	  go w.runDriftPatrol(runCtx)
	   go w.runQuoteExpiryReaper(runCtx)
	  go w.runLeaseProgresser(runCtx)
   }

// Stop 优雅停止所有 goroutine，等待退出完成。

   func (w *Worker) Stop() {
	  w.mu.Lock()
	   cancel := w.cancel
	  w.running = false
	  w.mu.Unlock()
	   if cancel != nil {
		  cancel()
	  }
	   w.wg.Wait()
   }

// ─────────────────────────────────────────────────────────────────────────────
// FreezeScanner goroutine — 扫描 blocked 租约，冻额/Quote 清零后推进切换
// ─────────────────────────────────────────────────────────────────────────────

   func (w *Worker) runFreezeScanner(ctx context.Context) {
	   defer w.wg.Done()
	  ticker := time.NewTicker(w.cfg.freezeScanInterval())
	   defer ticker.Stop()
	   for {
		   select {
		   case <-ctx.Done():
			  return
		  case <-ticker.C:
			  if err := w.freezeScanOnce(ctx); err != nil {
				   w.log("[viptiergate/worker] freeze_scan error: %v", err)
			   }
		  }
	  }
   }

// freezeScanOnce 扫描一轮 blocked 租约，尝试推进。

  func (w *Worker) freezeScanOnce(ctx context.Context) error {
	  leases, err := w.svc.fsm.ListBlocked(ctx, w.cfg.scanBatch())
	   if err != nil {
		   return fmt.Errorf("list blocked leases: %w", err)
	  }
	   for _, lease := range leases {
		   w.tryUnblockLease(ctx, lease)
	  }
	  return nil
   }

// tryUnblockLease 检查 blocked 租约所属用户的冻额/Quote 状态，
// 若均已清零则推进 blocked → open → bound → switched。

   func (w *Worker) tryUnblockLease(ctx context.Context, lease *TierLease) {
	   // 检查活跃冻额。
	  freezeCnt, err := w.svc.freeze.CountActive(ctx, lease.UserID)
	  if err != nil {
		  w.log("[viptiergate/worker] count_active_freeze user=%s err=%v", lease.UserID, err)
		   return
	   }
	  // 检查活跃 QuoteBind。
	   quoteCnt, err := w.svc.quotes.CountActive(ctx, lease.UserID)
	  if err != nil {
		   w.log("[viptiergate/worker] count_active_quote user=%s err=%v", lease.UserID, err)
		  return
	   }
	   if freezeCnt > 0 || quoteCnt > 0 {
		   // 仍有阻塞源，等待下次扫描。
		   return
	   }
	   // 推进：blocked → open。
	   if _, err := w.svc.fsm.Unblock(ctx, lease.ID); err != nil {
		  w.log("[viptiergate/worker] unblock lease=%s err=%v", lease.ID, err)
		  return
	   }
	   // open → bound。
	   if _, err := w.svc.fsm.Bind(ctx, BindLeaseReq{
		  LeaseID: lease.ID, Generation: lease.Generation}); err != nil {
		  w.log("[viptiergate/worker] bind lease=%s err=%v", lease.ID, err)
		  return
	   }
	   // bound → switched。
	   if _, err := w.svc.fsm.Switch(ctx, lease.ID); err != nil {
		  w.log("[viptiergate/worker] switch lease=%s err=%v", lease.ID, err)
		  return
	  }
	  w.svc.metrics.leaseSwitched.Add(1)
	  w.log("[viptiergate/worker] lease_switched lease=%s user=%s %s→%s",
		  lease.ID, lease.UserID, lease.FromTier, lease.ToTier)
  }

// ─────────────────────────────────────────────────────────────────────────────
// DriftPatrol goroutine — 漂移案巡检与高危升级
// ─────────────────────────────────────────────────────────────────────────────

  func (w *Worker) runDriftPatrol(ctx context.Context) {
	   defer w.wg.Done()
	   ticker := time.NewTicker(w.cfg.driftScanInterval())
	   defer ticker.Stop()
	   for {
		   select {
		   case <-ctx.Done():
			  return
		   case <-ticker.C:
			   if err := w.driftPatrolOnce(ctx); err != nil {
				  w.log("[viptiergate/worker] drift_patrol error: %v", err)
			  }
		  }
	  }
  }

   func (w *Worker) driftPatrolOnce(ctx context.Context) error {
	  cases, err := w.svc.store.ListAllOpenDriftCases(ctx, w.cfg.scanBatch())
	   if err != nil {
		  return fmt.Errorf("list open drift cases: %w", err)
	  }
	   threshold := w.cfg.escalateThreshold()
	   now := w.svc.clock()
	   for _, dc := range cases {
		   // 若案件尚无证明，尝试自动构建（仅当数据完整时）。
		   if dc.ProofID == "" && dc.Status == DriftOpen {
			  if p, err := w.svc.proof.Build(ctx, dc.ID); err == nil {
				   if verified, verr := w.svc.proof.Verify(ctx, p.ID); verr == nil {
					   if _, aerr := w.svc.drift.AttachProof(ctx, dc.ID, verified.ID); aerr == nil {
						  w.svc.metrics.proofVerified.Add(1)
						  w.log("[viptiergate/worker] proof_auto_attached case=%s proof=%s",
							   dc.ID, verified.ID)
					  }
				  }
			   }
		  }
		  // 超时升级告警：open/proven 超过阈值未收口。
		  age := now.Sub(dc.DetectedAt)
		   if age >= threshold {
			   evt := AlertEvent{
				  Kind:   "drift_case_unresolved",
				   UserID: dc.UserID,
				   CaseID: dc.ID,
				  Severity: "high",
				   At: now,
				  Detail: fmt.Sprintf(
					   "drift_case age=%.0fs exceeds threshold=%.0fs; kind=%s asset=%s observed_bps=%d expected_bps=%d",
					  age.Seconds(), threshold.Seconds(),
					   dc.Kind, dc.Asset, dc.ObservedBPS, dc.ExpectedBPS),
			  }
			  if w.cfg.Alert != nil {
				  if aerr := w.cfg.Alert.Fire(ctx, evt); aerr != nil {
					  w.log("[viptiergate/worker] alert_fire err=%v", aerr)
				  }
			   } else {
				  w.log("[viptiergate/worker] ALERT %s case=%s user=%s age=%s",
					  evt.Kind, dc.ID, dc.UserID, age.Round(time.Second))
			   }
		  }
	   }
	   // 扫描在途冻额漂移：为世代不符的活跃冻额开立 DriftFreezeStale 案件。
	   if err := w.scanFreezesDrift(ctx); err != nil {
		   w.log("[viptiergate/worker] scan_freezes_drift error: %v", err)
	  }
	   return nil
   }

// scanFreezesDrift 遍历所有活跃冻额，按用户分组做世代，
// 对每个世代不符的冻额调用 DriftDetector.ScanFreezeForDrift。
  func (w *Worker) scanFreezesDrift(ctx context.Context) error {
	   // 获取所有存在活跃冻额的用户列表（通过查询 blocked 租约近似代替）。
	  blockedLeases, err := w.svc.fsm.ListBlocked(ctx, w.cfg.scanBatch())
	  if err != nil {
		  return err
	  }
	   seen := make(map[string]bool)
	  for _, l := range blockedLeases {
		  if seen[l.UserID] {
			  continue
		   }
		  seen[l.UserID] = true
		  w.tryDetectFreezeForUser(ctx, l.UserID)
	  }
	  return nil
  }

   func (w *Worker) tryDetectFreezeForUser(ctx context.Context, userID string) {
	   currentGen, err := w.svc.store.CurrentGeneration(ctx, userID)
	   if err != nil {
		   return
	  }
	   genRecord, _ := w.svc.store.GetGeneration(ctx, userID)
	   currentTier := TierStandard
	  if genRecord != nil {
		   currentTier = genRecord.LastTier
	  }
	   actives, err := w.svc.freeze.ListActive(ctx, userID)
	   if err != nil || len(actives) == 0 {
		  return
	   }
	  drifts,
		  err := w.svc.drift.ScanFreezeForDrift(ctx, userID, actives,
		  currentGen, currentTier, currentTier, 0)
	   if err != nil {
		  w.log("[viptiergate/worker] scan_freeze_drift user=%s err=%v", userID, err)
		  return
	   }
	   for _, dc := range drifts {
		   w.svc.metrics.driftCasesOpened.Add(1)
		  w.log("[viptiergate/worker] drift_case_opened case=%s user=%s kind=%s",
			  dc.ID, userID, dc.Kind)
	   }
   }

// ─────────────────────────────────────────────────────────────────────────────
// QuoteExpiryReaper goroutine — TTL 到期 QuoteBind 清理
// ─────────────────────────────────────────────────────────────────────────────

   func (w *Worker) runQuoteExpiryReaper(ctx context.Context) {
	  defer w.wg.Done()
	   ticker := time.NewTicker(w.cfg.quoteExpiryInterval())
	   defer ticker.Stop()
	   for {
		   select {
		   case <-ctx.Done():
			   return
		   case <-ticker.C:
			  if err := w.quoteExpiryOnce(ctx); err != nil {
				   w.log("[viptiergate/worker] quote_expiry error: %v", err)
			  }
		   }
	  }
   }

   func (w *Worker) quoteExpiryOnce(ctx context.Context) error {
	  n, err := w.svc.quotes.ExpireStale(ctx, w.cfg.scanBatch())
	  if err != nil {
		  return fmt.Errorf("expire stale quotes: %w", err)
	   }
	  if n > 0 {
		   w.svc.metrics.quotesExpired.Add(int64(n))
		  w.log("[viptiergate/worker] expired_quotes count=%d", n)
	  }
	  return nil
   }

// ─────────────────────────────────────────────────────────────────────────────
// LeaseProgresser goroutine — bound 租约推进至 switched 终态
// ─────────────────────────────────────────────────────────────────────────────

  func (w *Worker) runLeaseProgresser(ctx context.Context) {
	   defer w.wg.Done()
	  ticker := time.NewTicker(w.cfg.leaseProgressInterval())
	  defer ticker.Stop()
	   for {
		   select {
		  case <-ctx.Done():
			   return
		  case <-ticker.C:
			  if err := w.leaseProgressOnce(ctx); err != nil {
				  w.log("[viptiergate/worker] lease_progress error: %v", err)
			   }
		   }
	   }
  }

  func (w *Worker) leaseProgressOnce(ctx context.Context) error {
	  boundLeases, err := w.svc.store.ListLeasesByStatus(ctx, LeaseBound, w.cfg.scanBatch())
	  if err != nil {
		  return fmt.Errorf("list bound leases: %w", err)
	  }
	   for _, lease := range boundLeases {
		  // 确认该用户没有活跃的旧世代 QuoteBind（活跃且世代 == lease.Generation-1）。
		   activeQuotes,
			  qerr := w.svc.quotes.CountActive(ctx, lease.UserID)
		  if qerr != nil {
			  continue
		  }
		   if activeQuotes > 0 {
			   // 仍有未结算的报价，等待。
			  continue
		  }
		   // 推进至 switched。
		   if _, serr := w.svc.fsm.Switch(ctx, lease.ID); serr == nil {
			   w.svc.metrics.leaseSwitched.Add(1)
			   w.log("[viptiergate/worker] lease_progressed lease=%s user=%s gen=%d",
				   lease.ID, lease.UserID, lease.Generation)
		   }
	   }
	   return nil
   }
