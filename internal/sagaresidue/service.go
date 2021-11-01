// 功能：service.go
// 资金编排补偿失败残留发现与强制收口引擎 — 服务主入口。
// Service 聚合 Detector、CloseoutOrchestrator、MutexManager、Prover 的调用，
// 对外暴露以下核心方法：
// DetectResidue — 扫描指定 saga Run 的残留科目，产出 ResidueReport；
// 若无残留返回 ErrNoResidues。
// RegisterRun — 将 partial_failed 的 Run 注册到 sr_residue_runs，
// 建立监控条目（幂等）。
// ConfirmItems — 双人授权确认残留科目为有效残留（IsConfirmed=true）。
// InitiateCloseout — 双人授权启动强制收口：BuildPlan + 持锁 + 异步触发 Execute。
// ExecuteCloseout — worker 调用：按计划顺序执行收口腿。
// ResumeCloseout — worker 调用：断点续跑。
// FinalizeRun — 收口腿全完成后生成 CloseProof 并标 Run 为 closed。
// CheckCompensate — safesaga 钩子：检查 Compensate 是否被互斥锁阻止。
// GetRun — 读取 ResidueRun。
// ListRuns — 列举 ResidueRun（带筛选）。
// GetProof — 读取 CloseProof。
// VerifyProof — 重校验 CloseProof。
// GetMutexStatus — 查询互斥锁状态。
// CreateDate: 2022.04.20

// 作者：Gunner
// 日期：2021.10.31

package sagaresidue

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// Service 资金残留收口引擎。

type Service struct {
	detector *Detector
	orch     *CloseoutOrchestrator
	mutexMgr *MutexManager
	prover *Prover
	store  Store
	clock  func() time.Time
}

// NewService 构造 Service，所有依赖须非空。
   func NewService(
	  detector *Detector,
	   orch *CloseoutOrchestrator,
	  mutexMgr *MutexManager,
	  prover *Prover,
	  store Store,
   ) *Service {
	  return &Service{
		  detector: detector,
		   orch:     orch,
		  mutexMgr: mutexMgr,
		  prover:   prover,
		  store: store,
		  clock: func() time.Time { return time.Now().UTC() },
	   }
  }

// ── 残留发现 ──────────────────────────────────────────────────────────────────
// DetectResidue 扫描 saga Run 的残留科目。
// 若 ResidueRun 已存在且不要求强制重扫（req.ForceRescan=false），
// 则返回最新一份 ResidueReport（缓存命中）。
// 若无残留（ErrNoResidues），不创建 ResidueRun。
  func (s *Service) DetectResidue(ctx context.Context, req DetectReq) (*ResidueReport, *ResidueRun, error) {
	  if s == nil || s.detector == nil || s.store == nil {
		   return nil, nil, ErrUnavailable
	  }
	  // ── 1. 检查是否已有 Run，且不要求重扫 ────────────────────────────────────
	  if !req.ForceRescan {
		   existing, err := s.store.GetResidueRunBySagaRunID(ctx, req.SagaRunID)
		  if err == nil && existing != nil {
			  report, _ := s.store.GetLatestReport(ctx, req.SagaRunID)
			  return report, existing, nil
		   }
	  }
	   // ── 2. 执行扫描 ───────────────────────────────────────────────────────────
	   report, err := s.detector.Detect(ctx, req)
	  if err != nil {
		  return nil, nil, err
	  }
	   // ── 3. 注册 ResidueRun（或更新已有）───────────────────────────────────────
	   rrun,
		  err := s.RegisterRun(ctx, req.SagaRunID, report)
	   if err != nil {
		  return nil, nil, err
	  }
	  // ── 4. 绑定 ResidueRunID 到 items ────────────────────────────────────────
	  for i := range report.Items {
		  report.Items[i].ResidueRunID = rrun.ID
	   }
	  // ── 5. 持久化 report 和 items ─────────────────────────────────────────────
	  if err := s.store.SaveReport(ctx, report); err != nil {
		  return nil, rrun, err
	  }
	  for i := range report.Items {
		  if err := s.store.SaveResidueItem(ctx, &report.Items[i]); err != nil {
			   return nil, rrun, err
		  }
	   }
	   return report, rrun, nil
   }

