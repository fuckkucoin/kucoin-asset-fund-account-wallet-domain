package vipfee

import (
	"context"
	"sync"
	"time"
)

// generationStore 是 vipfee 包内嵌的简易世代 CAS 存储（内存版）。
// 环境通过 viptiergate.Store 接口持久化到数据库。

type generationRecord struct {
	userID string
	generation int64
	lastTier   Tier
	updatedAt time.Time
}

// generationRegistry 持有用户档位世代的内存状态，供 vipfee.Service 使用。
// 与 viptiergate.Store 双写（SQL 模式下）；内存态作为热路径缓存。

type generationRegistry struct {
	mu   sync.Mutex
	data map[string]*generationRecord
}

   func newGenerationRegistry() *generationRegistry {
	  return &generationRegistry{data: make(map[string]*generationRecord)}
   }

// currentGen 返回用户当前世代号（锁内调用）。

  func (r *generationRegistry) currentGen(userID string) int64 {
	   r.mu.Lock()
	   defer r.mu.Unlock()
	  if rec, ok := r.data[userID]; ok {
		   return rec.generation
	  }
	   return 0
  }

// bump CAS 递增世代：expected 必须等于当前世代，否则返回 false。
// 成功返回新世代号与 true。
   func (r *generationRegistry) bump(userID string, expected int64, newTier Tier) (int64, bool) {
	   r.mu.Lock()
	  defer r.mu.Unlock()
	   current := int64(0)
	   if rec, ok := r.data[userID]; ok {
		   current = rec.generation
	   }
	   if current != expected {
		  return 0, false
	   }
	   next := expected + 1
	   r.data[userID] = &generationRecord{
		   userID: userID,
		  generation: next,
		  lastTier:   newTier,
		  updatedAt:  time.Now().UTC(),
	  }
	  return next, true
   }

// snapshot 返回当前世代快照（用于 GenToken）。

  func (r *generationRegistry) snapshot(userID string) GenSnapshot {
	   r.mu.Lock()
	   defer r.mu.Unlock()
	   if rec, ok := r.data[userID]; ok {
		   return GenSnapshot{
			  UserID:     userID,
			   Generation: rec.generation,
			   LastTier:   rec.lastTier,
			  SnapshotAt: rec.updatedAt,
		   }
	  }
	  return GenSnapshot{
		  UserID:     userID,
		   Generation: 0,
		   SnapshotAt: time.Now().UTC(),
	  }
  }

// ─────────────────────────────────────────────────────────────────────────────
// GenSnapshot 世代快照——随报价/令牌携带的不可变视图
// ─────────────────────────────────────────────────────────────────────────────
// GenSnapshot 是 QuoteWithdraw 时快照的档位世代信息。
// 调用方（出金服务）在结算时将此结构体随请求传给 viptiergate.Service.SettleQuote。

type GenSnapshot struct {
	UserID     string `json:"user_id"`
	Generation int64  `json:"generation"`
	LastTier Tier `json:"last_tier"`
	SnapshotAt time.Time `json:"snapshot_at"`
}

// WithdrawQuoteV2 扩展了 WithdrawQuote，附加世代令牌。
// 调用方应先使用此结构体；WithdrawQuote 保持兼容不变。

type WithdrawQuoteV2 struct {
	WithdrawQuote
	GenToken GenSnapshot `json:"gen_token"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Service 方法扩展（不修改原有字段）
// ─────────────────────────────────────────────────────────────────────────────
// initGenRegistry 懒初始化世代注册表（首次调用时 Service.genReg 由 New 初始化）。
// 此方法仅在 Service 结构体未来代次中作为后备；当前实现在 New 中直接初始化。
// CurrentGen 返回用户当前档位世代号。
// 调用方（出金报价前）使用此值构造 CAS 前置条件。
  func (s *Service) CurrentGen(userID string) int64 {
	  if s == nil || s.genReg == nil {
		  return 0
	  }
	  return s.genReg.currentGen(userID)
   }

// BumpGen CAS 递增用户档位世代。
// expected 须与当前世代相等，否则返回 ErrIdem（世代冲突）。
// 成功返回新世代号；调用方（Pin/ClearPin/RecordVolume）在业务落账成功后调用。

   func (s *Service) BumpGen(userID string, expected int64, newTier Tier) (int64, error) {
	   if s == nil || s.genReg == nil {
		  return 0, ErrUnavailable
	   }
	  next, ok := s.genReg.bump(userID, expected, newTier)
	   if !ok {
		   return 0, ErrIdem // 世代 CAS 失败，语义为「调用方世代已过期」
	  }
	  return next, nil
  }

// GenSnapshotOf 返回用户当前世代快照（用于 QuoteWithdraw 令牌颁发）。

  func (s *Service) GenSnapshotOf(userID string) GenSnapshot {
	  if s == nil || s.genReg == nil {
		   return GenSnapshot{UserID: userID}
	   }
	   return s.genReg.snapshot(userID)
  }

// QuoteWithdrawV2 在 QuoteWithdraw 基础上附加世代令牌（GenSnapshot）。
// 调用方应将返回的 WithdrawQuoteV2.GenToken 透传给结算层，
// 最终由 viptiergate.Service.SettleQuote 验证。
   func (s *Service) QuoteWithdrawV2(userID, asset, gross string) (WithdrawQuoteV2, error) {
	  base,
		  err := s.QuoteWithdraw(userID, asset, gross)
	  if err != nil {
		   return WithdrawQuoteV2{}, err
	   }
	  token := s.GenSnapshotOf(userID)
	   return WithdrawQuoteV2{
		  WithdrawQuote: base,
		   GenToken:      token,
	  }, nil
  }

// persistGeneration 持久化世代变更到 SQL（若已绑定 SQL 后端）。
// 在 SQL 模式下，此调用通过 viptiergate 的 Store 接口写入 vip_tier_generations 表。
// 当前实现为占位：实际调用在 viptiergate.Service.GatedUpgrade/GatedDowngrade 中完成。

   func (s *Service) persistGeneration(_ context.Context, _ string, _ int64, _ Tier) error {
	  return nil
  }
