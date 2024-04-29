package hopcompgate

import (
	"context"
	"log"
	"time"
)

// WorkerConfig 后台 worker 配置参数。
type WorkerConfig struct {
	// ScanInterval 补偿扫描周期（默认 30s）。
	ScanInterval time.Duration
	// PatrolInterval 双移巡检周期（默认 60s）。
	PatrolInterval time.Duration
	// RevokeInterval 过期吊销周期（默认 15s）。
	RevokeInterval time.Duration
	// ScanBatchSize 每批扫描的最大 Case 数量（默认 100）。
	ScanBatchSize int
	// LeaseTTL 自动申请租约时的 TTL（秒，默认 300）。
	LeaseTTL int64
	// MissedThreshold 漏补判定阈值：Case 在 open/compensating 状态超过此时长则视为漏补。
	MissedThreshold time.Duration
}

   func (c WorkerConfig) withDefaults() WorkerConfig {
	   if c.ScanInterval <= 0 {
		  c.ScanInterval = 30 * time.Second
	  }
	   if c.PatrolInterval <= 0 {
		  c.PatrolInterval = 60 * time.Second
	   }
	  if c.RevokeInterval <= 0 {
		   c.RevokeInterval = 15 * time.Second
	   }
	  if c.ScanBatchSize <= 0 {
		  c.ScanBatchSize = 100
	   }
	   if c.LeaseTTL <= 0 {
		   c.LeaseTTL = DefaultLeaseTTL
	  }
	   if c.MissedThreshold <= 0 {
		   c.MissedThreshold = 10 * time.Minute
	   }
	   return c
   }

// Workers 后台任务集合，由外部调用 Run 启动。
type Workers struct {
	gate *Gate
	cfg  WorkerConfig
}

// NewWorkers 创建 Workers 集合。
  func NewWorkers(g *Gate, cfg WorkerConfig) *Workers {
	  return &Workers{gate: g, cfg: cfg.withDefaults()}
   }

// Run 启动所有 Worker，阻塞直到 ctx 取消。

  func (w *Workers) Run(ctx context.Context) {
	  go w.loopScanOpen(ctx)
	   go w.loopPatrol(ctx)
	  go w.loopRevoke(ctx)
	   <-ctx.Done()
   }

// ──────────────────────────────────────────────────────────────────────────────
// ScanOpenCompensate
// ──────────────────────────────────────────────────────────────────────────────

  func (w *Workers) loopScanOpen(ctx context.Context) {
	   t := time.NewTicker(w.cfg.ScanInterval)
	   defer t.Stop()
	  for {
		   select {
		   case <-ctx.Done():
			   return
		  case <-t.C:
			   if err := w.ScanOpenCompensate(ctx); err != nil {
				  log.Printf("[hopcompgate/worker] scan-open error: %v", err)
			   }
		   }
	   }
  }

// ScanOpenCompensate 扫描状态为 open 的 Case，为每个 Case 申请租约并执行补偿。
// 补偿失败时不 panic，由 Case FSM 决定下次是否重入或进入 blocked。

  func (w *Workers) ScanOpenCompensate(ctx context.Context) error {
	  if w.gate == nil {
		  return ErrUnavailable
	   }
	   cases, err := w.gate.store.ListCases(ctx, CaseQueryReq{
		  State: CaseOpen,
		  Limit: w.cfg.ScanBatchSize,
	   })
	   if err != nil {
		  return err
	  }
	  for _, c := range cases {
		   if err := w.compensateOne(ctx, c); err != nil {
			  log.Printf("[hopcompgate/worker] compensate case=%s err=%v", c.ID, err)
		  }
	   }
	  return nil
   }

  func (w *Workers) compensateOne(ctx context.Context, c *HopCompCase) error {
	   if c.State == CaseClosed || c.State == CaseBlocked {
		   return nil
	  }
	  lease, err := w.gate.AcquireLease(ctx, AcquireLeaseReq{
		   CaseID: c.ID,
		   TTL: w.cfg.LeaseTTL,
	   })
	   if err != nil {
		  return err
	   }
	   _, err = w.gate.ExecuteCompensate(ctx, CompensateReq{
		   CaseID:  c.ID,
		   LeaseID: lease.ID,
		  Token:      lease.Token,
		  Generation: lease.Generation,
	   })
	  return err
   }

// ──────────────────────────────────────────────────────────────────────────────
// PatrolDoubleMove
// ──────────────────────────────────────────────────────────────────────────────

   func (w *Workers) loopPatrol(ctx context.Context) {
	  t := time.NewTicker(w.cfg.PatrolInterval)
	   defer t.Stop()
	   for {
		   select {
		   case <-ctx.Done():
			  return
		  case <-t.C:
			   if err := w.PatrolDoubleMove(ctx); err != nil {
				  log.Printf("[hopcompgate/worker] patrol error: %v", err)
			   }
		  }
	  }
   }

// PatrolDoubleMove 扫描长时间未闭合的 Case（漏补嫌疑），写入证明。
  func (w *Workers) PatrolDoubleMove(ctx context.Context) error {
	  if w.gate == nil {
		  return ErrUnavailable
	  }
	   threshold := w.gate.now().Add(-w.cfg.MissedThreshold)
	  cases, err := w.gate.store.ListCases(ctx, CaseQueryReq{
		   State: CaseOpen,
		  Before: threshold,
		   Limit:  w.cfg.ScanBatchSize,
	   })
	  if err != nil {
		   return err
	  }
	   for _, c := range cases {
		   if _, err := w.gate.DetectDoubleMove(ctx, c.ID, "", ""); err != nil {
			   log.Printf("[hopcompgate/worker] patrol detect case=%s err=%v", c.ID, err)
		   }
	   }
	   // 检查 compensating 状态超时（可能意味着 compensate move 提交后 response 丢失）
	  compCases, err := w.gate.store.ListCases(ctx, CaseQueryReq{
		   State: CaseCompensating,
		   Before: threshold,
		   Limit: w.cfg.ScanBatchSize,
	   })
	  if err != nil {
		  return err
	   }
	   for _, c := range compCases {
		  log.Printf("[hopcompgate/worker] patrol: case=%s stuck in compensating since %s",
			  c.ID,
			   c.UpdatedAt.Format(time.RFC3339))
	   }
	  return nil
   }

// ──────────────────────────────────────────────────────────────────────────────
// RevokeExpiredLeases
// ──────────────────────────────────────────────────────────────────────────────

  func (w *Workers) loopRevoke(ctx context.Context) {
	  t := time.NewTicker(w.cfg.RevokeInterval)
	   defer t.Stop()
	   for {
		   select {
		  case <-ctx.Done():
			   return
		  case <-t.C:
			   if err := w.RevokeExpiredLeases(ctx); err != nil {
				  log.Printf("[hopcompgate/worker] revoke error: %v", err)
			   }
		  }
	   }
  }

// RevokeExpiredLeases 将已超时的租约迁移为 expired。

  func (w *Workers) RevokeExpiredLeases(ctx context.Context) error {
	  if w.gate == nil {
		  return ErrUnavailable
	  }
	   now := w.gate.now()
	  leases, err := w.gate.store.ListExpiredLeases(ctx, now, 200)
	   if err != nil {
		   return err
	   }
	   for _, l := range leases {
		  if _, err := w.gate.store.ExpireLease(ctx, l.ID, now); err != nil {
			   log.Printf("[hopcompgate/worker] expire lease=%s err=%v", l.ID, err)
		  }
	   }
	   return nil
   }
