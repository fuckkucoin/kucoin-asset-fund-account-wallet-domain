package viptiergate

import (
	"context"
	"strings"
	"time"
	"github.com/google/uuid"
)

// LeaseFSM 管理 TierLease 生命周期与 TierGeneration CAS。
type LeaseFSM struct {
	store Store
	clock func() time.Time
}

  func newLeaseFSM(store Store, clock func() time.Time) *LeaseFSM {
	  return &LeaseFSM{store: store,
		   clock: clock}
   }

   // Acquire 申请档位切换租约。
   // 流程：
   // 1. 读取 TierGeneration.Generation；若 gen != req.ExpectedGen → ErrStaleGen。
  // 2. 原子递增世代至 ExpectedGen+1，写入 store（CAS UPDATE）。
  // 3. 创建 TierLease（状态 open）。
   // 4. 返回新租约与新世代。
  // 此调用是幂等的：同一 TriggerRef 若已有 open/bound/blocked 租约，直接返回现有租约。
   func (f *LeaseFSM) Acquire(ctx context.Context, req AcquireLeaseReq) (*TierLease, int64, error) {
	   if strings.TrimSpace(req.UserID) == "" {
		   return nil, 0, ErrUserInvalid
	   }
	   if TierRank(req.FromTier) < 0 || TierRank(req.ToTier) < 0 {
		  return nil, 0, ErrTierInvalid
	  }
	  // 幂等：TriggerRef 已有活跃租约，直接返回。
	   if req.TriggerRef != "" {
		   existing, err := f.store.GetLeaseByTriggerRef(ctx, req.UserID, req.TriggerRef)
		   if err == nil && existing != nil {
			  return existing, existing.Generation, nil
		  }
	   }
	  // CAS 世代递增。
	   gen,
		  err := f.store.BumpGeneration(ctx, req.UserID, req.ExpectedGen, req.ToTier)
	  if err != nil {
		  return nil, 0, err // ErrStaleGen 向上传播
	   }
	  now := f.clock()
	   lease := &TierLease{
		  ID:     uuid.NewString(),
		   UserID: req.UserID,
		   FromTier:    req.FromTier,
		   ToTier:      req.ToTier,
		  Generation:  gen,
		  TriggerKind: req.TriggerKind,
		   TriggerRef:  req.TriggerRef,
		  Status:    LeaseOpen,
		  CreatedAt: now,
		  UpdatedAt: now,
	  }
	  if err := f.store.SaveLease(ctx, lease); err != nil {
		   return nil, 0, err
	   }
	  return lease, gen, nil
  }

// Bind 将 open 租约推进至 bound，并记录绑定时间戳。
// 调用方在世代令牌颁发（QuoteBind 发出）后调用此方法。

   func (f *LeaseFSM) Bind(ctx context.Context, req BindLeaseReq) (*TierLease, error) {
	  lease, err := f.store.GetLease(ctx, req.LeaseID)
	   if err != nil {
		  return nil, ErrLeaseNotFound
	   }
	   if lease.Status != LeaseOpen {
		   return nil, ErrLeaseStale
	   }
	   now := f.clock()
	   lease.Status = LeaseBound
	  lease.Generation = req.Generation
	   lease.BoundAt = &now
	   lease.UpdatedAt = now
	  if err := f.store.SaveLease(ctx, lease); err != nil {
		   return nil, err
	  }
	   return lease, nil
  }

// Switch 将 bound 租约标记为 switched（切换完成终态）。
// 由 worker 在确认所有在途报价均已过期或重算后调用。
  func (f *LeaseFSM) Switch(ctx context.Context, leaseID string) (*TierLease, error) {
	  lease, err := f.store.GetLease(ctx, leaseID)
	  if err != nil {
		   return nil, ErrLeaseNotFound
	  }
	   if lease.Status != LeaseBound {
		  return nil, ErrLeaseStale
	   }
	  now := f.clock()
	   lease.Status = LeaseSwitched
	  lease.SwitchedAt = &now
	   lease.UpdatedAt = now
	  if err := f.store.SaveLease(ctx, lease); err != nil {
		   return nil, err
	  }
	   return lease, nil
  }

// Block 将 open/bound 租约挂起为 blocked，并记录阻塞原因。
// 由在途冻额/Quote 互斥检查失败时调用，或由 worker 扫描到新冻额后补充调用。
  func (f *LeaseFSM) Block(ctx context.Context, leaseID, reason string) (*TierLease, error) {
	  lease, err := f.store.GetLease(ctx, leaseID)
	   if err != nil {
		  return nil, ErrLeaseNotFound
	   }
	  if lease.Status != LeaseOpen && lease.Status != LeaseBound {
		   return nil, ErrLeaseStale
	  }
	  now := f.clock()
	   lease.Status = LeaseBlocked
	  lease.BlockReason = reason
	   lease.BlockedAt = &now
	  lease.UpdatedAt = now
	  if err := f.store.SaveLease(ctx, lease); err != nil {
		  return nil, err
	   }
	  return lease, nil
   }

// Unblock 将 blocked 租约重新推进至 open，供 worker 在冻额/Quote 清零后调用。
  func (f *LeaseFSM) Unblock(ctx context.Context, leaseID string) (*TierLease, error) {
	  lease, err := f.store.GetLease(ctx, leaseID)
	  if err != nil {
		  return nil, ErrLeaseNotFound
	   }
	  if lease.Status != LeaseBlocked {
		  return nil, ErrLeaseStale
	  }
	   now := f.clock()
	   lease.Status = LeaseOpen
	   lease.BlockReason = ""
	  lease.UpdatedAt = now
	   if err := f.store.SaveLease(ctx, lease); err != nil {
		   return nil, err
	  }
	  return lease, nil
   }

// GetLease 按 lease_id 查询。

  func (f *LeaseFSM) GetLease(ctx context.Context, leaseID string) (*TierLease, error) {
	  return f.store.GetLease(ctx, leaseID)
  }

// ActiveByUser 返回用户所有非终态（未 switched）的租约列表。

   func (f *LeaseFSM) ActiveByUser(ctx context.Context, userID string) ([]*TierLease, error) {
	   return f.store.ListActiveLeasesByUser(ctx, userID)
  }

// ListBlocked 返回所有 blocked 状态的租约（批量，供 worker 定期推进）。
  func (f *LeaseFSM) ListBlocked(ctx context.Context, limit int) ([]*TierLease, error) {
	   return f.store.ListLeasesByStatus(ctx, LeaseBlocked, limit)
  }
