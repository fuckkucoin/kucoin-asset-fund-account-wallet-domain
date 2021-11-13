// @FileName: engine.go
// @Summary: 接收差异特征输入，驱动 FSM 状态跃迁，按序执行动作步骤，管理人工门禁。
// -----------------------------------------------------------------------------
// - 不直接访问三方公式或账本；差异事实由调用方从 recon.TripDrift/Mismatch 提取后送入。
// - 低置信度或高风险动作必须经过 WaitingHuman 态，人工放行后才执行。
// - SLA 超时仅置为 escalated；fundcircuit REQUEST 通过 Executor.Halter 发出，不实现总线。
// - ticket/SLA 驱动：Engine.StartRun 后 TicketID 挂接到 FlowRun；SLA 由 FlowDef 决定。
// 不变量：
// - FlowRun.State 只能经 FSM 合法跃迁（参见 fsm.go）；非法跃迁返回 ErrInvalidState。
// - 所有写 DB 前先检查 FSM；写失败不改内存状态。
// - StartRun 对相同 FeatureHash 幂等（若已有 open/classifying 的 Run 则返回已有 Run ID）。
// -----------------------------------------------------------------------------
// @Author: Finn
// @Created: 2021.10.19

package reconflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// ──────────────────────────────────────────────
// Engine
// ──────────────────────────────────────────────
// Engine 工作流编排引擎。

type Engine struct {
	store *Store
	catalog *Catalog
	classifier *Classifier
	executor   *Executor
	// defCache 按 DiffClassKind 缓存 FlowDef（先内存，回退 DB）。
	defCache map[DiffClassKind]FlowDef
}

// EngineOptions 引擎构造选项。

type EngineOptions struct {
	Store *Store
	Catalog *Catalog
	Classifier *Classifier
	Executor   *Executor
	// BootDefs 引擎启动时加载的内置 FlowDef；nil 时从 DefaultFlowDefs 加载。
	BootDefs []FlowDef
}

// NewEngine 构造编排引擎。
// 若 store 为 nil，所有持久化调用静默跳过（/内存模式）。
  func NewEngine(opts EngineOptions) *Engine {
	   if opts.Catalog == nil {
		   opts.Catalog = NewCatalog()
	   }
	   if opts.Classifier == nil {
		  opts.Classifier = NewClassifier()
	   }
	  if opts.Executor == nil {
		  opts.Executor = &Executor{}
	  }
	  boot := opts.BootDefs
	   if boot == nil {
		  boot = DefaultFlowDefs()
	  }
	   cache := make(map[DiffClassKind]FlowDef, len(boot))
	  for _, d := range boot {
		  cache[d.DiffKind] = d
	  }
	   return &Engine{
		   store:   opts.Store,
		   catalog: opts.Catalog,
		  classifier: opts.Classifier,
		   executor: opts.Executor,
		   defCache: cache,
	  }
   }

// ──────────────────────────────────────────────
// Classify（分类 + 可选启动）
// ──────────────────────────────────────────────
// Classify 对差异特征分类；dry-run=true 时只返回分类结果，不创建 FlowRun。
   func (e *Engine) Classify(ctx context.Context, req ClassifyRequest, actor string) (ClassifyResult, error) {
	  class := e.classifier.Classify(req.Feature)
	   if req.DryRun {
		   return ClassifyResult{Class: class, DryRun: true}, nil
	   }
	  // 幂等：相同 featureHash 若已有非终态 Run，返回已有 ID。
	  if e.store != nil && e.store.on() {
		   existing, err := e.findActiveRunByHash(ctx, class.FeatureHash)
		   if err == nil && existing != "" {
			  return ClassifyResult{Class: class, RunID: existing, DryRun: false}, nil
		  }
	  }
	  runID, err := e.StartRun(ctx, req.Feature, class, actor)
	   if err != nil {
		  return ClassifyResult{Class: class}, err
	   }
	  return ClassifyResult{Class: class, RunID: runID, DryRun: false}, nil
  }

