package swapthaw

import (
	"fmt"
	"sync"
	"time"
	"github.com/google/uuid"
)

// LeaseStore FreezeLease 持久化接口。

type LeaseStore interface {
	// SaveLease 新建 FreezeLease（INSERT，重复 ID 报错）。
	SaveLease(lease *FreezeLease) error
	// UpdateLease 更新 FreezeLease 状态（必须已存在）。
	UpdateLease(lease *FreezeLease) error
	// GetLease 按 ID 获取 FreezeLease。
	GetLease(id string) (*FreezeLease,
		error)
	// GetLeaseByOrder 按订单 ID 获取活跃 FreezeLease（status=open）。
	GetLeaseByOrder(orderID string) (*FreezeLease, error)
	// ListExpiredOpen 列出已过期且状态为 open 的 Lease（供 Worker 扫描）。
	ListExpiredOpen(limit int, before time.Time) ([]*FreezeLease, error)
}

// BindStore SettleBind 持久化接口。
type BindStore interface {
	// InsertBindOnce INSERT IGNORE / ON CONFLICT DO NOTHING 写入 SettleBind。
	// 若 order_id 已存在且 Leg 不同，返回 ErrSettleBindConflict。
	// 若 order_id 已存在且 Leg 相同，返回 nil（幂等）。
	InsertBindOnce(bind *SettleBind) error
	// GetBind 按订单 ID 获取已有 SettleBind。
	GetBind(orderID string) (*SettleBind, error)
}

// ThawMutex 解冻互斥引擎，组合 LeaseStore + BindStore。

type ThawMutex struct {
	leases LeaseStore
	binds  BindStore
}

// NewThawMutex 构造解冻互斥引擎。
  func NewThawMutex(leases LeaseStore, binds BindStore) *ThawMutex {
	   return &ThawMutex{leases: leases, binds: binds}
   }

// AcquireLease 在冻额路径（Execute）中创建 FreezeLease 并绑定世代。
// 调用方须在事务外先完成 GuardNewFreeze；本方法在 freeze journal 同事务内调用。
  func (tm *ThawMutex) AcquireLease(req AcquireLeaseReq, generation int64) (*FreezeLease, error) {
	   if req.OrderID == "" || req.UserID == "" || req.Asset == "" || req.FrozenAmount == "" {
		   return nil, fmt.Errorf("swapthaw: AcquireLease: missing required fields")
	  }
	  now := time.Now().UTC()
	  lease := &FreezeLease{
		  ID:      uuid.NewString(),
		  OrderID: req.OrderID,
		  UserID:  req.UserID,
		  Asset:        req.Asset,
		   FrozenAmount: req.FrozenAmount,
		   Generation:   generation,
		   Token:        uuid.NewString(),
		   Status:       LeaseStatusOpen,
		   CreatedAt: now,
		  UpdatedAt: now,
	   }
	   if req.TTL > 0 {
		  lease.ExpiresAt = now.Add(req.TTL)
	   }
	   if err := tm.leases.SaveLease(lease); err != nil {
		   return nil, fmt.Errorf("swapthaw: AcquireLease.SaveLease: %w", err)
	   }
	   return lease, nil
  }

// ClaimTerminal 声明终态腿（failUnfreeze 或 settle）。
// 流程：
// 1. 按 LeaseID 读取 FreezeLease，校验 Token、过期、状态。
// 2. 通过 BindStore.InsertBindOnce CAS 互斥绑定终态腿。
// - CAS 成功：本腿胜出，接着账本操作。
// - CAS 失败（ErrSettleBindConflict）：另一腿已胜出，返回 ErrThawMutexDenied。
// 3. 若 Lease 已处于终态（ErrDoubleRelease）：返回 ErrDoubleRelease，由上层升 ThawCase。
// 4. 更新 Lease 状态为 settled/thawed/blocked。
// 调用方须在持用户锁（ledger.UserLockKey）、数据库事务内调用。
  func (tm *ThawMutex) ClaimTerminal(req ClaimTerminalReq) (*SettleBind, *FreezeLease, error) {
	   if req.OrderID == "" || req.LeaseID == "" || req.LeaseToken == "" {
		  return nil, nil, fmt.Errorf("swapthaw: ClaimTerminal: orderID/leaseID/leaseToken required")
	   }
	   now := time.Now().UTC()
	   lease, err := tm.leases.GetLease(req.LeaseID)
	  if err != nil {
		  return nil, nil, fmt.Errorf("swapthaw: ClaimTerminal.GetLease: %w", err)
	  }
	   if lease.OrderID != req.OrderID {
		  return nil, nil, fmt.Errorf("%w: lease.OrderID=%s req.OrderID=%s", ErrLeaseConflict, lease.OrderID, req.OrderID)
	  }
	  // 校验 Lease 是否允许本次声明（检查 token、过期、状态）。
	   if err := ValidateLeaseForClaim(lease, req, now); err != nil {
		  return nil, lease, err
	   }
	  // SettleBind CAS：互斥争抢终态腿。
	   bind := &SettleBind{
		  ID:      uuid.NewString(),
		   OrderID: req.OrderID,
		  LeaseID: req.LeaseID,
		   Leg:     req.Leg,
		   Actor:   req.Actor,
		  BoundAt: now,
	   }
	  if err := tm.binds.InsertBindOnce(bind); err != nil {
		  if err == ErrSettleBindConflict {
			  // 另一腿已胜出。
			  return nil, lease, ErrThawMutexDenied
		  }
		  return nil, lease, fmt.Errorf("swapthaw: ClaimTerminal.InsertBindOnce: %w", err)
	   }
	  // 读取实际胜出的 Bind（幂等时可能不是本次的 bind）。
	   winner, err := tm.binds.GetBind(req.OrderID)
	  if err != nil {
		  return nil, lease, fmt.Errorf("swapthaw: ClaimTerminal.GetBind: %w", err)
	  }
	   if winner.Leg != req.Leg {
		   // 幂等路径：同 order_id 相同 Leg，直接返回已存在的 bind。
		  return winner, lease, ErrThawMutexDenied
	  }
	  // 更新 Lease 状态。
	   var toStatus LeaseStatus
	   switch req.Leg {
	   case BindLegSettle:
		  toStatus = LeaseStatusSettled
	  case BindLegFailUnfreeze:
		   toStatus = LeaseStatusThawed
	   default:
		   return nil, lease, fmt.Errorf("swapthaw: ClaimTerminal: unknown leg %q", req.Leg)
	  }
	   if err := TransitLeaseWithLeg(lease, toStatus, req.Leg, now); err != nil {
		   return nil, lease, err
	   }
	  if err := tm.leases.UpdateLease(lease); err != nil {
		  return nil, lease, fmt.Errorf("swapthaw: ClaimTerminal.UpdateLease: %w", err)
	   }
	  return winner, lease, nil
  }

