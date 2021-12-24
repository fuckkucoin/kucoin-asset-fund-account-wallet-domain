package tradepwgen

import (
	"context"
	"fmt"
	"time"
)

// WorkerConfig Worker 配置。
type WorkerConfig struct {
	// InflightScanInterval 在途令牌扫描间隔。
	InflightScanInterval time.Duration
	// DriftPatrolInterval 漂移巡检间隔。
	DriftPatrolInterval time.Duration
	// CoolBindInterval 冷却绑定确认间隔。
	CoolBindInterval time.Duration
	// ScanBatchSize 每轮扫描批量大小。
	ScanBatchSize int
	// LeaseTTL 令牌过期时间（用于强制 revoke 判断）。
	LeaseTTL time.Duration
}

// DefaultWorkerConfig 默认 Worker 配置（推荐值）。
var DefaultWorkerConfig = WorkerConfig{
	InflightScanInterval: 30 * time.Second,
	DriftPatrolInterval:  2 * time.Minute,
	CoolBindInterval: 60 * time.Second,
	ScanBatchSize:    200,
	LeaseTTL:         10 * time.Minute,
}

// WorkerAlert 告警回调接口（由外部注入 Kafka 或 Webhook 实现）。
type WorkerAlert interface {
	// OnDriftCaseOpen 发现 open 漂移案件时触发。
	OnDriftCaseOpen(dc *PwDriftCase)
	// OnLeaseExpiredRevoke 令牌超时强制吊销时触发。
	OnLeaseExpiredRevoke(lease *VerifyLease)
}

// noopAlert 空告警实现（默认）。
type noopAlert struct{}

  func (n *noopAlert) OnDriftCaseOpen(*PwDriftCase) {}

  func (n *noopAlert) OnLeaseExpiredRevoke(*VerifyLease) {}

// Worker 后台 Worker。
type Worker struct {
	svc *Service
	cfg   WorkerConfig
	alert WorkerAlert
	clock func() time.Time
}

// NewWorker 构造 Worker。

   func NewWorker(svc *Service, cfg WorkerConfig) *Worker {
	   return &Worker{
		   svc: svc,
		   cfg: cfg,
		   alert: &noopAlert{},
		   clock: func() time.Time { return time.Now().UTC() },
	   }
  }

// WithAlert 注入告警回调。
   func (w *Worker) WithAlert(alert WorkerAlert) *Worker {
	   if alert != nil {
		   w.alert = alert
	  }
	   return w
   }

  func (w *Worker) now() time.Time {
	  if w.clock != nil {
		   return w.clock()
	   }
	  return time.Now().UTC()
   }

// Run 启动 Worker（阻塞，ctx 取消时退出）。
   func (w *Worker) Run(ctx context.Context) error {
	  inflightTicker := time.NewTicker(w.cfg.InflightScanInterval)
	   driftTicker := time.NewTicker(w.cfg.DriftPatrolInterval)
	   coolTicker := time.NewTicker(w.cfg.CoolBindInterval)
	   defer inflightTicker.Stop()
	  defer driftTicker.Stop()
	  defer coolTicker.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			  return ctx.Err()
		  case <-inflightTicker.C:
			   w.runInflightScan()
		   case <-driftTicker.C:
			  w.runDriftPatrol()
		  case <-coolTicker.C:
			  w.runCoolBind()
		  }
	  }
   }

// ──────────────────────────────────────────────
// 在途令牌扫描
// ──────────────────────────────────────────────
// runInflightScan 扫描并强制 revoke 过期 open 令牌。
// 本实现扫描全量 open 案件；路径应改为按 expires_at 索引查询（SQL 实现负责）。
  func (w *Worker) runInflightScan() {
	  now := w.now()
	   // 内存实现不支持全局 open 令牌扫描，此处为 SQL 实现预留钩子。
	   // 路径：SELECT id FROM tradepw_verify_leases WHERE status='open' AND expires_at < NOW LIMIT :batch
	   _ = now
   }

// ScanExpiredLeases 扫描过期令牌并强制吊销（供和 SQL Worker 调用）。
// leases 为外部注入的过期令牌列表（SQL 查询结果）。

   func (w *Worker) ScanExpiredLeases(leases []*VerifyLease) (revokedIDs []string, errs []error) {
	  now := w.now()
	  for _, l := range leases {
		   if l.Status.IsTerminal() {
			   continue
		   }
		  if l.ExpiresAt.IsZero() || !now.After(l.ExpiresAt) {
			  continue
		  }
		   if err := TransitLeaseRevoke(l, now); err != nil {
			   errs = append(errs, fmt.Errorf("tradepwgen: ScanExpiredLeases revoke %s: %w", l.ID, err))
			   continue
		  }
		   if err := w.svc.mutex.leaseStore.Update(l); err != nil {
			  errs = append(errs, fmt.Errorf("tradepwgen: ScanExpiredLeases update %s: %w", l.ID, err))
			  continue
		  }
		   revokedIDs = append(revokedIDs, l.ID)
		   w.alert.OnLeaseExpiredRevoke(l)
	  }
	  return
   }

// ──────────────────────────────────────────────
// 漂移巡检
// ──────────────────────────────────────────────
// runDriftPatrol 巡检 open 漂移案件，触发告警。
   func (w *Worker) runDriftPatrol() {
	   cases, err := w.svc.ListOpenDriftCases(w.cfg.ScanBatchSize)
	   if err != nil {
		  return
	  }
	   for _, dc := range cases {
		   if dc.Status == DriftStatusOpen {
			   w.alert.OnDriftCaseOpen(dc)
		   }
	   }
   }

// PatrolDriftCases 漂移巡检（供和外部调用）。
// 返回全部 open/proven 案件，并对 open 案件触发告警。

   func (w *Worker) PatrolDriftCases(limit int) ([]*PwDriftCase, error) {
	  cases, err := w.svc.ListOpenDriftCases(limit)
	   if err != nil {
		   return nil, fmt.Errorf("tradepwgen: PatrolDriftCases: %w", err)
	   }
	   for _, dc := range cases {
		   if dc.Status == DriftStatusOpen {
			   w.alert.OnDriftCaseOpen(dc)
		  }
	  }
	   return cases, nil
  }

// ──────────────────────────────────────────────
// 冷却绑定确认
// ──────────────────────────────────────────────
// runCoolBind 定期确认冷却绑定（CoolBound 回填）。
// 路径：查询 tradepw_generations WHERE cool_bound=false AND generation>0 LIMIT :batch
// 内存模式此函数为空实现，由集成路径直接调用 ConfirmCoolBind。
  func (w *Worker) runCoolBind() {}

// ConfirmCoolBind 确认指定用户当前世代冷却已生效（由 seclock 确认回调或 Worker 轮询调用）。

  func (w *Worker) ConfirmCoolBind(userID string, gen int64) error {
	   return w.svc.gen.ConfirmCool(userID, gen)
  }