// RegisterRun 将 partial_failed saga Run 注册为 ResidueRun（幂等）。
// 若已存在同 SagaRunID 的 ResidueRun，直接返回（不更新）。
// ResidueRun.PartialDoneSteps 来自 report.Items 中每条 item 的 Step 集合。
  func (s *Service) RegisterRun(ctx context.Context, sagaRunID string, report *ResidueReport) (*ResidueRun, error) {
	   if s == nil || s.store == nil {
		  return nil, ErrUnavailable
	  }
	  sagaRunID = strings.TrimSpace(sagaRunID)
	   // 幂等：若已注册则直接返回
	   existing, err := s.store.GetResidueRunBySagaRunID(ctx, sagaRunID)
	   if err == nil && existing != nil {
		   return existing, nil
	  }
	   // 从 items 中提取步骤列表
	  doneSteps := make([]string,
		  0, len(report.Items))
	   userID := ""
	  asset := ""
	   for _, item := range report.Items {
		  doneSteps = append(doneSteps, item.Step)
		   if asset == "" {
			  asset = item.Asset
		   }
	  }
	  now := s.clock()
	   rrun := &ResidueRun{
		  ID:        newID(),
		   SagaRunID: sagaRunID,
		  UserID: userID,
		  Asset:  asset,
		   PartialDoneSteps: doneSteps,
		  PartialCause:     report.ScanNote,
		  State:            RunStateResidueOpen,
		   Severity:         report.Severity,
		   MutexGeneration:  0,
		   CreatedAt:        now,
		  UpdatedAt: now,
	  }
	   if err := s.store.SaveResidueRun(ctx, rrun); err != nil {
		  return nil, err
	  }
	   return rrun, nil
  }

// ── 确认残留 ──────────────────────────────────────────────────────────────────
// ConfirmItems 双人授权确认残留科目为有效残留（IsConfirmed=true）。

  func (s *Service) ConfirmItems(ctx context.Context, req ConfirmResidueReq) error {
	  if s == nil || s.store == nil {
		  return ErrUnavailable
	   }
	  if err := checkDualAuth(req.Authorizer1, req.Authorizer2); err != nil {
		  return err
	  }
	   if len(req.ItemIDs) == 0 {
		   return fmt.Errorf("sagaresidue confirm items: item_ids required")
	   }
	  return s.store.ConfirmResidueItems(ctx, req.ItemIDs, s.clock())
  }