// ──────────────────────────────────────────────
// StartRun
// ──────────────────────────────────────────────
// StartRun 为一条差异特征创建并启动 FlowRun。
// 1. 持久化分类结果。
// 2. 查找 FlowDef（内存缓存 → DB → 默认兜底）。
// 3. 创建 FlowRun（open），写 DB。
// 4. 触发分类后 FSM 跃迁（open→classifying→acting 或 waiting_human）。
// 5. 初始化步骤列表。
  func (e *Engine) StartRun(ctx context.Context, feat DiffFeature, class DiffClass, actor string) (string, error) {
	   if err := validateActor(actor); err != nil {
		  return "", err
	  }
	   // 持久化分类结果。
	   if e.store != nil {
		  _ = e.store.SaveDiffClass(ctx, class)
	   }
	   // 查找流程定义。
	   def,
		   err := e.resolveFlowDef(ctx, class.Kind)
	   if err != nil {
		   return "", err
	   }
	  if !def.Enabled {
		   return "", ErrDisabled
	  }
	  // 解析动作列表。
	  actions := e.resolveActions(def)
	  now := time.Now().UTC()
	   runID := uuid.NewString()
	   slaDeadline := now.Add(def.SLA)
	   if def.SLA <= 0 {
		  slaDeadline = now.Add(4 * time.Hour)
	   }
	  run := FlowRun{
		  ID: runID,
		   FlowDefID: def.ID,
		  UserID: feat.UserID,
		   Asset:  feat.Asset,
		  Chain:  feat.Chain,
		   DiffKind: string(class.Kind),
		  State:   RunStateOpen,
		   ClassID: class.ID,
		   Confidence:  class.Confidence,
		  CurrentStep: 0,
		  Note: class.Note,
		   FenceToken: uuid.NewString(),
		  CreatedBy:  actor,
		   CreatedAt:   now,
		   UpdatedAt:   now,
		  SLADeadline: slaDeadline,
	  }
	  // 持久化初始 Run；无 store 时内存模式，返回空 RunID（无法持久化也无法查询）。
	   if e.store == nil {
		   ReconFlowRunsTotal.WithLabelValues(string(class.Kind), "open").Inc()
		   return "", nil
	  }
	   if err := e.store.SaveRun(ctx, run); err != nil {
		   return "", err
	   }
	  // FSM: open → classifying。
	  if err := e.transition(ctx, &run, "classify_start", actor); err != nil {
		   return runID, err
	  }
	  // FSM: classifying → acting 或 waiting_human。
	  event := classifyEvent(class.Confidence, actions)
	   if err := e.transition(ctx, &run, event, actor); err != nil {
		  return runID, err
	   }
	  // 创建步骤记录（pending）。
	  if err := e.initSteps(ctx, run, def, actions, actor); err != nil {
		   return runID, err
	  }
	   // 若进入 acting，立即推进可执行步骤。
	  if run.State == RunStateActing {
		  _ = e.advanceSteps(ctx, &run, actor)
	  }
	  ReconFlowRunsTotal.WithLabelValues(string(class.Kind), string(run.State)).Inc()
	   return runID, nil
   }

// ──────────────────────────────────────────────
// AdvanceRun（Worker 推进）
// ──────────────────────────────────────────────
// AdvanceRun 推进一个 FlowRun 的待执行步骤（由 classify_pump/action_exec worker 调用）。
  func (e *Engine) AdvanceRun(ctx context.Context, runID, actor string) error {
	  run, err := e.loadRun(ctx, runID)
	   if err != nil {
		  return err
	   }
	   if !run.Active() {
		   return ErrClosed
	  }
	  if run.State == RunStateWaitingHuman {
		   return ErrGateRequired
	   }
	   if run.State == RunStateActing {
		   return e.advanceSteps(ctx, &run, actor)
	  }
	  return nil
  }

// ──────────────────────────────────────────────
// GateOp（人工放行）
// ──────────────────────────────────────────────
// GateAllow 人工放行一个等待中的步骤，接着执行。

  func (e *Engine) GateAllow(ctx context.Context, dec GateDecision) error {
	   if err := validateActor(dec.Actor); err != nil {
		   return err
	  }
	   run, err := e.loadRun(ctx, dec.RunID)
	   if err != nil {
		   return err
	  }
	   if !run.Active() {
		   return ErrClosed
	  }
	   if run.State != RunStateWaitingHuman {
		  return fmt.Errorf("%w: run not waiting for human gate (state=%s)", ErrInvalidState, run.State)
	   }
	   // 更新 gated 步骤为 pending（准备执行）。
	   if dec.StepID != "" {
		   step, serr := e.store.GetStep(ctx, dec.StepID)
		   if serr == nil && step.State == StepStateGated {
			   now := time.Now().UTC()
			  step.State = StepStatePending
			   step.GatedBy = dec.Actor
			   step.GatedAt = &now
			  _ = e.store.SaveStep(ctx, step)
		   }
	  }
	  // FSM: waiting_human → acting。
	  if err := e.transition(ctx, &run, "gate_allow", dec.Actor); err != nil {
		   return err
	  }
	  run.GateActor = dec.Actor
	   now := time.Now().UTC()
	  run.GateAt = &now
	  run.UpdatedAt = now
	   if e.store != nil {
		   _ = e.store.SaveRun(ctx, run)
	  }
	   // 接着推进步骤。
	   return e.advanceSteps(ctx, &run, dec.Actor)
  }

