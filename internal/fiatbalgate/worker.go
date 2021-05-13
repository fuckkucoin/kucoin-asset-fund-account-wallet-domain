package fiatbalgate

import (
	"context"
	"time"
	"github.com/google/uuid"
)

const (
	defaultScanInterval = 60 * time.Second
	defaultExpiryInterval = 30 * time.Second
	defaultGenSyncInterval = 15 * time.Second
	defaultOpenStaleTTL    = 2 * time.Minute
	workerBatchSize        = 200
	workerActor = "fiatbalgate.worker"
)

// GenProducer 世代快照者接口；由 fiatjnl.Service 实现（或注入存根）。
// 供 GenSyncWorker 拉取新世代快照写入 fiat_jnl_gens 表。
// SyncGenReq 与 fiatjnl.JournalGen 字段一一对应；调用方通过适配器转换。
type GenProducer interface {
	// PollGens 返回 seq > sinceSeq 的新世代快照（最多 limit 条）。
	PollGens(ctx context.Context, sinceSeq uint64, limit int) ([]SyncGenReq, error)
}

// WorkerConfig Worker 配置。

type WorkerConfig struct {
	// ScanInterval 不平衡巡检周期（默认 60s）。
	ScanInterval time.Duration
	// ExpiryInterval open→blocking 补推进周期（默认 30s）。
	ExpiryInterval time.Duration
	// GenSyncInterval 世代快照同步周期（默认 15s）。
	GenSyncInterval time.Duration
	// OpenStaleTTL open 状态容忍时间（默认 2m）；超过则强制推进到 blocking。
	OpenStaleTTL time.Duration
	// BatchSize 单批最大处理数（默认 200）。
	BatchSize int
	// Assets 指定只扫描的资产列表；空表示扫描所有已知资产。
	Assets []string
}

  func (c *WorkerConfig) fill() {
	   if c.ScanInterval <= 0 {
		  c.ScanInterval = defaultScanInterval
	  }
	  if c.ExpiryInterval <= 0 {
		   c.ExpiryInterval = defaultExpiryInterval
	  }
	  if c.GenSyncInterval <= 0 {
		  c.GenSyncInterval = defaultGenSyncInterval
	  }
	   if c.OpenStaleTTL <= 0 {
		  c.OpenStaleTTL = defaultOpenStaleTTL
	  }
	  if c.BatchSize <= 0 {
		   c.BatchSize = workerBatchSize
	  }
  }

// Worker fiatbalgate 后台 Worker 集合。
type Worker struct {
	engine  *Engine
	scanner *Scanner
	store   Store
	producer  GenProducer
	config    WorkerConfig
	clock     func() time.Time
	scannerID string
}

// NewWorker 构造 Worker。producer 为 nil 时跳过世代同步。
   func NewWorker(engine *Engine, scanner *Scanner, store Store, producer GenProducer, cfg WorkerConfig) *Worker {
	   cfg.fill()
	  clk := func() time.Time { return time.Now().UTC() }
	  if engine != nil && engine.clock != nil {
		   clk = engine.clock
	  }
	  return &Worker{
		  engine:  engine,
		   scanner: scanner,
		   store:    store,
		  producer: producer,
		   config:    cfg,
		   clock:     clk,
		  scannerID: "worker-" + uuid.NewString()[:8],
	  }
   }

// Run 启动所有后台 goroutine，直到 ctx 取消。
// 调用方应在独立 goroutine 中运行：go worker.Run(ctx)。

   func (w *Worker) Run(ctx context.Context) {
	  scanTicker := time.NewTicker(w.config.ScanInterval)
	   expiryTicker := time.NewTicker(w.config.ExpiryInterval)
	  genTicker := time.NewTicker(w.config.GenSyncInterval)
	  defer scanTicker.Stop()
	   defer expiryTicker.Stop()
	   defer genTicker.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			  return
		  case <-scanTicker.C:
			  w.runScan(ctx)
		   case <-expiryTicker.C:
			   w.runExpiryPromotion(ctx)
		  case <-genTicker.C:
			   w.runGenSync(ctx)
		   }
	  }
  }

// runScan 执行一次不平衡巡检（每个 config.Assets 资产或全量）。
   func (w *Worker) runScan(ctx context.Context) {
	   if w.scanner == nil {
		   return
	   }
	  scanAsset := ""
	  if len(w.config.Assets) == 1 {
		   scanAsset = w.config.Assets[0]
	   }
	   req := ScanReq{
		  Asset:     scanAsset,
		  ScannerID: w.scannerID,
	   }
	   _, _ = w.scanner.Scan(ctx, req)
   }

// runExpiryPromotion 推进超期 open→blocking（Worker 补丁：防止扫描器丢失）。
  func (w *Worker) runExpiryPromotion(ctx context.Context) {
	  if w.store == nil || w.engine == nil {
		  return
	   }
	   threshold := w.clock().Add(-w.config.OpenStaleTTL)
	   cases, err := w.store.ListExpiredOpenCases(ctx, threshold, w.config.BatchSize)
	  if err != nil {
		  return
	  }
	  for _, c := range cases {
		   _,
			  _ = w.engine.PromoteToBlocking(ctx, c.CaseID)
	   }
   }

// runGenSync 从 GenProducer 拉取新世代快照并写入 fiat_jnl_gens。

   func (w *Worker) runGenSync(ctx context.Context) {
	  if w.producer == nil || w.store == nil || w.engine == nil {
		   return
	  }
	  sinceSeq, err := w.store.MaxGenSeq(ctx)
	   if err != nil {
		   return
	  }
	  gens, err := w.producer.PollGens(ctx, sinceSeq, w.config.BatchSize)
	   if err != nil {
		   return
	   }
	   for _, g := range gens {
		   _ = w.engine.SyncGen(ctx, g)
	  }
   }
