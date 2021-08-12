package ledseal

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/snap"
)

// Service 账本快照差分封堵主服务。

type Service struct {
	store   Store
	snapSvc *snap.Service
	engine  *DiffEngine
	clock func() time.Time
}

// New 构造 Service。snapSvc 只读；store 只写 ledseal 表。
   func New(snapSvc *snap.Service, store Store) *Service {
	   if store == nil {
		   store = NewMemStore()
	  }
	  clock := func() time.Time { return time.Now().UTC() }
	  return &Service{
		   store:   store,
		  snapSvc: snapSvc,
		   engine:  NewDiffEngine(snapSvc, store, clock),
		  clock: clock,
	   }
   }

// WithClock 注入时钟（用）。
  func (s *Service) WithClock(c func() time.Time) *Service {
	   if s != nil && c != nil {
		  s.clock = c
		  s.engine = NewDiffEngine(s.snapSvc, s.store, c)
	   }
	   return s
  }

  // Store 返回底层存储（供和 patrol 使用）。
  func (s *Service) Store() Store {
	   if s == nil {
		  return nil
	   }
	  return s.store
   }

// ─────────────────────────────────────────────────────────
// SnapPlan CRUD
// ─────────────────────────────────────────────────────────
// CreatePlan 创建快照调度计划。名称全局唯一；重复返回 ErrPlanDuplicate。

  func (s *Service) CreatePlan(ctx context.Context, req CreatePlanReq) (SnapPlan, error) {
	  if s == nil {
		   return SnapPlan{}, ErrUnavailable
	  }
	   req.Name = strings.TrimSpace(req.Name)
	   if req.Name == "" {
		  return SnapPlan{}, fmt.Errorf("%w: name required", ErrParam)
	  }
	  if req.IntervalSeconds <= 0 {
		  req.IntervalSeconds = 3600
	   }
	   if req.RetainCount <= 0 {
		   req.RetainCount = 48
	  }
	  // 重复名称校验。
	  if _, err := s.store.GetPlanByName(ctx, req.Name); err == nil {
		   return SnapPlan{}, ErrPlanDuplicate
	  }
	  now := s.clock()
	   next := now.Add(time.Duration(req.IntervalSeconds) * time.Second)
	  plan := SnapPlan{
		   ID:        uuid.NewString(),
		  Name:      req.Name,
		  PlanScope: req.PlanScope,
		   ShardKey: strings.TrimSpace(req.ShardKey),
		   AssetFamily: strings.TrimSpace(req.AssetFamily),
		  IntervalSeconds: req.IntervalSeconds,
		   RetainCount:     req.RetainCount,
		  State:     PlanActive,
		   NextRunAt: &next,
		  CreatedBy: strings.TrimSpace(req.CreatedBy),
		  CreatedAt: now,
		   UpdatedAt: now,
	  }
	   if err := s.store.PutPlan(ctx, plan); err != nil {
		  return SnapPlan{}, err
	   }
	  return plan, nil
  }

// GetPlan 按 ID 取计划。

   func (s *Service) GetPlan(ctx context.Context, id string) (SnapPlan, error) {
	  if s == nil {
		  return SnapPlan{}, ErrUnavailable
	  }
	   return s.store.GetPlan(ctx, id)
   }

// ListPlans 列出所有计划。

  func (s *Service) ListPlans(ctx context.Context, limit int) ([]SnapPlan, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	  }
	   return s.store.ListPlans(ctx, limit)
   }

// ActivatePlan 把暂停的计划激活。
   func (s *Service) ActivatePlan(ctx context.Context, id string) (SnapPlan, error) {
	  if s == nil {
		  return SnapPlan{}, ErrUnavailable
	   }
	   plan,
		  err := s.store.GetPlan(ctx, id)
	  if err != nil {
		   return SnapPlan{}, err
	  }
	   if !CanActivatePlan(plan.State) {
		   return plan, fmt.Errorf("%w: current=%s", ErrPlanState, plan.State)
	   }
	  now := s.clock()
	  next := now.Add(time.Duration(plan.IntervalSeconds) * time.Second)
	  plan.State = PlanActive
	  plan.NextRunAt = &next
	  plan.UpdatedAt = now
	   if err := s.store.PutPlan(ctx, plan); err != nil {
		  return plan, err
	  }
	   return plan, nil
   }

