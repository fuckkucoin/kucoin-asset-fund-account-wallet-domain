package viptiergate

import (
	"context"
	"strings"
	"sync"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// Store 接口
// ─────────────────────────────────────────────────────────────────────────────
// Store 是 viptiergate 所有实体的持久化抽象。
// 实现必须保证所有方法的并发安全。
type Store interface {
	// ── TierGeneration ──────────────────────────────────────────────────────
	// CurrentGeneration 返回用户当前的世代号。
	// 若记录不存在，返回 0（首次为 kyc0/standard 档，世代 0）。
	CurrentGeneration(ctx context.Context, userID string) (int64, error)
	// BumpGeneration CAS 递增世代：若当前世代 == expected，原子递增至 expected+1 并返回新值。
	// 若当前世代 != expected，返回 ErrStaleGen。
	BumpGeneration(ctx context.Context,
		userID string, expected int64, newTier Tier) (int64, error)
	// GetGeneration 返回完整 TierGeneration 记录。
	GetGeneration(ctx context.Context, userID string) (*TierGeneration, error)
	// ── TierLease ────────────────────────────────────────────────────────────
	// SaveLease 创建或更新 TierLease（按 ID 更新）。
	SaveLease(ctx context.Context,
		l *TierLease) error
	// GetLease 按 lease_id 查询。
	GetLease(ctx context.Context, leaseID string) (*TierLease, error)
	// GetLeaseByTriggerRef 幂等查找：按 user_id + trigger_ref 查找活跃租约。
	GetLeaseByTriggerRef(ctx context.Context,
		userID, triggerRef string) (*TierLease, error)
	// ListActiveLeasesByUser 返回用户所有非终态租约（未 switched）。
	ListActiveLeasesByUser(ctx context.Context, userID string) ([]*TierLease, error)
	// ListLeasesByStatus 按状态批量查询（供 worker 扫描）。
	ListLeasesByStatus(ctx context.Context, status LeaseStatus, limit int) ([]*TierLease, error)
	// ── QuoteBind ────────────────────────────────────────────────────────────
	// SaveQuoteBind 创建或更新 QuoteBind。
	SaveQuoteBind(ctx context.Context, q *QuoteBind) error
	// GetQuoteBind 按 quote_bind_id 查询。
	GetQuoteBind(ctx context.Context,
		id string) (*QuoteBind, error)
	// ListOpenQuoteBindsByUser 返回用户所有 open 状态 QuoteBind。
	// userID 为空时返回所有用户的 open QuoteBind（供 worker TTL 巡检）。
	ListOpenQuoteBindsByUser(ctx context.Context, userID string) ([]*QuoteBind, error)
	// ── InflightFreeze ───────────────────────────────────────────────────────
	// SaveInflightFreeze 创建或更新 InflightFreeze。
	SaveInflightFreeze(ctx context.Context, f *InflightFreeze) error
	// GetInflightFreeze 按 freeze_id 查询。
	GetInflightFreeze(ctx context.Context, id string) (*InflightFreeze, error)
	// ListActiveFreezesForUser 返回用户所有活跃（未释放）冻额。
	ListActiveFreezesForUser(ctx context.Context, userID string) ([]*InflightFreeze, error)
	// ── DriftCase ────────────────────────────────────────────────────────────
	// SaveDriftCase 创建或更新 DriftCase。
	SaveDriftCase(ctx context.Context, dc *DriftCase) error
	// GetDriftCase 按 drift_case_id 查询。
	GetDriftCase(ctx context.Context, id string) (*DriftCase, error)
	// FindOpenDriftCase 按 user_id + asset + kind 查找 open 状态案件（幂等用）。
	// 若不存在返回 (nil, ErrDriftCaseNotFound)。
	FindOpenDriftCase(ctx context.Context, userID, asset string, kind DriftKind) (*DriftCase, error)
	// ListOpenDriftCasesByUser 返回用户所有 open/proven 状态的漂移案。
	ListOpenDriftCasesByUser(ctx context.Context, userID string) ([]*DriftCase, error)
	// ListAllOpenDriftCases 返回所有 open/proven 状态的漂移案（供 worker 批量巡检）。
	ListAllOpenDriftCases(ctx context.Context, limit int) ([]*DriftCase, error)
	// ── DriftProof ───────────────────────────────────────────────────────────
	// SaveDriftProof 创建或更新 DriftProof。
	SaveDriftProof(ctx context.Context,
		p *DriftProof) error
	// GetDriftProof 按 proof_id 查询。
	GetDriftProof(ctx context.Context, proofID string) (*DriftProof, error)
}

// ─────────────────────────────────────────────────────────────────────────────
// MemStore 内存实现
// ─────────────────────────────────────────────────────────────────────────────
// MemStore 是 Store 的线程安全内存实现。
// 仅用于和无 SQL 环境；环境使用 SQLStore。
type MemStore struct {
	mu sync.RWMutex
	gens       map[string]*TierGeneration // key = userID
	leases     map[string]*TierLease      // key = lease_id
	quoteBinds map[string]*QuoteBind      // key = quote_bind_id
	freezes map[string]*InflightFreeze // key = freeze_id
	driftCases map[string]*DriftCase // key = drift_case_id
	driftProofs map[string]*DriftProof // key = proof_id
}

// NewMemStore 创建空的内存 Store。
  func NewMemStore() *MemStore {
	   return &MemStore{
		  gens: make(map[string]*TierGeneration),
		  leases:     make(map[string]*TierLease),
		   quoteBinds: make(map[string]*QuoteBind),
		  freezes:     make(map[string]*InflightFreeze),
		  driftCases:  make(map[string]*DriftCase),
		  driftProofs: make(map[string]*DriftProof),
	  }
  }

// ── TierGeneration ──────────────────────────────────────────────────────────

  func (m *MemStore) CurrentGeneration(_ context.Context, userID string) (int64, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  if g, ok := m.gens[userID]; ok {
		   return g.Generation, nil
	  }
	   return 0, nil
  }

   func (m *MemStore) BumpGeneration(_ context.Context, userID string, expected int64, newTier Tier) (int64, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  current := int64(0)
	   if g, ok := m.gens[userID]; ok {
		  current = g.Generation
	   }
	   if current != expected {
		   return 0, ErrStaleGen
	   }
	  next := expected + 1
	   g := &TierGeneration{
		   ID: "gen-" + userID,
		   UserID:     userID,
		  Generation: next,
		   LastTier:   newTier,
		   UpdatedAt: time.Now().UTC(),
	   }
	   m.gens[userID] = g
	   return next, nil
  }

  func (m *MemStore) GetGeneration(_ context.Context, userID string) (*TierGeneration, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	  if g, ok := m.gens[userID]; ok {
		  cp := *g
		  return &cp, nil
	   }
	  return &TierGeneration{ID: "gen-" + userID, UserID: userID, Generation: 0}, nil
   }

// ── TierLease ────────────────────────────────────────────────────────────────

  func (m *MemStore) SaveLease(_ context.Context, l *TierLease) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  cp := l.Copy()
	  m.leases[l.ID] = &cp
	   return nil
   }

  func (m *MemStore) GetLease(_ context.Context, leaseID string) (*TierLease, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   if l, ok := m.leases[leaseID]; ok {
		  cp := l.Copy()
		   return &cp, nil
	  }
	   return nil, ErrLeaseNotFound
  }

   func (m *MemStore) GetLeaseByTriggerRef(_ context.Context, userID, triggerRef string) (*TierLease, error) {
	  if triggerRef == "" {
		   return nil, ErrLeaseNotFound
	  }
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   for _, l := range m.leases {
		  if l.UserID == userID && l.TriggerRef == triggerRef && !l.IsTerminal() {
			   cp := l.Copy()
			   return &cp, nil
		   }
	   }
	  return nil, ErrLeaseNotFound
  }

   func (m *MemStore) ListActiveLeasesByUser(_ context.Context, userID string) ([]*TierLease, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   var out []*TierLease
	   for _, l := range m.leases {
		   if l.UserID == userID && !l.IsTerminal() {
			   cp := l.Copy()
			  out = append(out, &cp)
		   }
	   }
	   return out, nil
  }

   func (m *MemStore) ListLeasesByStatus(_ context.Context, status LeaseStatus, limit int) ([]*TierLease, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   var out []*TierLease
	  for _, l := range m.leases {
		   if limit > 0 && len(out) >= limit {
			  break
		  }
		  if l.Status == status {
			  cp := l.Copy()
			   out = append(out, &cp)
		  }
	  }
	  return out, nil
  }

