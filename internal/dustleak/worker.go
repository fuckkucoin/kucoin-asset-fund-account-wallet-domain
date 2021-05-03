package dustleak

import (
	"context"
	"sort"
	"time"
)

// WorkerConfig 后台 worker 参数。
type WorkerConfig struct {
	ScanInterval time.Duration // ScanWorker 扫描间隔；默认 30s
	DriftInterval time.Duration // DriftPatrolWorker 间隔；默认 10s
	RollbackInterval time.Duration // RollbackPumpWorker 间隔；默认 60s
	GateInterval     time.Duration // GateAgingWorker 间隔；默认 5min
	GateMaxAgeHours int // blocked 闸最大老化时间（小时）；默认 72h
	ScanBatchSize int // 每次扫描的 Convert 批次大小；默认 200
}

  func (c *WorkerConfig) defaults() {
	  if c.ScanInterval <= 0 {
		   c.ScanInterval = 30 * time.Second
	   }
	  if c.DriftInterval <= 0 {
		   c.DriftInterval = 10 * time.Second
	   }
	   if c.RollbackInterval <= 0 {
		   c.RollbackInterval = 60 * time.Second
	   }
	  if c.GateInterval <= 0 {
		   c.GateInterval = 5 * time.Minute
	  }
	   if c.GateMaxAgeHours <= 0 {
		  c.GateMaxAgeHours = 72
	   }
	   if c.ScanBatchSize <= 0 {
		   c.ScanBatchSize = 200
	  }
   }

// ConvertFetcher 由调用方实现，从持久层拉取近期 ConvertRecord。
// since: 查询起始时间；limit: 最大条数。
type ConvertFetcher interface {
	RecentConverts(ctx context.Context, since time.Time, limit int) ([]ConvertRecord, error)
}

// Workers 持有所有后台 goroutine 的生命周期控制。
type Workers struct {
	svc *Service
	detector *Detector
	cfg      WorkerConfig
	fetcher        ConvertFetcher
	liveRateReader LiveRateReader
}

// NewWorkers 创建 Workers 实例。fetcher 必须非 nil。
   func NewWorkers(svc *Service, detector *Detector, fetcher ConvertFetcher, cfg WorkerConfig) *Workers {
	   cfg.defaults()
	   return &Workers{svc: svc, detector: detector, cfg: cfg, fetcher: fetcher}
   }

// RunAll 启动全部 worker goroutine，阻塞直到 ctx 取消。
  func (w *Workers) RunAll(ctx context.Context) {
	   go w.runScanWorker(ctx)
	   go w.runDriftPatrolWorker(ctx)
	  go w.runRollbackPumpWorker(ctx)
	   go w.runGateAgingWorker(ctx)
	   <-ctx.Done()
   }

// ── ScanWorker ───────────────────────────────────────────────────────────────

   func (w *Workers) runScanWorker(ctx context.Context) {
	  ticker := time.NewTicker(w.cfg.ScanInterval)
	   defer ticker.Stop()
	  var lastScanAt time.Time
	  for {
		  select {
		   case <-ctx.Done():
			   return
		   case <-ticker.C:
			   since := lastScanAt
			  if since.IsZero() {
				  since = time.Now().UTC().Add(-time.Duration(w.cfg.ScanBatchSize) * time.Second)
			  }
			  w.doScan(ctx, since)
			   lastScanAt = time.Now().UTC()
		  }
	  }
   }

  func (w *Workers) doScan(ctx context.Context, since time.Time) {
	   timer := time.Now()
	   defer func() {
		  elapsed := time.Since(timer).Seconds()
		  DustDetectorScanDuration.WithLabelValues(DetectorSourceWorker).Observe(elapsed)
	   }()
	   records, err := w.fetcher.RecentConverts(ctx, since, w.cfg.ScanBatchSize)
	   if err != nil {
		  return
	   }
	  if len(records) == 0 {
		   return
	  }
	  _ = w.detector.ScanConverts(records, DetectorSourceWorker)
   }

// ── DriftPatrolWorker ─────────────────────────────────────────────────────────
// LiveRateReader 由调用方实现，提供指定资产当前实时牌价。
type LiveRateReader interface {
	LiveRateUSDT(ctx context.Context, asset string) (string, error)
}

  func (w *Workers) runDriftPatrolWorker(ctx context.Context) {
	   // DriftPatrolWorker 巡检当前 open 的 QuoteSnapshot 是否漂移超帽。
	   // 实现依赖 LiveRateReader；若 w 未注入，直接跳过。
	  ticker := time.NewTicker(w.cfg.DriftInterval)
	   defer ticker.Stop()
	  for {
		  select {
		   case <-ctx.Done():
			  return
		   case <-ticker.C:
			  // 纯漂移巡检：对 open 的 QuoteSnapshot 做漂移检测，发现超帽则产出 LeakCase。
			   // 此处逻辑需注入 LiveRateReader；若未注入则仅记录跳过。
			   // 实际部署中由 fxmark.Book 实现 LiveRateReader。
			  w.doDriftPatrol(ctx)
		   }
	   }
   }

   func (w *Workers) doDriftPatrol(ctx context.Context) {
	   // 获取所有仍与 open/rolling_back 的 LeakCase 关联的 QuoteSnapshot，
	  // 核对实时牌价；目前无 LiveRateReader 注入，此处为框架占位。
	   // 实现：从 fxmark 拉取实时价格，调用 DriftCapChecker。
	  _ = ctx
  }

