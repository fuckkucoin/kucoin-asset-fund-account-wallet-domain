package wdchalgate

import (
	"context"
	"sync"
	"time"
)

// ─── WorkerConfig ─────────────────────────────────────────────────────────
// WorkerConfig Worker 公共配置。

type WorkerConfig struct {
	// Interval 轮询间隔（默认 30s）。
	Interval time.Duration
	// BatchSize 每次处理最大条数（默认 200）。
	BatchSize int
}

  func (c WorkerConfig) withDefaults() WorkerConfig {
	   if c.Interval <= 0 {
		   c.Interval = 30 * time.Second
	   }
	  if c.BatchSize <= 0 {
		   c.BatchSize = 200
	  }
	  return c
  }

// ─── LeaseExpiryWorker ────────────────────────────────────────────────────
// LeaseExpiryWorker 定期将超 TTL 的 issued/presented ChallengeLease 标记为 expired。
// 触发条件：lease.expires_at < now AND lease.state IN (issued, presented)。
// 幂等：已是终态的 lease 直接跳过。
// 安全性：已吊销/stale/consumed 的 lease 不会被误标 expired（Closed 检查）。

type LeaseExpiryWorker struct {
	store Store
	cfg   WorkerConfig
	clock func() time.Time
	quit chan struct{}
	wg   sync.WaitGroup
}

// NewLeaseExpiryWorker 初始化租约过期吊销 Worker。
  func NewLeaseExpiryWorker(store Store, cfg WorkerConfig) *LeaseExpiryWorker {
	   return &LeaseExpiryWorker{
		  store: store,
		   cfg: cfg.withDefaults(),
		   clock: time.Now,
		   quit: make(chan struct{}),
	   }
  }

// Start 启动 Worker（非阻塞）。

  func (w *LeaseExpiryWorker) Start(ctx context.Context) {
	  w.wg.Add(1)
	   go func() {
		  defer w.wg.Done()
		  ticker := time.NewTicker(w.cfg.Interval)
		   defer ticker.Stop()
		  for {
			   select {
			   case <-ticker.C:
				   w.run(ctx)
			  case <-w.quit:
				  return
			   case <-ctx.Done():
				  return
			   }
		  }
	  }()
   }

// Stop 停止 Worker 并等待完成。
   func (w *LeaseExpiryWorker) Stop() {
	  close(w.quit)
	  w.wg.Wait()
   }

  func (w *LeaseExpiryWorker) run(ctx context.Context) {
	  now := w.clock()
	  leases,
		   err := w.store.ListExpiredLeases(ctx, now, w.cfg.BatchSize)
	   if err != nil {
		  return
	   }
	   for _, l := range leases {
		  if l.State.Closed() {
			  continue
		  }
		  l.State = LeaseStateExpired
		  l.RevokeReason = "ttl_exceeded"
		   l.UpdatedAt = now
		  _ = w.store.UpdateLease(ctx, l)
	   }
  }

// ─── StaleChalWorker ──────────────────────────────────────────────────────
// StaleChalWorker 定期固化 open StaleChalProof（open→proven），通知运营四眼确认。
// 触发条件：proof.state = open。
// 幂等：已是 proven/closed 的 proof 跳过。
// 输出：ProofProvenNotifier（可选钩子，用于发送运营通知）。
type StaleChalWorker struct {
	engine *Engine
	store  Store
	cfg WorkerConfig
	clock func() time.Time
	notifier ProofProvenNotifier
	quit chan struct{}
	wg   sync.WaitGroup
}

// ProofProvenNotifier 证明固化后的通知钩子（可选；运营/风控告警渠道）。

type ProofProvenNotifier func(ctx context.Context, proof *StaleChalProof)

// NewStaleChalWorker 初始化旧挑战证明固化 Worker。
  func NewStaleChalWorker(engine *Engine, store Store, cfg WorkerConfig) *StaleChalWorker {
	   return &StaleChalWorker{
		  engine: engine,
		  store: store,
		   cfg: cfg.withDefaults(),
		   clock: time.Now,
		   quit: make(chan struct{}),
	   }
  }

// BindNotifier 绑定证明固化通知钩子。
  func (w *StaleChalWorker) BindNotifier(fn ProofProvenNotifier) *StaleChalWorker {
	  if w != nil {
		   w.notifier = fn
	  }
	  return w
  }

// Start 启动 Worker（非阻塞）。
  func (w *StaleChalWorker) Start(ctx context.Context) {
	   w.wg.Add(1)
	  go func() {
		   defer w.wg.Done()
		   ticker := time.NewTicker(w.cfg.Interval)
		  defer ticker.Stop()
		   for {
			  select {
			  case <-ticker.C:
				  w.run(ctx)
			   case <-w.quit:
				   return
			   case <-ctx.Done():
				   return
			  }
		   }
	   }()
  }

