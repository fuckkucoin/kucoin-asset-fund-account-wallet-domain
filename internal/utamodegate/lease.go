package utamodegate

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// LeaseFSM 管理 ModeLease 生命周期与 ModeGeneration CAS。

type LeaseFSM struct {
	store  Store
	genEng *GenerationEngine
	clock  func() time.Time
}

  func newLeaseFSM(store Store, genEng *GenerationEngine, clock func() time.Time) *LeaseFSM {
	  return &LeaseFSM{store: store,
		   genEng: genEng, clock: clock}
  }

  // Acquire 申请模式切换租约。
  // 调用流程：
   // 1. 检查 TriggerRef 幂等（已有活跃租约直接返回）。
  // 2. 检查是否存在阻塞性 DriftCase（ErrDriftBlocked）。
   // 3. 检查在途 LiabilityBind（ErrInflightLiability → 创建 blocked 租约入队）。
  // 4. CAS 递增 ModeGeneration（ErrStaleGen）。
  // 5. 创建 open 状态租约并持久化。
  // 参数 driftBlocked 由调用方通过 DriftDetector.IsBlocked 预先查询后传入，
  // 避免在 FSM 内部引入 DriftDetector 的循环依赖。
  func (f *LeaseFSM) Acquire(
	   ctx context.Context,
	   req AcquireLeaseReq,
	  inflightCount int,
	  driftBlocked bool,
   ) (*ModeLease, int64, error) {
	  userID := strings.TrimSpace(req.UserID)
	  if userID == "" {
		  return nil, 0, ErrUserInvalid
	   }
	   if req.FromMode != MarginModeUnified && req.FromMode != MarginModeClassic {
		  return nil, 0, ErrModeInvalid
	  }
	   if req.ToMode != MarginModeUnified && req.ToMode != MarginModeClassic {
		   return nil, 0, ErrModeInvalid
	   }
	   if req.FromMode == req.ToMode {
		  return nil, 0, ErrSameMode
	  }
	   // 幂等：TriggerRef 已有活跃租约，直接返回（携带对应的语义错误，便于调用方判断状态）。
	   if req.TriggerRef != "" {
		  existing, err := f.store.GetLeaseByTriggerRef(ctx, userID, req.TriggerRef)
		   if err == nil && existing != nil {
			   // 若已有租约仍处于 blocked 状态，向上传播 ErrInflightLiability 让调用方感知。
			   if existing.Status == LeaseBlocked {
				  return existing, existing.Generation, ErrInflightLiability
			  }
			  return existing,
				  existing.Generation, nil
		   }
	  }
	  // 漂移阻塞：存在 open/proven DriftCase 时拒绝切换。
	  if driftBlocked {
		   return nil, 0, ErrDriftBlocked
	  }
	  now := f.clock()
	  // 在途负债/抵押互斥：创建 blocked 租约入待切换队列。
	  if inflightCount > 0 {
		   blocked := &ModeLease{
			   ID:       uuid.NewString(),
			  UserID:   userID,
			  FromMode: req.FromMode,
			  ToMode:      req.ToMode,
			  Generation:  req.ExpectedGen, // 世代尚未递增，仍为旧值
			  TriggerKind: req.TriggerKind,
			  TriggerRef: req.TriggerRef,
			   Status:     LeaseBlocked,
			  Note:       fmt.Sprintf("blocked: %d open liability/collateral bind(s) in flight", inflightCount),
			  CreatedAt:  now,
			  UpdatedAt:  now,
		   }
		   if err := f.store.SaveLease(ctx, blocked); err != nil {
			  return nil, 0, err
		   }
		  return blocked, req.ExpectedGen, ErrInflightLiability
	  }
	  // CAS 递增世代。
	  newGen, err := f.genEng.BumpGen(ctx, userID, req.ExpectedGen, req.ToMode)
	  if err != nil {
		   return nil, 0, err // ErrStaleGen 向上传播
	   }
	   lease := &ModeLease{
		   ID:       uuid.NewString(),
		   UserID:   userID,
		  FromMode: req.FromMode,
		   ToMode:      req.ToMode,
		   Generation:  newGen,
		  TriggerKind: req.TriggerKind,
		   TriggerRef:  req.TriggerRef,
		  Status: LeaseOpen,
		   CreatedAt: now,
		  UpdatedAt: now,
	   }
	  if err := f.store.SaveLease(ctx, lease); err != nil {
		  return nil, 0, err
	  }
	   return lease, newGen, nil
  }

