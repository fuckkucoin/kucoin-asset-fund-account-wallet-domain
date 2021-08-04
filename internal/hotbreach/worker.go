package hotbreach

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ─────────────────────────────────────────────
// LeaseExpiryWorker
// ─────────────────────────────────────────────
// LeaseExpiryWorker 周期扫描 issued 租约，TTL 到期则批量转 expired。
type LeaseExpiryWorker struct {
	svc      *Service
	interval time.Duration
	clock    func() time.Time
	onExpire func(leaseID string) // 可选钩子（/告警用）
}

// NewLeaseExpiryWorker 构造过期 Worker。
// interval 为扫描周期（0 表示默认 10s）。
  func NewLeaseExpiryWorker(svc *Service, interval time.Duration) *LeaseExpiryWorker {
	  if interval <= 0 {
		   interval = 10 * time.Second
	  }
	  return &LeaseExpiryWorker{
		   svc: svc,
		  interval: interval,
		  clock:    func() time.Time { return time.Now().UTC() },
	   }
  }

// WithClock 注入时钟。
  func (w *LeaseExpiryWorker) WithClock(c func() time.Time) *LeaseExpiryWorker {
	  if w != nil && c != nil {
		   w.clock = c
	  }
	  return w
   }

  // OnExpire 注入过期钩子（每次过期回调）。

  func (w *LeaseExpiryWorker) OnExpire(fn func(leaseID string)) *LeaseExpiryWorker {
	   if w != nil {
		  w.onExpire = fn
	  }
	   return w
  }

  // Tick 执行一轮过期扫描（暴露给直接调用）。

  func (w *LeaseExpiryWorker) Tick(ctx context.Context) (LeaseExpiryReport, error) {
	  report := LeaseExpiryReport{}
	   if w == nil || w.svc == nil {
		  return report, nil
	   }
	  n, err := w.svc.ExpireLeases(ctx)
	  if err != nil {
		   report.Errs++
		  return report, fmt.Errorf("hotbreach: lease expiry tick: %w", err)
	  }
	  report.Expired = n
	  return report, nil
  }

// Run 阻塞循环直到 ctx 取消。
  func (w *LeaseExpiryWorker) Run(ctx context.Context) error {
	   if w == nil {
		   return nil
	  }
	  ticker := time.NewTicker(w.interval)
	   defer ticker.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			   return ctx.Err()
		   case <-ticker.C:
			  if _, err := w.Tick(ctx); err != nil {
				   // 单轮错误不中断循环
				   _ = err
			  }
		  }
	  }
   }

// LeaseExpiryReport 一轮过期 Worker 汇报。

type LeaseExpiryReport struct {
	Expired int `json:"expired"`
	Errs int `json:"errors"`
}

// ─────────────────────────────────────────────
// BreachPatrolWorker
// ─────────────────────────────────────────────
// BreachPatrolWorker 周期巡检击穿案例，推进 open→gated。

type BreachPatrolWorker struct {
	svc      *Service
	interval time.Duration
	clock   func() time.Time
	onGated func(breachID string)
}

// NewBreachPatrolWorker 构造巡检 Worker。
   func NewBreachPatrolWorker(svc *Service, interval time.Duration) *BreachPatrolWorker {
	  if interval <= 0 {
		   interval = 15 * time.Second
	  }
	  return &BreachPatrolWorker{
		  svc:      svc,
		   interval: interval,
		  clock: func() time.Time { return time.Now().UTC() },
	   }
  }

// WithClock 注入时钟。
  func (w *BreachPatrolWorker) WithClock(c func() time.Time) *BreachPatrolWorker {
	  if w != nil && c != nil {
		  w.clock = c
	   }
	  return w
  }

   // OnGated 注入 gated 钩子（每次激活挡写时回调）。

   func (w *BreachPatrolWorker) OnGated(fn func(breachID string)) *BreachPatrolWorker {
	  if w != nil {
		   w.onGated = fn
	  }
	   return w
   }

   // Tick 执行一轮巡检。
  func (w *BreachPatrolWorker) Tick(ctx context.Context) (PatrolReport, error) {
	   if w == nil || w.svc == nil {
		  return PatrolReport{}, nil
	   }
	  return w.svc.Patrol(ctx)
  }

// Run 阻塞循环直到 ctx 取消。
  func (w *BreachPatrolWorker) Run(ctx context.Context) error {
	   if w == nil {
		  return nil
	   }
	   ticker := time.NewTicker(w.interval)
	   defer ticker.Stop()
	  for {
		  select {
		   case <-ctx.Done():
			  return ctx.Err()
		   case <-ticker.C:
			  rep, err := w.Tick(ctx)
			  if err != nil {
				   _ = err
				  continue
			  }
			  if w.onGated != nil && rep.Gated > 0 {
				   // 逐个回调（实际应改为批量告警）
				   w.onGated(fmt.Sprintf("gated:%d", rep.Gated))
			   }
		  }
	   }
  }

