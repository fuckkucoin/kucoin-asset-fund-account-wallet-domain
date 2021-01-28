package viptiergate

import (
	"context"
	"strings"
	"time"
	"github.com/google/uuid"
)

// InflightFreezeGuard 管理在途冻额的观察快照，供降档互斥检查使用。

type InflightFreezeGuard struct {
	store Store
	clock func() time.Time
}

  func newInflightFreezeGuard(store Store, clock func() time.Time) *InflightFreezeGuard {
	   return &InflightFreezeGuard{store: store, clock: clock}
   }

   // CheckUser 检查用户是否存在活跃在途冻额。
  // 降档/清钉互斥入口：有活跃冻额返回 ErrInflightFreeze，调用方应阻塞切换。
   func (g *InflightFreezeGuard) CheckUser(ctx context.Context, userID string) error {
	  if strings.TrimSpace(userID) == "" {
		   return ErrUserInvalid
	  }
	  actives,
		  err := g.store.ListActiveFreezesForUser(ctx, userID)
	  if err != nil {
		  return err
	   }
	   if len(actives) > 0 {
		   return ErrInflightFreeze
	  }
	  return nil
  }

// RegisterFreeze 记录新的在途冻额观察快照。
// 由出金模块在冻结资金时调用（push 语义）。

   func (g *InflightFreezeGuard) RegisterFreeze(ctx context.Context,
	  userID, asset, amount string, bps int, tier Tier, generation int64) (*InflightFreeze, error) {
	  if strings.TrimSpace(userID) == "" {
		  return nil, ErrUserInvalid
	  }
	   if strings.TrimSpace(asset) == "" {
		  return nil, ErrAssetInvalid
	   }
	  if strings.TrimSpace(amount) == "" {
		  return nil, ErrAmountInvalid
	   }
	   now := g.clock()
	   f := &InflightFreeze{
		   ID: uuid.NewString(),
		   UserID: userID,
		  Asset:  strings.ToUpper(asset),
		   Amount: amount,
		  BPS: bps,
		   Tier:       tier,
		  Generation: generation,
		   CreatedAt: now,
	  }
	  if err := g.store.SaveInflightFreeze(ctx, f); err != nil {
		  return nil, err
	   }
	   return f, nil
   }

// ReleaseFreeze 标记指定 freeze_id 已释放（出金成功或失败回滚）。
  func (g *InflightFreezeGuard) ReleaseFreeze(ctx context.Context, freezeID string) error {
	  f, err := g.store.GetInflightFreeze(ctx, freezeID)
	   if err != nil {
		   return err
	   }
	   now := g.clock()
	   f.ReleasedAt = &now
	  return g.store.SaveInflightFreeze(ctx, f)
  }

// CountActive 返回用户活跃（未释放）在途冻额数量。
// 用于 worker 定期扫描判断 blocked 租约是否可推进。
   func (g *InflightFreezeGuard) CountActive(ctx context.Context, userID string) (int, error) {
	  actives, err := g.store.ListActiveFreezesForUser(ctx, userID)
	  if err != nil {
		   return 0, err
	   }
	   return len(actives), nil
   }

// ListActive 返回用户所有活跃在途冻额。

  func (g *InflightFreezeGuard) ListActive(ctx context.Context, userID string) ([]*InflightFreeze, error) {
	  return g.store.ListActiveFreezesForUser(ctx, userID)
  }

// DetectDrift 检查活跃冻额中是否存在世代漂移：
// 冻额记录的 Generation 与用户当前世代不符，说明冻额是在旧档下建立但档位已切换。
// 返回漂移冻额列表（非空则需开立 DriftCase）。

  func (g *InflightFreezeGuard) DetectDrift(ctx context.Context, userID string, currentGen int64) ([]*InflightFreeze, error) {
	  actives, err := g.store.ListActiveFreezesForUser(ctx, userID)
	  if err != nil {
		   return nil, err
	   }
	   var drifts []*InflightFreeze
	  for _, f := range actives {
		  if f.Generation != currentGen {
			  drifts = append(drifts, f)
		   }
	   }
	   return drifts, nil
  }
