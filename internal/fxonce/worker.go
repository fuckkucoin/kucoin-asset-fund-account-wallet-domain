package fxonce

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ────────────────────────────────────────────────────────────────────────────
// WorkerConfig — Worker 配置
// ────────────────────────────────────────────────────────────────────────────
// WorkerConfig Worker 运行参数。

type WorkerConfig struct {
	// InspectInterval Effect 巡检间隔（默认 30s）。
	InspectInterval time.Duration
	// ProposedTimeout proposed 状态超时阈值：超过此时长视为滞留，升级为 poisoned（默认 5min）。
	ProposedTimeout time.Duration
	// MaxRetryBeforePoison 在 Worker 发现 effect 滞留前允许的最大重试（默认 5）。
	MaxRetryBeforePoison int
	// GenLagThreshold gen lag 告警阈值（expected_gen - applied_gen，默认 10）。
	GenLagThreshold int64
	// ReaperBatchSize GenReaper 每次扫描上限（默认 200）。
	ReaperBatchSize int
	// AlertSink 告警回调（nil 则不告警）。
	AlertSink func(alert WorkerAlert)
	// DupProofReviewLimit ListUnreviewed 每次上限（默认 50）。
	DupProofReviewLimit int
}

// defaultWorkerConfig 返回默认配置。

   func defaultWorkerConfig() WorkerConfig {
	  return WorkerConfig{
		  InspectInterval: 30 * time.Second,
		  ProposedTimeout:      5 * time.Minute,
		  MaxRetryBeforePoison: 5,
		   GenLagThreshold:      10,
		  ReaperBatchSize:      200,
		   DupProofReviewLimit: 50,
	   }
  }

// ────────────────────────────────────────────────────────────────────────────
// WorkerAlert — 告警载体
// ────────────────────────────────────────────────────────────────────────────
// AlertLevel 告警级别。
type AlertLevel string

const (
	AlertWarn  AlertLevel = "warn"
	AlertError AlertLevel = "error"
	AlertCritical AlertLevel = "critical"
)

// WorkerAlert Worker 触发的告警。

type WorkerAlert struct {
	// Level 告警级别。
	Level AlertLevel `json:"level"`
	// Kind 告警类型（如 "effect_stuck"、"poison_isolated"、"gen_lag"）。
	Kind string `json:"kind"`
	// BusinessKey 关联的业务主键（可为空）。
	BusinessKey string `json:"business_key,omitempty"`
	// EffectID 关联的 FundEffect.ID（可为空）。
	EffectID string `json:"effect_id,omitempty"`
	// Message 告警摘要（面向 SRE）。
	Message string `json:"message"`
	// At 告警产生时刻。
	At time.Time `json:"at"`
}

// ────────────────────────────────────────────────────────────────────────────
// WorkerReport — 单次 Run 统计报告
// ────────────────────────────────────────────────────────────────────────────
// WorkerReport 单次 Worker.Run 的统计。

type WorkerReport struct {
	// Inspected 本次巡检扫描的 proposed effect 数量。
	Inspected int `json:"inspected"`
	// Poisoned 本次新升级为 poisoned 的 effect 数量。
	Poisoned int `json:"poisoned"`
	// Alerted 本次发出告警数量。
	Alerted int `json:"alerted"`
	// DupProofsUnreviewed 本次检测到的未审阅 DupProof 数量。
	DupProofsUnreviewed int `json:"dup_proofs_unreviewed"`
	// Errs 本次遇到的错误列表（非致命）。
	Errs []string `json:"errs,omitempty"`
	// RunAt 本次 Run 的时刻。
	RunAt time.Time `json:"run_at"`
}

// ────────────────────────────────────────────────────────────────────────────
// Worker
// ────────────────────────────────────────────────────────────────────────────
// Worker 后台巡检与隔离 Worker。
type Worker struct {
	store Store
	engine *CASEngine
	cfg    WorkerConfig
	metrics *Metrics
	clock func() time.Time
	mu      sync.Mutex
	running bool
	stopCh  chan struct{}
}

// NewWorker 构造 Worker。
  func NewWorker(store Store, engine *CASEngine, cfg WorkerConfig, m *Metrics) *Worker {
	  d := defaultWorkerConfig()
	   if cfg.InspectInterval == 0 {
		   cfg.InspectInterval = d.InspectInterval
	  }
	  if cfg.ProposedTimeout == 0 {
		  cfg.ProposedTimeout = d.ProposedTimeout
	  }
	  if cfg.MaxRetryBeforePoison == 0 {
		   cfg.MaxRetryBeforePoison = d.MaxRetryBeforePoison
	   }
	   if cfg.GenLagThreshold == 0 {
		  cfg.GenLagThreshold = d.GenLagThreshold
	  }
	  if cfg.ReaperBatchSize == 0 {
		   cfg.ReaperBatchSize = d.ReaperBatchSize
	   }
	  if cfg.DupProofReviewLimit == 0 {
		   cfg.DupProofReviewLimit = d.DupProofReviewLimit
	   }
	   if m == nil {
		   m = NewMetrics("fxonce")
	  }
	  return &Worker{
		   store:  store,
		  engine: engine,
		  cfg:     cfg,
		  metrics: m,
		  clock:   func() time.Time { return time.Now().UTC() },
		  stopCh:  make(chan struct{}),
	   }
   }

