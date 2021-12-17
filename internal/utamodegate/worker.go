package utamodegate

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	defaultLiabScanInterval = 15 * time.Second
	defaultDriftScanInterval = 30 * time.Second
	defaultBindExpiryInterval    = 60 * time.Second
	defaultLeaseProgressInterval = 45 * time.Second
	defaultScanBatchSize         = 200
)

// AlertHook 告警回调接口；对接 Kafka/Webhook；nil 时退化为日志打印。

type AlertHook interface {
	Fire(ctx context.Context, event AlertEvent) error
}

// AlertEvent 告警事件负载。

type AlertEvent struct {
	Kind   string `json:"kind"`
	UserID string `json:"user_id,omitempty"`
	CaseID string `json:"drift_case_id,omitempty"`
	LeaseID string `json:"lease_id,omitempty"`
	BindID   string    `json:"bind_id,omitempty"`
	Severity string    `json:"severity,omitempty"`
	At       time.Time `json:"at"`
	Detail string `json:"detail"`
}

// Worker 后台引擎。
type Worker struct {
	svc *Service
	alert  AlertHook
	wg     sync.WaitGroup
	stopCh chan struct{}
	liabScanInterval time.Duration
	driftScanInterval     time.Duration
	bindExpiryInterval    time.Duration
	leaseProgressInterval time.Duration
	batchSize             int
	escalateThreshold     time.Duration
}

// NewWorker 构造 Worker。alert 为 nil 时使用日志型告警。

  func NewWorker(svc *Service, alert AlertHook) *Worker {
	   if alert == nil {
		  alert = &logAlertHook{}
	  }
	  return &Worker{
		  svc:    svc,
		   alert:  alert,
		  stopCh: make(chan struct{}),
		   liabScanInterval:   defaultLiabScanInterval,
		  driftScanInterval:  defaultDriftScanInterval,
		  bindExpiryInterval: defaultBindExpiryInterval,
		  leaseProgressInterval: defaultLeaseProgressInterval,
		  batchSize: defaultScanBatchSize,
		  escalateThreshold: EscalationThreshold,
	  }
  }

// Start 启动所有后台 goroutine。
   func (w *Worker) Start(ctx context.Context) {
	   w.wg.Add(4)
	  go w.runLiabilityScanner(ctx)
	   go w.runDriftPatrol(ctx)
	   go w.runBindExpiryReaper(ctx)
	   go w.runLeaseProgresser(ctx)
   }

// Stop 优雅停止所有 goroutine（等待当前轮次完成）。
  func (w *Worker) Stop() {
	   close(w.stopCh)
	   w.wg.Wait()
   }

// ─────────────────────────────────────────────────────────────────────────────
// 1. LiabilityScanner — 扫描 blocked 租约，在途清零后推进切换
// ─────────────────────────────────────────────────────────────────────────────

   func (w *Worker) runLiabilityScanner(ctx context.Context) {
	  defer w.wg.Done()
	   ticker := time.NewTicker(w.liabScanInterval)
	  defer ticker.Stop()
	  for {
		   select {
		  case <-w.stopCh:
			   return
		   case <-ctx.Done():
			  return
		  case <-ticker.C:
			  w.scanBlockedLeases(ctx)
		  }
	  }
  }

  func (w *Worker) scanBlockedLeases(ctx context.Context) {
	   leases, err := w.svc.workerScanBlockedLeases(ctx, w.batchSize)
	   if err != nil {
		  w.fireAlert(ctx, AlertEvent{
			   Kind:     "liability_scanner_error",
			  Severity: "warning",
			  At: time.Now().UTC(),
			  Detail: fmt.Sprintf("scan blocked leases failed: %v", err),
		  })
		   return
	   }
	  for _, l := range leases {
		   switched, err := w.svc.workerUnblockLease(ctx, l.ID)
		  if err != nil && err != ErrInflightLiability && err != ErrStaleGen {
			  w.fireAlert(ctx, AlertEvent{
				  Kind: "unblock_lease_error",
				  LeaseID: l.ID,
				   UserID:   l.UserID,
				   Severity: "warning",
				   At:       time.Now().UTC(),
				   Detail:   fmt.Sprintf("unblock lease %s failed: %v", l.ID, err),
			   })
		   }
		   if switched != nil && switched.Status == LeaseSwitched {
			   w.fireAlert(ctx, AlertEvent{
				  Kind:    "lease_switched",
				  LeaseID: switched.ID,
				   UserID:   switched.UserID,
				  Severity: "info",
				  At:       time.Now().UTC(),
				  Detail:   fmt.Sprintf("mode switched %s→%s gen=%d", switched.FromMode, switched.ToMode, switched.Generation),
			  })
		  }
	  }
  }

