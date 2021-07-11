package haltclear

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ---- Worker 接口 ---------------------------------------------------
// Worker 单个后台任务；Run 阻塞直到 ctx 取消。
type Worker interface {
	Name() string
	Run(ctx context.Context) error
}

// ---- InspectorWorker -----------------------------------------------
// InspectorWorker 定期调用 FalseClearDetector.Detect，将发现的误清案例送入 ReEngageRollback。
// 功能：以 interval 为周期扫描各作用域；发现 open 案例后按 autoReengage 配置触发再熔断。
// 不写战争室计划；只操作 custodyhalt Flag 和 haltclear 案例。
// 不变量：ctx 取消时退出；panic 由 recover 捕获并记录，不中断 Worker。
type InspectorWorker struct {
	detector *FalseClearDetector
	rollback *ReEngageRollback
	interval     time.Duration
	autoReengage bool                 // 发现误清后是否自动触发再熔断
	onDetect     func(r DetectResult) // 可注入观测钩子（如 metrics）
	onError func(err error)
}

// NewInspectorWorker 构造巡检 Worker。
// interval：扫描间隔；autoReengage：是否自动触发再熔断；
// onDetect/onError：观测钩子，可为 nil。
   func NewInspectorWorker(
	  detector *FalseClearDetector,
	   rollback *ReEngageRollback,
	  interval time.Duration,
	  autoReengage bool,
	  onDetect func(DetectResult),
	onError func(error),
) *InspectorWorker {
	if interval < time.Second {
		interval = 30 * time.Second
	}
	return &InspectorWorker{
		detector: detector,
		rollback: rollback,
		interval: interval,
		autoReengage: autoReengage,
		onDetect:     onDetect,
		onError: onError,
	}
}

  func (w *InspectorWorker) Name() string { return "haltclear.inspector" }

   func (w *InspectorWorker) Run(ctx context.Context) error {
	  ticker := time.NewTicker(w.interval)
	  defer ticker.Stop()
	  for {
		  select {
		  case <-ctx.Done():
			   return ctx.Err()
		   case <-ticker.C:
			   w.runOnce(ctx)
		  }
	  }
  }

   func (w *InspectorWorker) runOnce(ctx context.Context) {
	  defer func() {
		  if r := recover(); r != nil {
			  if w.onError != nil {
				   w.onError(fmt.Errorf("inspector panic: %v", r))
			  }
		  }
	   }()
	   results := w.detector.Detect(ctx)
	  for _, r := range results {
		   if w.onDetect != nil {
			  w.onDetect(r)
		  }
		   if r.Error != nil {
			   if w.onError != nil {
				   w.onError(r.Error)
			   }
			  continue
		  }
		   if r.MisclearID == "" || r.Existing {
			  continue
		   }
		  if !w.autoReengage {
			   continue
		   }
		  // 自动再熔断。
		  if _, err := w.rollback.AutoReEngage(ctx, r.MisclearID); err != nil {
			   if w.onError != nil {
				  w.onError(fmt.Errorf("auto reengage %s: %w", r.MisclearID, err))
			   }
		  }
	   }
  }

// ---- ReEngageExecutorWorker ----------------------------------------
// ReEngageExecutorWorker 轮询 pending 状态的 ReEngageLeg，执行半自动再熔断。
// 功能：人工触发调度后，由 Worker 异步执行 ExecuteLeg，供半自动流程使用。
// 只执行 is_auto=false 的 pending 腿（auto 腿由 InspectorWorker 直接调用）。
// 不变量：同一腿不重复执行；失败后腿变为 failed，不重试（重试需人工再调度）。

type ReEngageExecutorWorker struct {
	rollback *ReEngageRollback
	caseRepo CaseRepo
	legRepo  LegRepo
	interval  time.Duration
	onExecute func(legID string, proof *RollbackProof, err error)
	onError   func(error)
}

   func NewReEngageExecutorWorker(
	   rollback *ReEngageRollback,
	  caseRepo CaseRepo,
	  legRepo LegRepo,
	  interval time.Duration,
	   onExecute func(string, *RollbackProof, error),
	onError func(error),
) *ReEngageExecutorWorker {
	if interval < time.Second {
		interval = 15 * time.Second
	}
	return &ReEngageExecutorWorker{
		rollback: rollback,
		caseRepo: caseRepo,
		legRepo:   legRepo,
		interval:  interval,
		onExecute: onExecute,
		onError:   onError,
	}
}

   func (w *ReEngageExecutorWorker) Name() string { return "haltclear.executor" }

   func (w *ReEngageExecutorWorker) Run(ctx context.Context) error {
	   ticker := time.NewTicker(w.interval)
	   defer ticker.Stop()
	  for {
		   select {
		   case <-ctx.Done():
			  return ctx.Err()
		  case <-ticker.C:
			  w.runOnce(ctx)
		  }
	  }
  }

   func (w *ReEngageExecutorWorker) runOnce(ctx context.Context) {
	  defer func() {
		   if r := recover(); r != nil {
			  if w.onError != nil {
				  w.onError(fmt.Errorf("executor panic: %v", r))
			   }
		  }
	   }()
	   // 列出所有 open 案例，找到活跃的 pending 腿（is_auto=false）并执行。
	  cases, err := w.caseRepo.List(ctx, CaseFilter{State: MisClearStateOpen, Limit: 100})
	   if err != nil {
		  if w.onError != nil {
			   w.onError(fmt.Errorf("list open cases: %w", err))
		  }
		   return
	   }
	  for _, c := range cases {
		   leg, err := w.legRepo.ActiveLeg(ctx, c.MisclearID)
		   if err != nil || leg == nil {
			   continue
		   }
		  if leg.IsAuto || leg.State != ReEngageStatePending {
			   continue // auto 腿由 InspectorWorker 处理
		  }
		  proof,
			  err := w.rollback.ExecuteLeg(ctx, leg.LegID)
		   if w.onExecute != nil {
			  w.onExecute(leg.LegID, proof, err)
		   }
		   if err != nil && w.onError != nil {
			  w.onError(fmt.Errorf("execute leg %s: %w", leg.LegID, err))
		  }
	   }
   }

