package destlife

import (
	"context"
	"fmt"
	"log"
	"time"
)

const (
	// DefaultWorkerBatch 默认每批处理条目数。
	DefaultWorkerBatch = 50
	// DefaultCoolingOverstay 冷却老化告警阈值（冷却截止后仍未激活超过此时长）。
	DefaultCoolingOverstay = 2 * time.Hour
)

// WorkerConfig Worker 配置。
type WorkerConfig struct {
	// ActivateInterval 冷却激活轮询间隔。
	ActivateInterval time.Duration
	// RevokeInterval 撤销冲刷轮询间隔。
	RevokeInterval time.Duration
	// PatrolInterval 绕过巡检轮询间隔。
	PatrolInterval time.Duration
	// AgingInterval 老化告警轮询间隔。
	AgingInterval time.Duration
	// Batch 每批处理条目数。
	Batch int
	// CoolingOverstay 冷却老化告警阈值。
	CoolingOverstay time.Duration
}

// DefaultWorkerConfig 返回默认 Worker 配置。
  func DefaultWorkerConfig() WorkerConfig {
	  return WorkerConfig{
		   ActivateInterval: 30 * time.Second,
		  RevokeInterval: 60 * time.Second,
		  PatrolInterval: 5 * time.Minute,
		  AgingInterval:  15 * time.Minute,
		  Batch: DefaultWorkerBatch,
		   CoolingOverstay: DefaultCoolingOverstay,
	   }
  }

// LifecycleWorker 生命周期后台 Worker。

type LifecycleWorker struct {
	cal *Calendar
	lock  *InflightLock
	guard *BypassGuard
	cfg   WorkerConfig
	clock func() time.Time
	bookGenFn func(ctx context.Context) (int64, error) // 从 Versioned 获取当前 gen
}

// NewLifecycleWorker 构造 LifecycleWorker。
// bookGenFn 用于获取 destallow.Versioned 当前世代（避免循环依赖）。

  func NewLifecycleWorker(
	  cal *Calendar,
	   lock *InflightLock,
	  guard *BypassGuard,
	  cfg WorkerConfig,
	  clock func() time.Time,
	bookGenFn func(ctx context.Context) (int64, error),
) *LifecycleWorker {
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	if cfg.Batch <= 0 {
		cfg.Batch = DefaultWorkerBatch
	}
	return &LifecycleWorker{
		cal: cal, lock: lock, guard: guard,
		cfg: cfg,
		clock: clock, bookGenFn: bookGenFn,
	}
}

// RunActivate 单次冷却激活扫描：Cooling→Active。
// 返回激活数与错误列表（单条失败不中断）。

  func (w *LifecycleWorker) RunActivate(ctx context.Context) (int, []error) {
	   now := w.clock()
	   entries,
		  err := w.cal.ListPendingActivation(ctx, w.cfg.Batch)
	   if err != nil {
		  return 0, []error{fmt.Errorf("destlife worker activate list: %w", err)}
	   }
	  CoolingEntries.Set(float64(len(entries)))
	  var errs []error
	  activated := 0
	  for _, e := range entries {
		   bookGen := e.Generation
		  if w.bookGenFn != nil {
			   if g, err := w.bookGenFn(ctx); err == nil {
				  bookGen = g
			  }
		  }
		  _,
			   err := w.cal.ActivateCooled(ctx, e.EntryLifeID, now, bookGen)
		   if err != nil {
			   errs = append(errs, fmt.Errorf("destlife worker activate %s: %w", e.EntryLifeID, err))
			  continue
		   }
		   StateTransitions.WithLabelValues(string(StateCooling),
			   string(StateActive)).Inc()
		  WorkerActivations.Inc()
		   activated++
	  }
	   return activated, errs
   }

// RunRevokeFlush 单次待撤销冲刷：检查引用计数归零后 Revoking→Revoked。
  func (w *LifecycleWorker) RunRevokeFlush(ctx context.Context) (int, []error) {
	  entries, err := w.cal.ListPendingRevoke(ctx, w.cfg.Batch)
	   if err != nil {
		  return 0, []error{fmt.Errorf("destlife worker revoke list: %w", err)}
	   }
	   RevokingEntries.Set(float64(len(entries)))
	  var errs []error
	   revoked := 0
	  for _, e := range entries {
		  cnt := 0
		  if w.lock != nil {
			  c, err := w.lock.ActiveCount(ctx, e.EntryLifeID)
			  if err != nil {
				  errs = append(errs, fmt.Errorf("destlife worker revoke count %s: %w", e.EntryLifeID, err))
				  continue
			   }
			  cnt = c
		   }
		  if cnt > 0 {
			   continue
		  }
		  bookGen := e.Generation
		   if w.bookGenFn != nil {
			   if g, err := w.bookGenFn(ctx); err == nil {
				  bookGen = g
			  }
		   }
		   _, err := w.cal.FinalizeRevoked(ctx, e.EntryLifeID, "revoke_worker", bookGen)
		  if err != nil {
			   errs = append(errs, fmt.Errorf("destlife worker revoke finalize %s: %w", e.EntryLifeID, err))
			  continue
		  }
		   StateTransitions.WithLabelValues(string(StateRevoking), string(StateRevoked)).Inc()
		  WorkerRevocations.Inc()
		   revoked++
	   }
	  return revoked, errs
   }

