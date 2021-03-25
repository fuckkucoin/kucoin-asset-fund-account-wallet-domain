package coolmarkgate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// WorkerConfig Worker 通用配置。

type WorkerConfig struct {
	// PatrolInterval 冷却窗巡检间隔，默认 30s。
	PatrolInterval time.Duration
	// BreachInterval 违约案例巡检间隔，默认 60s。
	BreachInterval time.Duration
	// BatchSize 每批处理条数，默认 50。
	BatchSize int
}

  func (c *WorkerConfig) defaults() {
	   if c.PatrolInterval <= 0 {
		  c.PatrolInterval = 30 * time.Second
	   }
	   if c.BreachInterval <= 0 {
		  c.BreachInterval = 60 * time.Second
	   }
	  if c.BatchSize <= 0 {
		   c.BatchSize = 50
	   }
   }

// ──────────────────────────────────────────────────────────────────────────────
// CoolWindowPatrolWorker
// ──────────────────────────────────────────────────────────────────────────────
// CoolWindowPatrolWorker 冷却窗巡检 Worker。
// 功能：周期性调用 Engine.ExpireStaleLeases，将 CoolUntil 已过期的租约触发 ClearCool，
// 签发 CoolProof 并流转到 cleared 状态，使后续 Exec 可以持新世代放行。
type CoolWindowPatrolWorker struct {
	eng *Engine
	cfg WorkerConfig
}

// NewCoolWindowPatrolWorker 构造冷却窗巡检 Worker。
   func NewCoolWindowPatrolWorker(eng *Engine, cfg WorkerConfig) *CoolWindowPatrolWorker {
	   cfg.defaults()
	  return &CoolWindowPatrolWorker{eng: eng, cfg: cfg}
   }

// Run 启动冷却窗巡检循环；context 取消时退出。

   func (w *CoolWindowPatrolWorker) Run(ctx context.Context) {
	   ticker := time.NewTicker(w.cfg.PatrolInterval)
	  defer ticker.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			  return
		   case <-ticker.C:
			   n, _ := w.eng.ExpireStaleLeases(ctx, w.cfg.BatchSize)
			   if n > 0 {
				   recordPatrolClearedMetric(n)
			   }
		  }
	   }
   }

// ──────────────────────────────────────────────────────────────────────────────
// BreachDetectorWorker
// ──────────────────────────────────────────────────────────────────────────────
// BreachAlert 违约告警回调，由业务层注入；用于上报 Pagerduty/企业微信/Kafka 等。
// 功能：Worker 发现未处置案例时回调，告警内容由外部系统决定。
type BreachAlert func(ctx context.Context, c *CoolBreachCase)

// BreachDetectorWorker 违约发现 Worker。
// 功能：定期扫描 Resolved=false 的 CoolBreachCase，回调 BreachAlert；
// 不自行修改案例 Resolved 状态（须由运营人工或独立修复流程处置）。
type BreachDetectorWorker struct {
	eng *Engine
	alert BreachAlert
	cfg   WorkerConfig
}

// NewBreachDetectorWorker 构造违约发现 Worker。
// alert 为 nil 时跳过告警（仅用于场景）。

  func NewBreachDetectorWorker(eng *Engine, alert BreachAlert, cfg WorkerConfig) *BreachDetectorWorker {
	  cfg.defaults()
	  return &BreachDetectorWorker{eng: eng, alert: alert, cfg: cfg}
  }

// Run 启动违约发现循环；context 取消时退出。
  func (w *BreachDetectorWorker) Run(ctx context.Context) {
	  ticker := time.NewTicker(w.cfg.BreachInterval)
	   defer ticker.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			   return
		  case <-ticker.C:
			  w.detectBreaches(ctx)
		   }
	  }
  }

// detectBreaches 单轮违约发现：取未处置案例列表，逐条回调告警。
// 风险点：回调失败不影响下一条接着处理；ctx 取消则提前退出循环。
   func (w *BreachDetectorWorker) detectBreaches(ctx context.Context) {
	  cases, err := w.eng.cases.ListUnresolved(ctx, w.cfg.BatchSize)
	   if err != nil {
		   return
	  }
	  if w.alert == nil {
		  return
	  }
	   for _, c := range cases {
		  select {
		  case <-ctx.Done():
			  return
		  default:
			  w.alert(ctx, c)
		  }
	  }
   }

