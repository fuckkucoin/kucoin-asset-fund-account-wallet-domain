package isodebt

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/isobook"
)

// ─────────────────────────────────────────────
// RatioRecomputePump
// ─────────────────────────────────────────────
// RatioRecomputePump 周期重算所有在 isobook 中存在的 Pair 的 DebtRatio。
// 重算仅更新比率快照，不递增世代。

type RatioRecomputePump struct {
	svc  *Service
	book *isobook.Book
	interval time.Duration
	clock func() time.Time
}

// NewRatioRecomputePump 构造重算泵。
  func NewRatioRecomputePump(svc *Service, book *isobook.Book, interval time.Duration) *RatioRecomputePump {
	   return &RatioRecomputePump{
		  svc:  svc,
		  book: book,
		   interval: interval,
		   clock:    func() time.Time { return time.Now().UTC() },
	  }
  }

// WithClock 注入时钟。
   func (p *RatioRecomputePump) WithClock(c func() time.Time) *RatioRecomputePump {
	  if p != nil && c != nil {
		  p.clock = c
	  }
	  return p
  }

  // Run 启动重算泵（阻塞直到 ctx 取消）。
  func (p *RatioRecomputePump) Run(ctx context.Context) error {
	  ticker := time.NewTicker(p.interval)
	   defer ticker.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			  return ctx.Err()
		  case <-ticker.C:
			  p.tick(ctx)
		  }
	  }
  }

// Tick 单次重算（可在中手动调用）。

  func (p *RatioRecomputePump) Tick(ctx context.Context) {
	  p.tick(ctx)
   }

   func (p *RatioRecomputePump) tick(ctx context.Context) {
	  // 遍历 book 中所有 Pair（通过 List("", "")）
	   // isobook.Book.List 支持空 userID 返回空；此处用已知 userID 模式不可行。
	   // 中应从 Store 读取世代索引；中通过显式 RecomputeRatio 调用触发。
	  // 此处以 Store 层扫描 generations 实现（内存实现通过公开方法暴露）。
	   pairs := p.scanActivePairs(ctx)
	  for _, pair := range pairs {
		  if _, err := p.svc.RecomputeRatio(ctx, pair[0], pair[1]); err != nil {
			  log.Printf("isodebt: recompute ratio [%s|%s]: %v", pair[0], pair[1], err)
		   }
	  }
   }

// scanActivePairs 扫描 Store 中有世代记录的 Pair 列表。
// 若 store 是 MemStore，直接遍历 generations；否则依赖接口扩展或外部注入。

  func (p *RatioRecomputePump) scanActivePairs(_ context.Context) [][2]string {
	  ms, ok := p.svc.store.(*MemStore)
	   if !ok {
		  return nil
	  }
	  ms.mu.Lock()
	   defer ms.mu.Unlock()
	  seen := map[string]bool{}
	   var out [][2]string
	  for key := range ms.generations {
		  if seen[key] {
			  continue
		   }
		   seen[key] = true
		  parts := strings.SplitN(key, "|", 2)
		  if len(parts) == 2 {
			   out = append(out, [2]string{parts[0], parts[1]})
		   }
	   }
	  return out
  }

// ─────────────────────────────────────────────
// DriftPatrolWorker
// ─────────────────────────────────────────────
// DriftPatrolWorker 周期扫描 open 漂移案例，重新检测并确认是否已收口。

type DriftPatrolWorker struct {
	svc      *Service
	interval time.Duration
	limit    int
}

// NewDriftPatrolWorker 构造漂移巡检 Worker。
  func NewDriftPatrolWorker(svc *Service, interval time.Duration) *DriftPatrolWorker {
	   return &DriftPatrolWorker{
		   svc:      svc,
		  interval: interval,
		   limit: 200,
	   }
   }