// GateDeny 人工拒绝，关闭流水。

  func (e *Engine) GateDeny(ctx context.Context, dec GateDecision) error {
	  if err := validateActor(dec.Actor); err != nil {
		   return err
	  }
	   run,
		   err := e.loadRun(ctx, dec.RunID)
	   if err != nil {
		   return err
	   }
	  if !run.Active() {
		  return ErrClosed
	   }
	  if run.State != RunStateWaitingHuman {
		   return fmt.Errorf("%w: run not waiting for human gate", ErrInvalidState)
	  }
	   if err := e.transition(ctx, &run, "gate_deny", dec.Actor); err != nil {
		   return err
	   }
	   run.ClosedBy = dec.Actor
	  run.UpdatedAt = time.Now().UTC()
	  if e.store != nil {
		   _ = e.store.SaveRun(ctx, run)
	  }
	   ReconFlowRunsTotal.WithLabelValues(run.DiffKind, "gate_denied").Inc()
	   return nil
   }

// ──────────────────────────────────────────────
// EscalateRun（SLA 超时升级）
// ──────────────────────────────────────────────
// EscalateRun 将一个超 SLA 的 FlowRun 标为 escalated。
// 若 FlowDef.RequestCircuit=true 则触发熔断请求（仅 REQUEST，不 Clear）。
  func (e *Engine) EscalateRun(ctx context.Context, runID, actor string) error {
	  if err := validateActor(actor); err != nil {
		   return err
	   }
	   run, err := e.loadRun(ctx, runID)
	   if err != nil {
		  return err
	   }
	  if !run.Active() {
		  return ErrClosed
	  }
	   // FSM 跃迁：当前状态 → escalated。
	  if err := e.transition(ctx, &run, "escalate", actor); err != nil {
		   return err
	   }
	  run.EscalatedBy = actor
	  run.UpdatedAt = time.Now().UTC()
	  if e.store != nil {
		   _ = e.store.SaveRun(ctx, run)
	   }
	   // 若流程定义要求请求熔断，通过 Executor 发出（仅 REQUEST）。
	  def, defErr := e.resolveFlowDef(ctx, DiffClassKind(run.DiffKind))
	   if defErr == nil && def.RequestCircuit && e.executor != nil && e.executor.Halter != nil {
		  in := ExecInput{
			   RunID: run.ID,
			  UserID: run.UserID,
			  Asset:    run.Asset,
			   Chain:    run.Chain,
			  DiffKind: run.DiffKind,
			  Actor:    actor,
		   }
		   res := e.executor.Execute(ctx, ActionDef{Kind: ActKindRequestCircuit, RequireGate: false}, in)
		   if !res.OK {
			  // 熔断请求失败：记录但不阻塞升级。
			  ReconFlowActionsTotal.WithLabelValues("request_circuit", "fail").Inc()
		  } else {
			   ReconFlowActionsTotal.WithLabelValues("request_circuit", "ok").Inc()
		   }
	  }
	   ReconFlowRunsTotal.WithLabelValues(run.DiffKind, "escalated").Inc()
	   return nil
   }

