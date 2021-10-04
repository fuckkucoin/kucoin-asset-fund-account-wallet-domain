package prefroute

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/fundpref"
)

// LeaseManager 管理 PrefLease 的签发、绑定与校验。

type LeaseManager struct {
	store Store
	cfg Config
	clock func() time.Time
}

// NewLeaseManager 构造 LeaseManager。

  func NewLeaseManager(store Store, cfg Config) *LeaseManager {
	   return &LeaseManager{
		  store: store,
		  cfg:   cfg,
		   clock: func() time.Time { return time.Now().UTC() },
	   }
  }

// WithClock 注入时钟。

  func (m *LeaseManager) WithClock(c func() time.Time) *LeaseManager {
	   if m != nil && c != nil {
		  m.clock = c
	   }
	  return m
  }

  // IssueLease 签发偏好租约。
   // 检查：
  // 1. 用户是否有未收口的漂移案例（open），有则拒绝。
  // 2. 快照当前偏好世代。
  func (m *LeaseManager) IssueLease(ctx context.Context, req IssuePrefLeaseReq) (PrefLease, error) {
	  if m == nil || m.store == nil {
		   return PrefLease{}, fmt.Errorf("%w: lease manager unavailable", ErrInvalidInput)
	   }
	  userID := strings.TrimSpace(req.UserID)
	   if userID == "" {
		  return PrefLease{}, fmt.Errorf("%w: userID required", ErrInvalidInput)
	  }
	  if req.CreditTxnID == "" {
		   return PrefLease{}, fmt.Errorf("%w: creditTxnID required", ErrInvalidInput)
	  }
	   // 检查未收口漂移案例
	   blocked, caseID, err := m.store.HasOpenDriftCase(ctx, userID)
	   if err != nil {
		   return PrefLease{}, fmt.Errorf("check drift case: %w", err)
	  }
	  if blocked {
		   return PrefLease{}, fmt.Errorf("%w: open drift case %s", ErrDriftUnproven, caseID)
	   }
	   // 获取当前偏好世代
	   gen, err := m.store.GetGeneration(ctx, userID)
	  if err != nil {
		   if err == fundpref.ErrNotFound {
			  // 用户尚无偏好世代，使用初始值（generation=0，book=MAIN）
			   gen = fundpref.PrefGeneration{
				  UserID:      userID,
				   Generation:  0,
				  DefaultBook: fundpref.BookMain,
			   }
		  } else {
			   return PrefLease{}, fmt.Errorf("get generation: %w", err)
		  }
	  }
	  now := m.clock()
	   l := PrefLease{
		  ID: uuid.NewString(),
		  UserID: userID,
		  Generation: gen.Generation,
		  DefaultBook: gen.DefaultBook,
		   State: LeaseStateOpen,
		  CreditTxnID: req.CreditTxnID,
		   IssuedAt:    now,
		  ExpiresAt: now.Add(m.cfg.LeaseTTL),
		   IssuedBy:  req.IssuedBy,
		   FenceEpoch: 0,
		   FenceToken: uuid.NewString(),
	  }
	   if err := m.store.SavePrefLease(ctx, l); err != nil {
		  return PrefLease{}, fmt.Errorf("save pref lease: %w", err)
	  }
	  return l, nil
   }

// BindRouteSnap 将路由快照绑定到租约（贷记执行前调用）。
// 校验租约仍有效（世代未推进，未过期）后，创建 RouteSnap 并将租约状态→bound。
  func (m *LeaseManager) BindRouteSnap(ctx context.Context, req BindRouteSnapReq) (PrefLease, RouteSnap, error) {
	  if m == nil || m.store == nil {
		  return PrefLease{}, RouteSnap{}, fmt.Errorf("%w: lease manager unavailable", ErrInvalidInput)
	   }
	   if req.LeaseID == "" {
		  return PrefLease{}, RouteSnap{}, fmt.Errorf("%w: leaseID required", ErrInvalidInput)
	   }
	  l, err := m.store.GetPrefLease(ctx, req.LeaseID)
	  if err != nil {
		  return PrefLease{}, RouteSnap{}, fmt.Errorf("get lease: %w", err)
	   }
	  // 校验租约有效性（检查世代）
	   curGen, err := m.store.GetGeneration(ctx, l.UserID)
	   if err != nil && err != fundpref.ErrNotFound {
		  return PrefLease{}, RouteSnap{}, fmt.Errorf("get generation: %w", err)
	   }
	  var currentGenNum int64
	   if err == nil {
		   currentGenNum = curGen.Generation
	  }
	   if err2 := l.IsValid(m.clock(), currentGenNum); err2 != nil {
		  return PrefLease{}, RouteSnap{}, err2
	   }
	  now := m.clock()
	   snap := RouteSnap{
		   ID: uuid.NewString(),
		   LeaseID: l.ID,
		   UserID:  l.UserID,
		   Generation: l.Generation,
		  BoundBook:  l.DefaultBook,
		   CreditTxnID: req.CreditTxnID,
		  Asset: req.Asset,
		   Amount: req.Amount,
		   SnapAt: now,
	  }
	   if err := m.store.SaveRouteSnap(ctx, snap); err != nil {
		  return PrefLease{}, RouteSnap{}, fmt.Errorf("save route snap: %w", err)
	   }
	  l.State = LeaseStateBound
	   l.RouteSnapID = snap.ID
	  l.BoundAt = &now
	  l.FenceEpoch++
	   l.FenceToken = uuid.NewString()
	   if err := m.store.UpdatePrefLease(ctx, l); err != nil {
		   return PrefLease{}, RouteSnap{}, fmt.Errorf("update lease: %w", err)
	   }
	   return l, snap, nil
  }

// ValidateLease 校验租约在当前时刻与世代是否有效（不改变状态）。
  func (m *LeaseManager) ValidateLease(ctx context.Context, leaseID string) (PrefLease, error) {
	  if m == nil || m.store == nil {
		  return PrefLease{}, fmt.Errorf("%w: lease manager unavailable", ErrInvalidInput)
	  }
	   l, err := m.store.GetPrefLease(ctx, leaseID)
	  if err != nil {
		  return PrefLease{}, fmt.Errorf("get lease: %w", err)
	  }
	  curGen, err := m.store.GetGeneration(ctx, l.UserID)
	  var currentGenNum int64
	   if err == nil {
		  currentGenNum = curGen.Generation
	  } else if err != fundpref.ErrNotFound {
		  return PrefLease{}, fmt.Errorf("get generation: %w", err)
	   }
	   if err2 := l.IsValid(m.clock(), currentGenNum); err2 != nil {
		  return l, err2
	  }
	   return l, nil
   }

// BlockLease 将租约标记为 blocked（漂移或违规触发）。

   func (m *LeaseManager) BlockLease(ctx context.Context, leaseID, reason string) (PrefLease, error) {
	  if m == nil || m.store == nil {
		   return PrefLease{}, fmt.Errorf("%w: lease manager unavailable", ErrInvalidInput)
	  }
	  l, err := m.store.GetPrefLease(ctx, leaseID)
	   if err != nil {
		  return PrefLease{}, fmt.Errorf("get lease: %w", err)
	   }
	  if l.State == LeaseStateBlocked {
		  return l, nil // 幂等
	  }
	  now := m.clock()
	  l.State = LeaseStateBlocked
	   l.BlockedAt = &now
	  l.BlockReason = reason
	  l.FenceEpoch++
	   l.FenceToken = uuid.NewString()
	   if err := m.store.UpdatePrefLease(ctx, l); err != nil {
		  return PrefLease{}, fmt.Errorf("update lease: %w", err)
	  }
	   return l, nil
  }
