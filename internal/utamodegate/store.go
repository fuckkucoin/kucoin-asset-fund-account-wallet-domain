package utamodegate

import (
	"context"
	"strings"
	"sync"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// Store 接口
// ─────────────────────────────────────────────────────────────────────────────
// Store 是 utamodegate 所有实体的持久化抽象。
// 所有实现必须保证并发安全。
type Store interface {
	// ── ModeGeneration ──────────────────────────────────────────────────────
	// CurrentGeneration 返回用户当前的模式世代号。
	// 若记录不存在，返回 0（首次为 UNIFIED，世代 0）。
	CurrentGeneration(ctx context.Context, userID string) (int64, error)
	// BumpGeneration CAS 递增世代：若当前世代 == expected，原子递增至 expected+1。
	// 若当前世代 != expected，返回 ErrStaleGen。
	BumpGeneration(ctx context.Context, userID string, expected int64, newMode MarginMode) (int64, error)
	// GetGeneration 返回完整 ModeGeneration 记录（含 ID / CurrentMode / UpdatedAt）。
	GetGeneration(ctx context.Context, userID string) (*ModeGeneration, error)
	// ── ModeLease ────────────────────────────────────────────────────────────
	// SaveLease 创建或按 ID 更新 ModeLease。
	SaveLease(ctx context.Context, l *ModeLease) error
	// GetLease 按 lease_id 查询。
	GetLease(ctx context.Context, leaseID string) (*ModeLease, error)
	// GetLeaseByTriggerRef 幂等查找：按 user_id + trigger_ref 查找活跃租约。
	// 若不存在返回 nil, nil。
	GetLeaseByTriggerRef(ctx context.Context, userID, triggerRef string) (*ModeLease, error)
	// ListActiveLeasesByUser 返回用户所有非终态租约（非 switched）。
	ListActiveLeasesByUser(ctx context.Context, userID string) ([]*ModeLease, error)
	// ListLeasesByStatus 按状态批量查询（供 worker 扫描）。
	ListLeasesByStatus(ctx context.Context, status LeaseStatus, limit int) ([]*ModeLease, error)
	// ── LiabilityBind ────────────────────────────────────────────────────────
	// SaveBind 创建或按 ID 更新 LiabilityBind。
	SaveBind(ctx context.Context, b *LiabilityBind) error
	// GetBind 按 bind_id 查询。
	GetBind(ctx context.Context, bindID string) (*LiabilityBind, error)
	// GetBindByRef 按 user_id + ref_id 查询（定位借还/抵押订单对应的绑定）。
	GetBindByRef(ctx context.Context, userID, refID string) (*LiabilityBind, error)
	// ListOpenBindsByUser 返回用户所有 open 状态 LiabilityBind。
	// userID 为空时返回所有用户的 open 绑定（供 worker TTL 巡检）。
	ListOpenBindsByUser(ctx context.Context, userID string) ([]*LiabilityBind, error)
	// ListExpiredOpenBinds 返回所有过期但仍 open 的绑定（供 worker 标记 expired）。
	ListExpiredOpenBinds(ctx context.Context, before time.Time, limit int) ([]*LiabilityBind, error)
	// ── ModeDriftCase ────────────────────────────────────────────────────────
	// SaveDriftCase 创建或按 ID 更新 ModeDriftCase。
	SaveDriftCase(ctx context.Context, dc *ModeDriftCase) error
	// GetDriftCase 按 case_id 查询。
	GetDriftCase(ctx context.Context, caseID string) (*ModeDriftCase, error)
	// FindOpenDriftCase 幂等查找：按 user_id + asset + kind 查找 open 状态的漂移案。
	// 若不存在返回 nil, nil。
	FindOpenDriftCase(ctx context.Context, userID, asset string, kind DriftKind) (*ModeDriftCase, error)
	// ListBlockingDriftCases 返回用户所有 open 或 proven 状态的漂移案（阻塞性）。
	ListBlockingDriftCases(ctx context.Context, userID string) ([]*ModeDriftCase, error)
	// ListOpenDriftCases 返回所有 open 状态的漂移案（供 worker 漂移巡检）。
	ListOpenDriftCases(ctx context.Context, limit int) ([]*ModeDriftCase, error)
	// ── ModeDriftProof ───────────────────────────────────────────────────────
	// SaveProof 创建或按 ID 更新 ModeDriftProof。
	SaveProof(ctx context.Context,
		p *ModeDriftProof) error
	// GetProof 按 proof_id 查询。
	GetProof(ctx context.Context, proofID string) (*ModeDriftProof, error)
	// GetProofByCaseID 按 drift_case_id 查询（每个漂移案最多一份证明）。
	GetProofByCaseID(ctx context.Context, caseID string) (*ModeDriftProof, error)
}

// ─────────────────────────────────────────────────────────────────────────────
// MemStore 内存实现
// ─────────────────────────────────────────────────────────────────────────────
// MemStore 是 Store 的线程安全内存实现，用于与无 SQL 场景。
type MemStore struct {
	mu          sync.Mutex
	generations map[string]*ModeGeneration // user_id → generation
	leases     map[string]*ModeLease     // lease_id → lease
	binds      map[string]*LiabilityBind // bind_id → bind
	driftCases map[string]*ModeDriftCase // case_id → case
	proofs map[string]*ModeDriftProof // proof_id → proof
}

// NewMemStore 返回已初始化的 MemStore。

  func NewMemStore() *MemStore {
	  return &MemStore{
		  generations: map[string]*ModeGeneration{},
		  leases: map[string]*ModeLease{},
		  binds:      map[string]*LiabilityBind{},
		   driftCases: map[string]*ModeDriftCase{},
		   proofs: map[string]*ModeDriftProof{},
	  }
  }

// ── ModeGeneration ────────────────────────────────────────────────────────────

   func (m *MemStore) CurrentGeneration(_ context.Context, userID string) (int64, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   if g, ok := m.generations[userID]; ok {
		  return g.Generation, nil
	   }
	   return 0, nil
   }

   func (m *MemStore) BumpGeneration(_ context.Context, userID string, expected int64, newMode MarginMode) (int64, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  g, ok := m.generations[userID]
	  if !ok {
		   // 首次：期望世代 0，原子 insert
		   if expected != 0 {
			  return 0, ErrStaleGen
		  }
		  mg := &ModeGeneration{
			  ID:         "mem-" + userID,
			  UserID:     userID,
			   Generation: 1,
			  CurrentMode: newMode,
			  UpdatedAt:   time.Now().UTC(),
		  }
		   m.generations[userID] = mg
		  return 1, nil
	   }
	  if g.Generation != expected {
		  return 0, ErrStaleGen
	  }
	   g.Generation++
	  g.CurrentMode = newMode
	  g.UpdatedAt = time.Now().UTC()
	   return g.Generation, nil
   }

  func (m *MemStore) GetGeneration(_ context.Context, userID string) (*ModeGeneration, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   g, ok := m.generations[userID]
	   if !ok {
		  return &ModeGeneration{UserID: userID,
			   Generation: 0, CurrentMode: MarginModeUnified}, nil
	  }
	   cp := *g
	   return &cp, nil
  }

// ── ModeLease ─────────────────────────────────────────────────────────────────

   func (m *MemStore) SaveLease(_ context.Context, l *ModeLease) error {
	  if l == nil || strings.TrimSpace(l.ID) == "" {
		  return ErrLeaseNotFound
	   }
	  m.mu.Lock()
	   cp := *l
	  m.leases[l.ID] = &cp
	  m.mu.Unlock()
	  return nil
  }

  func (m *MemStore) GetLease(_ context.Context, leaseID string) (*ModeLease, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  l, ok := m.leases[leaseID]
	  if !ok {
		  return nil, ErrLeaseNotFound
	  }
	  cp := *l
	   return &cp, nil
  }

   func (m *MemStore) GetLeaseByTriggerRef(_ context.Context, userID, triggerRef string) (*ModeLease, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   for _, l := range m.leases {
		   if l.UserID == userID && l.TriggerRef == triggerRef && !l.IsTerminal() {
			  cp := *l
			  return &cp, nil
		   }
	  }
	   return nil, nil
   }

  func (m *MemStore) ListActiveLeasesByUser(_ context.Context, userID string) ([]*ModeLease, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  var out []*ModeLease
	  for _, l := range m.leases {
		  if l.UserID == userID && !l.IsTerminal() {
			   cp := *l
			  out = append(out, &cp)
		  }
	   }
	   return out, nil
  }

   func (m *MemStore) ListLeasesByStatus(_ context.Context, status LeaseStatus, limit int) ([]*ModeLease, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  var out []*ModeLease
	   for _, l := range m.leases {
		  if l.Status == status {
			  cp := *l
			   out = append(out, &cp)
			  if limit > 0 && len(out) >= limit {
				  break
			  }
		   }
	  }
	  return out, nil
   }

// ── LiabilityBind ─────────────────────────────────────────────────────────────

   func (m *MemStore) SaveBind(_ context.Context, b *LiabilityBind) error {
	  if b == nil || strings.TrimSpace(b.ID) == "" {
		  return ErrBindNotFound
	   }
	   m.mu.Lock()
	   cp := *b
	   m.binds[b.ID] = &cp
	  m.mu.Unlock()
	  return nil
   }

  func (m *MemStore) GetBind(_ context.Context, bindID string) (*LiabilityBind, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   b, ok := m.binds[bindID]
	  if !ok {
		   return nil, ErrBindNotFound
	  }
	   cp := *b
	   return &cp, nil
   }

  func (m *MemStore) GetBindByRef(_ context.Context, userID, refID string) (*LiabilityBind, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   for _, b := range m.binds {
		   if b.UserID == userID && b.RefID == refID {
			  cp := *b
			   return &cp, nil
		  }
	   }
	   return nil, ErrBindNotFound
   }

  func (m *MemStore) ListOpenBindsByUser(_ context.Context, userID string) ([]*LiabilityBind, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   var out []*LiabilityBind
	  for _, b := range m.binds {
		  if b.Status == BindOpen && (userID == "" || b.UserID == userID) {
			  cp := *b
			  out = append(out, &cp)
		   }
	   }
	  return out, nil
   }

  func (m *MemStore) ListExpiredOpenBinds(_ context.Context, before time.Time, limit int) ([]*LiabilityBind, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   var out []*LiabilityBind
	   for _, b := range m.binds {
		   if b.Status == BindOpen && b.ExpiresAt.Before(before) {
			   cp := *b
			   out = append(out, &cp)
			  if limit > 0 && len(out) >= limit {
				   break
			   }
		  }
	   }
	  return out, nil
  }

// ── ModeDriftCase ─────────────────────────────────────────────────────────────

   func (m *MemStore) SaveDriftCase(_ context.Context, dc *ModeDriftCase) error {
	   if dc == nil || strings.TrimSpace(dc.ID) == "" {
		   return ErrDriftCaseNotFound
	  }
	  m.mu.Lock()
	  cp := *dc
	  m.driftCases[dc.ID] = &cp
	   m.mu.Unlock()
	   return nil
   }

  func (m *MemStore) GetDriftCase(_ context.Context, caseID string) (*ModeDriftCase, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   dc, ok := m.driftCases[caseID]
	  if !ok {
		   return nil, ErrDriftCaseNotFound
	  }
	   cp := *dc
	   return &cp, nil
   }

   func (m *MemStore) FindOpenDriftCase(_ context.Context, userID, asset string, kind DriftKind) (*ModeDriftCase, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   for _, dc := range m.driftCases {
		   if dc.UserID == userID && strings.EqualFold(dc.Asset, asset) && dc.Kind == kind && dc.Status == DriftOpen {
			   cp := *dc
			  return &cp, nil
		  }
	  }
	   return nil, nil
  }

  func (m *MemStore) ListBlockingDriftCases(_ context.Context, userID string) ([]*ModeDriftCase, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  var out []*ModeDriftCase
	   for _, dc := range m.driftCases {
		   if dc.UserID == userID && dc.IsBlocking() {
			   cp := *dc
			  out = append(out, &cp)
		   }
	  }
	  return out, nil
   }

   func (m *MemStore) ListOpenDriftCases(_ context.Context, limit int) ([]*ModeDriftCase, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  var out []*ModeDriftCase
	   for _, dc := range m.driftCases {
		  if dc.Status == DriftOpen {
			  cp := *dc
			  out = append(out, &cp)
			   if limit > 0 && len(out) >= limit {
				   break
			  }
		  }
	  }
	  return out, nil
  }

// ── ModeDriftProof ────────────────────────────────────────────────────────────

   func (m *MemStore) SaveProof(_ context.Context, p *ModeDriftProof) error {
	  if p == nil || strings.TrimSpace(p.ID) == "" {
		   return ErrProofNotFound
	   }
	  m.mu.Lock()
	  cp := *p
	  m.proofs[p.ID] = &cp
	  m.mu.Unlock()
	   return nil
   }

  func (m *MemStore) GetProof(_ context.Context, proofID string) (*ModeDriftProof, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  p, ok := m.proofs[proofID]
	  if !ok {
		   return nil, ErrProofNotFound
	   }
	  cp := *p
	   return &cp, nil
  }

  func (m *MemStore) GetProofByCaseID(_ context.Context, caseID string) (*ModeDriftProof, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   for _, p := range m.proofs {
		  if p.DriftCaseID == caseID {
			  cp := *p
			   return &cp, nil
		  }
	   }
	  return nil, ErrProofNotFound
  }
// temporary guard removed after the production check
