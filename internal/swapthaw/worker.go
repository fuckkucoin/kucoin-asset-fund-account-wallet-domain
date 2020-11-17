package swapthaw

import (
	"context"
	"fmt"
	"time"
	"github.com/rs/zerolog"
)

// FrozenOrder 外部注入的 frozen 订单信息（由 swap.Service 提供）。
type FrozenOrder struct {
	OrderID string
	UserID string
	Asset        string
	FrozenAmount string
	CreatedAt    time.Time
	HasTxHash bool
}

// FrozenOrderFetcher 获取当前所有 frozen 状态订单的函数类型（由 swap.Service 注入）。
type FrozenOrderFetcher func(ctx context.Context) ([]*FrozenOrder, error)

// AlertFn 告警回调函数类型（可接入 PagerDuty / 内部告警总线）。

type AlertFn func(ctx context.Context, level, msg string, fields map[string]any)

// Worker 双释互斥引擎后台 Worker 集合。
type Worker struct {
	svc *Service
	fetchFrozen FrozenOrderFetcher
	alert       AlertFn
	log         zerolog.Logger
	// proofTTL 超过此时长仍为 open 的 ThawCase 发出告警。
	proofTTL time.Duration
	// orphanAge 冻结时长超过此阈值且无活跃 Lease 的订单视为孤儿。
	orphanAge time.Duration
}

// NewWorker 构造 Worker。fetchFrozen 由 swap.Service.FetchFrozenOrders 注入。

   func NewWorker(svc *Service, fetchFrozen FrozenOrderFetcher, alert AlertFn, log zerolog.Logger) *Worker {
	   return &Worker{
		  svc:         svc,
		   fetchFrozen: fetchFrozen,
		  alert:     alert,
		   log:       log,
		   proofTTL:  4 * time.Hour,
		  orphanAge: 30 * time.Minute,
	   }
   }

// WithProofTTL 设置 ThawCase 超期未证明告警阈值。

   func (w *Worker) WithProofTTL(d time.Duration) *Worker {
	   w.proofTTL = d
	   return w
   }

// WithOrphanAge 设置孤儿 Frozen 判断阈值。
  func (w *Worker) WithOrphanAge(d time.Duration) *Worker {
	  w.orphanAge = d
	  return w
  }

// ──────────────────────────────────────────────
// Worker 入口（各 Worker 由框架定时调用）
// ──────────────────────────────────────────────
// DoubleReleasePatrol 巡检未关闭 ThawCase，超期未证明则发出 critical 告警。
// 建议调用间隔：5 分钟。

   func (w *Worker) DoubleReleasePatrol(ctx context.Context) error {
	   cases, err := w.svc.ListOpenThawCases(200)
	  if err != nil {
		   return fmt.Errorf("swapthaw: DoubleReleasePatrol.ListOpenThawCases: %w", err)
	  }
	  now := time.Now().UTC()
	   alertCount := 0
	   for _, tc := range cases {
		  age := now.Sub(tc.DetectedAt)
		   if tc.Status == ThawCaseStatusOpen && age > w.proofTTL {
			   alertCount++
			  w.doAlert(ctx, "critical", "ThawCase 超期未证明", map[string]any{
				   "case_id": tc.ID,
				  "order_id": tc.OrderID,
				   "user_id":   tc.UserID,
				  "kind":      string(tc.Kind),
				   "age_hours": age.Hours(),
				   "detected_at": tc.DetectedAt.Format(time.RFC3339),
			   })
			  w.log.Error().
				  Str("case_id", tc.ID).
				  Str("order_id", tc.OrderID).
				  Str("kind", string(tc.Kind)).
				  Dur("age", age).
				   Msg("swapthaw: open ThawCase exceeded proof TTL")
		  } else if tc.Status == ThawCaseStatusProven {
			  // proven 但未 closed，记录 warn。
			  w.log.Warn().
				   Str("case_id", tc.ID).
				  Str("order_id", tc.OrderID).
				  Msg("swapthaw: ThawCase proven but not yet closed")
		  }
	   }
	  w.log.Info().
		  Int("open_cases", len(cases)).
		   Int("alerted", alertCount).
		   Msg("swapthaw: DoubleReleasePatrol done")
	   return nil
   }