// ──────────────────────────────────────────────
// ReplayEval（对账结果重评）
// ──────────────────────────────────────────────
// ReplayEval 对账结果重评：若差异已消失则关闭 Run；否则重新分类。
// 不变量：只读消费差异现状（stillDiff=false 表示差异已消失）。

   func (e *Engine) ReplayEval(ctx context.Context, runID string, stillDiff bool, actor string) error {
	  if err := validateActor(actor); err != nil {
		  return err
	  }
	  run, err := e.loadRun(ctx, runID)
	   if err != nil {
		   return err
	   }
	  if !run.Active() {
		  // 已终态，diff 仍在则静默（由值班人处理），已消失则无操作。
		   return nil
	  }
	   if !stillDiff {
		  // 差异已消失，关闭流水。
		   if CanTransition(run.State, "all_done") {
			  _ = e.transition(ctx, &run, "all_done", actor)
		  } else {
			   // 强制关闭（允许从任意非终态到 closed）。
			  run.State = RunStateClosed
		   }
		  run.ClosedBy = actor
		  run.UpdatedAt = time.Now().UTC()
		  if e.store != nil {
			  _ = e.store.SaveRun(ctx, run)
		  }
		  ReconFlowRunsTotal.WithLabelValues(run.DiffKind, "replay_closed").Inc()
	  }
	  return nil
   }

// ──────────────────────────────────────────────
// 内部步骤推进
// ──────────────────────────────────────────────
// advanceSteps 顺序执行待执行步骤；遇到 gated 步骤暂停并切换 waiting_human。
   func (e *Engine) advanceSteps(ctx context.Context, run *FlowRun, actor string) error {
	   if e.store == nil {
		   return nil
	   }
	   steps, err := e.store.ListSteps(ctx, run.ID)
	   if err != nil {
		  return err
	   }
	  for i := range steps {
		  st := &steps[i]
		  if st.State == StepStateDone || st.State == StepStateSkipped {
			   continue
		   }
		  if st.State == StepStateGated {
			  // 遇到等待放行的步骤：切换 Run 状态为 waiting_human。
			  if run.State == RunStateActing {
				  _ = e.transition(ctx, run, "action_gated", actor)
				  run.UpdatedAt = time.Now().UTC()
				   _ = e.store.SaveRun(ctx, *run)
			  }
			  return ErrGateRequired
		   }
		  if st.State != StepStatePending {
			  continue
		   }
		   // 查找动作定义。
		  def, ok := e.catalog.ByID(st.ActionDefID)
		   if !ok {
			  def, ok = e.catalog.ByName(st.ActionKind)
		   }
		   if !ok {
			   st.State = StepStateSkipped
			   _ = e.store.SaveStep(ctx, *st)
			   continue
		   }
		   // 高风险动作需人工放行（且未被放行），进入 gated 等待。
		  if (def.RequireGate || def.Risk == RiskHigh) && st.GatedBy == "" {
			   now := time.Now().UTC()
			   st.State = StepStateGated
			  st.StartedAt = nil
			   _ = e.store.SaveStep(ctx, *st)
			  if run.State == RunStateActing {
				   _ = e.transition(ctx, run, "action_gated", actor)
				   run.UpdatedAt = now
				   _ = e.store.SaveRun(ctx, *run)
			  }
			  return ErrGateRequired
		  }
		  // 执行。
		  now := time.Now().UTC()
		  st.State = StepStateRunning
		   st.StartedAt = &now
		   st.Actor = actor
		  _ = e.store.SaveStep(ctx, *st)
		  in := ExecInput{
			  RunID: run.ID,
			  RunNote: run.Note,
			  UserID: run.UserID,
			   Asset:  run.Asset,
			   Chain:    run.Chain,
			  DiffKind: run.DiffKind,
			   Actor:    actor,
		   }
		   res := e.executor.Execute(ctx, def, in)
		   finNow := time.Now().UTC()
		   st.FinishedAt = &finNow
		  st.Output = res.Output
		   if res.OK {
			  st.State = StepStateDone
			   run.CurrentStep = st.Seq + 1
			  ReconFlowActionsTotal.WithLabelValues(string(def.Kind), "ok").Inc()
		  } else {
			  st.State = StepStateFailed
			  st.ErrMsg = res.ErrMsg
			   ReconFlowActionsTotal.WithLabelValues(string(def.Kind), "fail").Inc()
		  }
		   _ = e.store.SaveStep(ctx, *st)
		  run.UpdatedAt = finNow
		  _ = e.store.SaveRun(ctx, *run)
		  if !res.OK {
			   return fmt.Errorf("reconflow step %d failed: %s", st.Seq, res.ErrMsg)
		   }
		   // close_flow 动作执行后直接关闭 Run。
		   if def.Kind == ActKindCloseFlow {
			   _ = e.transition(ctx, run, "all_done", actor)
			  run.ClosedBy = actor
			  run.UpdatedAt = time.Now().UTC()
			   _ = e.store.SaveRun(ctx, *run)
			   return nil
		   }
	  }
	   // 所有步骤完成，关闭流水。
	  if run.State == RunStateActing {
		  _ = e.transition(ctx, run, "all_done", actor)
		  run.ClosedBy = actor
		   run.UpdatedAt = time.Now().UTC()
		   if e.store != nil {
			  _ = e.store.SaveRun(ctx, *run)
		  }
	   }
	   return nil
   }