// ---- AuditSealWorker -----------------------------------------------
// AuditSealWorker 将 reengaged 状态的 MisClearCase 的 RollbackProof 固化进审计日志。
// 功能：定期扫描 reengaged 案例，验证证明有效后标记已固化（通过 close_note 记录）。
// 不关闭案例（关闭需运营人工操作）；只验证证明完整性。
type AuditSealWorker struct {
	caseRepo CaseRepo
	legRepo LegRepo
	interval time.Duration
	onSeal func(misclearID string,
		  valid bool)
	onError func(error)
}

   func NewAuditSealWorker(
	  caseRepo CaseRepo,
	   legRepo LegRepo,
	  interval time.Duration,
	   onSeal func(string, bool),
	onError func(error),
) *AuditSealWorker {
	if interval < time.Second {
		interval = 60 * time.Second
	}
	return &AuditSealWorker{
		caseRepo: caseRepo,
		legRepo: legRepo,
		interval: interval,
		onSeal:  onSeal,
		onError: onError,
	}
}

  func (w *AuditSealWorker) Name() string { return "haltclear.audit_seal" }

   func (w *AuditSealWorker) Run(ctx context.Context) error {
	  ticker := time.NewTicker(w.interval)
	   defer ticker.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			   return ctx.Err()
		  case <-ticker.C:
			   w.runOnce(ctx)
		  }
	  }
   }

  func (w *AuditSealWorker) runOnce(ctx context.Context) {
	   defer func() {
		  if r := recover(); r != nil {
			   if w.onError != nil {
				   w.onError(fmt.Errorf("audit seal panic: %v", r))
			  }
		  }
	  }()
	   cases, err := w.caseRepo.List(ctx, CaseFilter{State: MisClearStateReengaged, Limit: 100})
	  if err != nil {
		  if w.onError != nil {
			   w.onError(fmt.Errorf("list reengaged cases: %w", err))
		   }
		  return
	  }
	   for _, c := range cases {
		  if c.ReengageLegID == "" {
			   continue
		   }
		  leg, err := w.legRepo.Get(ctx, c.ReengageLegID)
		   if err != nil {
			   if w.onError != nil {
				   w.onError(fmt.Errorf("get leg %s: %w", c.ReengageLegID, err))
			   }
			   continue
		  }
		  valid := leg.RollbackProof != nil && VerifyRollbackProof(leg.RollbackProof)
		  if w.onSeal != nil {
			   w.onSeal(c.MisclearID, valid)
		  }
		   // 将验证状态写入 close_note（供运营审阅）。
		  if valid && !containsAuditSeal(c.CloseNote) {
			  c.CloseNote = fmt.Sprintf("[audit_seal:ok proof=%s]", leg.RollbackProof.ProofID)
			  c.UpdatedAt = time.Now().UTC()
			   if err := w.caseRepo.Update(ctx, c); err != nil && w.onError != nil {
				   w.onError(fmt.Errorf("update audit seal note %s: %w", c.MisclearID, err))
			   }
		   }
	  }
   }

   func containsAuditSeal(note string) bool {
	  return len(note) > 0 && note[:1] == "[" &&
		   len(note) >= 13 && note[:13] == "[audit_seal:ok"
  }

// ---- WorkerGroup -----------------------------------------------
// WorkerGroup 并发运行多个 Worker；任一出错即取消其余。
type WorkerGroup struct {
	workers []Worker
}

  func NewWorkerGroup(workers ...Worker) *WorkerGroup {
	  return &WorkerGroup{workers: workers}
  }

// Run 启动全部 Worker；ctx 取消或任一 Worker 返回错误时停止全部。
  func (g *WorkerGroup) Run(ctx context.Context) error {
	  ctx, cancel := context.WithCancel(ctx)
	  defer cancel()
	  errs := make(chan error, len(g.workers))
	   var wg sync.WaitGroup
	   for _, w := range g.workers {
		   w := w
		   wg.Add(1)
		   go func() {
			  defer wg.Done()
			   if err := w.Run(ctx); err != nil {
				   errs <- fmt.Errorf("%s: %w", w.Name(), err)
				  cancel()
			  }
		   }()
	   }
	   wg.Wait()
	  close(errs)
	   for e := range errs {
		   if e != nil {
			  return e
		  }
	  }
	   return nil
   }
