package feepathgate

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	defaultScanInterval = 30 * time.Second
	defaultProofInterval = 60 * time.Second
	defaultEscalateInterval = 120 * time.Second
	// escalateThreshold 超过此时长未 resolved 的 high severity DualHit 触发升级。
	escalateThreshold = 10 * time.Minute
)

// AlertHook 告警回调接口；接入 Kafka / Webhook / SMS 等。
// nil 时退化为日志输出。

type AlertHook interface {
	Fire(ctx context.Context, event AlertEvent) error
}

// AlertEvent 告警事件负载。
type AlertEvent struct {
	Kind        string `json:"kind"`
	BusinessKey string `json:"business_key"`
	DualHitID   string `json:"dual_hit_id,omitempty"`
	Severity string `json:"severity,omitempty"`
	At time.Time `json:"at"`
	Detail string `json:"detail"`
}

// WorkerConfig 配置注入（零值使用默认值）。

type WorkerConfig struct {
	ScanInterval      time.Duration
	ProofInterval     time.Duration
	EscalateInterval  time.Duration
	EscalateThreshold time.Duration
	Alert AlertHook
	// ScanBatchSize 每次扫描最多处理的租约数量，0 表示不限制。
	ScanBatchSize int
}

  func (c WorkerConfig) scanInterval() time.Duration {
	  if c.ScanInterval > 0 {
		   return c.ScanInterval
	  }
	   return defaultScanInterval
   }

   func (c WorkerConfig) proofInterval() time.Duration {
	   if c.ProofInterval > 0 {
		   return c.ProofInterval
	  }
	   return defaultProofInterval
   }

   func (c WorkerConfig) escalateInterval() time.Duration {
	   if c.EscalateInterval > 0 {
		   return c.EscalateInterval
	  }
	  return defaultEscalateInterval
  }

   func (c WorkerConfig) escalateThreshold() time.Duration {
	  if c.EscalateThreshold > 0 {
		   return c.EscalateThreshold
	  }
	   return escalateThreshold
   }

