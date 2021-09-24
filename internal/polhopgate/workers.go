package polhopgate

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/acctpol"
)

// ─── WorkerConfig ─────────────────────────────────────────────────────────
// WorkerConfig Worker 公共配置。

type WorkerConfig struct {
	Interval time.Duration
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

// ─── TokenExpiryWorker ────────────────────────────────────────────────────
// TokenExpiryWorker 定期将超 TTL 的 issued 令牌标记为 expired。
type TokenExpiryWorker struct {
	store Store
	cfg   WorkerConfig
	clock func() time.Time
	quit  chan struct{}
	wg    sync.WaitGroup
}

// NewTokenExpiryWorker 初始化令牌过期 Worker。
  func NewTokenExpiryWorker(store Store, cfg WorkerConfig) *TokenExpiryWorker {
	   return &TokenExpiryWorker{
		   store: store,
		   cfg: cfg.withDefaults(),
		  clock: time.Now,
		  quit:  make(chan struct{}),
	   }
   }

// Start 启动后台 Goroutine。

   func (w *TokenExpiryWorker) Start() {
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
					   _ = fmt.Errorf("TokenExpiryWorker: %v", err)
				  }
			   }
		   }
	  }()
  }

// Stop 优雅关闭。

   func (w *TokenExpiryWorker) Stop() { close(w.quit); w.wg.Wait() }

// RunOnce 执行一轮令牌过期扫描（供直接调用）。
   func (w *TokenExpiryWorker) RunOnce(ctx context.Context) error {
	   now := w.clock()
	   tokens, err := w.store.ListExpiredTokens(ctx, now, w.cfg.BatchSize)
	   if err != nil {
		  return fmt.Errorf("TokenExpiryWorker list: %w", err)
	  }
	  var lastErr error
	  for _, tok := range tokens {
		  if tok.State != TokenStateIssued {
			   continue
		  }
		  tok.State = TokenStateExpired
		  tok.UpdatedAt = now
		   if uErr := w.store.UpdateToken(ctx, tok); uErr != nil {
			  lastErr = uErr
		   }
	  }
	   return lastErr
   }

// ─── PublishWindowWorker ──────────────────────────────────────────────────
// PublishWindowWorker 扫描超 TTL 的策略发布互斥租约，防止死锁。

type PublishWindowWorker struct {
	store Store
	cfg   WorkerConfig
	clock func() time.Time
	quit  chan struct{}
	wg    sync.WaitGroup
}

// NewPublishWindowWorker 初始化发布窗口扫描 Worker。

   func NewPublishWindowWorker(store Store, cfg WorkerConfig) *PublishWindowWorker {
	   return &PublishWindowWorker{
		   store: store,
		   cfg:   cfg.withDefaults(),
		   clock: time.Now,
		  quit:  make(chan struct{}),
	   }
  }

// Start 启动后台 Goroutine。
   func (w *PublishWindowWorker) Start() {
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
					   _ = fmt.Errorf("PublishWindowWorker: %v", err)
				   }
			  }
		   }
	   }()
  }

// Stop 优雅关闭。

  func (w *PublishWindowWorker) Stop() { close(w.quit); w.wg.Wait() }

// RunOnce 执行一轮发布窗口扫描。

  func (w *PublishWindowWorker) RunOnce(ctx context.Context) error {
	  now := w.clock()
	   leases, err := w.store.ListExpiredLeases(ctx, now, w.cfg.BatchSize)
	  if err != nil {
		  return fmt.Errorf("PublishWindowWorker list leases: %w", err)
	   }
	  var lastErr error
	  for _, l := range leases {
		   if l.State != LeaseStateAcquired {
			  continue
		   }
		  l.State = LeaseStateExpired
		  l.UpdatedAt = now
		   if uErr := w.store.UpdateLease(ctx, l); uErr != nil {
			   lastErr = uErr
		   }
	  }
	   return lastErr
  }

// ─── DualReadEscalateWorker ───────────────────────────────────────────────
// DualReadEscalateWorker 周期统计 DualRead 分歧，超阈时触发升级挡写。
// 与 Engine.RecordDualRead 中的即时判断互补：
// - Engine.RecordDualRead 在每次双读时即时检查（实时路径）。
// - DualReadEscalateWorker 定期全量扫描（补偿丢失的即时升级）。

type DualReadEscalateWorker struct {
	engine   *Engine
	polStore acctpol.HopStore
	cfg WorkerConfig
	clock func() time.Time
	bizKinds []string // 待扫描的业务类型
	quit     chan struct{}
	wg       sync.WaitGroup
}

// NewDualReadEscalateWorker 初始化双读升级 Worker。

  func NewDualReadEscalateWorker(engine *Engine, polStore acctpol.HopStore, bizKinds []string, cfg WorkerConfig) *DualReadEscalateWorker {
	  return &DualReadEscalateWorker{
		  engine:   engine,
		  polStore: polStore,
		  cfg:      cfg.withDefaults(),
		  clock:    time.Now,
		  bizKinds: bizKinds,
		  quit:     make(chan struct{}),
	  }
  }

