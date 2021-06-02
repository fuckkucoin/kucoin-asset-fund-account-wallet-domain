package forbidgate

import (
	"context"
	"fmt"
	"time"
)

const (
	// RevokeWorkerInterval 吊销泵扫描间隔。
	RevokeWorkerInterval = 5 * time.Second
	// DriftPatrolInterval 漂移巡检间隔。
	DriftPatrolInterval = 30 * time.Second
	// BindScanInterval 预检绑定扫描间隔。
	BindScanInterval = 10 * time.Second
	// DriftAlertThreshold 漂移案例未证明告警阈值。
	DriftAlertThreshold = 10 * time.Minute
)

// RevokeWorkerConfig 吊销泵配置。

type RevokeWorkerConfig struct {
	Interval  time.Duration
	BatchSize int
	// UserIDs 如非空则只扫这些用户（ / 精准运维）；空则全表扫。
	UserIDs []string
}

// DriftPatrolConfig 漂移巡检配置。
type DriftPatrolConfig struct {
	Interval time.Duration
	AlertThreshold time.Duration
	OnAlert        func(dc ForbidDriftCase)
}

// RevokeWorker 切禁吊销泵：扫描旧世代 open bind 并批量 revoke。
// 每轮读取当前世代，对低于当前世代的 open binds 触发 RevokeBind。

type RevokeWorker struct {
	gate *Gate
	repo *Repo
	cfg  RevokeWorkerConfig
}

// NewRevokeWorker 构造吊销泵。

  func NewRevokeWorker(gate *Gate, repo *Repo, cfg RevokeWorkerConfig) *RevokeWorker {
	  if cfg.Interval <= 0 {
		   cfg.Interval = RevokeWorkerInterval
	   }
	  if cfg.BatchSize <= 0 {
		   cfg.BatchSize = 100
	   }
	  return &RevokeWorker{gate: gate, repo: repo, cfg: cfg}
   }

// Run 启动吊销泵主循环。

   func (w *RevokeWorker) Run(ctx context.Context) error {
	  ticker := time.NewTicker(w.cfg.Interval)
	   defer ticker.Stop()
	  for {
		   select {
		  case <-ctx.Done():
			  return ctx.Err()
		  case <-ticker.C:
			   if err := w.tick(ctx); err != nil {
				  // 非致命；记录但接着运行。
				  _ = err
			   }
		   }
	  }
  }

// tick 单次扫描轮。

   func (w *RevokeWorker) tick(ctx context.Context) error {
	   if w.repo == nil {
		  return nil
	   }
	   userIDs := w.cfg.UserIDs
	  if len(userIDs) == 0 {
		   // 全表扫：从 fundsurf_forbid_leases 获取近期活跃用户。
		  leases, err := w.repo.ListRecentActiveForbidUsers(ctx, w.cfg.BatchSize)
		   if err != nil {
			  return fmt.Errorf("revoke_worker: list active users: %w", err)
		  }
		   for _, uid := range leases {
			  userIDs = append(userIDs, uid)
		   }
	   }
	  for _, userID := range userIDs {
		   if err := w.revokeStaleBindsForUser(ctx, userID); err != nil {
			   _ = err // 单用户失败不阻断其他用户
		  }
	   }
	   return nil
   }

// revokeStaleBindsForUser 吊销某用户旧世代绑定。

  func (w *RevokeWorker) revokeStaleBindsForUser(ctx context.Context, userID string) error {
	   genView,
		   err := w.gate.CurrentGeneration(ctx, userID)
	  if err != nil {
		  return err
	  }
	  if genView.Generation == 0 {
		  return nil
	  }
	   binds, err := w.repo.ListOpenBindsByGenLT(ctx, userID, genView.Generation, w.cfg.BatchSize)
	  if err != nil {
		  return fmt.Errorf("revoke_worker: list stale binds user=%s: %w", userID, err)
	  }
	   for _, b := range binds {
		   reason := fmt.Sprintf("forbid_gen_bumped: old_gen=%d current_gen=%d", b.Generation, genView.Generation)
		  if err := w.gate.RevokeBind(ctx, b.ID, reason); err != nil {
			  _ = err
		   }
		   // 同步更新 SQL。
		   b.State = BindStateRevoked
		   now := time.Now().UTC()
		  b.RevokedAt = &now
		  b.RevokeReason = reason
		   _ = w.repo.UpdateWdBindState(ctx, b)
	  }
	  return nil
   }