// PausePlan 暂停活跃计划。
   func (s *Service) PausePlan(ctx context.Context, id string) (SnapPlan, error) {
	   if s == nil {
		   return SnapPlan{}, ErrUnavailable
	  }
	  plan, err := s.store.GetPlan(ctx, id)
	  if err != nil {
		   return SnapPlan{}, err
	   }
	   if !CanPausePlan(plan.State) {
		   return plan, fmt.Errorf("%w: current=%s", ErrPlanState, plan.State)
	  }
	   plan.State = PlanPaused
	   plan.UpdatedAt = s.clock()
	  if err := s.store.PutPlan(ctx, plan); err != nil {
		   return plan, err
	   }
	   return plan, nil
   }

// ─────────────────────────────────────────────────────────
// SnapSlice 管理
// ─────────────────────────────────────────────────────────
// RegisterSlice 将一个已完成的快照登记为切片（由 capture pump 调用）。

   func (s *Service) RegisterSlice(ctx context.Context, planID, snapID, userID, assetFamily, checksum string, posCount int, fenceEpoch int64, fenceToken string, takenAt time.Time) (SnapSlice, error) {
	  if s == nil {
		   return SnapSlice{}, ErrUnavailable
	   }
	   // 获取最新序号，序号单调递增。
	  latest, err := s.store.LatestSlice(ctx, planID)
	  var nextSeq int64 = 1
	   if err == nil {
		  nextSeq = latest.GenSeq + 1
	   } else if err != ErrSliceNotFound {
		   return SnapSlice{}, err
	   }
	  now := s.clock()
	   sl := SnapSlice{
		  ID:     uuid.NewString(),
		  PlanID: planID,
		   SnapID: snapID,
		  UserID: userID,
		   AssetFamily: assetFamily,
		  GenSeq:      nextSeq,
		  FenceEpoch: fenceEpoch,
		   FenceToken:    fenceToken,
		  Checksum:      checksum,
		  PositionCount: posCount,
		  State:         "ready",
		   TakenAt: takenAt,
		   CreatedAt: now,
	   }
	  if err := s.store.PutSlice(ctx, sl); err != nil {
		  return SnapSlice{}, err
	  }
	  return sl, nil
   }

// GetSlice 按 ID 取切片。

   func (s *Service) GetSlice(ctx context.Context, id string) (SnapSlice, error) {
	   if s == nil {
		   return SnapSlice{}, ErrUnavailable
	  }
	   return s.store.GetSlice(ctx, id)
  }

// LatestSlice 返回计划的最新切片。
   func (s *Service) LatestSlice(ctx context.Context, planID string) (SnapSlice, error) {
	  if s == nil {
		   return SnapSlice{}, ErrUnavailable
	  }
	  return s.store.LatestSlice(ctx, planID)
   }

// ListSlices 列出计划的切片（按 gen_seq 升序）。
  func (s *Service) ListSlices(ctx context.Context, planID string, limit int) ([]SnapSlice, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	   }
	   return s.store.ListSlices(ctx,
		  planID, limit)
   }

// ─────────────────────────────────────────────────────────
// DiffBatch 管理
// ─────────────────────────────────────────────────────────
// OpenBatch 开启一个差分批次（由 diff pump 调用）。
   func (s *Service) OpenBatch(ctx context.Context, planID, fromSliceID, toSliceID string, fromSeq, toSeq int64) (DiffBatch, error) {
	   if s == nil {
		  return DiffBatch{}, ErrUnavailable
	   }
	   now := s.clock()
	  b := DiffBatch{
		  ID:          uuid.NewString(),
		   PlanID:      planID,
		   FromSliceID: fromSliceID,
		  ToSliceID:   toSliceID,
		  FromGenSeq: fromSeq,
		  ToGenSeq:    toSeq,
		   State:       BatchRunning,
		   MaxSeverity: SeverityNone,
		   StartedAt: now,
		   CreatedAt: now,
		  UpdatedAt: now,
	   }
	  if err := s.store.PutBatch(ctx, b); err != nil {
		   return DiffBatch{}, err
	   }
	   return b, nil
   }