// WithClock 替换时钟（用）。

   func (w *Worker) WithClock(c func() time.Time) *Worker {
	  if w != nil && c != nil {
		   w.clock = c
	   }
	   return w
  }

  // Start 在 goroutine 中周期执行 Run；直到 Stop 被调用或 ctx 取消。

   func (w *Worker) Start(ctx context.Context) {
	  w.mu.Lock()
	   if w.running {
		   w.mu.Unlock()
		  return
	  }
	  w.running = true
	  w.mu.Unlock()
	  go func() {
		  ticker := time.NewTicker(w.cfg.InspectInterval)
		  defer ticker.Stop()
		  for {
			   select {
			   case <-ctx.Done():
				   return
			  case <-w.stopCh:
				  return
			   case <-ticker.C:
				   _, _ = w.Run(ctx)
			  }
		  }
	  }()
   }

// Stop 停止后台循环。
  func (w *Worker) Stop() {
	   w.mu.Lock()
	  defer w.mu.Unlock()
	  if !w.running {
		  return
	   }
	  close(w.stopCh)
	   w.running = false
	   w.stopCh = make(chan struct{})
  }

// Run 单次执行所有巡检任务（同步）。
// 外部可直接调用 Run 做或强制触发。
   func (w *Worker) Run(ctx context.Context) (WorkerReport, error) {
	   now := w.clock()
	  report := WorkerReport{RunAt: now}
	   // 任务 1：效果巡检（proposed → poisoned）
	  n, errs := w.inspectProposed(ctx, now)
	  report.Inspected = n
	  report.Errs = append(report.Errs,
		  errs...)
	  // 任务 2：毒丸隔离告警
	   na, errs2 := w.alertPoisoned(ctx, now)
	  report.Poisoned = na
	  report.Errs = append(report.Errs, errs2...)
	   // 任务 3：未审阅 DupProof 告警
	  nd,
		   errs3 := w.alertUnreviewedDups(ctx, now)
	  report.DupProofsUnreviewed = nd
	   report.Errs = append(report.Errs, errs3...)
	   // 更新指标
	  if w.metrics != nil {
		   w.metrics.WorkerRuns.Add(1)
		   w.metrics.PoisonedEffects.Add(float64(na))
		   w.metrics.UnreviewedDups.Set(float64(nd))
	  }
	  report.Alerted = na + nd
	  return report, nil
   }

// ─── 巡检任务 ─────────────────────────────────────────────────────────────
// inspectProposed 扫描 proposed 状态超时的 effect，升级为 poisoned。
// 返回：扫描数量，错误列表。
   func (w *Worker) inspectProposed(ctx context.Context, now time.Time) (int, []string) {
	   deadline := now.Add(-w.cfg.ProposedTimeout)
	   effects, err := w.store.ListEffects(ctx, "", w.cfg.ReaperBatchSize, 0)
	  if err != nil {
		  return 0, []string{fmt.Sprintf("inspectProposed list: %v", err)}
	  }
	   var errs []string
	   count := 0
	   for _, e := range effects {
		   if e.State != EffectProposed {
			  continue
		   }
		   count++
		  if e.CreatedAt.Before(deadline) {
			  // 超时未推进，升级毒丸
			   retries := e.PoisonRetries
			  if err2 := w.store.MarkPoisoned(ctx, e.ID, retries+1, now); err2 != nil {
				   errs = append(errs, fmt.Sprintf("markPoisoned %s: %v", e.ID, err2))
				  continue
			   }
			   w.sendAlert(WorkerAlert{
				   Level: AlertError,
				   Kind: "effect_stuck",
				  BusinessKey: e.BusinessKey,
				   EffectID: e.ID,
				  Message:  fmt.Sprintf("proposed effect stuck for %v, poisoned (retries=%d)", now.Sub(e.CreatedAt).Truncate(time.Second), retries+1),
				   At:       now,
			  })
		  }
	  }
	  return count, errs
   }

// alertPoisoned 对 poisoned effect 发出告警。

  func (w *Worker) alertPoisoned(ctx context.Context, now time.Time) (int, []string) {
	   effects, err := w.store.ListEffects(ctx, "", w.cfg.ReaperBatchSize, 0)
	   if err != nil {
		  return 0, []string{fmt.Sprintf("alertPoisoned list: %v", err)}
	  }
	   count := 0
	  for _, e := range effects {
		   if e.State != EffectPoisoned {
			   continue
		  }
		  count++
		  w.sendAlert(WorkerAlert{
			   Level: AlertCritical,
			  Kind:  "poison_isolated",
			   BusinessKey: e.BusinessKey,
			  EffectID:    e.ID,
			  Message: fmt.Sprintf("effect poisoned (retries=%d); manual dual-approval required for force-replay", e.PoisonRetries),
			  At: now,
		  })
	   }
	   return count, nil
  }

// alertUnreviewedDups 对未审阅的 DupProof 发出告警。

   func (w *Worker) alertUnreviewedDups(ctx context.Context, now time.Time) (int, []string) {
	   dups, err := w.store.ListUnreviewed(ctx, w.cfg.DupProofReviewLimit)
	  if err != nil {
		  return 0, []string{fmt.Sprintf("alertUnreviewedDups list: %v", err)}
	   }
	  for _, d := range dups {
		  w.sendAlert(WorkerAlert{
			  Level:       AlertWarn,
			  Kind:        "dup_proof_unreviewed",
			   BusinessKey: d.BusinessKey,
			  Message: fmt.Sprintf("dup proof %s (winner_gen=%d, loser_gen=%d, window=%dms) not reviewed", d.ID, d.WinnerGen, d.LoserGen, d.WindowMs),
			   At:      now,
		   })
	  }
	  return len(dups), nil
  }

   func (w *Worker) sendAlert(a WorkerAlert) {
	  if w.cfg.AlertSink != nil {
		   w.cfg.AlertSink(a)
	  }
  }