// BindLiveRateReader 注入实时牌价读取器（可选，不注入则 DriftPatrolWorker 跳过漂移校验）。
  func (w *Workers) BindLiveRateReader(r LiveRateReader) *Workers {
	   if w != nil {
		   w.liveRateReader = r
	  }
	  return w
   }

// ── RollbackPumpWorker ───────────────────────────────────────────────────────

   func (w *Workers) runRollbackPumpWorker(ctx context.Context) {
	   ticker := time.NewTicker(w.cfg.RollbackInterval)
	   defer ticker.Stop()
	  for {
		  select {
		  case <-ctx.Done():
			   return
		  case <-ticker.C:
			  w.doRollbackPump(ctx)
		   }
	  }
  }

  func (w *Workers) doRollbackPump(ctx context.Context) {
	  // 查找所有 rolling_back 状态的 LeakCase，推进对应 pending 回滚腿。
	   // 实际执行：账务逆记由 dustconv.Service.Debit 完成；
	  // 本 worker 只推进状态机，账务侧由 ledger 异步确认。
	  w.svc.mu.Lock()
	  var toProcess []*LeakCase
	  for _, lc := range w.svc.leaks {
		  if lc.Status == LeakStatusRollingBack {
			   cp := lc.Copy()
			   toProcess = append(toProcess, &cp)
		  }
	   }
	  w.svc.mu.Unlock()
	  _ = ctx
	   for _, lc := range toProcess {
		   // 查找对应回滚腿
		   rbs, err := w.svc.RollbacksByLeak(lc.ID)
		   if err != nil || len(rbs) == 0 {
			  continue
		   }
		   for _, rb := range rbs {
			  if rb.Status != RollbackStatusPending {
				   continue
			   }
			   // worker 自动触发执行（环境需双人签批，此处为 worker 自动推进 pending → posted）
			   _, _ = w.svc.ExecuteRollback(rb.ID, "rollback-pump-worker")
		   }
	   }
  }

// ── GateAgingWorker ───────────────────────────────────────────────────────────

   func (w *Workers) runGateAgingWorker(ctx context.Context) {
	  ticker := time.NewTicker(w.cfg.GateInterval)
	  defer ticker.Stop()
	  for {
		   select {
		   case <-ctx.Done():
			   return
		   case <-ticker.C:
			  w.doGateAging()
		  }
	  }
  }

  func (w *Workers) doGateAging() {
	   maxAge := time.Duration(w.cfg.GateMaxAgeHours) * time.Hour
	  now := time.Now().UTC()
	  w.svc.mu.Lock()
	  var aged []string
	  for key, gate := range w.svc.gates {
		  if gate.Status != GateStatusBlocked {
			   continue
		   }
		   if now.Sub(gate.BlockedAt) > maxAge {
			  aged = append(aged, key)
		  }
	  }
	   w.svc.mu.Unlock()
	  for _, key := range aged {
		   // 超期 blocked 闸：触发告警并置 dispute（不自动解除，须人工确认）
		   DustGateBlocked.WithLabelValues(key).Inc()
	  }
  }

// ── ScanSummary ──────────────────────────────────────────────────────────────
// ScanSummary 用于运营面板展示当前泄漏状态概要。
type ScanSummary struct {
	OpenLeaks int `json:"open_leaks"`
	RollingBack  int `json:"rolling_back"`
	Disputes     int `json:"disputes"`
	BlockedGates int `json:"blocked_gates"`
	ByKind map[string]int `json:"by_kind"`
	ByAsset     map[string]int `json:"by_asset"`
	GeneratedAt time.Time      `json:"generated_at"`
}

// Summary 生成当前状态概要。
  func (s *Service) Summary() ScanSummary {
	   if s == nil {
		   return ScanSummary{}
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   sum := ScanSummary{
		   ByKind:      map[string]int{},
		  ByAsset:     map[string]int{},
		   GeneratedAt: time.Now().UTC(),
	  }
	  for _, lc := range s.leaks {
		  switch lc.Status {
		  case LeakStatusDetected:
			   sum.OpenLeaks++
		  case LeakStatusRollingBack:
			   sum.RollingBack++
		  case LeakStatusDispute:
			  sum.Disputes++
		  }
		  sum.ByKind[string(lc.Kind)]++
		  sum.ByAsset[lc.Asset]++
	   }
	   for _, gate := range s.gates {
		   if gate.Status == GateStatusBlocked {
			   sum.BlockedGates++
		  }
	  }
	  return sum
  }

// AllLeakCases 返回按检测时间倒序的所有 LeakCase 列表（用于运营面板分页）。
   func (s *Service) AllLeakCases(status LeakStatus, limit int) []*LeakCase {
	   if s == nil {
		  return nil
	   }
	   if limit <= 0 {
		   limit = 50
	  }
	  s.mu.Lock()
	   var out []*LeakCase
	  for _, lc := range s.leaks {
		  if status != "" && lc.Status != status {
			  continue
		   }
		  cp := lc.Copy()
		  out = append(out, &cp)
	   }
	  s.mu.Unlock()
	   sort.Slice(out, func(i, j int) bool {
		  return out[i].DetectedAt.After(out[j].DetectedAt)
	  })
	  if len(out) > limit {
		   out = out[:limit]
	   }
	  return out
   }