// RunAndSealBatch 执行增量差分并尝试封印。
// 封印失败（seq 间隙/回滚）时批次标记 failed，返回对应错误。

   func (s *Service) RunAndSealBatch(ctx context.Context, batchID string) (DiffBatch, SealRoot, error) {
	   if s == nil {
		   return DiffBatch{}, SealRoot{}, ErrUnavailable
	  }
	  batch, err := s.store.GetBatch(ctx, batchID)
	  if err != nil {
		   return DiffBatch{}, SealRoot{}, err
	  }
	   if !CanSealBatch(batch.State) {
		  return batch,
			  SealRoot{}, ErrBatchState
	  }
	   fromSlice, err := s.store.GetSlice(ctx, batch.FromSliceID)
	  if err != nil {
		  return batch, SealRoot{}, fmt.Errorf("from slice: %w", err)
	   }
	   toSlice, err := s.store.GetSlice(ctx, batch.ToSliceID)
	   if err != nil {
		  return batch, SealRoot{}, fmt.Errorf("to slice: %w", err)
	   }
	   // 执行差分（可能把 batch.State 置 failed）。
	   batch, err = s.engine.RunBatch(ctx, batch, fromSlice, toSlice)
	   if err != nil {
		   return batch, SealRoot{}, err
	  }
	   // 差分成功，生成封印根。
	  root, batch, err := BuildSealRoot(ctx, s.store, batch, s.clock)
	  if err != nil {
		   return batch, SealRoot{}, err
	   }
	  return batch, root, nil
  }

// GetBatch 按 ID 取批次。
   func (s *Service) GetBatch(ctx context.Context, id string) (DiffBatch, error) {
	  if s == nil {
		   return DiffBatch{}, ErrUnavailable
	  }
	   return s.store.GetBatch(ctx, id)
   }

// ListBatches 列出批次。state 为空则不过滤。
  func (s *Service) ListBatches(ctx context.Context, planID string, state BatchState, limit int) ([]DiffBatch, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	  return s.store.ListBatches(ctx, planID, state, limit)
   }

// ─────────────────────────────────────────────────────────
// DriftRow 管理
// ─────────────────────────────────────────────────────────
// ListDriftRows 列出批次的漂移行。
  func (s *Service) ListDriftRows(ctx context.Context, batchID string, limit int) ([]DriftRow, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	   return s.store.ListDriftRows(ctx, batchID, limit)
  }

// ConfirmDrift 运营台人工确认漂移行（mark confirmed）。

  func (s *Service) ConfirmDrift(ctx context.Context, req ConfirmDriftReq) error {
	   if s == nil {
		  return ErrUnavailable
	  }
	  if strings.TrimSpace(req.DriftID) == "" {
		   return fmt.Errorf("%w: drift_id required", ErrParam)
	   }
	   if strings.TrimSpace(req.Actor) == "" {
		   return fmt.Errorf("%w: actor required", ErrParam)
	   }
	  return s.store.ConfirmDriftRow(ctx, req.DriftID, req.Actor, req.Note, s.clock())
  }

// ─────────────────────────────────────────────────────────
// SealRoot 管理
// ─────────────────────────────────────────────────────────
// ExportRoot 标记封印根已导出（由对账台调用）。
  func (s *Service) ExportRoot(ctx context.Context, req ExportRootReq) (SealRoot, error) {
	  if s == nil {
		   return SealRoot{}, ErrUnavailable
	  }
	  if strings.TrimSpace(req.BatchID) == "" {
		   return SealRoot{}, fmt.Errorf("%w: batch_id required", ErrParam)
	  }
	  root, err := s.store.GetSealRootByBatch(ctx, req.BatchID)
	  if err != nil {
		  return SealRoot{}, err
	   }
	   if err := s.store.MarkExported(ctx, root.ID, req.ExportedBy, s.clock()); err != nil {
		  return SealRoot{}, err
	   }
	   root.ExportedBy = req.ExportedBy
	   return root, nil
   }

// GetSealRoot 按 ID 取封印根。

  func (s *Service) GetSealRoot(ctx context.Context, id string) (SealRoot, error) {
	   if s == nil {
		  return SealRoot{}, ErrUnavailable
	  }
	   return s.store.GetSealRoot(ctx,
		  id)
   }

// VerifyRoot 校验封印根完整性（只读）。

  func (s *Service) VerifyRoot(ctx context.Context, rootID string) error {
	   if s == nil {
		   return ErrUnavailable
	  }
	   return VerifySealRoot(ctx, s.store, rootID)
  }
