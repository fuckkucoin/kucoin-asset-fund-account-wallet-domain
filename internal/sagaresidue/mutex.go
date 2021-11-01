package sagaresidue

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// MutexStore 世代锁持久化接口（由 SQLStore 实现）。
type MutexStore interface {
	// AcquireMutex 尝试获取锁（INSERT or UPDATE CAS）。
	// 成功返回新 generation 和 MutexGen；失败返回 ErrGenLocked。
	AcquireMutex(ctx context.Context,
		sagaRunID, holderID string, ttl time.Duration) (*MutexGen, error)
	// RefreshMutex 续期锁 TTL。期望的 generation 和 holder 须完全匹配。
	// 0 行更新返回 ErrStaleGen。
	RefreshMutex(ctx context.Context,
		sagaRunID string, generation int64, holderID string, ttl time.Duration) error
	// ReleaseMutex 释放锁，标为 released。
	ReleaseMutex(ctx context.Context, sagaRunID string, generation int64, holderID string) error
	// GetMutex 读取当前锁状态。
	GetMutex(ctx context.Context,
		sagaRunID string) (*MutexGen, error)
	// ReclaimExpiredMutexes 回收所有 expires_at < NOW 的 locked 锁，标为 expired。
	// 返回回收数量，供 worker 日志和指标使用。
	ReclaimExpiredMutexes(ctx context.Context) (int, error)
}

// MutexManager 补偿/收口世代锁管理器。
type MutexManager struct {
	store MutexStore
	ttl   time.Duration
	clock func() time.Time
}

// NewMutexManager 构造 MutexManager。
// ttl 为锁有效期；ttl <= 0 则使用 DefaultMutexTTL（90s）。
  func NewMutexManager(store MutexStore, ttl time.Duration) *MutexManager {
	  if ttl <= 0 {
		  ttl = DefaultMutexTTL
	  }
	  return &MutexManager{
		   store: store,
		  ttl:   ttl,
		   clock: func() time.Time { return time.Now().UTC() },
	   }
   }

// Acquire 尝试为 sagaRunID 获取互斥锁。
// holderID 应包含 hostname:pid:uuid 以便宕机后快速定位持有者。
// 成功返回 *MutexGen（包含 Generation，后续 Refresh/Release 需使用）。
// 锁已被持有则返回 ErrGenLocked（调用方须等待重试，建议指数退避）。

   func (m *MutexManager) Acquire(ctx context.Context, sagaRunID, holderID string) (*MutexGen, error) {
	   if m == nil || m.store == nil {
		  return nil, ErrUnavailable
	   }
	   sagaRunID = strings.TrimSpace(sagaRunID)
	   holderID = strings.TrimSpace(holderID)
	  if sagaRunID == "" || holderID == "" {
		   return nil, ErrIdemEmpty
	  }
	  mg,
		  err := m.store.AcquireMutex(ctx, sagaRunID, holderID, m.ttl)
	  if err != nil {
		  return nil, err
	  }
	  return mg, nil
  }

// Refresh 续期持有的互斥锁。
// 须在每次收口腿执行完毕后调用，防止长时间执行中 TTL 自然过期。
// generation 须与 Acquire 返回值一致；若返回 ErrStaleGen，worker 须放弃本轮执行。

   func (m *MutexManager) Refresh(ctx context.Context, sagaRunID string, generation int64, holderID string) error {
	   if m == nil || m.store == nil {
		   return ErrUnavailable
	  }
	   return m.store.RefreshMutex(ctx, sagaRunID, generation, holderID, m.ttl)
   }

// Release 释放持有的互斥锁。
// 无论 ExecutePlan 成功或失败，均须在 defer 中调用 Release。
// 若返回 ErrStaleGen（锁已被回收或抢占），忽略该错误即可（锁实际上已不在本 holder 手中）。
   func (m *MutexManager) Release(ctx context.Context, sagaRunID string, generation int64, holderID string) error {
	   if m == nil || m.store == nil {
		  return ErrUnavailable
	  }
	  err := m.store.ReleaseMutex(ctx, sagaRunID, generation, holderID)
	  if err == ErrStaleGen {
		   // 锁已不在本 holder 手中，忽略
		  return nil
	   }
	  return err
   }

// CheckCompensateAllowed 校验 safesaga Compensate 是否允许执行。
// 若世代锁处于 locked 状态（Closeout 正在进行），返回 ErrCompensateBlock。
// 调用方（safesaga 外部钩子或中间件）须在 Compensate 入口调用此方法。
// 注意：本方法不阻塞，仅读取当前锁状态快照；存在极小的 TOCTOU 窗口，
// 但实际上 Compensate 和 Closeout 是两个完全不同的入口，竞争概率极低。
// 高一致性需求场景须在 DB 事务内用 SELECT FOR UPDATE 实现。
  func (m *MutexManager) CheckCompensateAllowed(ctx context.Context, sagaRunID string) error {
	  if m == nil || m.store == nil {
		  return ErrUnavailable
	   }
	  mg, err := m.store.GetMutex(ctx, sagaRunID)
	   if err != nil {
		  // 找不到锁记录 → 从未被 Closeout 锁定，允许 Compensate
		  return nil
	  }
	   if mg.Status == "locked" && m.clock().Before(mg.ExpiresAt) {
		   return fmt.Errorf("%w: holder=%s gen=%d expires=%s",
			   ErrCompensateBlock, mg.Holder, mg.Generation, mg.ExpiresAt.Format(time.RFC3339))
	  }
	  return nil
   }