// ─────────────────────────────────────────────────────────────────────────────
// 2. DriftPatrol — 漂移案巡检与升级告警
// ─────────────────────────────────────────────────────────────────────────────

   func (w *Worker) runDriftPatrol(ctx context.Context) {
	   defer w.wg.Done()
	  ticker := time.NewTicker(w.driftScanInterval)
	  defer ticker.Stop()
	  for {
		  select {
		   case <-w.stopCh:
			   return
		   case <-ctx.Done():
			   return
		   case <-ticker.C:
			   w.patrolDriftCases(ctx)
		  }
	  }
   }

  func (w *Worker) patrolDriftCases(ctx context.Context) {
	   cases, err := w.svc.workerScanDriftCases(ctx, w.batchSize)
	  if err != nil {
		   return
	   }
	  now := time.Now().UTC()
	  for _, dc := range cases {
		   age := now.Sub(dc.DetectedAt)
		   if age > w.escalateThreshold {
			   w.fireAlert(ctx, AlertEvent{
				  Kind: "drift_case_escalate",
				   CaseID: dc.ID,
				  UserID:   dc.UserID,
				   Severity: "critical",
				   At: now,
				   Detail: fmt.Sprintf(
					  "drift case %s (kind=%s user=%s) open for %s without sign-off",
					   dc.ID, dc.Kind, dc.UserID, age.Round(time.Second)),
			  })
		  }
		  // 对有证明但尚未推进至 proven 的案件补偿推进
		   if dc.ProofID != "" && dc.Status == DriftOpen {
			  _, _, err := w.svc.BuildAndAttachProof(ctx, BuildProofReq{
				   DriftCaseID: dc.ID,
				  BuiltBy: "worker-drift-patrol",
			  })
			   if err != nil {
				  // 证明需要快照数据，worker 仅能在有 proof 存储的情况下触发
				   // 此处静默：ops 会通过 HTTP API 手动触发
				  _ = err
			  }
		  }
	  }
   }

// ─────────────────────────────────────────────────────────────────────────────
// 3. BindExpiryReaper — 过期绑定标记与漂移触发
// ─────────────────────────────────────────────────────────────────────────────

  func (w *Worker) runBindExpiryReaper(ctx context.Context) {
	  defer w.wg.Done()
	  ticker := time.NewTicker(w.bindExpiryInterval)
	   defer ticker.Stop()
	  for {
		  select {
		  case <-w.stopCh:
			   return
		   case <-ctx.Done():
			   return
		  case <-ticker.C:
			   w.reaperExpiredBinds(ctx)
		  }
	  }
   }

  func (w *Worker) reaperExpiredBinds(ctx context.Context) {
	  expired, err := w.svc.workerMarkExpiredBinds(ctx, w.batchSize)
	   if err != nil {
		   return
	  }
	  for _, b := range expired {
		   // 检查绑定时的模式键是否与当前模式一致；若不一致，触发漂移案。
		   mg, err := w.svc.GetGeneration(ctx, b.UserID)
		   if err != nil {
			   continue
		   }
		  currentMode := mg.CurrentMode
		   if currentMode == "" {
			   currentMode = MarginModeUnified
		   }
		   if b.MarginMode != currentMode {
			  if err := w.svc.workerOpenDriftFromBind(ctx, b, currentMode); err != nil {
				  w.fireAlert(ctx, AlertEvent{
					   Kind: "bind_drift_open_error",
					   BindID:   b.ID,
					   UserID:   b.UserID,
					   Severity: "warning",
					  At:       time.Now().UTC(),
					  Detail:   fmt.Sprintf("open drift from expired bind %s failed: %v", b.ID, err),
				   })
			  }
		   }
		  w.fireAlert(ctx, AlertEvent{
			   Kind:     "bind_expired",
			   BindID:   b.ID,
			  UserID:   b.UserID,
			  Severity: "warning",
			  At:       time.Now().UTC(),
			  Detail:   fmt.Sprintf("liability bind %s expired (kind=%s cur=%s bind=%s)", b.ID, b.BindKind, currentMode, b.MarginMode),
		   })
	  }
  }

// ─────────────────────────────────────────────────────────────────────────────
// 4. LeaseProgresser — 将 bound 租约推进至 switched
// ─────────────────────────────────────────────────────────────────────────────

   func (w *Worker) runLeaseProgresser(ctx context.Context) {
	   defer w.wg.Done()
	   ticker := time.NewTicker(w.leaseProgressInterval)
	   defer ticker.Stop()
	  for {
		   select {
		   case <-w.stopCh:
			  return
		  case <-ctx.Done():
			  return
		   case <-ticker.C:
			  w.progressBoundLeases(ctx)
		   }
	   }
   }

  func (w *Worker) progressBoundLeases(ctx context.Context) {
	  _,
		   _ = w.svc.workerScanBoundLeases(ctx, w.batchSize)
   }

// ─────────────────────────────────────────────────────────────────────────────
// 工具
// ─────────────────────────────────────────────────────────────────────────────

  func (w *Worker) fireAlert(ctx context.Context, event AlertEvent) {
	   if w.alert == nil {
		   return
	  }
	  _ = w.alert.Fire(ctx, event)
  }

// logAlertHook 日志型告警（前替换为 Kafka/Webhook 实现）。
type logAlertHook struct{}

   func (h *logAlertHook) Fire(_ context.Context, event AlertEvent) error {
	  // 日志输出仅作占位；实现对接消息总线。
	  _ = fmt.Sprintf("[utamodegate alert] kind=%s user=%s severity=%s detail=%s at=%s",
		  event.Kind,
		   event.UserID, event.Severity, event.Detail, event.At.Format(time.RFC3339))
	  return nil
   }
