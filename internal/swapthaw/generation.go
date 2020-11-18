package swapthaw

import (
	"fmt"
	"sync"
	"time"
)

// GenerationStore 冻额世代持久化接口。
type GenerationStore interface {
	// GetGeneration 获取订单当前世代；不存在时返回 generation=0 的空记录（非 error）。
	GetGeneration(orderID string) (*FreezeGeneration, error)
	// CASGeneration Compare-And-Swap 写入世代：仅当库中 generation == expected 时才更新为 next。
	// 返回 ErrGenerationCAS 表示 CAS 失败。
	CASGeneration(orderID string,
		expected, next int64, userID, activeLeaseID string) error
	// ClearActiveLease 将指定订单的 active_lease_id 置空（Lease 进入终态时调用）。
	ClearActiveLease(orderID string) error
	// ListOpenThawCasesByUser 查询用户是否存在未关闭 ThawCase（open/proven 均视为未关闭）。
	ListOpenThawCasesByUser(userID string) ([]string, error)
}

// GenerationManager 冻额世代管理器。

type GenerationManager struct {
	store GenerationStore
	mu    sync.Mutex
}

// NewGenerationManager 创建世代管理器。
   func NewGenerationManager(store GenerationStore) *GenerationManager {
	  return &GenerationManager{store: store}
  }

// Current 获取订单当前世代。不存在时返回零值世代（generation=0）。

  func (gm *GenerationManager) Current(orderID string) (*FreezeGeneration, error) {
	   if orderID == "" {
		   return nil, fmt.Errorf("swapthaw: GenerationManager.Current: empty orderID")
	  }
	   gen, err := gm.store.GetGeneration(orderID)
	   if err != nil {
		   return nil, err
	   }
	  if gen == nil {
		  return &FreezeGeneration{OrderID: orderID, Generation: 0}, nil
	   }
	   return gen, nil
   }

// BumpGeneration CAS 抬升订单冻额世代并绑定新 Lease ID。
// expected 须与当前库中值一致，否则返回 ErrGenerationCAS。
// 调用方须在数据库事务内调用，保证与 freeze journal 的原子性。

  func (gm *GenerationManager) BumpGeneration(orderID, userID, leaseID string, expected int64) (int64, error) {
	  if orderID == "" || userID == "" || leaseID == "" {
		  return 0, fmt.Errorf("swapthaw: BumpGeneration: orderID/userID/leaseID required")
	  }
	  next := expected + 1
	  if err := gm.store.CASGeneration(orderID, expected, next, userID, leaseID); err != nil {
		   return 0, err
	  }
	  return next, nil
   }

// ClearActiveLease 将订单的 active_lease_id 置空（Lease 进入终态时调用）。

   func (gm *GenerationManager) ClearActiveLease(orderID string) error {
	   return gm.store.ClearActiveLease(orderID)
  }

// UserHasOpenThawCase 查询该用户是否存在未关闭的 ThawCase，用于 QuoteFreeze 阻断路径。
// 返回 true 表示存在，调用方应返回 ErrThawCaseOpen。
   func (gm *GenerationManager) UserHasOpenThawCase(userID string) (bool, error) {
	   if userID == "" {
		   return false, fmt.Errorf("swapthaw: UserHasOpenThawCase: empty userID")
	  }
	   cases,
		   err := gm.store.ListOpenThawCasesByUser(userID)
	   if err != nil {
		  return false, err
	   }
	   return len(cases) > 0, nil
   }

// ──────────────────────────────────────────────
// 内存实现（用）
// ──────────────────────────────────────────────
// MemGenerationStore GenerationStore 内存实现，仅用于单元。

type MemGenerationStore struct {
	mu   sync.RWMutex
	gens map[string]*FreezeGeneration
	openCases map[string][]string // userID → []caseID
}

// NewMemGenerationStore 创建内存世代存储。
   func NewMemGenerationStore() *MemGenerationStore {
	   return &MemGenerationStore{
		  gens:      make(map[string]*FreezeGeneration),
		  openCases: make(map[string][]string),
	  }
  }

   func (m *MemGenerationStore) GetGeneration(orderID string) (*FreezeGeneration, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	   g, ok := m.gens[orderID]
	   if !ok {
		  return &FreezeGeneration{OrderID: orderID, Generation: 0}, nil
	   }
	  cp := *g
	  return &cp, nil
   }

  func (m *MemGenerationStore) CASGeneration(orderID string, expected, next int64, userID, activeLeaseID string) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   cur, ok := m.gens[orderID]
	   if !ok {
		  if expected != 0 {
			   return fmt.Errorf("%w: expected=%d but order has no generation record", ErrGenerationCAS, expected)
		   }
		   m.gens[orderID] = &FreezeGeneration{
			  OrderID:    orderID,
			  UserID:     userID,
			   Generation: next,
			  ActiveLeaseID: activeLeaseID,
			   UpdatedAt:     time.Now().UTC(),
		  }
		  return nil
	   }
	   if cur.Generation != expected {
		  return fmt.Errorf("%w: expected=%d actual=%d", ErrGenerationCAS, expected, cur.Generation)
	   }
	  cur.Generation = next
	   cur.ActiveLeaseID = activeLeaseID
	   cur.UpdatedAt = time.Now().UTC()
	   if userID != "" {
		  cur.UserID = userID
	  }
	  return nil
   }

   func (m *MemGenerationStore) ClearActiveLease(orderID string) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   if g, ok := m.gens[orderID]; ok {
		   g.ActiveLeaseID = ""
		  g.UpdatedAt = time.Now().UTC()
	  }
	  return nil
  }

// RegisterOpenCase 在内存实现中注册一条未关闭 ThawCase（辅助方法）。

  func (m *MemGenerationStore) RegisterOpenCase(userID, caseID string) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   m.openCases[userID] = append(m.openCases[userID], caseID)
   }

// RemoveOpenCase 在内存实现中移除一条已关闭 ThawCase（辅助方法）。
   func (m *MemGenerationStore) RemoveOpenCase(userID, caseID string) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  list := m.openCases[userID]
	   var out []string
	  for _, id := range list {
		  if id != caseID {
			   out = append(out, id)
		  }
	   }
	  m.openCases[userID] = out
  }

  func (m *MemGenerationStore) ListOpenThawCasesByUser(userID string) ([]string, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  cp := make([]string, len(m.openCases[userID]))
	   copy(cp, m.openCases[userID])
	   return cp, nil
  }
