package restoregate

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"
)

// Workers 包含四个后台 worker：
// 1. WriteFreeze Watchdog：监控活跃写互斥锁，到期自动标 expired 并告警
// 2. HalfRestore Seal Worker：扫描 importing 状态超时计划，自动封印
// 3. SplitBrain Patrol：定期检查 open/clearing 证明，超过 SLA 告警
// 4. Unlock Aging Worker：清理超时已释放的写互斥锁记录（老化）

const (
	defaultWatchdogInterval = 30 * time.Second
	defaultPatrolInterval   = 60 * time.Second
	defaultImportTimeout    = 2 * time.Hour
	defaultProofSLAAlert = 4 * time.Hour
	defaultMutexAgingThreshold = 24 * time.Hour
)

// WriteFreezeWatchdog 写冻结看门狗：周期性检查活锁是否即将超时，并续租。
type WriteFreezeWatchdog struct {
	svc *Service
	interval time.Duration
	clock    func() time.Time
}

// NewWriteFreezeWatchdog 构造看门狗。

   func NewWriteFreezeWatchdog(svc *Service) *WriteFreezeWatchdog {
	  return &WriteFreezeWatchdog{
		  svc:      svc,
		   interval: defaultWatchdogInterval,
		  clock: func() time.Time { return time.Now().UTC() },
	  }
  }

// WithInterval 设置巡检间隔。

   func (w *WriteFreezeWatchdog) WithInterval(d time.Duration) *WriteFreezeWatchdog {
	  w.interval = d
	  return w
  }

// Run 启动看门狗（阻塞，ctx 取消时退出）。
  func (w *WriteFreezeWatchdog) Run(ctx context.Context) error {
	  ticker := time.NewTicker(w.interval)
	   defer ticker.Stop()
	   for {
		   select {
		  case <-ctx.Done():
			  return ctx.Err()
		  case <-ticker.C:
			   w.tick(ctx)
		  }
	  }
  }

   func (w *WriteFreezeWatchdog) tick(ctx context.Context) {
	  activePlanID := w.svc.WriteGuard().ActivePlanID(ctx)
	  if activePlanID == "" {
		   return // 无活锁
	   }
	   // 活跃计划存在，续租
	  if err := w.svc.WriteGuard().RenewLease(ctx, activePlanID); err != nil {
		  if !errors.Is(err, ErrMutexNotFound) {
			   log.Printf("restoregate watchdog: renew lease failed plan=%s err=%v", activePlanID, err)
		   }
		  return
	   }
	  log.Printf("restoregate watchdog: lease renewed plan=%s", activePlanID)
  }

// HalfRestoreSealWorker 半恢复封印 worker：
// 扫描 importing 超时（>2h）的计划，自动中止并封印。
type HalfRestoreSealWorker struct {
	svc *Service
	interval time.Duration
	importTimeout time.Duration
	clock func() time.Time
}

// NewHalfRestoreSealWorker 构造封印 worker。

  func NewHalfRestoreSealWorker(svc *Service) *HalfRestoreSealWorker {
	  return &HalfRestoreSealWorker{
		  svc:           svc,
		  interval:      defaultPatrolInterval,
		  importTimeout: defaultImportTimeout,
		  clock: func() time.Time { return time.Now().UTC() },
	   }
   }

// WithInterval 设置巡检间隔。
   func (w *HalfRestoreSealWorker) WithInterval(d time.Duration) *HalfRestoreSealWorker {
	   w.interval = d
	   return w
   }

// WithImportTimeout 设置 Import 超时阈值。
   func (w *HalfRestoreSealWorker) WithImportTimeout(d time.Duration) *HalfRestoreSealWorker {
	   w.importTimeout = d
	   return w
   }

// Run 启动 worker。
   func (w *HalfRestoreSealWorker) Run(ctx context.Context) error {
	  ticker := time.NewTicker(w.interval)
	  defer ticker.Stop()
	   for {
		   select {
		   case <-ctx.Done():
			   return ctx.Err()
		   case <-ticker.C:
			  w.tick(ctx)
		  }
	  }
   }

   func (w *HalfRestoreSealWorker) tick(ctx context.Context) {
	   plans, err := w.svc.store.ListActivePlans(ctx)
	  if err != nil {
		   log.Printf("restoregate seal-worker: list active plans err=%v", err)
		  return
	   }
	   now := w.clock()
	  for _, p := range plans {
		  if p.State != StateImporting {
			   continue
		  }
		   if p.ImportStartAt == nil {
			  continue
		  }
		   if now.Sub(*p.ImportStartAt) < w.importTimeout {
			  continue
		   }
		   // 超时：自动封印并中止
		  sealErr := w.svc.seal.Seal(ctx, p.ID, nil, fmt.Sprintf("import timeout after %v", w.importTimeout))
		   if sealErr != nil && !errors.Is(sealErr, ErrHalfSealExists) {
			  log.Printf("restoregate seal-worker: seal failed plan=%s err=%v", p.ID, sealErr)
		   }
		  _, abortErr := w.svc.AbortPlan(ctx, p.ID, "auto-aborted by half-restore seal worker: import timeout")
		   if abortErr != nil {
			  log.Printf("restoregate seal-worker: abort failed plan=%s err=%v", p.ID, abortErr)
		   } else {
			   log.Printf("restoregate seal-worker: plan auto-aborted plan=%s", p.ID)
		  }
	   }
  }

