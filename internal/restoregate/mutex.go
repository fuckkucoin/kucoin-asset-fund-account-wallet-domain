package restoregate

import (
	"context"
	"errors"
	"strings"
	"time"
	"github.com/google/uuid"
)

const defaultMutexTTL = 30 * time.Minute

// WriteGuard 活账写互斥锁管理器。
// 回灌进入 freezing_writes 时调用 Acquire；sealed/aborted 后调用 Release。
// 活锁期间，贷记/冻结/划转意图必须先调用 CheckWrite，若被阻断则调用 RecordBlocked。
type WriteGuard struct {
	store PlanStore
	clock func() time.Time
	ttl time.Duration
}

// NewWriteGuard 构造写互斥锁管理器。
  func NewWriteGuard(store PlanStore) *WriteGuard {
	   return &WriteGuard{
		   store: store,
		   clock: func() time.Time { return time.Now().UTC() },
		  ttl:   defaultMutexTTL,
	   }
   }

// WithTTL 设置锁超时时长（看门狗用）。
  func (w *WriteGuard) WithTTL(d time.Duration) *WriteGuard {
	  w.ttl = d
	   return w
   }

// WithClock 注入时钟（用）。

  func (w *WriteGuard) WithClock(c func() time.Time) *WriteGuard {
	   w.clock = c
	  return w
   }

   // Acquire 为指定 plan 申请全局写互斥锁。
  // 若已有其他 plan 持锁且未过期，返回 ErrMutexConflict。
  // 自己已持锁则幂等返回现有锁。
   func (w *WriteGuard) Acquire(ctx context.Context, planID string, holderGen int64) (WriteMutex, error) {
	   if w == nil || w.store == nil {
		  return WriteMutex{}, ErrUnavailable
	   }
	  planID = strings.TrimSpace(planID)
	  if planID == "" {
		  return WriteMutex{}, ErrEmpty
	  }
	  // 检查是否自己已持锁（幂等）
	  existing,
		  err := w.store.GetMutexByPlan(ctx, planID)
	  if err == nil && existing.State == MutexActive {
		   return existing, nil
	  }
	  // 检查是否有其他 plan 持锁
	   active, err := w.store.GetActiveMutex(ctx)
	  if err == nil {
		  // 活锁未过期时冲突
		  if w.clock().Before(active.ExpiresAt) {
			  return WriteMutex{}, ErrMutexConflict
		  }
		   // 活锁已过期，标为 expired
		  active.State = MutexExpired
		   active.UpdatedAt = w.clock()
		  _ = w.store.UpdateMutex(ctx,
			   active)
	  }
	   now := w.clock()
	   m := WriteMutex{
		   ID: uuid.New().String(),
		   PlanID:    planID,
		   State:     MutexActive,
		   HolderGen: holderGen,
		   ExpiresAt:  now.Add(w.ttl),
		  AcquiredAt: now,
		  CreatedAt:  now,
		   UpdatedAt: now,
	  }
	  if err := w.store.CreateWriteMutex(ctx, m); err != nil {
		  return WriteMutex{}, err
	   }
	   return m, nil
   }

// Release 释放写互斥锁（sealed / aborted 时调用）。
   func (w *WriteGuard) Release(ctx context.Context, planID string) error {
	   if w == nil || w.store == nil {
		   return ErrUnavailable
	  }
	   m,
		   err := w.store.GetMutexByPlan(ctx, planID)
	   if err != nil {
		  if errors.Is(err, ErrMutexNotFound) {
			   return nil // 幂等
		  }
		  return err
	  }
	   if m.State != MutexActive {
		  return nil // 已释放，幂等
	  }
	   now := w.clock()
	   m.State = MutexReleased
	   m.ReleasedAt = &now
	  m.UpdatedAt = now
	   return w.store.UpdateMutex(ctx, m)
   }

// CheckWrite 检查当前是否有活跃锁阻断写路径。
// 返回 nil 表示可写；返回 ErrWriteFrozen 表示写被冻结。
   func (w *WriteGuard) CheckWrite(ctx context.Context) error {
	  if w == nil || w.store == nil {
		   return nil // store 不可用时放行，不阻断正常路径
	  }
	   active, err := w.store.GetActiveMutex(ctx)
	  if err != nil {
		   if errors.Is(err, ErrMutexNotFound) {
			   return nil // 无活锁，放行
		   }
		  return nil // 查询失败时放行（fail-open 保守策略，防止 store 故障阻断所有写）
	  }
	  if w.clock().After(active.ExpiresAt) {
		  // 锁已过期，自动标 expired 并放行
		   active.State = MutexExpired
		  active.UpdatedAt = w.clock()
		  _ = w.store.UpdateMutex(ctx, active)
		   return nil
	  }
	  return ErrWriteFrozen
   }

// RecordBlocked 记录被阻断的写意图审计日志。
   func (w *WriteGuard) RecordBlocked(ctx context.Context, planID, kind, userID, asset, amount, reason string) error {
	  if w == nil || w.store == nil {
		   return nil
	  }
	  m, err := w.store.GetMutexByPlan(ctx, planID)
	   if err != nil {
		  return nil // 找不到锁记录时不报错，避免影响主链路
	   }
	   bw := BlockedWrite{
		  ID:      uuid.New().String(),
		  MutexID: m.ID,
		   PlanID: planID,
		   Kind:   kind,
		   UserID:    userID,
		  Asset:     asset,
		   Amount:    amount,
		  Reason:    reason,
		  CreatedAt: w.clock(),
	  }
	  return w.store.AppendBlockedWrite(ctx, bw)
  }

// RenewLease 续租锁（看门狗调用，防止长回灌超时）。

  func (w *WriteGuard) RenewLease(ctx context.Context, planID string) error {
	   if w == nil || w.store == nil {
		  return ErrUnavailable
	   }
	  m, err := w.store.GetMutexByPlan(ctx, planID)
	   if err != nil {
		   return err
	  }
	  if m.State != MutexActive {
		  return ErrMutexNotFound
	  }
	  now := w.clock()
	  m.ExpiresAt = now.Add(w.ttl)
	  m.UpdatedAt = now
	   return w.store.UpdateMutex(ctx, m)
   }

// ActivePlanID 返回持锁的 plan ID；无活锁返回空串。
   func (w *WriteGuard) ActivePlanID(ctx context.Context) string {
	  if w == nil || w.store == nil {
		   return ""
	  }
	   m,
		   err := w.store.GetActiveMutex(ctx)
	  if err != nil {
		   return ""
	  }
	  if w.clock().After(m.ExpiresAt) {
		   return ""
	  }
	   return m.PlanID
   }