// Worker 管理三个后台 goroutine 的生命周期。
type Worker struct {
	svc *Service
	cfg WorkerConfig
	log func(format string, args ...any)
	mu sync.Mutex
	running bool
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// NewWorker 构造 Worker；log 为日志回调，nil 时静默。

  func NewWorker(svc *Service, cfg WorkerConfig, log func(string, ...any)) *Worker {
	  if log == nil {
		   log = func(string, ...any) {}
	}
	return &Worker{svc: svc, cfg: cfg, log: log}
}

// Start 启动三个后台 goroutine；幂等，重复调用无效。
   func (w *Worker) Start(ctx context.Context) {
	  w.mu.Lock()
	  defer w.mu.Unlock()
	  if w.running {
		   return
	   }
	  runCtx,
		   cancel := context.WithCancel(ctx)
	  w.cancel = cancel
	  w.running = true
	  w.wg.Add(3)
	  go w.runScanner(runCtx)
	   go w.runSolidifier(runCtx)
	  go w.runEscalator(runCtx)
   }

// Stop 优雅停止所有 goroutine，等待退出。
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

// ──────────────────────────────────────────────────────────────────────────────
// DualHitScanner goroutine
// ──────────────────────────────────────────────────────────────────────────────

   func (w *Worker) runScanner(ctx context.Context) {
	   defer w.wg.Done()
	   ticker := time.NewTicker(w.cfg.scanInterval())
	   defer ticker.Stop()
	  for {
		   select {
		   case <-ctx.Done():
			   return
		  case <-ticker.C:
			   if err := w.scanOnce(ctx); err != nil {
				   w.log("[feepathgate/worker] scanner error: %v", err)
			  }
		   }
	   }
  }

// scanOnce 扫描一轮 bound 状态的租约对，发现双贷。

   func (w *Worker) scanOnce(ctx context.Context) error {
	  // 从 Store 获取所有 bound 状态租约（批量，按 biz_key 分组）。
	  limit := w.cfg.ScanBatchSize
	   if limit <= 0 {
		   limit = 200
	  }
	   boundLeases, err := w.svc.store.ListLeasesByStatus(ctx, LeaseBound, limit)
	   if err != nil {
		   return fmt.Errorf("list bound leases: %w", err)
	  }
	   // 按 business_key 分组。
	  byBiz := make(map[string][]*PathLease)
	   for _, l := range boundLeases {
		   byBiz[l.BusinessKey] = append(byBiz[l.BusinessKey], l)
	   }
	   for bizKey, group := range byBiz {
		  var takeL, rebateL *PathLease
		  for _, l := range group {
			   switch l.Kind {
			  case KindTake:
				  takeL = l
			  case KindRebate:
				   rebateL = l
			  }
		   }
		   if takeL == nil || rebateL == nil {
			   continue
		   }
		   hit, err := w.svc.detect.Scan(ctx, takeL, rebateL)
		   if err != nil {
			   w.log("[feepathgate/worker] scan biz=%s error: %v", bizKey, err)
			   continue
		   }
		  if hit != nil {
			  w.svc.metrics.dualHitDetected.Add(1)
			   w.log("[feepathgate/worker] dual_hit detected biz=%s id=%s severity=%s",
				  bizKey, hit.ID, hit.Severity)
			   // 将两条租约都标记为冲突状态。
			  for _, l := range group {
				  if _, merr := w.svc.fsm.MarkConflicted(ctx, l.ID); merr != nil && merr != ErrLeaseNotFound {
					   w.log("[feepathgate/worker] mark_conflicted lease=%s err=%v", l.ID, merr)
				  }
			   }
		   }
	   }
	   return nil
   }

// ──────────────────────────────────────────────────────────────────────────────
// ProofSolidifier goroutine
// ──────────────────────────────────────────────────────────────────────────────

   func (w *Worker) runSolidifier(ctx context.Context) {
	  defer w.wg.Done()
	   ticker := time.NewTicker(w.cfg.proofInterval())
	   defer ticker.Stop()
	   for {
		   select {
		  case <-ctx.Done():
			  return
		   case <-ticker.C:
			   if err := w.solidifyOnce(ctx); err != nil {
				   w.log("[feepathgate/worker] solidifier error: %v", err)
			  }
		  }
	  }
   }

// solidifyOnce 扫描 settled 状态的租约对，尝试构建并验证对冲证明。
// settled 状态租约可能尚未有对应 HedgeProof（首次固化）。

   func (w *Worker) solidifyOnce(ctx context.Context) error {
	  limit := w.cfg.ScanBatchSize
	  if limit <= 0 {
		  limit = 100
	  }
	  settledLeases, err := w.svc.store.ListLeasesByStatus(ctx, LeaseSettled, limit)
	  if err != nil {
		   return fmt.Errorf("list settled leases: %w", err)
	   }
	  // 找到没有 ProofID 的 settled 租约对，尝试建立证明。
	   byBiz := make(map[string][]*PathLease)
	   for _, l := range settledLeases {
		   if l.ProofID == "" {
			   byBiz[l.BusinessKey] = append(byBiz[l.BusinessKey], l)
		   }
	  }
	   for bizKey, group := range byBiz {
		  var takeL, rebateL *PathLease
		   for _, l := range group {
			   switch l.Kind {
			   case KindTake:
				  takeL = l
			   case KindRebate:
				  rebateL = l
			  }
		  }
		   if takeL == nil || rebateL == nil {
			   // 尝试从 bound 状态补齐另一侧。
			  boundGroup, _ := w.svc.store.ListLeasesByStatus(ctx, LeaseBound, 20)
			   for _, bl := range boundGroup {
				  if bl.BusinessKey != bizKey {
					   continue
				  }
				  if bl.Kind == KindTake && takeL == nil {
					   takeL = bl
				  }
				   if bl.Kind == KindRebate && rebateL == nil {
					  rebateL = bl
				   }
			   }
		  }
		  if takeL == nil || rebateL == nil {
			   continue
		  }
		  proof, err := w.svc.vault.Build(ctx, takeL, rebateL)
		   if err != nil {
			   w.log("[feepathgate/worker] build_proof biz=%s err=%v", bizKey, err)
			  continue
		  }
		   if proof.Status == HedgePending {
			   proof, err = w.svc.vault.Verify(ctx, proof.ID)
			  if err != nil {
				  w.log("[feepathgate/worker] verify_proof id=%s err=%v", proof.ID, err)
				   continue
			  }
		  }
		   w.log("[feepathgate/worker] proof_solidified biz=%s proof=%s", bizKey, proof.ID)
	  }
	  return nil
   }

// ──────────────────────────────────────────────────────────────────────────────
// ConflictEscalator goroutine
// ──────────────────────────────────────────────────────────────────────────────

   func (w *Worker) runEscalator(ctx context.Context) {
	  defer w.wg.Done()
	  ticker := time.NewTicker(w.cfg.escalateInterval())
	  defer ticker.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			  return
		   case <-ticker.C:
			   if err := w.escalateOnce(ctx); err != nil {
				   w.log("[feepathgate/worker] escalator error: %v", err)
			  }
		   }
	   }
   }

// escalateOnce 遍历所有 open 状态 DualHit；高危且超时未处理时触发告警。
  func (w *Worker) escalateOnce(ctx context.Context) error {
	   hits,
		   err := w.svc.detect.ListOpen(ctx)
	   if err != nil {
		   return fmt.Errorf("list open dual_hits: %w", err)
	  }
	   threshold := w.cfg.escalateThreshold()
	  now := w.svc.clock()
	   for _, h := range hits {
		  if h.Severity != "high" {
			   continue
		  }
		   age := now.Sub(h.DetectedAt)
		  if age < threshold {
			  continue
		  }
		   evt := AlertEvent{
			   Kind:        "dual_hit_escalated",
			   BusinessKey: h.BusinessKey,
			  DualHitID:   h.ID,
			  Severity:    h.Severity,
			  At:          now,
			   Detail: fmt.Sprintf("dual_hit age=%.0fs exceeds threshold=%.0fs; take_ledger=%s rebate_ledger=%s",
				  age.Seconds(), threshold.Seconds(),
				   h.TakeLedgerID,
				   h.RebateLedgerID),
		   }
		  if w.cfg.Alert != nil {
			   if aerr := w.cfg.Alert.Fire(ctx, evt); aerr != nil {
				  w.log("[feepathgate/worker] alert_fire err=%v", aerr)
			   }
		   } else {
			  w.log("[feepathgate/worker] ALERT %s biz=%s hit=%s age=%s",
				  evt.Kind, evt.BusinessKey, evt.DualHitID, age.Round(time.Second))
		  }
	  }
	   return nil
  }