// ── QuoteBind ────────────────────────────────────────────────────────────────

   func (m *MemStore) SaveQuoteBind(_ context.Context, q *QuoteBind) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  cp := q.Copy()
	   m.quoteBinds[q.ID] = &cp
	  return nil
  }

  func (m *MemStore) GetQuoteBind(_ context.Context, id string) (*QuoteBind, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  if q, ok := m.quoteBinds[id]; ok {
		   cp := q.Copy()
		   return &cp, nil
	   }
	   return nil, ErrQuoteBindNotFound
  }

   func (m *MemStore) ListOpenQuoteBindsByUser(_ context.Context, userID string) ([]*QuoteBind, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   var out []*QuoteBind
	   for _, q := range m.quoteBinds {
		  if q.Status != QuoteBindOpen {
			  continue
		   }
		   if userID != "" && q.UserID != userID {
			  continue
		  }
		   cp := q.Copy()
		  out = append(out, &cp)
	   }
	  return out, nil
   }

// ── InflightFreeze ───────────────────────────────────────────────────────────

   func (m *MemStore) SaveInflightFreeze(_ context.Context, f *InflightFreeze) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   cp := *f
	  m.freezes[f.ID] = &cp
	  return nil
   }

  func (m *MemStore) GetInflightFreeze(_ context.Context, id string) (*InflightFreeze, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   if f, ok := m.freezes[id]; ok {
		  cp := *f
		   return &cp, nil
	  }
	   return nil, ErrLeaseNotFound
  }

   func (m *MemStore) ListActiveFreezesForUser(_ context.Context, userID string) ([]*InflightFreeze, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   var out []*InflightFreeze
	   for _, f := range m.freezes {
		   if f.UserID != userID {
			   continue
		   }
		   if f.ReleasedAt != nil {
			  continue
		  }
		   cp := *f
		  out = append(out, &cp)
	   }
	   return out, nil
   }

