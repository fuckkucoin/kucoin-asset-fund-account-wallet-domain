package stucksettle

import (
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
)

const defaultAdvanceLockTTL = 60 * time.Second

// MutexStore 内存互斥锁仓（环境替换为 SQL/Redis 实现）。

type MutexStore struct {
	mu    sync.Mutex
	locks map[string]*AdvanceLock // key = transfer_id
}

// NewMutexStore 创建内存互斥锁仓。
  func NewMutexStore() *MutexStore {
	  return &MutexStore{locks: map[string]*AdvanceLock{}}
  }

// TryHold 申请 AdvanceStuck 互斥锁。
// 返回 ErrAdvanceLocked 时调用方（Advance 路径）应等待或中止。
// 幂等：同 holderNode 再次调用返回已有锁（不报错）。
   func (m *MutexStore) TryHold(transferID, holderNode string, now time.Time) (*AdvanceLock, error) {
	   if m == nil {
		  return nil, ErrUnavailable
	  }
	  trID := strings.TrimSpace(transferID)
	  node := strings.TrimSpace(holderNode)
	   if trID == "" || node == "" {
		   return nil, ErrInvalidInput
	  }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  // 超时锁视为已释放。
	  if existing, ok := m.locks[trID]; ok {
		  if existing.State == ALockHeld {
			   if now.Before(existing.ExpiresAt) {
				   if existing.HolderNode == node {
					  return existing, nil // 同节点重入，幂等
				   }
				  return nil, ErrAdvanceLocked
			   }
			  // 超时自动释放
			   existing.State = ALockReleased
			   existing.ReleasedAt = now
			  existing.UpdatedAt = now
		   }
	  }
	  lock := &AdvanceLock{
		  LockID: uuid.NewString(),
		   TransferID: trID,
		  State:      ALockHeld,
		   HeldAt:     now,
		  HolderNode: node,
		   ExpiresAt: now.Add(defaultAdvanceLockTTL),
		   CreatedAt: now,
		   UpdatedAt: now,
	  }
	  m.locks[trID] = lock
	  return lock, nil
  }

// Release 释放 AdvanceStuck 互斥锁。

  func (m *MutexStore) Release(transferID, holderNode string, now time.Time) error {
	  if m == nil {
		  return ErrUnavailable
	   }
	   trID := strings.TrimSpace(transferID)
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   lock, ok := m.locks[trID]
	  if !ok {
		   return nil // 不存在视为已释放
	   }
	  if lock.State == ALockReleased {
		  return nil
	  }
	   if lock.HolderNode != strings.TrimSpace(holderNode) {
		   return ErrAdvanceLocked // 非持有者不得释放
	  }
	  if err := AdvanceLockTransition(lock.State, ALockReleased); err != nil {
		  return err
	   }
	   lock.State = ALockReleased
	  lock.ReleasedAt = now
	   lock.UpdatedAt = now
	  return nil
  }

// CheckRecoverAllowed 检查 Recover 是否被 Advance 锁阻塞。
// isHoldState=true 表示 StuckCase 当前为 Hold 态（禁止旁路 Recover）。

  func (m *MutexStore) CheckRecoverAllowed(transferID string, isHoldState bool, now time.Time) error {
	   if m == nil {
		  return nil
	   }
	  if isHoldState {
		   return ErrHoldBypass
	   }
	  trID := strings.TrimSpace(transferID)
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   lock, ok := m.locks[trID]
	  if !ok {
		  return nil
	  }
	   if lock.State == ALockHeld && now.Before(lock.ExpiresAt) {
		   return ErrAdvanceLocked
	  }
	   return nil
  }

// Snapshot 返回全部锁状态快照（运维查询）。

   func (m *MutexStore) Snapshot() []AdvanceLock {
	  if m == nil {
		   return nil
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  out := make([]AdvanceLock, 0, len(m.locks))
	   for _, l := range m.locks {
		   cp := *l
		  out = append(out, cp)
	   }
	  return out
   }

// SweepExpired 清理已超时的 Held 锁（置为 Released）。

  func (m *MutexStore) SweepExpired(now time.Time) int {
	   if m == nil {
		   return 0
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  n := 0
	   for _, l := range m.locks {
		   if l.State == ALockHeld && !now.Before(l.ExpiresAt) {
			   l.State = ALockReleased
			  l.ReleasedAt = now
			  l.UpdatedAt = now
			   n++
		  }
	   }
	  return n
  }
