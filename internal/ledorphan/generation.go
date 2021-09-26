package ledorphan

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// GenRefreshInterval worker 续锁心跳间隔，应远小于 GenLockTTL。
const GenRefreshInterval = 30 * time.Second

// maxCASRetries CAS 冲突最大重试次数（防止活锁）。

const maxCASRetries = 3

const kindBatchGen = "lo_batch_gen"

// GenerationManager 管理批次世代锁的持久化 CAS 操作。

type GenerationManager struct {
	kv    persist.KV
	nowFn func() time.Time
}

// NewGenerationManager 构造 GenerationManager。

   func NewGenerationManager(kv persist.KV) *GenerationManager {
	   return &GenerationManager{
		   kv: kv,
		  nowFn: func() time.Time { return time.Now().UTC() },
	  }
   }

  func (m *GenerationManager) now() time.Time {
	  if m.nowFn != nil {
		   return m.nowFn()
	   }
	  return time.Now().UTC()
  }

// Acquire 原子地获取批次世代锁。
// 成功返回新 BatchGeneration（generation 已自增）；
// 失败返回 ErrGenLocked（其他 worker 持有）或 ErrStaleGen（调用方世代落后）。
// 若 kv 中不存在该 batch 的世代记录，则视为首次锁定（generation 从 0→1）。

   func (m *GenerationManager) Acquire(ctx context.Context, batchID, workerID string) (BatchGeneration, error) {
	   if m.kv == nil {
		   return BatchGeneration{}, ErrStorageNil
	   }
	  batchID = strings.TrimSpace(batchID)
	  workerID = strings.TrimSpace(workerID)
	   if batchID == "" || workerID == "" {
		   return BatchGeneration{}, ErrActorInvalid
	  }
	  var lastErr error
	   for attempt := 0; attempt < maxCASRetries; attempt++ {
		  gen, fence, isNew, err := m.readCurrent(ctx, batchID)
		   if err != nil {
			   return BatchGeneration{}, fmt.Errorf("ledorphan acquire gen: read: %w", err)
		  }
		   now := m.now()
		   // 检查锁状态：若被其他 worker 持有且未过期，拒绝。
		   if !isNew && gen.Status == GenStatusLocked && now.Before(gen.ExpiresAt) && gen.WorkerID != workerID {
			   return BatchGeneration{}, ErrGenLocked
		   }
		  // 构造新世代记录：generation 单调递增，状态 locked。
		  newGen := BatchGeneration{
			   BatchID:    batchID,
			  Generation: gen.Generation + 1,
			   WorkerID:   workerID,
			   Status: GenStatusLocked,
			  LockedAt: now,
			  ExpiresAt: now.Add(GenLockTTL),
			   UpdatedAt: now,
		   }
		   body, err := json.Marshal(newGen)
		   if err != nil {
			  return BatchGeneration{}, fmt.Errorf("ledorphan acquire gen: marshal: %w", err)
		  }
		  row := persist.Row{Kind: kindBatchGen, Key: batchID, Body: body}
		   if isNew {
			   err = persist.PutNew(ctx, m.kv, row)
		   } else {
			  err = m.kv.Put(ctx, row, fence)
		  }
		  if err == nil {
			   return newGen, nil
		   }
		  if err == persist.ErrStaleFence || err == persist.ErrConflict {
			  // CAS 冲突，短暂等待后重试（指数退避简化版：50ms * (attempt+1)）。
			   lastErr = ErrGenLocked
			  time.Sleep(time.Duration(50*(attempt+1)) * time.Millisecond)
			   continue
		  }
		  return BatchGeneration{}, fmt.Errorf("ledorphan acquire gen: put: %w", err)
	  }
	  if lastErr != nil {
		  return BatchGeneration{},
			   lastErr
	   }
	  return BatchGeneration{}, ErrGenLocked
  }

// Refresh 续期世代锁的 ExpiresAt（worker 心跳）。
// 调用方必须是当前持有者（workerID 和 generation 匹配）；否则返回 ErrGenNotHeld。
// 续期成功后 ExpiresAt 延长 GenLockTTL。

   func (m *GenerationManager) Refresh(ctx context.Context, batchID, workerID string, generation int64) error {
	  if m.kv == nil {
		  return ErrStorageNil
	   }
	   batchID = strings.TrimSpace(batchID)
	  for attempt := 0; attempt < maxCASRetries; attempt++ {
		   gen,
			  fence, isNew, err := m.readCurrent(ctx, batchID)
		  if err != nil {
			   return fmt.Errorf("ledorphan refresh gen: read: %w", err)
		  }
		  if isNew {
			   return ErrGenNotHeld
		  }
		  if gen.WorkerID != workerID || gen.Generation != generation {
			   return ErrGenNotHeld
		   }
		  if gen.Status != GenStatusLocked {
			   return ErrGenNotHeld
		  }
		   now := m.now()
		  gen.ExpiresAt = now.Add(GenLockTTL)
		   gen.UpdatedAt = now
		   body, err := json.Marshal(gen)
		   if err != nil {
			  return err
		   }
		  row := persist.Row{Kind: kindBatchGen, Key: batchID, Body: body}
		   err = m.kv.Put(ctx, row, fence)
		  if err == nil {
			   return nil
		  }
		  if err == persist.ErrStaleFence || err == persist.ErrConflict {
			   time.Sleep(time.Duration(50*(attempt+1)) * time.Millisecond)
			  continue
		  }
		   return fmt.Errorf("ledorphan refresh gen: put: %w", err)
	   }
	   return ErrGenNotHeld
   }

