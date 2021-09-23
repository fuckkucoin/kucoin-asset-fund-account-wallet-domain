package overwdlease

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"
	"github.com/google/uuid"
)

// ─── WorkerConfig ─────────────────────────────────────────────────────────
// WorkerConfig Worker 公共配置。
type WorkerConfig struct {
	Interval  time.Duration
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

// ─── BindExpiryWorker ─────────────────────────────────────────────────────
// BindExpiryWorker 定期将超 TTL 的 open PrecheckBind 标记为 expired。
// 触发条件：bind.expires_at < now AND bind.state = open。
// 幂等：已是终态的 bind 直接跳过。

type BindExpiryWorker struct {
	store Store
	cfg   WorkerConfig
	clock func() time.Time
	quit  chan struct{}
	wg sync.WaitGroup
}

// NewBindExpiryWorker 初始化绑定过期吊销 Worker。
   func NewBindExpiryWorker(store Store, cfg WorkerConfig) *BindExpiryWorker {
	   return &BindExpiryWorker{
		  store: store,
		  cfg:   cfg.withDefaults(),
		   clock: time.Now,
		   quit: make(chan struct{}),
	  }
   }

// Start 启动后台 Goroutine。
   func (w *BindExpiryWorker) Start() {
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
				  if err := w.RunOnce(context.Background()); err != nil {
					  _ = fmt.Errorf("BindExpiryWorker: %v", err)
				  }
			   }
		   }
	  }()
  }

// Stop 优雅关闭。
   func (w *BindExpiryWorker) Stop() { close(w.quit); w.wg.Wait() }

// RunOnce 执行一轮绑定过期扫描（供直接调用）。
   func (w *BindExpiryWorker) RunOnce(ctx context.Context) error {
	   now := w.clock()
	  binds, err := w.store.ListExpiredBinds(ctx, now, w.cfg.BatchSize)
	   if err != nil {
		  return fmt.Errorf("BindExpiryWorker list: %w", err)
	   }
	  var lastErr error
	   for _, b := range binds {
		  if b.State != BindStateOpen {
			  continue
		   }
		  b.State = BindStateExpired
		  b.UpdatedAt = now
		  if uErr := w.store.UpdateBind(ctx, b); uErr != nil {
			   lastErr = uErr
		   }
	  }
	   return lastErr
  }

// ─── ProofChaseWorker ─────────────────────────────────────────────────────
// ProofChaseWorker 差异证明催办 Worker。
// 周期扫描所有 open VarianceProof；对于每个 open 证明，记录催办日志并可触发外部告警。
// 实际告警机制（Slack/PagerDuty 等）由调用方注入 NotifyFn 实现；Worker 不硬编码通知渠道。
type ProofChaseWorker struct {
	store    Store
	cfg      WorkerConfig
	clock    func() time.Time
	notifyFn func(proof *VarianceProof) error // 可为 nil（仅日志）
	quit     chan struct{}
	wg       sync.WaitGroup
}

// NewProofChaseWorker 初始化差异证明催办 Worker。
  func NewProofChaseWorker(store Store, cfg WorkerConfig, notifyFn func(*VarianceProof) error) *ProofChaseWorker {
	  return &ProofChaseWorker{
		  store: store,
		  cfg:   cfg.withDefaults(),
		   clock: time.Now,
		  notifyFn: notifyFn,
		  quit:     make(chan struct{}),
	}
}

// Start 启动后台 Goroutine。
  func (w *ProofChaseWorker) Start() {
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
				  if err := w.RunOnce(context.Background()); err != nil {
					  _ = fmt.Errorf("ProofChaseWorker: %v", err)
				   }
			   }
		  }
	  }()
  }

// Stop 优雅关闭。
   func (w *ProofChaseWorker) Stop() { close(w.quit); w.wg.Wait() }

// RunOnce 执行一轮差异证明催办（供直接调用）。
  func (w *ProofChaseWorker) RunOnce(ctx context.Context) error {
	  proofs, err := w.store.ListProofsByState(ctx, ProofStateOpen, w.cfg.BatchSize)
	   if err != nil {
		  return fmt.Errorf("ProofChaseWorker list: %w", err)
	   }
	   var lastErr error
	   for _, p := range proofs {
		  if w.notifyFn != nil {
			  if nErr := w.notifyFn(p); nErr != nil {
				  lastErr = nErr
			   }
		   }
		   // 记录催办标记（仅日志路径，不改写证明状态）
		   _ = fmt.Sprintf("ProofChaseWorker: proof_id=%s user=%s asset=%s trigger=%s drift_gen=%d sealed_at=%s",
			   p.ID, p.UserID, p.Asset, p.Trigger, p.DriftGen, p.SealedAt.Format(time.RFC3339))
	  }
	  return lastErr
  }

