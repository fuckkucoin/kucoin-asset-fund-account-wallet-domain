package wdremgate

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/wdpartial"
)

// Worker 后台 Worker。
type Worker struct {
	gate *RemReleaseGate
	svc  *wdpartial.Service
	cfg  WorkerConfig
	alert WorkerAlert
	clock func() time.Time
}

// NewWorker 构造 Worker。
  func NewWorker(gate *RemReleaseGate, svc *wdpartial.Service, cfg WorkerConfig) *Worker {
	   return &Worker{
		  gate: gate,
		  svc:  svc,
		   cfg:  cfg,
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
	   holdTicker := time.NewTicker(w.cfg.HoldPatrolInterval)
	  conserveTicker := time.NewTicker(w.cfg.ConserveEscalateInterval)
	   leaseTicker := time.NewTicker(w.cfg.LeaseRevokeInterval)
	   defer holdTicker.Stop()
	   defer conserveTicker.Stop()
	  defer leaseTicker.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			   return ctx.Err()
		  case <-holdTicker.C:
			   w.runHoldPatrol()
		   case <-conserveTicker.C:
			   w.runConservePatrol()
		   case <-leaseTicker.C:
			   w.runLeaseRevoke()
		  }
	   }
   }

// ──────────────────────────────────────────────
// 开放 Hold 巡检
// ──────────────────────────────────────────────
// runHoldPatrol 巡检 open Hold，触发告警（不自动关闭）。

  func (w *Worker) runHoldPatrol() {}

// PatrolHolds 巡检 open Hold 列表（供和外部调用）。
// holds 为外部注入的 open Hold 列表；返回超时 Hold 的 ID 列表。
  func (w *Worker) PatrolHolds(holds []wdpartial.ShardHold, threshold time.Duration) []string {
	  now := w.now()
	   var aged []string
	  for _, h := range holds {
		   if h.State != wdpartial.HoldOpen {
			   continue
		  }
		   if now.Sub(h.CreatedAt) > threshold {
			   aged = append(aged, h.ID)
		   }
	  }
	   return aged
   }

// ──────────────────────────────────────────────
// 守恒失败升级巡检
// ──────────────────────────────────────────────
// runConservePatrol 巡检 open 守恒案件，触发升级告警。

  func (w *Worker) runConservePatrol() {
	  cases := w.svc.ListOpenConserveCases(w.cfg.ScanBatchSize)
	   for _, c := range cases {
		  if c.Status == wdpartial.ConserveCaseOpen {
			   w.alert.OnConserveCaseOpen(c)
		   }
	  }
  }

// PatrolConserveCases 守恒案件巡检（供和外部调用）。
  func (w *Worker) PatrolConserveCases(limit int) []*wdpartial.ConserveCase {
	  cases := w.svc.ListOpenConserveCases(limit)
	  for _, c := range cases {
		   if c.Status == wdpartial.ConserveCaseOpen {
			  w.alert.OnConserveCaseOpen(c)
		   }
	  }
	   return cases
   }

// ──────────────────────────────────────────────
// 过期租约吊销
// ──────────────────────────────────────────────
// runLeaseRevoke 吊销过期租约。

   func (w *Worker) runLeaseRevoke() {
	   revokedIDs := w.svc.RevokeExpiredLeases()
	  if len(revokedIDs) > 0 {
		  w.alert.OnLeaseExpiredRevoke(revokedIDs)
	   }
   }

// RevokeExpiredLeases 吊销过期租约（供和外部调用）。
   func (w *Worker) RevokeExpiredLeases() []string {
	  return w.svc.RevokeExpiredLeases()
   }

// ──────────────────────────────────────────────
// 守卫世代巡检
// ──────────────────────────────────────────────
// PatrolGuardGens 守卫世代巡检（供和外部调用）。
   func (w *Worker) PatrolGuardGens(guardGens []*wdpartial.BroadcastGuardGen) []string {
	   var invalidatedIDs []string
	  for _, g := range guardGens {
		  if g.Status == wdpartial.GuardGenInvalidated {
			   w.alert.OnGuardGenInvalidated(g)
			   invalidatedIDs = append(invalidatedIDs, g.ID)
		  }
	  }
	  return invalidatedIDs
  }