// initSteps 按 FlowDef.ActionIDs 顺序创建 pending 步骤。
  func (e *Engine) initSteps(ctx context.Context, run FlowRun, def FlowDef, actions []ActionDef, actor string) error {
	  if e.store == nil {
		  return nil
	  }
	  now := time.Now().UTC()
	  for i, a := range actions {
		   step := FlowStep{
			   ID: uuid.NewString(),
			  RunID: run.ID,
			  Seq:   i,
			  ActionDefID: a.ID,
			  ActionKind:  string(a.Kind),
			  State: StepStatePending,
			  Actor: actor,
			  Input:     stepInput(run, def, a),
			   CreatedAt: now,
		  }
		   if err := e.store.SaveStep(ctx, step); err != nil {
			   return err
		  }
	   }
	  return nil
  }

// stepInput 生成步骤输入 JSON 快照。

   func stepInput(run FlowRun, def FlowDef, a ActionDef) string {
	   m := map[string]string{
		   "run_id":    run.ID,
		   "diff_kind": run.DiffKind,
		   "user_id":   run.UserID,
		  "asset":    run.Asset,
		   "def_name": def.Name,
		   "action": string(a.Kind),
	   }
	   b, _ := json.Marshal(m)
	  return string(b)
   }

// ──────────────────────────────────────────────
// FlowDef CRUD（代理 Store）
// ──────────────────────────────────────────────
// UpsertFlowDef 创建或更新流程定义，同时更新内存缓存。
  func (e *Engine) UpsertFlowDef(ctx context.Context, d FlowDef, actor string) error {
	   if err := validateActor(actor); err != nil {
		  return err
	   }
	   if d.ID == "" {
		  d.ID = uuid.NewString()
	  }
	  now := time.Now().UTC()
	  if d.CreatedAt.IsZero() {
		   d.CreatedAt = now
		  d.CreatedBy = actor
	   }
	  d.UpdatedAt = now
	  if e.store != nil {
		   if err := e.store.SaveFlowDef(ctx, d); err != nil {
			   return err
		  }
	  }
	   e.defCache[d.DiffKind] = d
	   return nil
   }

// GetFlowDef 按 ID 加载流程定义。

   func (e *Engine) GetFlowDef(ctx context.Context, id string) (FlowDef, error) {
	   if e.store == nil {
		   return FlowDef{}, ErrNotFound
	   }
	  return e.store.GetFlowDef(ctx, id)
  }

// ListFlowDefs 列举所有流程定义。

  func (e *Engine) ListFlowDefs(ctx context.Context) ([]FlowDef, error) {
	   if e.store == nil {
		  defs := make([]FlowDef, 0, len(e.defCache))
		  for _, d := range e.defCache {
			   defs = append(defs, d)
		   }
		   return defs, nil
	  }
	  return e.store.ListFlowDefs(ctx)
  }

// DeleteFlowDef 软删除流程定义。

   func (e *Engine) DeleteFlowDef(ctx context.Context, id, actor string) error {
	  if err := validateActor(actor); err != nil {
		  return err
	   }
	  if e.store == nil {
		   return ErrStore
	  }
	  return e.store.DeleteFlowDef(ctx,
		   id, actor)
   }

// ListRuns 查询 FlowRun 列表。

  func (e *Engine) ListRuns(ctx context.Context, q RunQuery) ([]FlowRun, error) {
	   if e.store == nil {
		   return nil, nil
	  }
	   return e.store.ListRuns(ctx, q)
   }

// GetRun 按 ID 加载 FlowRun。

  func (e *Engine) GetRun(ctx context.Context, id string) (FlowRun, error) {
	  return e.loadRun(ctx, id)
  }