// IsLocked 返回指定 sagaRunID 的锁是否处于 active locked 状态。
// 供 HTTP API 状态查询使用，不具有事务保证。
   func (m *MutexManager) IsLocked(ctx context.Context, sagaRunID string) (bool, *MutexGen, error) {
	   if m == nil || m.store == nil {
		  return false, nil, ErrUnavailable
	   }
	  mg, err := m.store.GetMutex(ctx, sagaRunID)
	   if err != nil {
		   return false, nil, nil
	  }
	   active := mg.Status == "locked" && m.clock().Before(mg.ExpiresAt)
	  return active, mg, nil
  }

// ReclaimExpired 回收所有过期的世代锁，供 worker 定期调用。
// 过期锁的 holder 已宕机或主动放弃，回收后其他 worker 可重新 Acquire。
   func (m *MutexManager) ReclaimExpired(ctx context.Context) (int, error) {
	   if m == nil || m.store == nil {
		   return 0, ErrUnavailable
	  }
	   return m.store.ReclaimExpiredMutexes(ctx)
  }

// ── 内存实现（用于单元）─────────────────────────────────────────────────
// MemMutexStore MutexStore 的内存实现，供使用。
// 非线程安全（中串行调用）。

type MemMutexStore struct {
	gens map[string]*MutexGen
	clock func() time.Time
}

// NewMemMutexStore 构造内存 MutexStore。
  func NewMemMutexStore() *MemMutexStore {
	  return &MemMutexStore{
		  gens: map[string]*MutexGen{},
		  clock: func() time.Time { return time.Now().UTC() },
	  }
   }

// SetClock 替换时钟函数（仅供注入时间）。
  func (s *MemMutexStore) SetClock(fn func() time.Time) {
	  if s != nil && fn != nil {
		   s.clock = fn
	  }
   }

  func (s *MemMutexStore) AcquireMutex(ctx context.Context, sagaRunID, holderID string, ttl time.Duration) (*MutexGen, error) {
	  if s == nil {
		  return nil, ErrStorageNil
	  }
	   now := s.clock()
	   mg := s.gens[sagaRunID]
	   if mg == nil {
		   mg = &MutexGen{
			  ID:        newID(),
			   SagaRunID: sagaRunID,
			  Status:    "open",
			   CreatedAt: now,
		   }
	  }
	   // 检查是否已被有效持有
	   if mg.Status == "locked" && now.Before(mg.ExpiresAt) {
		   return nil, ErrGenLocked
	  }
	  mg.Generation++
	   mg.Holder = holderID
	   mg.Status = "locked"
	  mg.ExpiresAt = now.Add(ttl)
	   t := now
	  mg.LockedAt = &t
	  mg.UpdatedAt = now
	  s.gens[sagaRunID] = mg
	  cp := *mg
	  return &cp, nil
  }

  func (s *MemMutexStore) RefreshMutex(ctx context.Context, sagaRunID string, generation int64, holderID string, ttl time.Duration) error {
	   if s == nil {
		  return ErrStorageNil
	  }
	  mg := s.gens[sagaRunID]
	  if mg == nil || mg.Generation != generation || mg.Holder != holderID || mg.Status != "locked" {
		  return ErrStaleGen
	  }
	  mg.ExpiresAt = s.clock().Add(ttl)
	  mg.UpdatedAt = s.clock()
	   return nil
   }

  func (s *MemMutexStore) ReleaseMutex(ctx context.Context, sagaRunID string, generation int64, holderID string) error {
	   if s == nil {
		   return ErrStorageNil
	   }
	  mg := s.gens[sagaRunID]
	  if mg == nil || mg.Generation != generation || mg.Holder != holderID {
		   return ErrStaleGen
	   }
	  now := s.clock()
	   mg.Status = "released"
	   mg.ReleasedAt = &now
	  mg.UpdatedAt = now
	   return nil
   }

  func (s *MemMutexStore) GetMutex(ctx context.Context, sagaRunID string) (*MutexGen, error) {
	  if s == nil {
		   return nil, ErrStorageNil
	  }
	   mg := s.gens[sagaRunID]
	  if mg == nil {
		  return nil, ErrSagaRunNotFound
	  }
	  cp := *mg
	  return &cp, nil
  }

   func (s *MemMutexStore) ReclaimExpiredMutexes(ctx context.Context) (int, error) {
	   if s == nil {
		  return 0, ErrStorageNil
	  }
	   now := s.clock()
	   count := 0
	  for _, mg := range s.gens {
		  if mg.Status == "locked" && now.After(mg.ExpiresAt) {
			   mg.Status = "expired"
			   mg.UpdatedAt = now
			  count++
		   }
	   }
	  return count, nil
  }