// ── 强制收口 ──────────────────────────────────────────────────────────────────
// InitiateCloseout 双人授权启动强制收口。
// 步骤：
// 1. 校验 Run 状态须为 residue_open。
// 2. 校验 Run 下已有确认的 ResidueItem。
// 3. Acquire 互斥锁（获取世代）。
// 4. 加载最新 ResidueReport。
// 5. BuildPlan + SavePlan + SaveLegs。
// 6. 更新 ResidueRun.State = closing，记录 PlanID。
// 执行（ExecuteCloseout）由 worker 在独立 goroutine 内调用，不在此方法中。
// 调用方须随后调用 ExecuteCloseout 或等待 worker 自动拾取。

   func (s *Service) InitiateCloseout(ctx context.Context, req CloseoutReq) (*CloseoutPlan, error) {
	  if s == nil || s.store == nil || s.orch == nil || s.mutexMgr == nil {
		  return nil, ErrUnavailable
	  }
	   if err := checkDualAuth(req.Authorizer1, req.Authorizer2); err != nil {
		   return nil, err
	   }
	   // ── 1. 加载 ResidueRun ────────────────────────────────────────────────────
	   rrun, err := s.store.GetResidueRun(ctx, req.ResidueRunID)
	   if err != nil {
		   return nil, ErrResidueRunNotFound
	  }
	  if rrun.State == RunStateClosed {
		   return nil, ErrAlreadyClosed
	   }
	  if rrun.State != RunStateResidueOpen && rrun.State != RunStateEscalated {
		   return nil, fmt.Errorf("%w: current state=%s", ErrBadRunState, rrun.State)
	  }
	  // ── 2. 确认有 IsConfirmed=true 的 items ──────────────────────────────────
	   items, err := s.store.ListResidueItems(ctx, rrun.ID)
	   if err != nil {
		   return nil, err
	   }
	  var confirmed []ResidueItem
	   for _, item := range items {
		  if item.IsConfirmed {
			   confirmed = append(confirmed, item)
		   }
	   }
	   if len(confirmed) == 0 {
		   return nil, fmt.Errorf("sagaresidue initiate closeout: no confirmed items")
	   }
	  // ── 3. Acquire 互斥锁 ─────────────────────────────────────────────────────
	   holderID := fmt.Sprintf("closeout-initiator:%s", newID()[:8])
	   mg, err := s.mutexMgr.Acquire(ctx, rrun.SagaRunID, holderID)
	   if err != nil {
		   return nil, err
	   }
	  // 记录世代，之后 Release 用
	  rrun.MutexGeneration = mg.Generation
	  generation := mg.Generation
	   // 函数退出时释放锁（仅当 BuildPlan 成功后由 worker 接管；此处仅为 InitiateCloseout 释放）
	  defer func() {
		   _ = s.mutexMgr.Release(ctx, rrun.SagaRunID, generation, holderID)
	   }()
	   // ── 4. 加载最新 ResidueReport ─────────────────────────────────────────────
	   report, err := s.store.GetLatestReport(ctx, rrun.SagaRunID)
	   if err != nil {
		  return nil, fmt.Errorf("sagaresidue initiate closeout: no report found")
	   }
	  // 将 confirmed items 合并入 report，供 BuildPlan 使用
	  report.Items = confirmed
	  // ── 5. BuildPlan ──────────────────────────────────────────────────────────
	   plan, legs, err := s.orch.BuildPlan(ctx, req, report, rrun)
	  if err != nil {
		  return nil, err
	   }
	   // ── 6. 持久化 ─────────────────────────────────────────────────────────────
	   if err := s.store.SavePlan(ctx, plan); err != nil {
		  return nil, err
	  }
	   for i := range legs {
		  if err := s.store.SaveLeg(ctx, &legs[i]); err != nil {
			   return nil, err
		   }
	   }
	   // 更新 Run 状态
	   rrun.State = RunStateClosing
	  rrun.CloseoutPlanID = plan.ID
	  rrun.UpdatedAt = s.clock()
	   if err := s.store.SaveResidueRun(ctx, rrun); err != nil {
		   return nil, err
	  }
	   return plan, nil
  }

// ExecuteCloseout worker 调用：执行收口计划的所有 pending 腿。
// holderID 须与 Acquire 时一致；generation 须与 Plan.Generation 一致。

   func (s *Service) ExecuteCloseout(ctx context.Context, planID, holderID string, generation int64) error {
	   if s == nil || s.orch == nil || s.store == nil {
		  return ErrUnavailable
	   }
	  plan, err := s.store.GetPlan(ctx, planID)
	  if err != nil {
		   return ErrResidueRunNotFound
	   }
	  defer func() {
		   _ = s.mutexMgr.Release(ctx, plan.SagaRunID, generation, holderID)
	   }()
	   if err := s.orch.ExecutePlan(ctx, plan, generation, holderID); err != nil {
		  return err
	   }
	  return s.tryFinalize(ctx, plan)
  }

// tryFinalize 检查 Plan 是否满足 Finalize 条件（无 pending / failed 腿），
// 若满足则生成 CloseProof 并标 Run 为 closed。

  func (s *Service) tryFinalize(ctx context.Context, plan *CloseoutPlan) error {
	   if plan.State != RunStateClosed {
		  return nil
	   }
	  rrun, err := s.store.GetResidueRun(ctx, plan.ResidueRunID)
	   if err != nil {
		  return err
	   }
	   return s.FinalizeRun(ctx, rrun.ID)
   }