// ── DriftCase ────────────────────────────────────────────────────────────────

  func (m *MemStore) SaveDriftCase(_ context.Context, dc *DriftCase) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  cp := dc.Copy()
	  m.driftCases[dc.ID] = &cp
	   return nil
  }

  func (m *MemStore) GetDriftCase(_ context.Context, id string) (*DriftCase, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  if dc, ok := m.driftCases[id]; ok {
		  cp := dc.Copy()
		  return &cp, nil
	   }
	   return nil, ErrDriftCaseNotFound
  }

  func (m *MemStore) FindOpenDriftCase(_ context.Context, userID, asset string, kind DriftKind) (*DriftCase, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  asset = strings.ToUpper(asset)
	   for _, dc := range m.driftCases {
		   if dc.UserID == userID && dc.Asset == asset && dc.Kind == kind && dc.Status == DriftOpen {
			   cp := dc.Copy()
			   return &cp, nil
		   }
	  }
	  return nil, ErrDriftCaseNotFound
  }

   func (m *MemStore) ListOpenDriftCasesByUser(_ context.Context, userID string) ([]*DriftCase, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  var out []*DriftCase
	  for _, dc := range m.driftCases {
		  if dc.UserID == userID && dc.IsBlocking() {
			   cp := dc.Copy()
			  out = append(out, &cp)
		   }
	  }
	  return out, nil
  }

   func (m *MemStore) ListAllOpenDriftCases(_ context.Context, limit int) ([]*DriftCase, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   var out []*DriftCase
	   for _, dc := range m.driftCases {
		  if limit > 0 && len(out) >= limit {
			  break
		  }
		  if dc.IsBlocking() {
			   cp := dc.Copy()
			   out = append(out, &cp)
		   }
	  }
	   return out, nil
   }

// ── DriftProof ───────────────────────────────────────────────────────────────

   func (m *MemStore) SaveDriftProof(_ context.Context, p *DriftProof) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   cp := p.Copy()
	  m.driftProofs[p.ID] = &cp
	   return nil
  }

  func (m *MemStore) GetDriftProof(_ context.Context, proofID string) (*DriftProof, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   if p, ok := m.driftProofs[proofID]; ok {
		   cp := p.Copy()
		  return &cp, nil
	  }
	  return nil, ErrProofNotFound
   }
