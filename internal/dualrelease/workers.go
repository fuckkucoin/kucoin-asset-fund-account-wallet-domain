package dualrelease

import (
	"fmt"
	"sync"
	"time"
)

// ──────────────────────────────────────────────────
// WorkerConfig
// ──────────────────────────────────────────────────
// WorkerConfig Worker 公共配置。

type WorkerConfig struct {
	// Interval Worker 两次扫描之间的间隔。
	Interval time.Duration
	// BatchSize 单次扫描最大处理条目数（防止长尾）。
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

// ──────────────────────────────────────────────────
// TicketAgingWorker
// ──────────────────────────────────────────────────
// TicketAgingWorker 定期扫描超 TTL 的放行案例，标记 expired 并写审计记录。

type TicketAgingWorker struct {
	engine *Engine
	cfg    WorkerConfig
	mu     sync.Mutex
	quit   chan struct{}
	wg     sync.WaitGroup
}

// NewTicketAgingWorker 初始化老化 Worker。
   func NewTicketAgingWorker(engine *Engine, cfg WorkerConfig) *TicketAgingWorker {
	  return &TicketAgingWorker{engine: engine,
		   cfg: cfg.withDefaults(), quit: make(chan struct{})}
   }

// Start 启动后台老化 Goroutine。

   func (w *TicketAgingWorker) Start() {
	  w.wg.Add(1)
	   go func() {
		  defer w.wg.Done()
		  ticker := time.NewTicker(w.cfg.Interval)
		   defer ticker.Stop()
		  for {
			   select {
			   case <-w.quit:
				   return
			  case <-ticker.C:
				  if err := w.RunOnce(); err != nil {
					  // 环境此处应接入告警；此处只 log 占位。
					   _ = fmt.Errorf("TicketAgingWorker error: %v", err)
				   }
			  }
		  }
	  }()
   }

// Stop 优雅关闭。

   func (w *TicketAgingWorker) Stop() {
	   close(w.quit)
	   w.wg.Wait()
  }

// RunOnce 执行一轮老化扫描；供直接调用。
   func (w *TicketAgingWorker) RunOnce() error {
	   now := w.engine.clock()
	   cases, err := w.engine.store.ListExpiredCases(now)
	  if err != nil {
		   return fmt.Errorf("TicketAgingWorker ListExpiredCases: %w", err)
	   }
	   batch := cases
	   if len(batch) > w.cfg.BatchSize {
		  batch = batch[:w.cfg.BatchSize]
	   }
	   if len(batch) == 0 {
		   return nil
	   }
	  errs := w.engine.MarkExpired(batch)
	   for _, rc := range batch {
		   // 写入 TicketExpiry 审计（尽力，不影响标 expired 结果）。
		   exp := &TicketExpiry{
			  ID:        newID(),
			   TicketID:  rc.TicketID,
			  RequestID: rc.RequestID,
			   Reason:    ReasonTTLExceeded,
			  EpochAt:   rc.Gen,
			   TTLSecs:   0,
			   ExpiredAt: now,
			  Actor:     "system:aging-worker",
			   AuditHash: ComputeAuditHash(rc.TicketID, rc.RequestID, ReasonTTLExceeded, rc.Gen, now),
			  CreatedAt: now,
		  }
		   _ = w.engine.store.AppendExpiry(exp)
	  }
	   if len(errs) > 0 {
		   return fmt.Errorf("TicketAgingWorker mark errors: %v", errs)
	  }
	   return nil
  }

// ──────────────────────────────────────────────────
// MutexReclaimWorker
// ──────────────────────────────────────────────────
// ReclaimStore 互斥租约回收所需的额外读写能力；由 MemStore / sqlrepo 实现。
type ReclaimStore interface {
	Store
	// ListStaleLeaseCases 返回租约已过期但 State 仍为 unlocking/executing 的案例。
	ListStaleLeaseCases(now time.Time) ([]*ReleaseCase, error)
}

// MutexReclaimWorker 定期回收超时互斥租约，将 unlocking/executing 回退到 ready，
// 防止单个 worker 崩溃后案例永久卡在中间状态。

type MutexReclaimWorker struct {
	engine *Engine
	store ReclaimStore
	cfg   WorkerConfig
	quit  chan struct{}
	wg    sync.WaitGroup
}

// NewMutexReclaimWorker 初始化租约回收 Worker。
  func NewMutexReclaimWorker(engine *Engine, rs ReclaimStore, cfg WorkerConfig) *MutexReclaimWorker {
	  return &MutexReclaimWorker{engine: engine,
		  store: rs, cfg: cfg.withDefaults(), quit: make(chan struct{})}
   }

// Start 启动后台回收 Goroutine。
   func (w *MutexReclaimWorker) Start() {
	   w.wg.Add(1)
	  go func() {
		  defer w.wg.Done()
		   ticker := time.NewTicker(w.cfg.Interval)
		  defer ticker.Stop()
		  for {
			  select {
			   case <-w.quit:
				   return
			   case <-ticker.C:
				   _ = w.RunOnce()
			   }
		   }
	   }()
  }

// Stop 优雅关闭。
   func (w *MutexReclaimWorker) Stop() {
	  close(w.quit)
	  w.wg.Wait()
  }

// RunOnce 执行一轮租约回收。

  func (w *MutexReclaimWorker) RunOnce() error {
	   now := w.engine.clock()
	  cases, err := w.store.ListStaleLeaseCases(now)
	   if err != nil {
		  return fmt.Errorf("MutexReclaimWorker ListStaleLeaseCases: %w", err)
	  }
	   batch := cases
	  if len(batch) > w.cfg.BatchSize {
		   batch = batch[:w.cfg.BatchSize]
	   }
	  for _, rc := range batch {
		   // 回退状态到 ready；世代保持，防止旧世代 Execute 通过。
		   rc.State = StateReady
		  rc.UpdatedAt = now
		   rc.Fence.Epoch++
		  if err := w.engine.store.PutCase(rc); err != nil {
			   continue
		   }
		   // 过期租约标记。
		   if l, lerr := w.engine.store.GetLease(rc.RequestID); lerr == nil {
			  l.ExpiresAt = now.Add(-time.Second)
			   l.UpdatedAt = now
			   _ = w.engine.store.PutLease(l)
		   }
	  }
	  return nil
  }

// ──────────────────────────────────────────────────
// EffectAlignWorker
// ──────────────────────────────────────────────────
// AlignStore 一致巡检所需的额外读写能力。

type AlignStore interface {
	Store
	// ListExecutingCases 返回 State==executing 的案例（用于效果证明一致巡检）。
	ListExecutingCases() ([]*ReleaseCase, error)
}

// EffectAlignWorker 巡检 State==executing 但 EffectProof 已存在的案例，
// 补偿调 MarkExecuted 确保 case 最终一致性。
// 场景：Execute 中 proof 写成功，但 PutCase 失败导致状态未翻。
type EffectAlignWorker struct {
	engine *Engine
	store AlignStore
	cfg WorkerConfig
	quit chan struct{}
	wg   sync.WaitGroup
}

// NewEffectAlignWorker 初始化一致巡检 Worker。
   func NewEffectAlignWorker(engine *Engine, as AlignStore, cfg WorkerConfig) *EffectAlignWorker {
	   return &EffectAlignWorker{engine: engine, store: as, cfg: cfg.withDefaults(), quit: make(chan struct{})}
  }

// Start 启动后台巡检 Goroutine。
  func (w *EffectAlignWorker) Start() {
	  w.wg.Add(1)
	   go func() {
		  defer w.wg.Done()
		  ticker := time.NewTicker(w.cfg.Interval)
		  defer ticker.Stop()
		   for {
			  select {
			   case <-w.quit:
				   return
			   case <-ticker.C:
				   _ = w.RunOnce()
			   }
		   }
	   }()
  }

// Stop 优雅关闭。

   func (w *EffectAlignWorker) Stop() {
	   close(w.quit)
	  w.wg.Wait()
   }

// RunOnce 执行一轮一致巡检。
  func (w *EffectAlignWorker) RunOnce() error {
	  cases, err := w.store.ListExecutingCases()
	   if err != nil {
		  return fmt.Errorf("EffectAlignWorker ListExecutingCases: %w", err)
	   }
	  batch := cases
	   if len(batch) > w.cfg.BatchSize {
		  batch = batch[:w.cfg.BatchSize]
	   }
	   now := w.engine.clock()
	  for _, rc := range batch {
		  // 检查是否已有效果证明。
		  proof, err := w.engine.store.GetProof(rc.RequestID, rc.Gen)
		  if err != nil {
			  // 证明不存在，无法补偿标 executed；跳过。
			   continue
		  }
		   // 补偿：将 State 推进到 executed。
		   rc.State = StateExecuted
		   rc.FundActionID = proof.FundActionID
		  rc.EffectProofID = proof.ID
		   rc.UpdatedAt = now
		   rc.Fence.Epoch++
		  rc.Fence.Token = newID()
		  _ = w.engine.store.PutCase(rc)
	  }
	   return nil
  }

// ──────────────────────────────────────────────────
// FullMemReclaimStore / FullMemAlignStore（辅助包装）
// ──────────────────────────────────────────────────
// FullMemStore 扩展 MemStore 实现 ReclaimStore 和 AlignStore 所需的额外方法。

type FullMemStore struct {
	*MemStore
}

// NewFullMemStore 初始化完整内存 Store。
  func NewFullMemStore() *FullMemStore {
	   return &FullMemStore{MemStore: NewMemStore()}
   }

// ListStaleLeaseCases 返回租约过期但状态仍为中间态的案例。
  func (f *FullMemStore) ListStaleLeaseCases(now time.Time) ([]*ReleaseCase, error) {
	   f.mu.Lock()
	   defer f.mu.Unlock()
	   out := make([]*ReleaseCase, 0)
	   for _, rc := range f.cases {
		  if rc.State != StateUnlocking && rc.State != StateExecuting {
			  continue
		   }
		   l, ok := f.leases[rc.RequestID]
		  if !ok {
			  continue
		   }
		  if now.After(l.ExpiresAt) {
			  cp := *rc
			   out = append(out, &cp)
		  }
	   }
	   return out, nil
   }

// ListExecutingCases 返回所有 State==executing 的案例。

   func (f *FullMemStore) ListExecutingCases() ([]*ReleaseCase, error) {
	  f.mu.Lock()
	  defer f.mu.Unlock()
	  out := make([]*ReleaseCase, 0)
	  for _, rc := range f.cases {
		  if rc.State == StateExecuting {
			  cp := *rc
			  out = append(out, &cp)
		   }
	  }
	  return out, nil
   }