// Run 启动巡检（阻塞直到 ctx 取消）。
   func (w *DriftPatrolWorker) Run(ctx context.Context) error {
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

// Tick 单次巡检（中手动调用）。

  func (w *DriftPatrolWorker) Tick(ctx context.Context) {
	   w.tick(ctx)
  }

  func (w *DriftPatrolWorker) tick(ctx context.Context) {
	  cases, err := w.svc.store.ListAllOpenDriftCases(ctx, w.limit)
	   if err != nil {
		   log.Printf("isodebt: drift patrol list: %v", err)
		   return
	  }
	   for _, dc := range cases {
		  res, err := w.svc.DetectDrift(ctx, dc.UserID, dc.Symbol)
		  if err != nil {
			   log.Printf("isodebt: drift patrol detect [%s|%s]: %v", dc.UserID, dc.Symbol, err)
			  continue
		   }
		  if !res.Drifted {
			   // 漂移已自然消失（比率已同步）——此处仅记录日志；实际关闭需要人工四眼确认
			  log.Printf("isodebt: drift patrol [%s|%s] case=%s delta resolved", dc.UserID, dc.Symbol, dc.ID)
		  }
	  }
   }

// ─────────────────────────────────────────────
// TransferGateBlockWorker
// ─────────────────────────────────────────────
// TransferGateBlockWorker 扫描 pending TransferGate，对因漂移/超限应被拒绝的请求主动 reject。
type TransferGateBlockWorker struct {
	svc      *Service
	interval time.Duration
	limit    int
}

// NewTransferGateBlockWorker 构造转入门阻断 Worker。
  func NewTransferGateBlockWorker(svc *Service, interval time.Duration) *TransferGateBlockWorker {
	   return &TransferGateBlockWorker{
		  svc: svc,
		   interval: interval,
		   limit: 100,
	   }
  }

// Run 启动（阻塞直到 ctx 取消）。
   func (w *TransferGateBlockWorker) Run(ctx context.Context) error {
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

// Tick 单次扫描（中手动调用）。
   func (w *TransferGateBlockWorker) Tick(ctx context.Context) {
	   w.tick(ctx)
  }

  func (w *TransferGateBlockWorker) tick(ctx context.Context) {
	   gates, err := w.svc.store.ListPendingGates(ctx, w.limit)
	  if err != nil {
		   log.Printf("isodebt: gate block worker list: %v", err)
		   return
	   }
	   now := time.Now().UTC()
	  for _, g := range gates {
		  blocked, reason, err := w.svc.IsTransferInBlocked(ctx, g.UserID, g.Symbol)
		  if err != nil {
			   log.Printf("isodebt: gate block check [%s]: %v", g.ID, err)
			   continue
		  }
		  if blocked {
			   g.State = GateStateRejected
			  g.RejectReason = fmt.Sprintf("blocked by worker: %s", reason)
			   g.AdjudicatedAt = &now
			   if err := w.svc.store.SaveGate(ctx, g); err != nil {
				  log.Printf("isodebt: gate block save [%s]: %v", g.ID, err)
			   }
		  }
	   }
   }

// ─────────────────────────────────────────────
// WorkerGroup
// ─────────────────────────────────────────────
// WorkerGroup 聚合三类 Worker 的启动控制。
type WorkerGroup struct {
	recompute   *RatioRecomputePump
	driftPatrol *DriftPatrolWorker
	gateBlock *TransferGateBlockWorker
}

// NewWorkerGroup 构造 WorkerGroup。

  func NewWorkerGroup(svc *Service, book *isobook.Book, cfg Config) *WorkerGroup {
	   return &WorkerGroup{
		   recompute: NewRatioRecomputePump(svc, book, cfg.RecomputeInterval),
		  driftPatrol: NewDriftPatrolWorker(svc, cfg.DriftPatrolInterval),
		  gateBlock: NewTransferGateBlockWorker(svc,
			   20*time.Second),
	  }
   }

// Run 并行启动所有 Worker（阻塞直到第一个 Worker 退出或 ctx 取消）。

   func (wg *WorkerGroup) Run(ctx context.Context) error {
	   errCh := make(chan error, 3)
	  go func() { errCh <- wg.recompute.Run(ctx) }()
	  go func() { errCh <- wg.driftPatrol.Run(ctx) }()
	  go func() { errCh <- wg.gateBlock.Run(ctx) }()
	   return <-errCh
   }