// OrphanFrozenScanner 扫描孤儿 Frozen 订单（无活跃 FreezeLease 覆盖），升 ThawCase。
// 建议调用间隔：10 分钟。

   func (w *Worker) OrphanFrozenScanner(ctx context.Context) error {
	  if w.fetchFrozen == nil {
		  return fmt.Errorf("swapthaw: OrphanFrozenScanner: fetchFrozen not set")
	   }
	   orders, err := w.fetchFrozen(ctx)
	   if err != nil {
		  return fmt.Errorf("swapthaw: OrphanFrozenScanner.fetchFrozen: %w", err)
	  }
	  now := time.Now().UTC()
	   orphanCount := 0
	  for _, o := range orders {
		  // 仅检查冻结时长超过阈值的订单。
		  if !o.CreatedAt.IsZero() && now.Sub(o.CreatedAt) < w.orphanAge {
			  continue
		  }
		   // 检查是否存在活跃 FreezeLease。
		  lease, err := w.svc.GetLeaseByOrder(o.OrderID)
		  if err == nil && lease != nil && lease.Status == LeaseStatusOpen {
			   // 存在活跃 Lease，不是孤儿。
			  continue
		  }
		   // 检查是否已有 ThawCase。
		   existing, err := w.svc.GetThawCaseByOrder(o.OrderID)
		  if err == nil && existing != nil {
			  // 已有 ThawCase，跳过重复创建。
			  continue
		   }
		  // 创建孤儿 ThawCase。
		  tc, err := w.svc.detector.DetectOrphan(
			   o.OrderID, o.UserID, "", o.Asset, o.FrozenAmount,
			   o.FrozenAmount, "unknown",
		   )
		   if err != nil {
			   w.log.Error().
				  Str("order_id", o.OrderID).
				   Err(err).
				   Msg("swapthaw: OrphanFrozenScanner.DetectOrphan failed")
			  continue
		  }
		   orphanCount++
		   // 注册阻断（内存模式）。
		   if ms, ok := w.svc.gen.store.(*MemGenerationStore); ok {
			   ms.RegisterOpenCase(o.UserID, tc.ID)
		   }
		  w.doAlert(ctx, "warning", "孤儿 Frozen 订单已升 ThawCase", map[string]any{
			  "case_id":  tc.ID,
			  "order_id": o.OrderID,
			  "user_id": o.UserID,
			  "asset":   o.Asset,
			  "amount":  o.FrozenAmount,
		  })
		  w.log.Warn().
			   Str("case_id", tc.ID).
			  Str("order_id", o.OrderID).
			  Str("user_id", o.UserID).
			  Msg("swapthaw: orphan frozen order detected and ThawCase created")
	  }
	  w.log.Info().
		   Int("scanned", len(orders)).
		   Int("orphans", orphanCount).
		   Msg("swapthaw: OrphanFrozenScanner done")
	   return nil
   }

// ProofConsolidator 重试 open 状态超期 ThawCase 的证明固化（防止首次写失败导致漏记）。
// 本 Worker 不自动固化证明——仅检测已有 ProofPayload 但未转 proven 的记录并重试。
// 建议调用间隔：15 分钟。
  func (w *Worker) ProofConsolidator(ctx context.Context) error {
	  cases, err := w.svc.ListOpenThawCases(100)
	  if err != nil {
		   return fmt.Errorf("swapthaw: ProofConsolidator.ListOpenThawCases: %w", err)
	  }
	   retried := 0
	  for _, tc := range cases {
		  if tc.Status != ThawCaseStatusOpen || tc.ProofPayload == "" {
			   continue
		   }
		  // ProofPayload 已有但状态仍为 open，说明之前 proven 写失败，重试。
		   _, err := w.svc.ProveThawCase(ProveThawCaseReq{
			  CaseID: tc.ID,
			   ProofPayload: tc.ProofPayload,
			   Approver:     tc.Approver,
		  })
		  if err != nil {
			  w.log.Error().
				  Str("case_id", tc.ID).
				   Err(err).
				   Msg("swapthaw: ProofConsolidator retry ProveThawCase failed")
			  continue
		   }
		  retried++
		  w.log.Info().
			   Str("case_id", tc.ID).
			  Msg("swapthaw: ProofConsolidator retried proven transition")
	  }
	  w.log.Info().
		  Int("retried", retried).
		  Msg("swapthaw: ProofConsolidator done")
	   return nil
  }

// ExpiredLeaseReaper 扫描已过期但仍 open 的 FreezeLease，通知 swap.Service 执行 failUnfreeze。
// 本 Worker 不直接操作账本——通过 OnExpiredLease 回调通知外层（swap.Service.RecoverFrozen）。
// 建议调用间隔：2 分钟。
  func (w *Worker) ExpiredLeaseReaper(ctx context.Context, onExpiredLease func(ctx context.Context, orderID, leaseID, leaseToken string) error) error {
	  if onExpiredLease == nil {
		   return fmt.Errorf("swapthaw: ExpiredLeaseReaper: onExpiredLease callback required")
	   }
	  now := time.Now().UTC()
	  leases, err := w.svc.mutex.leases.ListExpiredOpen(100, now)
	  if err != nil {
		   return fmt.Errorf("swapthaw: ExpiredLeaseReaper.ListExpiredOpen: %w", err)
	   }
	  reaped := 0
	   for _, lease := range leases {
		  if err := onExpiredLease(ctx, lease.OrderID, lease.ID, lease.Token); err != nil {
			   w.log.Error().
				   Str("order_id", lease.OrderID).
				   Str("lease_id", lease.ID).
				  Err(err).
				   Msg("swapthaw: ExpiredLeaseReaper onExpiredLease callback failed")
			   continue
		   }
		   reaped++
		  w.log.Info().
			   Str("order_id", lease.OrderID).
			  Str("lease_id", lease.ID).
			  Msg("swapthaw: expired lease reaped and failUnfreeze triggered")
	   }
	  w.log.Info().
		  Int("reaped", reaped).
		  Msg("swapthaw: ExpiredLeaseReaper done")
	  return nil
   }

   // ──────────────────────────────────────────────
  // 内部辅助
  // ──────────────────────────────────────────────

   func (w *Worker) doAlert(ctx context.Context, level, msg string, fields map[string]any) {
	   if w.alert != nil {
		   w.alert(ctx, level, msg, fields)
	   }
  }
// temporary guard removed after the production check