// ──────────────────────────────────────────────────────────────────────────────
// StalePlanDetectorWorker
// ──────────────────────────────────────────────────────────────────────────────
// StalePlanEvent 漂移事件：冷却窗内检测到 MarkSent/归集成功信号时产生。
// 功能：由外部归集系统在完成 MarkSent 后回调此 Worker，触发后验漂移检测。
type StalePlanEvent struct {
	// Asset 归集资产代码。
	Asset string
	// ExecID 归集执行标识。
	ExecID string
	// SentAt MarkSent 发生时刻。
	SentAt time.Time
	// CoolGen 执行时携带的世代号（可能是旧世代）。
	CoolGen int64
}

// StalePlanDetectorWorker 漂移执行发现 Worker。
// 功能：接收外部 StalePlanEvent，判断执行是否发生在冷却窗内，
// 若是则构造 CoolBreachCase（Decision=drifted）落库。
type StalePlanDetectorWorker struct {
	eng *Engine
	events chan StalePlanEvent
}

// NewStalePlanDetectorWorker 构造漂移发现 Worker。
// bufSize 为事件队列缓冲大小，建议 256。
   func NewStalePlanDetectorWorker(eng *Engine, bufSize int) *StalePlanDetectorWorker {
	   if bufSize <= 0 {
		  bufSize = 256
	  }
	   return &StalePlanDetectorWorker{
		   eng: eng,
		  events: make(chan StalePlanEvent, bufSize),
	   }
  }

// Submit 向 Worker 提交一个漂移事件（非阻塞）。
// 队列满时返回 false（告警由 Prometheus 指标覆盖）。
   func (w *StalePlanDetectorWorker) Submit(ev StalePlanEvent) bool {
	   select {
	  case w.events <- ev:
		   return true
	  default:
		   return false
	  }
  }

// Run 启动漂移发现循环；context 取消时退出。

  func (w *StalePlanDetectorWorker) Run(ctx context.Context) {
	   for {
		  select {
		  case <-ctx.Done():
			  return
		  case ev := <-w.events:
			  w.processEvent(ctx, ev)
		   }
	   }
   }

// processEvent 处理单个漂移事件。
// 功能：查询 ev.SentAt 时刻该资产的活跃 CoolLease，判断 SentAt 是否在冷却窗内。
// 风险点：
// SentAt 若早于当前 CoolLease 的 MarkedAt，说明执行先于冷却标记，不视为漂移。
// 同一 ExecID 重复提交须幂等（通过 CaseID 散列去重）。

  func (w *StalePlanDetectorWorker) processEvent(ctx context.Context, ev StalePlanEvent) {
	   if ev.Asset == "" || ev.ExecID == "" {
		  return
	   }
	  asset := ev.Asset
	  // 读取当前世代信息。
	   gen, err := w.eng.gens.GetGen(ctx, asset)
	   if err != nil {
		  return
	  }
	  // 世代超前不处理。
	  if ev.CoolGen > gen.Generation {
		  return
	  }
	  // 查找执行时刻对应的 CoolLease。
	  var targetLease *CoolLease
	  if gen.ActiveLeaseID != "" {
		  targetLease, err = w.eng.leases.GetLease(ctx, gen.ActiveLeaseID)
		  if err != nil {
			  return
		  }
	   } else {
		  targetLease,
			  err = w.eng.leases.GetActiveLeaseByAsset(ctx, asset)
		   if err != nil {
			  return
		  }
	   }
	   if targetLease == nil {
		   return
	   }
	   // 判断 SentAt 是否落在冷却窗内。
	  inWindow := !ev.SentAt.Before(targetLease.MarkedAt) && ev.SentAt.Before(targetLease.CoolUntil)
	   if !inWindow {
		  return
	   }
	   bc := &CoolBreachCase{
		  CaseID: driftCaseID(asset, ev.ExecID),
		  Asset:  asset,
		   Generation: targetLease.Generation,
		  LeaseID:    targetLease.LeaseID,
		  ExecID:     ev.ExecID,
		   AttemptedAt: ev.SentAt,
		   Decision: BreachDrifted,
		   Reason: "MarkSent occurred within cool window [" +
			   targetLease.MarkedAt.UTC().Format(time.RFC3339) + ", " +
			   targetLease.CoolUntil.UTC().Format(time.RFC3339) + "]",
		   Resolved: false,
	  }
	  _ = w.eng.cases.SaveCase(ctx, bc)
	  recordBreachMetric(asset, BreachDrifted)
	   recordStalePlanDetectedMetric(asset)
  }

// driftCaseID 基于 asset+execID 生成幂等漂移案例 ID，防止重复插入。

   func driftCaseID(asset, execID string) string {
	  raw := "drift:" + asset + ":" + execID
	   sum := sha256.Sum256([]byte(raw))
	   return "drift-" + hex.EncodeToString(sum[:16])
  }
