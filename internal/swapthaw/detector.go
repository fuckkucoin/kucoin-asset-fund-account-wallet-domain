package swapthaw

import (
	"fmt"
	"sync"
	"time"
	"github.com/google/uuid"
)

// ThawCaseStore ThawCase 持久化接口。
type ThawCaseStore interface {
	// SaveCase 新建 ThawCase（INSERT）。
	SaveCase(tc *ThawCase) error
	// UpdateCase 更新 ThawCase 状态（必须已存在）。
	UpdateCase(tc *ThawCase) error
	// GetCase 按 ID 获取 ThawCase。
	GetCase(id string) (*ThawCase, error)
	// GetCaseByOrder 按订单 ID 获取最新 ThawCase（可能有多条，取最新）。
	GetCaseByOrder(orderID string) (*ThawCase,
		error)
	// ListOpenByUser 列出指定用户未关闭的 ThawCase（open/proven）。
	ListOpenByUser(userID string, limit int) ([]*ThawCase, error)
	// ListOpenCases 列出全部未关闭 ThawCase（供 Worker 巡检）。
	ListOpenCases(limit int) ([]*ThawCase, error)
}

// DoubleReleaseDetector 双释发现引擎。
type DoubleReleaseDetector struct {
	store ThawCaseStore
}

// NewDoubleReleaseDetector 构造双释发现引擎。
   func NewDoubleReleaseDetector(store ThawCaseStore) *DoubleReleaseDetector {
	  return &DoubleReleaseDetector{store: store}
  }

// Detect 在 ClaimTerminal 返回 ErrDoubleRelease 时创建 ThawCase。
// 调用方须在持用户锁、数据库事务内调用（与 ClaimTerminal 同一事务）。
   func (d *DoubleReleaseDetector) Detect(ev DoubleReleaseEvent) (*ThawCase, error) {
	   if ev.OrderID == "" || ev.UserID == "" {
		   return nil, fmt.Errorf("swapthaw: Detect: orderID and userID required")
	   }
	  now := time.Now().UTC()
	   tc := &ThawCase{
		   ID:      uuid.NewString(),
		   OrderID: ev.OrderID,
		  UserID:  ev.UserID,
		   LeaseID: ev.LeaseID,
		  Asset:          ev.Asset,
		  Kind:           ev.Kind,
		  Status:         ThawCaseStatusOpen,
		   DetectedAmount: ev.DetectedAmount,
		   FrozenSnapshot: ev.FrozenSnapshot,
		   AvailableSnapshot: ev.AvailableSnapshot,
		   DetectedAt: now,
		  UpdatedAt: now,
	  }
	  if err := d.store.SaveCase(tc); err != nil {
		  return nil, fmt.Errorf("swapthaw: Detect.SaveCase: %w", err)
	  }
	   return tc, nil
   }

// DetectOrphan 创建孤儿 Frozen ThawCase（由 Worker 调用）。
// 参数与 Detect 相同，Kind 固定为 ThawCaseKindOrphanFrozen。
   func (d *DoubleReleaseDetector) DetectOrphan(orderID, userID, leaseID, asset, frozenAmt, frozenSnap, availSnap string) (*ThawCase, error) {
	  return d.Detect(DoubleReleaseEvent{
		   OrderID: orderID,
		  UserID:  userID,
		   LeaseID: leaseID,
		  Asset:   asset,
		  Kind:              ThawCaseKindOrphanFrozen,
		   DetectedAmount:    frozenAmt,
		   FrozenSnapshot:    frozenSnap,
		   AvailableSnapshot: availSnap,
	  })
  }

// ProveCase 固化 ThawCase 证明：open→proven。
// ProofPayload 须非空；Approver 须填写。

  func (d *DoubleReleaseDetector) ProveCase(req ProveThawCaseReq) (*ThawCase, error) {
	  if req.CaseID == "" {
		  return nil, fmt.Errorf("swapthaw: ProveCase: caseID required")
	  }
	  if req.ProofPayload == "" {
		   return nil, ErrThawCaseProofMissing
	   }
	   tc, err := d.store.GetCase(req.CaseID)
	  if err != nil {
		  return nil, err
	   }
	   tc.ProofPayload = req.ProofPayload
	   tc.Approver = req.Approver
	  now := time.Now().UTC()
	  if err := TransitThawCase(tc, ThawCaseStatusProven, now); err != nil {
		  return nil, err
	   }
	   if err := d.store.UpdateCase(tc); err != nil {
		  return nil, fmt.Errorf("swapthaw: ProveCase.UpdateCase: %w", err)
	   }
	  return tc, nil
   }

// CloseCase 收口 ThawCase：proven→closed。
// 调用前须确认案件已处于 proven 状态（有证明）。
  func (d *DoubleReleaseDetector) CloseCase(req CloseThawCaseReq) (*ThawCase, error) {
	   if req.CaseID == "" {
		   return nil, fmt.Errorf("swapthaw: CloseCase: caseID required")
	   }
	   tc, err := d.store.GetCase(req.CaseID)
	   if err != nil {
		   return nil, err
	  }
	  if tc.Status != ThawCaseStatusProven {
		   return nil, fmt.Errorf("%w: case %s is in status %q, not proven", ErrThawCaseProofMissing, req.CaseID, tc.Status)
	   }
	  now := time.Now().UTC()
	   if err := TransitThawCase(tc, ThawCaseStatusClosed, now); err != nil {
		   return nil, err
	   }
	  if req.Approver != "" {
		   tc.Approver = req.Approver
	  }
	  if err := d.store.UpdateCase(tc); err != nil {
		  return nil, fmt.Errorf("swapthaw: CloseCase.UpdateCase: %w", err)
	  }
	  return tc, nil
  }