// SplitBrainPatrolWorker 脑裂巡检 worker：周期性检查 open 证明，超 SLA 告警。
type SplitBrainPatrolWorker struct {
	svc *Service
	interval time.Duration
	slaAlert time.Duration
	clock    func() time.Time
}

// NewSplitBrainPatrolWorker 构造巡检 worker。
   func NewSplitBrainPatrolWorker(svc *Service) *SplitBrainPatrolWorker {
	   return &SplitBrainPatrolWorker{
		   svc:      svc,
		   interval: defaultPatrolInterval,
		   slaAlert: defaultProofSLAAlert,
		   clock: func() time.Time { return time.Now().UTC() },
	   }
   }

// WithInterval 设置巡检间隔。

   func (w *SplitBrainPatrolWorker) WithInterval(d time.Duration) *SplitBrainPatrolWorker {
	   w.interval = d
	  return w
   }

// WithSLAAlert 设置告警阈值。

  func (w *SplitBrainPatrolWorker) WithSLAAlert(d time.Duration) *SplitBrainPatrolWorker {
	   w.slaAlert = d
	  return w
  }

// Run 启动 worker。
  func (w *SplitBrainPatrolWorker) Run(ctx context.Context) error {
	  ticker := time.NewTicker(w.interval)
	  defer ticker.Stop()
	   for {
		   select {
		   case <-ctx.Done():
			   return ctx.Err()
		  case <-ticker.C:
			  w.tick(ctx)
		  }
	   }
   }

  func (w *SplitBrainPatrolWorker) tick(ctx context.Context) {
	   proofs, err := w.svc.sbGuard.ListOpenProofs(ctx)
	  if err != nil {
		   log.Printf("restoregate patrol: list open proofs err=%v", err)
		   return
	  }
	  now := w.clock()
	  for _, proof := range proofs {
		   age := now.Sub(proof.OpenedAt)
		  if age > w.slaAlert {
			  log.Printf("restoregate patrol: ALERT split-brain proof open too long proof_id=%s plan_id=%s age=%v",
				  proof.ID, proof.PlanID, age)
		   } else {
			   log.Printf("restoregate patrol: open proof proof_id=%s plan_id=%s state=%s age=%v",
				  proof.ID,
				  proof.PlanID, proof.State, age)
		  }
	   }
   }

// UnlockAgingWorker 解锁老化 worker：清理超时已释放/过期的写互斥锁记录（标记）。
// 实际存储删除由 SQLStore 的 purge 方法执行；MemStore 不清理。
type UnlockAgingWorker struct {
	svc      *Service
	interval time.Duration
	threshold time.Duration
	clock     func() time.Time
}

// NewUnlockAgingWorker 构造老化 worker。
  func NewUnlockAgingWorker(svc *Service) *UnlockAgingWorker {
	  return &UnlockAgingWorker{
		  svc:      svc,
		  interval: defaultPatrolInterval,
		  threshold: defaultMutexAgingThreshold,
		  clock: func() time.Time { return time.Now().UTC() },
	   }
  }

// WithInterval 设置巡检间隔。
  func (w *UnlockAgingWorker) WithInterval(d time.Duration) *UnlockAgingWorker {
	   w.interval = d
	  return w
  }

// WithThreshold 设置老化阈值。

  func (w *UnlockAgingWorker) WithThreshold(d time.Duration) *UnlockAgingWorker {
	   w.threshold = d
	  return w
  }

// Run 启动 worker（当前仅打 log，SQL 实现需扩展 PlanStore 接口添加 PurgeMutex）。
   func (w *UnlockAgingWorker) Run(ctx context.Context) error {
	   ticker := time.NewTicker(w.interval)
	  defer ticker.Stop()
	  for {
		  select {
		   case <-ctx.Done():
			  return ctx.Err()
		  case <-ticker.C:
			  log.Printf("restoregate aging: sweep released/expired mutexes threshold=%v", w.threshold)
		  }
	   }
  }

// WorkerSet 将四个 worker 绑定到同一 Service，便于统一启动。

type WorkerSet struct {
	Watchdog *WriteFreezeWatchdog
	SealWk   *HalfRestoreSealWorker
	Patrol   *SplitBrainPatrolWorker
	Aging    *UnlockAgingWorker
}

// NewWorkerSet 构造 worker 集合。

   func NewWorkerSet(svc *Service) *WorkerSet {
	  return &WorkerSet{
		   Watchdog: NewWriteFreezeWatchdog(svc),
		  SealWk:   NewHalfRestoreSealWorker(svc),
		   Patrol: NewSplitBrainPatrolWorker(svc),
		   Aging:  NewUnlockAgingWorker(svc),
	   }
   }

// RunAll 并发启动所有 worker；任意 worker 非 ctx 取消错误时记录日志后接着。
  func (ws *WorkerSet) RunAll(ctx context.Context) {
	  run := func(name string, fn func(context.Context) error) {
		   go func() {
			  if err := fn(ctx); err != nil && !errors.Is(err, context.Canceled) {
				   log.Printf("restoregate worker[%s] exited: %v", name, err)
			   }
		   }()
	   }
	   run("watchdog", ws.Watchdog.Run)
	   run("seal", ws.SealWk.Run)
	   run("patrol", ws.Patrol.Run)
	   run("aging", ws.Aging.Run)
  }