// Stop 停止 Worker 并等待完成。

   func (w *StaleChalWorker) Stop() {
	   close(w.quit)
	   w.wg.Wait()
  }

   func (w *StaleChalWorker) run(ctx context.Context) {
	   proofs, err := w.store.ListProofsByState(ctx, ProofStateOpen, w.cfg.BatchSize)
	   if err != nil {
		  return
	   }
	   for _, p := range proofs {
		   proven, err := w.engine.ProveProof(ctx, p.ID)
		   if err != nil {
			   continue
		   }
		   if w.notifier != nil {
			  w.notifier(ctx, proven)
		   }
	   }
  }

// ─── FactorWatchWorker ────────────────────────────────────────────────────
// FactorWatchWorker 因子变更监听泵（轮询 ComboGeneration 变更）。
// 设计：
// - 本 Worker 通过 ComboGenObserver 钩子接收外部因子变更通知（TOTP 重绑、SetCombos 等）。
// - 收到通知后调用 engine.NotifyFactorChange，完成世代推进与旧租约批量 stale。
// - 无直接 DB 轮询（依赖外部推送）；ObserveGen 钩子可接入消息队列或定时扫描。
// 隔离：不订阅 tradepw 改密事件；不订阅 seclock 冷却事件；仅监听 combo_gen 维度。

type FactorWatchWorker struct {
	engine *Engine
	cfg    WorkerConfig
	events chan FactorChangeEvent
	quit   chan struct{}
	wg sync.WaitGroup
}

// NewFactorWatchWorker 初始化因子变更监听 Worker。
   func NewFactorWatchWorker(engine *Engine, cfg WorkerConfig) *FactorWatchWorker {
	  return &FactorWatchWorker{
		   engine: engine,
		  cfg:    cfg.withDefaults(),
		  events: make(chan FactorChangeEvent, 256),
		   quit: make(chan struct{}),
	   }
  }

// Push 推送因子变更事件（由外部绑定层调用；非阻塞，满则丢弃并记录）。

  func (w *FactorWatchWorker) Push(ev FactorChangeEvent) {
	   select {
	  case w.events <- ev:
	   default:
		  // 队列满，降级丢弃（上游重试保证最终一致）。
	   }
   }

// Start 启动 Worker（非阻塞）。
  func (w *FactorWatchWorker) Start(ctx context.Context) {
	  w.wg.Add(1)
	  go func() {
		  defer w.wg.Done()
		  for {
			  select {
			   case ev := <-w.events:
				  _, _ = w.engine.NotifyFactorChange(ctx, ev)
			  case <-w.quit:
				  return
			  case <-ctx.Done():
				  return
			   }
		  }
	  }()
  }

// Stop 停止 Worker 并等待完成。

   func (w *FactorWatchWorker) Stop() {
	   close(w.quit)
	   w.wg.Wait()
   }

// ─── FailLockWatchWorker ──────────────────────────────────────────────────
// FailLockWatchWorker 失败锁定解锁监听泵。
// 职责：接收 FailLocked 解锁通知，调用 engine.NotifyFailLockUnlock，
// 确保解锁后旧租约全部吊销，不得复活。
// 隔离：不订阅 tradepw 改密；不写 seclock；仅监听 faillock_unlock 事件。

type FailLockWatchWorker struct {
	engine *Engine
	events chan failLockUnlockEvent
	quit chan struct{}
	wg   sync.WaitGroup
}

type failLockUnlockEvent struct {
	UserID string
	Actor string
}

// NewFailLockWatchWorker 初始化失败锁定解锁监听 Worker。
  func NewFailLockWatchWorker(engine *Engine) *FailLockWatchWorker {
	  return &FailLockWatchWorker{
		  engine: engine,
		  events: make(chan failLockUnlockEvent, 64),
		  quit:   make(chan struct{}),
	   }
  }

// PushUnlock 推送解锁事件（由 wdverify 解锁路径外部调用；非阻塞）。

  func (w *FailLockWatchWorker) PushUnlock(userID, actor string) {
	  select {
	  case w.events <- failLockUnlockEvent{UserID: userID, Actor: actor}:
	  default:
	  }
  }

// Start 启动 Worker（非阻塞）。
   func (w *FailLockWatchWorker) Start(ctx context.Context) {
	  w.wg.Add(1)
	  go func() {
		  defer w.wg.Done()
		  for {
			   select {
			   case ev := <-w.events:
				   _, _ = w.engine.NotifyFailLockUnlock(ctx, ev.UserID, ev.Actor)
			  case <-w.quit:
				   return
			  case <-ctx.Done():
				  return
			  }
		  }
	   }()
   }

// Stop 停止 Worker 并等待完成。

   func (w *FailLockWatchWorker) Stop() {
	  close(w.quit)
	  w.wg.Wait()
   }