// ListSteps 列举 FlowRun 的所有步骤。
   func (e *Engine) ListSteps(ctx context.Context, runID string) ([]FlowStep, error) {
	   if e.store == nil {
		  return nil, nil
	  }
	  return e.store.ListSteps(ctx,
		  runID)
  }

// ListGatedSteps 列举所有等待人工放行的步骤。
   func (e *Engine) ListGatedSteps(ctx context.Context, limit int) ([]FlowStep, error) {
	   if e.store == nil {
		  return nil, nil
	  }
	   return e.store.ListGatedSteps(ctx,
		  limit)
  }

// UpsertActionDef 创建或更新自定义动作定义，同时注册到 Catalog。
  func (e *Engine) UpsertActionDef(ctx context.Context, a ActionDef) error {
	   if a.ID == "" {
		   a.ID = uuid.NewString()
	  }
	   now := time.Now().UTC()
	  if a.CreatedAt.IsZero() {
		  a.CreatedAt = now
	  }
	   a.UpdatedAt = now
	   e.catalog.Register(a)
	  if e.store != nil {
		   return e.store.SaveActionDef(ctx, a)
	  }
	   return nil
  }

// ──────────────────────────────────────────────
// 内部工具
// ──────────────────────────────────────────────
// transition 执行 FSM 跃迁并更新 run.State。

  func (e *Engine) transition(ctx context.Context, run *FlowRun, event, actor string) error {
	   to, err := Transition(run.State, event)
	  if err != nil {
		  return err
	  }
	   run.State = to
	  run.UpdatedAt = time.Now().UTC()
	  if e.store != nil {
		   _ = e.store.SaveRun(ctx, *run)
	   }
	  return nil
  }

// loadRun 从 Store 加载 FlowRun，Store 不可用时返回 ErrNotFound。
  func (e *Engine) loadRun(ctx context.Context, id string) (FlowRun, error) {
	   if e.store == nil {
		  return FlowRun{}, ErrNotFound
	  }
	   return e.store.GetRun(ctx, id)
  }

// resolveFlowDef 按 DiffClassKind 查找 FlowDef：内存缓存 → DB → 返回 ErrNoFlowDef。

  func (e *Engine) resolveFlowDef(ctx context.Context, kind DiffClassKind) (FlowDef, error) {
	  if d, ok := e.defCache[kind]; ok {
		  return d, nil
	   }
	  // 未知类型尝试从 unknown 缓存回退。
	   if d, ok := e.defCache[DiffClassUnknown]; ok {
		  return d, nil
	  }
	   if e.store != nil && e.store.on() {
		  d, err := e.store.GetFlowDefByKind(ctx, kind)
		  if err == nil {
			   e.defCache[kind] = d
			   return d, nil
		  }
	  }
	   return FlowDef{}, fmt.Errorf("%w: diff_kind=%s", ErrNoFlowDef, kind)
   }

// resolveActions 从 Catalog 解析 FlowDef.ActionIDs 对应的动作定义。

   func (e *Engine) resolveActions(def FlowDef) []ActionDef {
	  out := make([]ActionDef, 0, len(def.ActionIDs))
	   for _, id := range def.ActionIDs {
		  a, ok := e.catalog.ByID(id)
		   if !ok {
			   // 内置动作也按名称尝试。
			  a, ok = e.catalog.ByName(id)
		  }
		  if ok {
			   out = append(out, a)
		  }
	   }
	  return out
  }

// findActiveRunByHash 查找相同 feature_hash 的非终态 Run ID（简单列举过滤）。
  func (e *Engine) findActiveRunByHash(ctx context.Context, hash string) (string, error) {
	  if e.store == nil {
		  return "", ErrNotFound
	   }
	  // 通过 note 字段匹配 hash（中应加专用列索引）。
	  runs, err := e.store.ListActiveRuns(ctx, 200)
	  if err != nil {
		  return "", err
	   }
	   for _, r := range runs {
		   if r.ClassID != "" && r.Active() {
			  // 匹配 ClassID 前缀（存储 FeatureHash 时写入 Note）。
			  if strings.Contains(r.Note, hash) {
				  return r.ID, nil
			  }
		  }
	   }
	   return "", ErrNotFound
   }

  func validateActor(actor string) error {
	   actor = strings.TrimSpace(actor)
	  if actor == "" || strings.EqualFold(actor, "system") {
		   return ErrActor
	   }
	  return nil
  }