// Release 主动释放世代锁（Submit/Seal 完成后调用）。
// 调用方必须是当前持有者；释放后状态变为 released，其他 worker 可立即抢锁。

   func (m *GenerationManager) Release(ctx context.Context, batchID, workerID string, generation int64) error {
	   if m.kv == nil {
		  return ErrStorageNil
	  }
	   batchID = strings.TrimSpace(batchID)
	  for attempt := 0; attempt < maxCASRetries; attempt++ {
		  gen, fence, isNew, err := m.readCurrent(ctx, batchID)
		   if err != nil {
			  return fmt.Errorf("ledorphan release gen: read: %w", err)
		   }
		   if isNew {
			  return nil // 不存在，无需释放
		   }
		  if gen.WorkerID != workerID || gen.Generation != generation {
			   // 不是当前持有者，静默返回（可能已被接管或过期）。
			  return nil
		  }
		   now := m.now()
		   gen.Status = GenStatusReleased
		   gen.ReleasedAt = &now
		  gen.UpdatedAt = now
		  body, err := json.Marshal(gen)
		   if err != nil {
			  return err
		  }
		  row := persist.Row{Kind: kindBatchGen, Key: batchID, Body: body}
		  err = m.kv.Put(ctx, row, fence)
		   if err == nil {
			   return nil
		   }
		   if err == persist.ErrStaleFence || err == persist.ErrConflict {
			  time.Sleep(time.Duration(50*(attempt+1)) * time.Millisecond)
			   continue
		   }
		  return fmt.Errorf("ledorphan release gen: put: %w", err)
	   }
	  return nil
  }

// Check 验证调用方是否仍是当前合法持有者。
// 用于在 Submit/Seal 的每个关键步骤前校验；若世代落后，立即中止操作。
  func (m *GenerationManager) Check(ctx context.Context, batchID, workerID string, generation int64) error {
	  if m.kv == nil {
		   return ErrStorageNil
	  }
	   gen, _, isNew, err := m.readCurrent(ctx, strings.TrimSpace(batchID))
	   if err != nil {
		   return fmt.Errorf("ledorphan check gen: %w", err)
	   }
	  if isNew {
		  // 没有持久化记录：若调用方世代是 0，允许；否则 stale。
		  if generation != 0 {
			   return ErrStaleGen
		  }
		   return nil
	  }
	  if gen.Generation > generation {
		  // 持久化世代比调用方世代高：调用方已过时。
		  return ErrStaleGen
	  }
	   if gen.WorkerID != workerID && gen.Status == GenStatusLocked {
		  // 锁被他人持有且世代相同（理论上不应发生，但防御性检查）。
		  return ErrGenNotHeld
	   }
	   return nil
  }

// ExpireStale 将超时未续期的 locked 状态锁标记为 expired。
// 由周期性 GenerationReclaimer worker 调用；每次扫描最多处理 limit 条。
  func (m *GenerationManager) ExpireStale(ctx context.Context, limit int) (int, error) {
	  if m.kv == nil {
		  return 0, ErrStorageNil
	  }
	  if limit <= 0 {
		   limit = 100
	   }
	   rows, err := m.kv.List(ctx, kindBatchGen, limit)
	   if err != nil {
		  return 0, fmt.Errorf("ledorphan expire stale: list: %w", err)
	   }
	   now := m.now()
	   expired := 0
	  for _, row := range rows {
		   var gen BatchGeneration
		   if err := json.Unmarshal(row.Body, &gen); err != nil {
			  continue
		   }
		  if gen.Status != GenStatusLocked {
			   continue
		   }
		   if now.Before(gen.ExpiresAt) {
			  continue
		  }
		   // 已超时，CAS 更新为 expired。
		   gen.Status = GenStatusExpired
		   gen.UpdatedAt = now
		   body, err := json.Marshal(gen)
		  if err != nil {
			  continue
		   }
		  newRow := persist.Row{Kind: kindBatchGen, Key: gen.BatchID, Body: body}
		   if putErr := m.kv.Put(ctx, newRow, row.Fence); putErr != nil {
			  // CAS 冲突：锁可能刚被续期，忽略本次，下次扫描再判断。
			  continue
		   }
		   expired++
	   }
	  return expired, nil
  }

// Get 读取批次的当前世代记录（无记录时返回 ErrBatchNotFound）。

  func (m *GenerationManager) Get(ctx context.Context, batchID string) (BatchGeneration, error) {
	   if m.kv == nil {
		   return BatchGeneration{}, ErrStorageNil
	  }
	  gen, _, isNew, err := m.readCurrent(ctx, strings.TrimSpace(batchID))
	  if err != nil {
		  return BatchGeneration{}, err
	  }
	   if isNew {
		   return BatchGeneration{},
			   ErrBatchNotFound
	  }
	  return gen, nil
  }

// readCurrent 读取当前世代记录及围栏（用于后续 CAS）。
// isNew == true 表示 kv 中不存在该记录（首次锁定）。

   func (m *GenerationManager) readCurrent(ctx context.Context, batchID string) (BatchGeneration, persist.Fence, bool, error) {
	  row, err := m.kv.Get(ctx, kindBatchGen, batchID)
	  if err == persist.ErrNotFound {
		  return BatchGeneration{BatchID: batchID, Generation: 0}, persist.Fence{}, true, nil
	  }
	  if err != nil {
		  return BatchGeneration{},
			  persist.Fence{}, false, err
	  }
	  var gen BatchGeneration
	   if err := json.Unmarshal(row.Body, &gen); err != nil {
		  return BatchGeneration{}, persist.Fence{}, false, fmt.Errorf("ledorphan read gen: unmarshal: %w", err)
	  }
	  return gen, row.Fence, false, nil
  }