// GetCase 按 ID 查询 ThawCase。
  func (d *DoubleReleaseDetector) GetCase(caseID string) (*ThawCase, error) {
	   return d.store.GetCase(caseID)
   }

// GetCaseByOrder 按订单 ID 查询最新 ThawCase。
   func (d *DoubleReleaseDetector) GetCaseByOrder(orderID string) (*ThawCase, error) {
	   return d.store.GetCaseByOrder(orderID)
   }

// ListOpenCases 列出全部未关闭 ThawCase（Worker 巡检入口）。

  func (d *DoubleReleaseDetector) ListOpenCases(limit int) ([]*ThawCase, error) {
	  return d.store.ListOpenCases(limit)
  }

// ListOpenByUser 列出指定用户的未关闭 ThawCase（阻断新冻额判断）。
   func (d *DoubleReleaseDetector) ListOpenByUser(userID string, limit int) ([]*ThawCase, error) {
	   return d.store.ListOpenByUser(userID, limit)
  }

// ──────────────────────────────────────────────
// 内存 ThawCaseStore 实现（用）
// ──────────────────────────────────────────────
// MemThawCaseStore ThawCaseStore 内存实现。

type MemThawCaseStore struct {
	mu      sync.RWMutex
	cases   map[string]*ThawCase // id → ThawCase
	byOrder map[string][]string  // orderID → []caseID
	byUser map[string][]string // userID → []caseID (open/proven only)
}

// NewMemThawCaseStore 创建内存 ThawCaseStore。
  func NewMemThawCaseStore() *MemThawCaseStore {
	  return &MemThawCaseStore{
		   cases:   make(map[string]*ThawCase),
		  byOrder: make(map[string][]string),
		   byUser:  make(map[string][]string),
	   }
   }

   func (m *MemThawCaseStore) SaveCase(tc *ThawCase) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   if _, exists := m.cases[tc.ID]; exists {
		  return fmt.Errorf("swapthaw: MemThawCaseStore.SaveCase: duplicate ID %s", tc.ID)
	   }
	   cp := *tc
	  m.cases[tc.ID] = &cp
	   m.byOrder[tc.OrderID] = append(m.byOrder[tc.OrderID], tc.ID)
	   if tc.Status == ThawCaseStatusOpen || tc.Status == ThawCaseStatusProven {
		   m.byUser[tc.UserID] = append(m.byUser[tc.UserID], tc.ID)
	  }
	   return nil
   }

  func (m *MemThawCaseStore) UpdateCase(tc *ThawCase) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   old, ok := m.cases[tc.ID]
	   if !ok {
		   return ErrThawCaseNotFound
	   }
	   // 若从非终态→closed，从 byUser 中移除。
	   if old.Status != ThawCaseStatusClosed && tc.Status == ThawCaseStatusClosed {
		   list := m.byUser[tc.UserID]
		   var out []string
		   for _, id := range list {
			  if id != tc.ID {
				   out = append(out, id)
			   }
		  }
		   m.byUser[tc.UserID] = out
	   }
	  cp := *tc
	  m.cases[tc.ID] = &cp
	   return nil
  }

  func (m *MemThawCaseStore) GetCase(id string) (*ThawCase, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  tc, ok := m.cases[id]
	  if !ok {
		  return nil, ErrThawCaseNotFound
	   }
	   cp := *tc
	   return &cp, nil
  }

   func (m *MemThawCaseStore) GetCaseByOrder(orderID string) (*ThawCase, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	   ids, ok := m.byOrder[orderID]
	  if !ok || len(ids) == 0 {
		  return nil, ErrThawCaseNotFound
	   }
	  // 取最新（最后写入的）。
	   tc := m.cases[ids[len(ids)-1]]
	  cp := *tc
	  return &cp, nil
   }

   func (m *MemThawCaseStore) ListOpenByUser(userID string, limit int) ([]*ThawCase, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	   ids := m.byUser[userID]
	   var out []*ThawCase
	   for i := len(ids) - 1; i >= 0 && len(out) < limit; i-- {
		  tc, ok := m.cases[ids[i]]
		  if ok && (tc.Status == ThawCaseStatusOpen || tc.Status == ThawCaseStatusProven) {
			   cp := *tc
			  out = append(out, &cp)
		   }
	  }
	  return out, nil
   }

  func (m *MemThawCaseStore) ListOpenCases(limit int) ([]*ThawCase, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  var out []*ThawCase
	  for _, tc := range m.cases {
		   if tc.Status == ThawCaseStatusOpen || tc.Status == ThawCaseStatusProven {
			  cp := *tc
			   out = append(out, &cp)
			   if len(out) >= limit {
				   break
			  }
		   }
	   }
	  return out, nil
  }
