package restoregate

import (
	"context"
	"errors"
	"strings"
	"time"
	"github.com/google/uuid"
)

// HalfSealManager 半恢复封印管理器。
// Import 中途失败时：
// 1. 调用 Seal(planID, writtenKeys, failReason) 创建封印记录
// 2. 后续对同一 planID 的 Import 尝试必须先调用 CheckResumable 判断是否需要续/中止
// 3. 显式调用 Resume(planID) 后可重 Import（Resume 会标记 halfSeal.Resumed=true）
// 隔离：HalfSeal 只记录本次 Import 写入的 kind|key 键集合；
// 不执行回滚，回滚由调用方负责（Import 失败后 KV 层应能做幂等清理）。

type HalfSealManager struct {
	store PlanStore
	clock func() time.Time
}

// NewHalfSealManager 构造封印管理器。

  func NewHalfSealManager(store PlanStore) *HalfSealManager {
	  return &HalfSealManager{
		  store: store,
		   clock: func() time.Time { return time.Now().UTC() },
	   }
   }

// WithClock 注入时钟（用）。
  func (h *HalfSealManager) WithClock(c func() time.Time) *HalfSealManager {
	  h.clock = c
	   return h
   }

  // Seal 创建半恢复封印记录。
  // writtenKeys 形态为 "kind|key"，即本次 Import 已成功 PutNew 的键列表。
   // 若同一 planID 已有封印记录（未续），返回 ErrHalfSealExists。

   func (h *HalfSealManager) Seal(ctx context.Context, planID string, writtenKeys []string, failReason string) error {
	   if h == nil || h.store == nil {
		   return ErrUnavailable
	  }
	  planID = strings.TrimSpace(planID)
	   if planID == "" {
		  return ErrEmpty
	   }
	   // 检查是否已有封印（未续）
	   existing, err := h.store.GetHalfSeal(ctx, planID)
	   if err == nil && !existing.Resumed {
		   return ErrHalfSealExists
	  }
	   now := h.clock()
	  keys := make([]string, len(writtenKeys))
	   copy(keys, writtenKeys)
	  hs := HalfSeal{
		  ID: uuid.New().String(),
		  PlanID: planID,
		  WrittenKeys: keys,
		   FailedAt: now,
		  FailReason: strings.TrimSpace(failReason),
		  Resumed:    false,
		   CreatedAt: now,
		   UpdatedAt: now,
	  }
	  return h.store.CreateHalfSeal(ctx,
		   hs)
   }

// CheckResumable 检查当前封印状态，决定是否允许重 Import。
// 返回 nil → 无封印或已续，可 Import。
// 返回 ErrHalfSealExists → 封印存在且未续，必须先显式 Resume 或 Abort。

   func (h *HalfSealManager) CheckResumable(ctx context.Context, planID string) error {
	  if h == nil || h.store == nil {
		   return nil
	   }
	   hs, err := h.store.GetHalfSeal(ctx, planID)
	  if errors.Is(err, ErrHalfSealNotFound) {
		  return nil // 无封印，可接着
	   }
	  if err != nil {
		   return err
	   }
	  if !hs.Resumed {
		   return ErrHalfSealExists
	  }
	  return nil
  }

// Resume 显式续封印（允许重 Import）。
// 调用后 HalfSeal.Resumed=true；同一封印只可续一次（设计上 plan 也应先 abort 再重 plan）。

  func (h *HalfSealManager) Resume(ctx context.Context, planID string) error {
	   if h == nil || h.store == nil {
		  return ErrUnavailable
	   }
	   hs, err := h.store.GetHalfSeal(ctx, planID)
	   if err != nil {
		   return err
	  }
	   if hs.Resumed {
		   return nil // 幂等
	   }
	   now := h.clock()
	   hs.Resumed = true
	  hs.ResumedAt = &now
	  hs.UpdatedAt = now
	  return h.store.UpdateHalfSeal(ctx, hs)
   }

// GetSeal 查询封印记录。
  func (h *HalfSealManager) GetSeal(ctx context.Context, planID string) (HalfSeal, error) {
	   if h == nil || h.store == nil {
		  return HalfSeal{}, ErrUnavailable
	   }
	  return h.store.GetHalfSeal(ctx, planID)
   }

// WrittenKeyCount 返回封印中已写键数量；无封印返回 0。

   func (h *HalfSealManager) WrittenKeyCount(ctx context.Context, planID string) int {
	   if h == nil || h.store == nil {
		   return 0
	  }
	  hs, err := h.store.GetHalfSeal(ctx, planID)
	  if err != nil {
		  return 0
	   }
	   return len(hs.WrittenKeys)
   }