// RunBypassPatrol 单次绕过巡检：补挡所有未挡写证明。
  func (w *LifecycleWorker) RunBypassPatrol(ctx context.Context) (int, []error) {
	  if w.guard == nil {
		  return 0, nil
	   }
	   processed, errs := w.guard.PatrolUnblocked(ctx, w.cfg.Batch)
	   WorkerBypassPatrol.Add(float64(processed))
	  return processed, errs
   }

// RunAgingAlert 单次老化告警扫描：冷却截止后超过 overstay 阈值的条目打印告警。

   func (w *LifecycleWorker) RunAgingAlert(ctx context.Context) (int, []error) {
	   now := w.clock()
	  overstayBefore := now.Add(-w.cfg.CoolingOverstay)
	  entries, err := w.cal.ListPendingActivation(ctx, w.cfg.Batch*4)
	  if err != nil {
		   return 0, []error{fmt.Errorf("destlife worker aging list: %w", err)}
	   }
	  aged := 0
	   for _, e := range entries {
		   deadline := e.CooldownDeadline(w.cal.defaultWindow)
		   if !deadline.IsZero() && deadline.Before(overstayBefore) {
			   log.Printf("[destlife] aging_alert entry_life_id=%s user=%s chain=%s addr=%s deadline=%s overstay=%.0fs",
				   e.EntryLifeID, e.UserID, e.Chain, e.Address, deadline.Format(time.RFC3339),
				   now.Sub(deadline).Seconds())
			   aged++
		  }
	   }
	   return aged, nil
   }

// LoopActivate 启动冷却激活周期轮询（阻塞，由 goroutine 调用）。
  func (w *LifecycleWorker) LoopActivate(ctx context.Context) {
	  ticker := time.NewTicker(w.cfg.ActivateInterval)
	  defer ticker.Stop()
	  for {
		  select {
		  case <-ctx.Done():
			  return
		  case <-ticker.C:
			   n, errs := w.RunActivate(ctx)
			  if n > 0 {
				   log.Printf("[destlife] activate_worker: activated=%d", n)
			   }
			   for _, e := range errs {
				   log.Printf("[destlife] activate_worker error: %v", e)
			   }
		  }
	  }
   }

// LoopRevokeFlush 启动待撤销冲刷周期轮询。

   func (w *LifecycleWorker) LoopRevokeFlush(ctx context.Context) {
	   ticker := time.NewTicker(w.cfg.RevokeInterval)
	  defer ticker.Stop()
	  for {
		   select {
		  case <-ctx.Done():
			   return
		  case <-ticker.C:
			   n, errs := w.RunRevokeFlush(ctx)
			  if n > 0 {
				   log.Printf("[destlife] revoke_worker: revoked=%d", n)
			  }
			   for _, e := range errs {
				  log.Printf("[destlife] revoke_worker error: %v", e)
			   }
		   }
	   }
  }

// LoopBypassPatrol 启动绕过巡检周期轮询。
  func (w *LifecycleWorker) LoopBypassPatrol(ctx context.Context) {
	  ticker := time.NewTicker(w.cfg.PatrolInterval)
	   defer ticker.Stop()
	  for {
		   select {
		   case <-ctx.Done():
			  return
		   case <-ticker.C:
			  n, errs := w.RunBypassPatrol(ctx)
			  if n > 0 {
				   log.Printf("[destlife] bypass_patrol: processed=%d", n)
			  }
			  for _, e := range errs {
				   log.Printf("[destlife] bypass_patrol error: %v", e)
			   }
		   }
	  }
  }

// LoopAgingAlert 启动老化告警周期轮询。

  func (w *LifecycleWorker) LoopAgingAlert(ctx context.Context) {
	  ticker := time.NewTicker(w.cfg.AgingInterval)
	   defer ticker.Stop()
	  for {
		  select {
		  case <-ctx.Done():
			   return
		   case <-ticker.C:
			  n, _ := w.RunAgingAlert(ctx)
			   if n > 0 {
				  log.Printf("[destlife] aging_alert: overstay_entries=%d", n)
			  }
		  }
	   }
   }

// StartAll 启动全部 Worker goroutine（非阻塞）。
   func (w *LifecycleWorker) StartAll(ctx context.Context) {
	  go w.LoopActivate(ctx)
	   go w.LoopRevokeFlush(ctx)
	  go w.LoopBypassPatrol(ctx)
	   go w.LoopAgingAlert(ctx)
  }