// ─── DriftPatrolWorker ────────────────────────────────────────────────────
// DriftPatrolWorker 漂移巡检 Worker。
// 周期扫描所有 open OverwdDriftCase；对于已有关联 VarianceProof 的案例，
// 产出固化证明摘要并将案例推进至 proven（待管理员四眼确认）。
// 对于无关联证明的 no_bind / expired_bind 类型案例，直接固化摘要并推进。

type DriftPatrolWorker struct {
	store Store
	cfg   WorkerConfig
	clock func() time.Time
	quit  chan struct{}
	wg sync.WaitGroup
}

// NewDriftPatrolWorker 初始化漂移巡检 Worker。
   func NewDriftPatrolWorker(store Store, cfg WorkerConfig) *DriftPatrolWorker {
	  return &DriftPatrolWorker{
		   store: store,
		   cfg:   cfg.withDefaults(),
		  clock: time.Now,
		  quit: make(chan struct{}),
	  }
  }

// Start 启动后台 Goroutine。
   func (w *DriftPatrolWorker) Start() {
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
				   if err := w.RunOnce(context.Background()); err != nil {
					   _ = fmt.Errorf("DriftPatrolWorker: %v", err)
				   }
			  }
		  }
	   }()
   }

// Stop 优雅关闭。

  func (w *DriftPatrolWorker) Stop() { close(w.quit); w.wg.Wait() }

// RunOnce 执行一轮漂移案例巡检（供直接调用）。
// 对每个 open 案例：
// 1. 若 ProofID 已存在，直接推进至 proven。
// 2. 若 ProofID 为空（no_bind/expired_bind 路径），产出摘要证明后推进。

  func (w *DriftPatrolWorker) RunOnce(ctx context.Context) error {
	   now := w.clock()
	   cases, err := w.store.ListDriftCasesByState(ctx, DriftCaseOpen, w.cfg.BatchSize)
	   if err != nil {
		   return fmt.Errorf("DriftPatrolWorker list: %w", err)
	   }
	   var lastErr error
	  for _, dc := range cases {
		   if err := w.processCase(ctx, dc, now); err != nil {
			  lastErr = err
		   }
	   }
	  return lastErr
   }

// processCase 处理单个漂移案例。
   func (w *DriftPatrolWorker) processCase(ctx context.Context, dc *OverwdDriftCase, now time.Time) error {
	  if dc.ProofID != "" {
		   // 已有证明，检查证明是否存在
		   _,
			   pErr := w.store.GetProofByID(ctx, dc.ProofID)
		   if pErr != nil {
			  return nil // 证明尚未写入，等待下一轮
		  }
		  // 推进至 proven
		  if tErr := dc.Transition(DriftCaseProven); tErr != nil {
			  return tErr
		   }
		  dc.State = DriftCaseProven
		   dc.UpdatedAt = now
		  return w.store.UpdateDriftCase(ctx,
			   dc)
	   }
	  // 无关联证明（no_bind/expired_bind 路径），产出内部摘要固化证明
	   if dc.Trigger != DriftTriggerNoBind && dc.Trigger != DriftTriggerExpiredBind {
		  return nil // 其他类型等待主流程写入证明
	  }
	  // 产出固化证明
	   proofID := uuid.NewString()
	   raw := fmt.Sprintf("patrol:%s:%s:%s:%d:%d", proofID, dc.ID, dc.Trigger, dc.DriftGen, now.Unix())
	   h := sha256.Sum256([]byte(raw))
	   proofHash := fmt.Sprintf("%x", h)
	  sealedProof := &VarianceProof{
		  ID: proofID,
		   BindID:   dc.BindID,
		   UserID:   dc.UserID,
		   Asset:    dc.Asset,
		   Trigger:  ProofTrigger(dc.Trigger),
		   DriftGen: dc.DriftGen,
		   State: ProofStateOpen,
		   ProofHash:   proofHash,
		  SealedAt:    now,
		   DriftCaseID: dc.ID,
		  IdemKey:     fmt.Sprintf("patrol:%s", dc.ID),
		  CreatedAt:   now,
		   UpdatedAt: now,
	   }
	  if iErr := w.store.InsertProof(ctx, sealedProof); iErr != nil {
		   return iErr
	  }
	  // 更新案例关联证明并推进至 proven
	  if tErr := dc.Transition(DriftCaseProven); tErr != nil {
		  return tErr
	   }
	  dc.State = DriftCaseProven
	   dc.ProofID = sealedProof.ID
	  dc.ProofHash = proofHash
	  dc.UpdatedAt = now
	  return w.store.UpdateDriftCase(ctx, dc)
   }