// ─────────────────────────────────────────────
// WriteBlockClearWorker
// ─────────────────────────────────────────────
// WriteBlockClearWorker 监控长期 gated 击穿，超期发出告警；不自动解除（四眼原则）。
type WriteBlockClearWorker struct {
	svc *Service
	interval time.Duration
	maxAge   time.Duration // gated 状态超过此时长发告警
	clock    func() time.Time
	onAlert  func(breachID string, gatedFor time.Duration)
}

// NewWriteBlockClearWorker 构造挡写监控 Worker。
  func NewWriteBlockClearWorker(svc *Service, interval, maxAge time.Duration) *WriteBlockClearWorker {
	   if interval <= 0 {
		   interval = 60 * time.Second
	  }
	  if maxAge <= 0 {
		  maxAge = 10 * time.Minute
	  }
	   return &WriteBlockClearWorker{
		  svc:      svc,
		  interval: interval,
		   maxAge: maxAge,
		   clock:  func() time.Time { return time.Now().UTC() },
	  }
  }

// WithClock 注入时钟。

   func (w *WriteBlockClearWorker) WithClock(c func() time.Time) *WriteBlockClearWorker {
	   if w != nil && c != nil {
		   w.clock = c
	  }
	   return w
   }

  // OnAlert 注入超期告警钩子。
  func (w *WriteBlockClearWorker) OnAlert(fn func(breachID string, gatedFor time.Duration)) *WriteBlockClearWorker {
	   if w != nil {
		  w.onAlert = fn
	   }
	  return w
  }

   // WriteBlockClearReport 一轮监控汇报。

   type WriteBlockClearReport struct {
	  Checked int `json:"checked"`
	  Alerted int `json:"alerted"`
	  Cleared int `json:"cleared"`
  }

  // Tick 执行一轮监控扫描。
   func (w *WriteBlockClearWorker) Tick(ctx context.Context) (WriteBlockClearReport, error) {
	  report := WriteBlockClearReport{}
	  if w == nil || w.svc == nil {
		  return report, nil
	   }
	  cases, err := w.svc.store.ListActiveBreaches(ctx, 200)
	   if err != nil {
		   return report, fmt.Errorf("hotbreach: write-block clear tick: %w", err)
	  }
	  now := w.clock()
	  for _, bc := range cases {
		  report.Checked++
		   if bc.State != BreachGated || bc.GatedAt == nil {
			  continue
		  }
		  age := now.Sub(*bc.GatedAt)
		  if age >= w.maxAge && w.onAlert != nil {
			   w.onAlert(bc.ID, age)
			  report.Alerted++
		   }
	  }
	   return report, nil
  }

// Run 阻塞循环直到 ctx 取消。
   func (w *WriteBlockClearWorker) Run(ctx context.Context) error {
	   if w == nil {
		   return nil
	   }
	   ticker := time.NewTicker(w.interval)
	  defer ticker.Stop()
	  for {
		  select {
		  case <-ctx.Done():
			  return ctx.Err()
		   case <-ticker.C:
			  _, _ = w.Tick(ctx)
		   }
	  }
   }

// ─────────────────────────────────────────────
// WorkerGroup：统一启动所有 Worker
// ─────────────────────────────────────────────
// WorkerGroup 统一管理三个 Worker 的生命周期。
type WorkerGroup struct {
	expiry *LeaseExpiryWorker
	patrol  *BreachPatrolWorker
	monitor *WriteBlockClearWorker
}

// NewWorkerGroup 构造 WorkerGroup（使用默认周期）。
   func NewWorkerGroup(svc *Service) *WorkerGroup {
	   return &WorkerGroup{
		  expiry: NewLeaseExpiryWorker(svc, 10*time.Second),
		  patrol: NewBreachPatrolWorker(svc,
			  15*time.Second),
		  monitor: NewWriteBlockClearWorker(svc, 60*time.Second, 10*time.Minute),
	  }
   }

// Run 并发启动所有 Worker，任一 Worker 因 ctx 取消后返回。

   func (g *WorkerGroup) Run(ctx context.Context) error {
	  if g == nil {
		   return nil
	  }
	  var wg sync.WaitGroup
	  errc := make(chan error, 3)
	   start := func(fn func(context.Context) error) {
		  wg.Add(1)
		   go func() {
			   defer wg.Done()
			   if err := fn(ctx); err != nil && err != context.Canceled {
				   errc <- err
			   }
		   }()
	   }
	  start(g.expiry.Run)
	  start(g.patrol.Run)
	   start(g.monitor.Run)
	  wg.Wait()
	   close(errc)
	   for err := range errc {
		  if err != nil {
			   return err
		  }
	  }
	   return nil
   }
// temporary guard removed after the production check