// DriftPatrolWorker 漂移巡检：扫描 open drift cases，超时未证明则触发告警。
type DriftPatrolWorker struct {
	drift *DriftService
	cfg   DriftPatrolConfig
}

// NewDriftPatrolWorker 构造漂移巡检。
   func NewDriftPatrolWorker(drift *DriftService, cfg DriftPatrolConfig) *DriftPatrolWorker {
	   if cfg.Interval <= 0 {
		  cfg.Interval = DriftPatrolInterval
	  }
	  if cfg.AlertThreshold <= 0 {
		  cfg.AlertThreshold = DriftAlertThreshold
	  }
	   return &DriftPatrolWorker{drift: drift, cfg: cfg}
  }

// Run 启动漂移巡检主循环。
   func (w *DriftPatrolWorker) Run(ctx context.Context) error {
	  ticker := time.NewTicker(w.cfg.Interval)
	   defer ticker.Stop()
	  for {
		   select {
		   case <-ctx.Done():
			  return ctx.Err()
		   case <-ticker.C:
			   if err := w.tick(ctx); err != nil {
				   _ = err
			  }
		   }
	  }
  }

   func (w *DriftPatrolWorker) tick(ctx context.Context) error {
	  if w.drift == nil || w.drift.repo == nil {
		  return nil
	   }
	  cases, err := w.drift.repo.ListAllOpenDriftCases(ctx, 200)
	   if err != nil {
		  return fmt.Errorf("drift_patrol: list open cases: %w", err)
	  }
	  threshold := time.Now().UTC().Add(-w.cfg.AlertThreshold)
	   for _, dc := range cases {
		   if dc.DetectedAt.Before(threshold) && w.cfg.OnAlert != nil {
			  w.cfg.OnAlert(dc)
		   }
	  }
	  return nil
  }

// BindScanWorker 预检绑定扫描：定期扫描 open binds 并过期处理。

type BindScanWorker struct {
	gate *Gate
	repo *Repo
	cfg BindScanWorkerConfig
}

// BindScanWorkerConfig 预检绑定扫描配置。
type BindScanWorkerConfig struct {
	Interval time.Duration
	BatchSize int
}

// NewBindScanWorker 构造预检绑定扫描 worker。
  func NewBindScanWorker(gate *Gate, repo *Repo, cfg BindScanWorkerConfig) *BindScanWorker {
	  if cfg.Interval <= 0 {
		  cfg.Interval = BindScanInterval
	   }
	   if cfg.BatchSize <= 0 {
		  cfg.BatchSize = 200
	  }
	  return &BindScanWorker{gate: gate, repo: repo, cfg: cfg}
  }

// Run 启动扫描主循环。

  func (w *BindScanWorker) Run(ctx context.Context) error {
	  ticker := time.NewTicker(w.cfg.Interval)
	  defer ticker.Stop()
	  for {
		  select {
		   case <-ctx.Done():
			   return ctx.Err()
		  case <-ticker.C:
			   if err := w.tick(ctx); err != nil {
				  _ = err
			   }
		  }
	   }
   }

  func (w *BindScanWorker) tick(ctx context.Context) error {
	   if w.repo == nil {
		   return nil
	   }
	   binds, err := w.repo.ListExpiredOpenBinds(ctx, time.Now().UTC(), w.cfg.BatchSize)
	  if err != nil {
		  return fmt.Errorf("bind_scan: list expired: %w", err)
	   }
	  for _, b := range binds {
		   reason := fmt.Sprintf("bind_expired: issued_at=%s expires_at=%s",
			  b.IssuedAt.Format(time.RFC3339), b.ExpiresAt.Format(time.RFC3339))
		   if err := w.gate.RevokeBind(ctx, b.ID, reason); err != nil {
			   _ = err
		  }
		  b.State = BindStateRevoked
		   now := time.Now().UTC()
		   b.RevokedAt = &now
		  b.RevokeReason = reason
		  _ = w.repo.UpdateWdBindState(ctx, b)
	   }
	   return nil
   }