// ──────────────────────────────────────────────
// 内存 BindStore 实现（用）
// ──────────────────────────────────────────────
// MemBindStore BindStore 内存实现。

type MemBindStore struct {
	mu sync.Mutex
	binds map[string]*SettleBind // orderID → SettleBind
}

// NewMemBindStore 创建内存 BindStore。
  func NewMemBindStore() *MemBindStore {
	   return &MemBindStore{binds: make(map[string]*SettleBind)}
  }

   func (m *MemBindStore) InsertBindOnce(bind *SettleBind) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  existing,
		   ok := m.binds[bind.OrderID]
	   if ok {
		   if existing.Leg == bind.Leg {
			  return nil // 幂等
		  }
		  return ErrSettleBindConflict
	   }
	   cp := *bind
	  m.binds[bind.OrderID] = &cp
	   return nil
   }

  func (m *MemBindStore) GetBind(orderID string) (*SettleBind, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  b, ok := m.binds[orderID]
	  if !ok {
		   return nil, ErrSettleBindNotFound
	   }
	   cp := *b
	  return &cp, nil
  }

// ──────────────────────────────────────────────
// 内存 LeaseStore 实现（用）
// ──────────────────────────────────────────────
// MemLeaseStore LeaseStore 内存实现。
type MemLeaseStore struct {
	mu     sync.RWMutex
	leases map[string]*FreezeLease // id → FreezeLease
	byOrder map[string]string // orderID → leaseID（只保活跃 open 的）
}

// NewMemLeaseStore 创建内存 LeaseStore。
  func NewMemLeaseStore() *MemLeaseStore {
	  return &MemLeaseStore{
		  leases:  make(map[string]*FreezeLease),
		   byOrder: make(map[string]string),
	  }
   }

  func (m *MemLeaseStore) SaveLease(lease *FreezeLease) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   if _, exists := m.leases[lease.ID]; exists {
		  return fmt.Errorf("swapthaw: MemLeaseStore.SaveLease: duplicate ID %s", lease.ID)
	  }
	   cp := *lease
	  m.leases[lease.ID] = &cp
	  if lease.Status == LeaseStatusOpen {
		   m.byOrder[lease.OrderID] = lease.ID
	   }
	   return nil
  }

  func (m *MemLeaseStore) UpdateLease(lease *FreezeLease) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   if _, ok := m.leases[lease.ID]; !ok {
		   return ErrLeaseNotFound
	  }
	  cp := *lease
	   m.leases[lease.ID] = &cp
	  if lease.Status.IsTerminal() {
		   // 终态后从活跃 byOrder 索引中移除。
		  delete(m.byOrder, lease.OrderID)
	   }
	   return nil
   }

   func (m *MemLeaseStore) GetLease(id string) (*FreezeLease, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  l, ok := m.leases[id]
	   if !ok {
		  return nil, ErrLeaseNotFound
	  }
	   cp := *l
	  return &cp, nil
  }

   func (m *MemLeaseStore) GetLeaseByOrder(orderID string) (*FreezeLease, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   id, ok := m.byOrder[orderID]
	   if !ok {
		   return nil, ErrLeaseNotFound
	   }
	  l, ok := m.leases[id]
	   if !ok {
		   return nil, ErrLeaseNotFound
	   }
	  cp := *l
	   return &cp, nil
   }

  func (m *MemLeaseStore) ListExpiredOpen(limit int, before time.Time) ([]*FreezeLease, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	  var out []*FreezeLease
	  for _, l := range m.leases {
		   if l.Status == LeaseStatusOpen && !l.ExpiresAt.IsZero() && l.ExpiresAt.Before(before) {
			   cp := *l
			   out = append(out, &cp)
			  if len(out) >= limit {
				  break
			   }
		   }
	  }
	   return out, nil
  }