// Start 启动后台 Goroutine。

  func (w *DualReadEscalateWorker) Start() {
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
					   _ = fmt.Errorf("DualReadEscalateWorker: %v", err)
				  }
			   }
		  }
	   }()
   }

// Stop 优雅关闭。
  func (w *DualReadEscalateWorker) Stop() { close(w.quit); w.wg.Wait() }

// RunOnce 执行一轮双读分歧统计与升级。
  func (w *DualReadEscalateWorker) RunOnce(ctx context.Context) error {
	  now := w.clock()
	   since := now.Add(-w.engine.cfg.DualDivergeWindow)
	  var lastErr error
	  for _, bk := range w.bizKinds {
		  count, err := w.polStore.CountDivergeLogs(ctx, acctpol.HopBizKind(bk), since)
		  if err != nil {
			  lastErr = err
			   continue
		   }
		  if count < w.engine.cfg.DualDivergeThreshold {
			   continue
		   }
		   // 检查是否已有活跃升级
		   if _, err := w.engine.store.GetActiveEscalate(ctx, "", "", bk); err == nil {
			   continue // 已有全局升级，幂等
		  }
		   // 取最近一条分歧日志获取世代信息
		  logs, logErr := w.polStore.ListDualReadLogs(ctx, acctpol.HopBizKind(bk), since, 1)
		   if logErr != nil || len(logs) == 0 {
			  continue
		  }
		  latest := logs[0]
		   _ = w.engine.RecordDualRead(ctx, DualReadRequest{
			   UserID: "",
			   Asset:      latest.Asset,
			   BizKind:    bk,
			   PrimaryGen: latest.PrimaryGen,
			   ReplicaGen: latest.ReplicaGen,
			   TraceID:    "worker-escalate",
		   })
	  }
	  return lastErr
  }

// ─── ProofSealWorker ──────────────────────────────────────────────────────
// ProofSealWorker 为 open PolDriftCase 产出 DriftProof，推进至 proven。
// 证明字段：
// ProofHash = SHA-256(case_id ‖ ":" ‖ trigger_gen ‖ ":" ‖ replica_gen ‖ ":" ‖ sealed_at_unix)
type ProofSealWorker struct {
	store Store
	cfg   WorkerConfig
	clock func() time.Time
	quit  chan struct{}
	wg    sync.WaitGroup
}

// NewProofSealWorker 初始化证明固化 Worker。
   func NewProofSealWorker(store Store, cfg WorkerConfig) *ProofSealWorker {
	   return &ProofSealWorker{
		   store: store,
		   cfg: cfg.withDefaults(),
		  clock: time.Now,
		  quit: make(chan struct{}),
	   }
   }

// Start 启动后台 Goroutine。

  func (w *ProofSealWorker) Start() {
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
					  _ = fmt.Errorf("ProofSealWorker: %v", err)
				   }
			   }
		  }
	   }()
   }

// Stop 优雅关闭。
   func (w *ProofSealWorker) Stop() { close(w.quit); w.wg.Wait() }

// RunOnce 执行一轮证明固化。
  func (w *ProofSealWorker) RunOnce(ctx context.Context) error {
	   now := w.clock()
	   cases, err := w.store.ListDriftCasesByState(ctx, DriftCaseOpen, w.cfg.BatchSize)
	   if err != nil {
		  return fmt.Errorf("ProofSealWorker list cases: %w", err)
	  }
	   var lastErr error
	   for _, dc := range cases {
		  // 幂等：已有证明则跳过
		   if _, proofErr := w.store.GetProofByCaseID(ctx, dc.ID); proofErr == nil {
			  // 证明存在但案例仍 open，推进到 proven
			  if tErr := dc.Transition(DriftCaseProven); tErr == nil {
				   dc.State = DriftCaseProven
				   dc.UpdatedAt = now
				  _ = w.store.UpdateDriftCase(ctx, dc)
			   }
			  continue
		  }
		   // 产出证明
		   sealedAt := now
		  raw := fmt.Sprintf("%s:%d:%d:%d", dc.ID, dc.TriggerGen, dc.ReplicaGen, sealedAt.Unix())
		   h := sha256.Sum256([]byte(raw))
		  proofHash := fmt.Sprintf("%x", h)
		   proof := &DriftProof{
			   ID:         uuid.NewString(),
			  CaseID:     dc.ID,
			  TriggerGen: dc.TriggerGen,
			   ReplicaGen: dc.ReplicaGen,
			   ProofHash: proofHash,
			  SealedAt: sealedAt,
			   CreatedAt: now,
		   }
		   if pErr := w.store.InsertProof(ctx, proof); pErr != nil {
			  lastErr = pErr
			  continue
		  }
		   // 推进案例至 proven
		  if tErr := dc.Transition(DriftCaseProven); tErr != nil {
			   lastErr = tErr
			   continue
		  }
		  dc.State = DriftCaseProven
		  dc.ProofID = proof.ID
		  dc.ProofHash = proofHash
		  dc.UpdatedAt = now
		   if uErr := w.store.UpdateDriftCase(ctx, dc); uErr != nil {
			   lastErr = uErr
		  }
	   }
	  return lastErr
   }
