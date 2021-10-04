package prefroute

import (
	"context"
	"log"
	"sync"
	"time"
)

// Worker prefroute 后台泵。

type Worker struct {
	svc   *Service
	cfg   Config
	clock func() time.Time
	mu    sync.Mutex
	users map[string]struct{} // 追踪中用户集合（可改为数据库扫描）
}

// NewWorker 构造 Worker。
  func NewWorker(svc *Service, cfg Config) *Worker {
	  return &Worker{
		  svc: svc,
		   cfg: cfg,
		  clock: func() time.Time { return time.Now().UTC() },
		  users: make(map[string]struct{}),
	  }
  }

// WithClock 注入时钟。
   func (w *Worker) WithClock(c func() time.Time) *Worker {
	  if w != nil && c != nil {
		  w.clock = c
	   }
	   return w
   }

   // TrackUser 注册需要巡检的用户（环境由数据库动态枚举替代）。
   func (w *Worker) TrackUser(userID string) {
	  if w == nil {
		  return
	   }
	  w.mu.Lock()
	   defer w.mu.Unlock()
	  w.users[userID] = struct{}{}
   }

// Run 启动所有 worker 泵，直到 ctx 取消。
  func (w *Worker) Run(ctx context.Context) {
	  var wg sync.WaitGroup
	  wg.Add(3)
	   go func() { defer wg.Done(); w.runInflightScan(ctx) }()
	  go func() { defer wg.Done(); w.runDriftPatrol(ctx) }()
	   go func() { defer wg.Done(); w.runPendingSwitchAdvance(ctx) }()
	   wg.Wait()
   }

// runInflightScan 扫描完成的在途贷记并触发漂移检测。
  func (w *Worker) runInflightScan(ctx context.Context) {
	  ticker := time.NewTicker(w.cfg.InflightScanInterval)
	  defer ticker.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			  return
		   case <-ticker.C:
			   w.scanInflightCredits(ctx)
		   }
	   }
   }

// runDriftPatrol 巡检路由漂移。

  func (w *Worker) runDriftPatrol(ctx context.Context) {
	  ticker := time.NewTicker(w.cfg.DriftPatrolInterval)
	  defer ticker.Stop()
	   for {
		   select {
		  case <-ctx.Done():
			  return
		   case <-ticker.C:
			  w.patrolDrift(ctx)
		   }
	  }
   }

// runPendingSwitchAdvance 推进待切换队列。

  func (w *Worker) runPendingSwitchAdvance(ctx context.Context) {
	   ticker := time.NewTicker(w.cfg.PendingSwitchAdvanceInterval)
	   defer ticker.Stop()
	   for {
		   select {
		  case <-ctx.Done():
			  return
		  case <-ticker.C:
			  w.advancePendingSwitches(ctx)
		   }
	   }
   }

   func (w *Worker) trackedUsers() []string {
	   w.mu.Lock()
	   defer w.mu.Unlock()
	   out := make([]string, 0, len(w.users))
	  for u := range w.users {
		  out = append(out, u)
	   }
	  return out
   }

   func (w *Worker) scanInflightCredits(ctx context.Context) {
	   users := w.trackedUsers()
	  for _, userID := range users {
		  credits,
			  err := w.svc.store.ListInflightCredits(ctx, userID, InflightStatePending)
		   if err != nil {
			   log.Printf("prefroute worker: list inflight credits user=%s err=%v", userID, err)
			   continue
		   }
		   for _, ic := range credits {
			   // 超时在途贷记标记失败（中按超时阈值决定）
			   if ic.RouteSnapID != "" && w.clock().Sub(ic.CreatedAt) > 5*w.cfg.LeaseTTL {
				  ic.State = InflightStateFailed
				   ic.UpdatedAt = w.clock()
				   if err := w.svc.store.UpdateInflightCredit(ctx, ic); err != nil {
					  log.Printf("prefroute worker: fail inflight credit txn=%s err=%v", ic.TxnID, err)
				  }
			  }
		   }
	   }
  }

  func (w *Worker) patrolDrift(ctx context.Context) {
	  users := w.trackedUsers()
	   cases, err := w.svc.detector.PatrolRouteSnaps(ctx, users)
	   if err != nil {
		   log.Printf("prefroute worker: patrol drift err=%v", err)
		   return
	   }
	  for _, dc := range cases {
		  log.Printf("prefroute worker: drift detected case=%s user=%s bound=%s credited=%s",
			  dc.ID, dc.UserID, dc.BoundBook, dc.CreditedBook)
	  }
  }

  func (w *Worker) advancePendingSwitches(ctx context.Context) {
	  users := w.trackedUsers()
	   for _, userID := range users {
		   applied, err := w.svc.AdvancePendingSwitches(ctx, userID)
		   if err != nil {
			  log.Printf("prefroute worker: advance pending switch user=%s err=%v", userID, err)
			  continue
		   }
		  for _, ps := range applied {
			   log.Printf("prefroute worker: pending switch applied id=%s user=%s from=%s to=%s",
				  ps.ID, ps.UserID, ps.FromBook, ps.ToBook)
		   }
	   }
   }