// FinalizeRun 收口腿全完成后生成 CloseProof 并标 Run 为 closed。
// 若证明生成失败，Run 保持 closing，不得标 closed。

   func (s *Service) FinalizeRun(ctx context.Context, residueRunID string) error {
	  if s == nil || s.store == nil || s.prover == nil {
		   return ErrUnavailable
	  }
	   rrun, err := s.store.GetResidueRun(ctx, residueRunID)
	   if err != nil {
		   return err
	  }
	   // 加载数据
	   items, err := s.store.ListResidueItems(ctx, rrun.ID)
	   if err != nil {
		  return err
	  }
	   var confirmedItems []ResidueItem
	   for _, item := range items {
		  if item.IsConfirmed {
			  confirmedItems = append(confirmedItems, item)
		   }
	  }
	   plan, err := s.store.GetActivePlan(ctx, rrun.ID)
	  if err != nil {
		  return ErrProofMissing
	   }
	   legs, err := s.store.ListLegs(ctx, LegListReq{PlanID: plan.ID, Limit: 10000})
	  if err != nil {
		   return err
	   }
	   // ── 检查腿终态 ────────────────────────────────────────────────────────────
	  for _, l := range legs {
		   if l.State == LegStatePending || l.State == LegStateFailed {
			   return ErrProofMissing
		   }
	   }
	   // ── 生成证明 ──────────────────────────────────────────────────────────────
	  proof,
		  err := s.prover.IssueProof(ctx, rrun, confirmedItems, legs)
	   if err != nil {
		   return err
	  }
	   // ── 标 Run closed ─────────────────────────────────────────────────────────
	  now := s.clock()
	   rrun.State = RunStateClosed
	  rrun.CloseProofID = proof.ID
	   rrun.ClosedAt = &now
	  rrun.UpdatedAt = now
	  return s.store.SaveResidueRun(ctx, rrun)
  }

// ── safesaga 补偿钩子 ─────────────────────────────────────────────────────────
// CheckCompensateAllowed 检查 Compensate 是否被互斥锁阻止。
// 由 safesaga 外部中间件在 Compensate 入口调用。

   func (s *Service) CheckCompensateAllowed(ctx context.Context, sagaRunID string) error {
	  if s == nil || s.mutexMgr == nil {
		   return nil // 服务未注入时不阻止
	   }
	  return s.mutexMgr.CheckCompensateAllowed(ctx, sagaRunID)
  }

// ── 查询方法 ──────────────────────────────────────────────────────────────────

   func (s *Service) GetRun(ctx context.Context, id string) (*ResidueRun, error) {
	  if s == nil || s.store == nil {
		  return nil, ErrUnavailable
	  }
	   return s.store.GetResidueRun(ctx, id)
  }

  func (s *Service) ListRuns(ctx context.Context, req RunListReq) ([]ResidueRun, error) {
	  if s == nil || s.store == nil {
		   return nil, ErrUnavailable
	  }
	  return s.store.ListResidueRuns(ctx, req)
  }

   func (s *Service) GetProof(ctx context.Context, id string) (*CloseProof, error) {
	  if s == nil || s.store == nil {
		  return nil, ErrUnavailable
	   }
	   return s.store.GetProof(ctx, id)
  }

   func (s *Service) VerifyProof(ctx context.Context, req ProofVerifyReq) (*CloseProof, error) {
	  if s == nil || s.prover == nil {
		   return nil, ErrUnavailable
	   }
	   return s.prover.VerifyProof(ctx, req)
   }

   func (s *Service) GetMutexStatus(ctx context.Context, sagaRunID string) (bool, *MutexGen, error) {
	   if s == nil || s.mutexMgr == nil {
		  return false, nil, ErrUnavailable
	   }
	   return s.mutexMgr.IsLocked(ctx, sagaRunID)
   }

  func (s *Service) SkipLeg(ctx context.Context, req SkipLegReq) error {
	   if s == nil || s.orch == nil {
		  return ErrUnavailable
	  }
	   return s.orch.SkipLeg(ctx, req)
   }

  func (s *Service) GetPlan(ctx context.Context, id string) (*CloseoutPlan, error) {
	  if s == nil || s.store == nil {
		   return nil, ErrUnavailable
	  }
	  return s.store.GetPlan(ctx, id)
  }

   func (s *Service) ListLegs(ctx context.Context, req LegListReq) ([]CloseoutLeg, error) {
	  if s == nil || s.store == nil {
		  return nil, ErrUnavailable
	   }
	  return s.store.ListLegs(ctx, req)
   }

   func (s *Service) GetReport(ctx context.Context, sagaRunID string) (*ResidueReport, error) {
	  if s == nil || s.store == nil {
		  return nil, ErrUnavailable
	  }
	  return s.store.GetLatestReport(ctx, sagaRunID)
   }

// ── 内部辅助 ──────────────────────────────────────────────────────────────────
// newID 生成全局唯一 ID（UUID v4）。
  func newID() string {
	   return uuid.NewString()
  }
