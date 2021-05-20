package floatdesk

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/metrics"
)

// ErrRefillConflict 并发补仓冲突：前一次补仓令牌尚未释放。
var ErrRefillConflict = fmt.Errorf("refill conflict: parallel refill in progress")

// ErrRefillStale 令牌代次过期：试图以旧令牌释放已被新代次覆盖的锁。
var ErrRefillStale = fmt.Errorf("refill token stale: generation mismatch")

// RefillToken 补仓令牌，由 RefillMutex.Acquire 签发，Release 时校验代次。

type RefillToken struct {
	Chain      string `json:"chain"`
	Generation uint64 `json:"generation"`
	AcquiredAt time.Time `json:"acquired_at"`
}

// refillEntry 每条链的锁状态。
type refillEntry struct {
	mu         sync.Mutex
	generation uint64
	active     bool
}

// RefillMutex 跨链补仓互斥锁注册中心。
// 零值不可用，须通过 NewRefillMutex 构造。
type RefillMutex struct {
	mu    sync.Mutex
	lanes map[string]*refillEntry
	clock func() time.Time
}

// NewRefillMutex 构造补仓互斥锁注册中心。

  func NewRefillMutex() *RefillMutex {
	   return &RefillMutex{
		  lanes: map[string]*refillEntry{},
		   clock: func() time.Time { return time.Now().UTC() },
	   }
   }

   func (m *RefillMutex) lane(chain string) *refillEntry {
	  key := strings.ToUpper(strings.TrimSpace(chain))
	  m.mu.Lock()
	  e, ok := m.lanes[key]
	  if !ok {
		   e = &refillEntry{}
		   m.lanes[key] = e
	   }
	  m.mu.Unlock()
	   return e
   }

// Acquire 为指定链申请补仓令牌。
// 若该链已有活跃令牌则返回 ErrRefillConflict，并递增冲突计数。
// 成功时代次加一，令牌持有方须在补仓完成后调用 Release。
  func (m *RefillMutex) Acquire(chain string) (RefillToken, error) {
	   if m == nil {
		  return RefillToken{}, ErrUnavailable
	  }
	  ch := strings.ToUpper(strings.TrimSpace(chain))
	  e := m.lane(ch)
	  e.mu.Lock()
	  defer e.mu.Unlock()
	   if e.active {
		  metrics.RefillMutexConflicts.WithLabelValues(ch).Inc()
		   return RefillToken{}, ErrRefillConflict
	   }
	  e.generation++
	  e.active = true
	  return RefillToken{
		   Chain: ch,
		  Generation: e.generation,
		  AcquiredAt: m.clock(),
	  }, nil
   }

// Release 释放补仓令牌。代次不符则返回 ErrRefillStale（旧持有者无法双释放）。
   func (m *RefillMutex) Release(tok RefillToken) error {
	  if m == nil {
		  return ErrUnavailable
	  }
	   ch := strings.ToUpper(strings.TrimSpace(tok.Chain))
	  e := m.lane(ch)
	  e.mu.Lock()
	  defer e.mu.Unlock()
	  if !e.active || e.generation != tok.Generation {
		   return ErrRefillStale
	   }
	  e.active = false
	   return nil
  }

// IsActive 查询指定链是否存在活跃补仓令牌（不获取锁）。
  func (m *RefillMutex) IsActive(chain string) bool {
	  if m == nil {
		  return false
	  }
	  ch := strings.ToUpper(strings.TrimSpace(chain))
	  e := m.lane(ch)
	  e.mu.Lock()
	  defer e.mu.Unlock()
	   return e.active
   }

// PlanRefillLocked 加互斥锁执行 PlanRefill，封堵并发双注。
// 若锁冲突则直接返回 ErrRefillConflict；成功则返回 Refill 和令牌（供 Release 使用）。
   func (m *RefillMutex) PlanRefillLocked(t Tank) (Refill, RefillToken, error) {
	   if m == nil {
		   return Refill{}, RefillToken{}, ErrUnavailable
	   }
	  tok, err := m.Acquire(t.Chain)
	   if err != nil {
		   return Refill{}, RefillToken{}, err
	   }
	   r, err := PlanRefill(t)
	   if err != nil {
		   // 计划失败立即释放锁，不阻塞后续补仓。
		  _ = m.Release(tok)
		  return Refill{}, RefillToken{}, err
	  }
	   return r, tok, nil
   }
