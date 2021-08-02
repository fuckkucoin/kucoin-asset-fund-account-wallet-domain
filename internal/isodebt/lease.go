package isodebt

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// LeaseManager 管理 RatioLease 的生命周期。
type LeaseManager struct {
	store  Store
	genMgr *GenerationManager
	cfg   Config
	clock func() time.Time
}

// NewLeaseManager 构造租约管理器。

   func NewLeaseManager(store Store, genMgr *GenerationManager, cfg Config) *LeaseManager {
	   return &LeaseManager{
		   store: store,
		  genMgr: genMgr,
		  cfg:    cfg,
		  clock: func() time.Time { return time.Now().UTC() },
	  }
  }

// WithClock 注入时钟。
  func (lm *LeaseManager) WithClock(c func() time.Time) *LeaseManager {
	   if lm != nil && c != nil {
		   lm.clock = c
	   }
	   return lm
  }

   // IssueRatioLease 签发比率租约。
   // 若当前 DebtRatio 已超 MaxRatio，直接返回 ErrRatioOverLimit（不签发租约）。
   func (lm *LeaseManager) IssueRatioLease(ctx context.Context, userID, symbol, issuedBy string) (RatioLease, error) {
	   userID = strings.TrimSpace(userID)
	   symbol = strings.ToUpper(strings.TrimSpace(symbol))
	   if userID == "" || symbol == "" {
		  return RatioLease{}, fmt.Errorf("%w: userID and symbol required", ErrInvalidInput)
	   }
	   // 检查是否有未关闭的漂移案例
	   openDrifts, err := lm.store.ListOpenDriftCases(ctx, userID, symbol)
	   if err != nil {
		  return RatioLease{}, fmt.Errorf("isodebt: list drift cases: %w", err)
	   }
	   if len(openDrifts) > 0 {
		   return RatioLease{}, fmt.Errorf("%w: drift case %s exists", ErrDriftUnproven, openDrifts[0].ID)
	  }
	   // 读取当前世代
	  gen, err := lm.store.GetGeneration(ctx, userID, symbol)
	  if err != nil {
		  return RatioLease{}, fmt.Errorf("isodebt: get generation: %w", err)
	   }
	  // 超限检查
	   if gen.DebtRatio.GreaterThanOrEqual(lm.cfg.MaxDebtRatio) {
		  return RatioLease{}, fmt.Errorf("%w: ratio=%s max=%s",
			  ErrRatioOverLimit, gen.DebtRatio.StringFixed(6), lm.cfg.MaxDebtRatio.StringFixed(6))
	   }
	  now := lm.clock()
	   lease := RatioLease{
		   ID:         uuid.NewString(),
		   UserID:     userID,
		   Symbol:     symbol,
		   Generation: gen.Generation,
		  DebtRatio:  gen.DebtRatio,
		  MaxRatio: lm.cfg.MaxDebtRatio,
		   State:     LeaseStateFresh,
		   IssuedAt:  now,
		  ExpiresAt: now.Add(lm.cfg.LeaseTTL),
		  IssuedBy: issuedBy,
	  }
	   if err := lm.store.SaveLease(ctx, lease); err != nil {
		  return RatioLease{}, fmt.Errorf("isodebt: save lease: %w", err)
	   }
	  return lease, nil
  }

// ValidateLease 在 TransferIn 执行前校验租约。
// 若世代已推进（新 Credit/Debit 发生），返回 ErrGenerationMismatch。

  func (lm *LeaseManager) ValidateLease(ctx context.Context, leaseID string) (RatioLease, error) {
	   lease, err := lm.store.GetLease(ctx, leaseID)
	   if err != nil {
		   return RatioLease{}, fmt.Errorf("isodebt: get lease: %w", err)
	  }
	   now := lm.clock()
	   currentGen, err := lm.genMgr.CurrentGeneration(ctx, lease.UserID, lease.Symbol)
	   if err != nil {
		   return RatioLease{}, fmt.Errorf("isodebt: get current generation: %w", err)
	   }
	  if err := lease.IsValid(now, currentGen); err != nil {
		   // 将租约标记为 stale（如果还是 fresh 状态）
		   if lease.State == LeaseStateFresh {
			  t := now
			   lease.State = LeaseStateStale
			  lease.StaledAt = &t
			   _ = lm.store.SaveLease(ctx, lease)
		   }
		   return lease, err
	   }
	  // 再次检查漂移
	   openDrifts, err := lm.store.ListOpenDriftCases(ctx, lease.UserID, lease.Symbol)
	   if err != nil {
		   return lease, fmt.Errorf("isodebt: list drift cases: %w", err)
	   }
	   if len(openDrifts) > 0 {
		   return lease, fmt.Errorf("%w: drift case %s", ErrDriftUnproven, openDrifts[0].ID)
	  }
	  return lease, nil
  }

// StaleLease 主动将租约标记为 stale（外部世代推进时调用）。

  func (lm *LeaseManager) StaleLease(ctx context.Context, leaseID string) error {
	  lease,
		  err := lm.store.GetLease(ctx, leaseID)
	   if err != nil {
		   return fmt.Errorf("isodebt: get lease: %w", err)
	  }
	   if lease.State != LeaseStateFresh {
		   return nil // 已非 fresh，幂等
	   }
	  now := lm.clock()
	  lease.State = LeaseStateStale
	  lease.StaledAt = &now
	   return lm.store.SaveLease(ctx, lease)
   }

// BlockLease 将租约标记为 blocked（DebtRatio 超限时调用）。
   func (lm *LeaseManager) BlockLease(ctx context.Context, leaseID, reason string) error {
	  lease, err := lm.store.GetLease(ctx, leaseID)
	  if err != nil {
		   return fmt.Errorf("isodebt: get lease: %w", err)
	   }
	   if lease.State != LeaseStateFresh {
		  return nil // 已非 fresh，幂等
	   }
	  now := lm.clock()
	  lease.State = LeaseStateBlocked
	  lease.BlockedAt = &now
	  lease.BlockReason = reason
	   return lm.store.SaveLease(ctx, lease)
   }

// ExpireStaleLeases 对 Pair 执行批量过期：世代不符或超时的 fresh 租约 → stale。
  func (lm *LeaseManager) ExpireStaleLeases(ctx context.Context, userID, symbol string) (int64, error) {
	  currentGen, err := lm.genMgr.CurrentGeneration(ctx, userID, symbol)
	   if err != nil {
		  return 0, err
	   }
	  return lm.store.ExpireStaleLeases(ctx,
		  userID, symbol, currentGen, lm.clock())
  }