// Bind 将 open 租约推进至 bound 状态。
// 约束：租约必须处于 open 状态；且所携带的世代号必须与 Generation 匹配。
// Bind 完成后，旧世代 LiabilityBind 的后续写操作将被 ValidateToken 拒绝。

  func (f *LeaseFSM) Bind(ctx context.Context, leaseID string, confirmedGen int64) (*ModeLease, error) {
	   lease, err := f.store.GetLease(ctx, leaseID)
	  if err != nil {
		  return nil, err
	   }
	   if lease.Status != LeaseOpen {
		   return nil, fmt.Errorf("%w: current status=%s", ErrLeaseStale, lease.Status)
	  }
	  if lease.Generation != confirmedGen {
		  return nil, ErrStaleGen
	  }
	   lease.Status = LeaseBound
	   lease.UpdatedAt = f.clock()
	  if err := f.store.SaveLease(ctx, lease); err != nil {
		   return nil, err
	   }
	   return lease, nil
   }

// Switch 将 bound 租约推进至 switched 终态（切换完成）。
// 由 worker 在确认所有旧世代在途负债均已 released 后调用。
   func (f *LeaseFSM) Switch(ctx context.Context, leaseID string) (*ModeLease, error) {
	  lease, err := f.store.GetLease(ctx, leaseID)
	  if err != nil {
		  return nil, err
	   }
	  if lease.Status != LeaseBound {
		  return nil, fmt.Errorf("%w: current status=%s, need bound", ErrLeaseStale, lease.Status)
	   }
	   lease.Status = LeaseSwitched
	   lease.UpdatedAt = f.clock()
	   if err := f.store.SaveLease(ctx, lease); err != nil {
		  return nil, err
	  }
	   return lease, nil
  }

// Block 将 open 租约挂起为 blocked 状态（在途负债/抵押仍存在）。
// 用于在 Acquire 后发现并发写入了新的在途负债时补偿挂起。
  func (f *LeaseFSM) Block(ctx context.Context, leaseID, reason string) (*ModeLease, error) {
	   lease, err := f.store.GetLease(ctx, leaseID)
	  if err != nil {
		  return nil, err
	  }
	  if lease.Status == LeaseSwitched {
		   return nil, fmt.Errorf("%w: already switched", ErrLeaseStale)
	   }
	  if lease.Status == LeaseBlocked {
		   return lease, nil // 幂等
	   }
	  lease.Status = LeaseBlocked
	  lease.Note = reason
	   lease.UpdatedAt = f.clock()
	  if err := f.store.SaveLease(ctx, lease); err != nil {
		  return nil, err
	  }
	   return lease, nil
  }

// Unblock 将 blocked 租约重新推进至 open（在途负债清零后）。
// 由 worker 的 LiabilityScanner 触发。
  func (f *LeaseFSM) Unblock(ctx context.Context, leaseID string) (*ModeLease, error) {
	   lease, err := f.store.GetLease(ctx, leaseID)
	  if err != nil {
		   return nil, err
	  }
	  if lease.Status != LeaseBlocked {
		   return nil, fmt.Errorf("%w: current status=%s, need blocked", ErrLeaseStale, lease.Status)
	   }
	  lease.Status = LeaseOpen
	   lease.Note = "unblocked by worker: in-flight liabilities cleared"
	  lease.UpdatedAt = f.clock()
	  if err := f.store.SaveLease(ctx, lease); err != nil {
		   return nil, err
	   }
	  return lease, nil
  }

// GetByUser 查询用户所有活跃（非 switched）租约。

  func (f *LeaseFSM) GetByUser(ctx context.Context, userID string) ([]*ModeLease, error) {
	  return f.store.ListActiveLeasesByUser(ctx, userID)
   }

// GetLease 查询单条租约。

  func (f *LeaseFSM) GetLease(ctx context.Context, leaseID string) (*ModeLease, error) {
	  return f.store.GetLease(ctx, leaseID)
  }
