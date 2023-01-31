package utamodegate

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

const (
	defaultBindTTLSeconds = 3600 // 默认绑定 TTL：1 小时
	maxBindTTLSeconds     = 7200 // 最大绑定 TTL：2 小时
)

// InflightBindGuard 管理借还/抵押操作的世代绑定与在途互斥。

type InflightBindGuard struct {
	store Store
	clock func() time.Time
}

  func newInflightBindGuard(store Store, clock func() time.Time) *InflightBindGuard {
	   return &InflightBindGuard{store: store, clock: clock}
  }

   // Register 为一次借出/抵押操作注册世代绑定（BindOpen）。
   // 调用方（借出/抵押 API）在落账前调用，确保操作被当前模式世代约束。
  // generation 参数来自调用方预先读取的 GenToken.Generation。
   // 幂等：同一 RefID 若已有 BindOpen，直接返回已有记录。
  func (g *InflightBindGuard) Register(ctx context.Context, req RegisterBindReq, generation int64) (*LiabilityBind, error) {
	  userID := strings.TrimSpace(req.UserID)
	   if userID == "" {
		  return nil, ErrUserInvalid
	   }
	   req.Currency = strings.ToUpper(strings.TrimSpace(req.Currency))
	  if req.Currency == "" {
		   return nil, ErrAssetInvalid
	   }
	  if req.MarginMode != MarginModeUnified && req.MarginMode != MarginModeClassic &&
		   req.MarginMode != MarginModeCross && req.MarginMode != MarginModeIsolated {
		  return nil, ErrModeInvalid
	  }
	  amount := strings.TrimSpace(req.Amount)
	  if amount == "" || amount == "0" {
		   return nil, ErrAmountInvalid
	   }
	   // 幂等：同一 RefID 已有活跃绑定，直接返回。
	   if req.RefID != "" {
		   existing, err := g.store.GetBindByRef(ctx, userID, req.RefID)
		  if err == nil && existing != nil && existing.Status == BindOpen {
			  return existing, nil
		  }
	   }
	  ttl := req.TTLSeconds
	  if ttl <= 0 {
		   ttl = defaultBindTTLSeconds
	   }
	  if ttl > maxBindTTLSeconds {
		   ttl = maxBindTTLSeconds
	  }
	  now := g.clock()
	  bind := &LiabilityBind{
		   ID:     uuid.NewString(),
		   UserID: userID,
		   BindKind: req.BindKind,
		   Currency: req.Currency,
		  MarginMode: req.MarginMode,
		  Amount:     amount,
		  Generation: generation,
		  RefID:     req.RefID,
		   Status:    BindOpen,
		  ExpiresAt: now.Add(time.Duration(ttl) * time.Second),
		   CreatedAt: now,
		  UpdatedAt: now,
	   }
	  if err := g.store.SaveBind(ctx, bind); err != nil {
		   return nil, err
	   }
	  return bind, nil
   }

// Release 将 BindOpen 推进至 BindReleased（还款/抵押释放时调用）。
// 支持通过 BindID 或 RefID 定位绑定。
// 幂等：已 released 的绑定直接返回。
   func (g *InflightBindGuard) Release(ctx context.Context, req ReleaseBindReq) (*LiabilityBind, error) {
	  userID := strings.TrimSpace(req.UserID)
	  if userID == "" {
		  return nil, ErrUserInvalid
	   }
	   var bind *LiabilityBind
	   var err error
	  if strings.TrimSpace(req.BindID) != "" {
		   bind, err = g.store.GetBind(ctx, req.BindID)
	  } else if strings.TrimSpace(req.RefID) != "" {
		  bind, err = g.store.GetBindByRef(ctx, userID, req.RefID)
	   } else {
		   return nil, fmt.Errorf("%w: bind_id or ref_id required", ErrBindNotFound)
	  }
	  if err != nil {
		  return nil, err
	  }
	   if bind.UserID != userID {
		   return nil, ErrBindNotFound // 跨用户保护
	  }
	  if bind.Status == BindReleased {
		  return bind, nil // 幂等
	  }
	   bind.Status = BindReleased
	  bind.UpdatedAt = g.clock()
	  if err := g.store.SaveBind(ctx, bind); err != nil {
		  return nil, err
	   }
	  return bind, nil
  }

// CountOpenBinds 返回用户当前 open 状态的 LiabilityBind 数量。
// 用于 LeaseFSM.Acquire 的在途互斥检查。
  func (g *InflightBindGuard) CountOpenBinds(ctx context.Context, userID string) (int, error) {
	  binds, err := g.store.ListOpenBindsByUser(ctx, userID)
	  if err != nil {
		  return 0, err
	  }
	   return len(binds), nil
  }

// ListOpenBinds 返回用户所有 BindOpen 记录（含完整字段）。
// 用于漂移巡检：检查各绑定的 MarginMode 是否与当前世代模式一致。
   func (g *InflightBindGuard) ListOpenBinds(ctx context.Context, userID string) ([]*LiabilityBind, error) {
	  return g.store.ListOpenBindsByUser(ctx, userID)
   }

// MarkExpired 将已过期的 BindOpen 标记为 BindExpired（供 worker 调用）。
// 返回被标记的绑定列表，供 worker 触发漂移告警。
  func (g *InflightBindGuard) MarkExpired(ctx context.Context, batchSize int) ([]*LiabilityBind, error) {
	   now := g.clock()
	  expired,
		   err := g.store.ListExpiredOpenBinds(ctx, now, batchSize)
	   if err != nil {
		  return nil, err
	  }
	   for _, b := range expired {
		  b.Status = BindExpired
		   b.UpdatedAt = now
		  _ = g.store.SaveBind(ctx, b)
	  }
	   return expired, nil
   }

// GetBind 查询单条绑定（ID 精确查询）。
   func (g *InflightBindGuard) GetBind(ctx context.Context, bindID string) (*LiabilityBind, error) {
	   return g.store.GetBind(ctx, bindID)
  }
