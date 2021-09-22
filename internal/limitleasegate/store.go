package limitleasegate

import (
	"context"
	"sort"
	"sync"
	"time"
)

// ─── Store 接口 ────────────────────────────────────────────────────────────
// Store limitleasegate 完整持久化接口。
type Store interface {
	// ─── LimitCheckLease ─────────────────────────────────────────────
	InsertLease(ctx context.Context, l *LimitCheckLease) error
	GetLeaseByID(ctx context.Context,
		id string) (*LimitCheckLease, error)
	GetLeaseByIdem(ctx context.Context, idemKey string) (*LimitCheckLease, error)
	UpdateLease(ctx context.Context, l *LimitCheckLease) error
	// ListExpiredLeases 列举已超 TTL 仍为非终态的租约（供 LeaseExpiryWorker）。
	ListExpiredLeases(ctx context.Context, before time.Time, limit int) ([]*LimitCheckLease, error)
	// ListActiveLeasesByUser 列举指定用户的 checked/bound 租约（供策略代次翻转批量吊销）。
	ListActiveLeasesByUser(ctx context.Context, userID string, limit int) ([]*LimitCheckLease, error)
	// ListActiveLeasesByPolicyVer 列举策略代次低于 newVer 的活跃租约（供 PolicyVersionRevokeWorker）。
	ListActiveLeasesByPolicyVer(ctx context.Context, maxVer int, limit int) ([]*LimitCheckLease, error)
	// ─── LimitDriftProof ─────────────────────────────────────────────
	InsertProof(ctx context.Context, p *LimitDriftProof) error
	GetProofByID(ctx context.Context, id string) (*LimitDriftProof, error)
	GetProofByIdem(ctx context.Context, idemKey string) (*LimitDriftProof, error)
	GetOpenProofByLease(ctx context.Context, leaseID string) (*LimitDriftProof, error)
	UpdateProof(ctx context.Context, p *LimitDriftProof) error
	ListProofsByState(ctx context.Context, state ProofState, limit int) ([]*LimitDriftProof, error)
	ListOpenProofsByUser(ctx context.Context, userID string, limit int) ([]*LimitDriftProof, error)
}

// ─── MemStore 内存实现 ─────────────────────────────────────────────────────
// MemStore 线程安全内存存储（供使用）。
// 深拷贝策略：所有读写操作均对结构体做值拷贝，防止调用方持有存储内部引用。

type MemStore struct {
	mu sync.RWMutex
	leases map[string]*LimitCheckLease // id → lease
	leaseIdem map[string]string // idemKey → id
	proofs map[string]*LimitDriftProof // id → proof
	proofIdem map[string]string // idemKey → id
	proofByLease map[string]string // leaseID → open proof id
}

// NewMemStore 创建内存存储实例。
  func NewMemStore() *MemStore {
	   return &MemStore{
		   leases:       map[string]*LimitCheckLease{},
		   leaseIdem:    map[string]string{},
		  proofs:       map[string]*LimitDriftProof{},
		  proofIdem:    map[string]string{},
		  proofByLease: map[string]string{},
	   }
   }

// ─── 深拷贝辅助 ────────────────────────────────────────────────────────────

   func cloneLease(l *LimitCheckLease) *LimitCheckLease {
	  if l == nil {
		  return nil
	  }
	   c := *l
	   if l.BoundAt != nil {
		  t := *l.BoundAt
		  c.BoundAt = &t
	   }
	  if l.ConsumedAt != nil {
		   t := *l.ConsumedAt
		  c.ConsumedAt = &t
	   }
	  if l.RevokedAt != nil {
		   t := *l.RevokedAt
		  c.RevokedAt = &t
	   }
	  return &c
   }

   func cloneProof(p *LimitDriftProof) *LimitDriftProof {
	  if p == nil {
		   return nil
	  }
	  c := *p
	  if p.ProvenAt != nil {
		   t := *p.ProvenAt
		   c.ProvenAt = &t
	   }
	   if p.ClosedAt != nil {
		   t := *p.ClosedAt
		  c.ClosedAt = &t
	  }
	  return &c
   }

// ─── LimitCheckLease 实现 ─────────────────────────────────────────────────

  func (m *MemStore) InsertLease(_ context.Context, l *LimitCheckLease) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  if _, ok := m.leases[l.ID]; ok {
		  return ErrDuplicate
	   }
	  if l.IdemKey != "" {
		  if _, ok := m.leaseIdem[l.IdemKey]; ok {
			  return ErrDuplicate
		   }
		  m.leaseIdem[l.IdemKey] = l.ID
	   }
	   m.leases[l.ID] = cloneLease(l)
	  return nil
   }

   func (m *MemStore) GetLeaseByID(_ context.Context, id string) (*LimitCheckLease, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  l, ok := m.leases[id]
	  if !ok {
		  return nil, ErrNotFound
	   }
	   return cloneLease(l), nil
   }

   func (m *MemStore) GetLeaseByIdem(_ context.Context, idemKey string) (*LimitCheckLease, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   id, ok := m.leaseIdem[idemKey]
	  if !ok {
		   return nil, ErrNotFound
	  }
	   l, ok := m.leases[id]
	   if !ok {
		  return nil, ErrNotFound
	  }
	   return cloneLease(l), nil
  }

  func (m *MemStore) UpdateLease(_ context.Context, l *LimitCheckLease) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   if _, ok := m.leases[l.ID]; !ok {
		   return ErrNotFound
	   }
	  m.leases[l.ID] = cloneLease(l)
	   return nil
   }

  func (m *MemStore) ListExpiredLeases(_ context.Context, before time.Time, limit int) ([]*LimitCheckLease, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	   var out []*LimitCheckLease
	   for _, l := range m.leases {
		   if l.State.Closed() {
			   continue
		  }
		   if l.ExpiresAt.Before(before) {
			  out = append(out, cloneLease(l))
		   }
		   if limit > 0 && len(out) >= limit {
			   break
		  }
	  }
	  sort.Slice(out, func(i, j int) bool {
		   return out[i].ExpiresAt.Before(out[j].ExpiresAt)
	  })
	   return out, nil
   }

  func (m *MemStore) ListActiveLeasesByUser(_ context.Context, userID string, limit int) ([]*LimitCheckLease, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	   var out []*LimitCheckLease
	   for _, l := range m.leases {
		  if l.State.Closed() {
			  continue
		  }
		  if l.UserID != userID {
			  continue
		   }
		   out = append(out, cloneLease(l))
		   if limit > 0 && len(out) >= limit {
			  break
		   }
	  }
	  return out, nil
   }

  func (m *MemStore) ListActiveLeasesByPolicyVer(_ context.Context, maxVer int, limit int) ([]*LimitCheckLease, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  var out []*LimitCheckLease
	   for _, l := range m.leases {
		  if l.State.Closed() {
			   continue
		   }
		  // 策略代次低于当前最新代次（maxVer）的活跃租约视为过期候选。
		  if l.PolicyVer < maxVer {
			   out = append(out, cloneLease(l))
		   }
		   if limit > 0 && len(out) >= limit {
			  break
		  }
	  }
	   sort.Slice(out, func(i, j int) bool {
		  return out[i].IssuedAt.Before(out[j].IssuedAt)
	   })
	   return out, nil
   }

// ─── LimitDriftProof 实现 ─────────────────────────────────────────────────

  func (m *MemStore) InsertProof(_ context.Context, p *LimitDriftProof) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  if _, ok := m.proofs[p.ID]; ok {
		   return ErrDuplicate
	  }
	   if p.IdemKey != "" {
		   if _, ok := m.proofIdem[p.IdemKey]; ok {
			   return ErrDuplicate
		   }
		   m.proofIdem[p.IdemKey] = p.ID
	  }
	  // 同一租约仅允许一个 open 证明。
	   if p.State == ProofStateOpen {
		   if _, ok := m.proofByLease[p.LeaseID]; ok {
			  return ErrDuplicate
		   }
		  m.proofByLease[p.LeaseID] = p.ID
	  }
	  m.proofs[p.ID] = cloneProof(p)
	   return nil
   }

   func (m *MemStore) GetProofByID(_ context.Context, id string) (*LimitDriftProof, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	   p, ok := m.proofs[id]
	   if !ok {
		  return nil, ErrNotFound
	   }
	  return cloneProof(p), nil
  }

   func (m *MemStore) GetProofByIdem(_ context.Context, idemKey string) (*LimitDriftProof, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	  id, ok := m.proofIdem[idemKey]
	  if !ok {
		  return nil, ErrNotFound
	   }
	   p, ok := m.proofs[id]
	  if !ok {
		   return nil, ErrNotFound
	  }
	  return cloneProof(p), nil
  }

   func (m *MemStore) GetOpenProofByLease(_ context.Context, leaseID string) (*LimitDriftProof, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  pid, ok := m.proofByLease[leaseID]
	  if !ok {
		  return nil, ErrNotFound
	   }
	  p, ok := m.proofs[pid]
	  if !ok {
		  return nil, ErrNotFound
	   }
	  if p.State != ProofStateOpen {
		   return nil, ErrNotFound
	  }
	  return cloneProof(p), nil
   }

   func (m *MemStore) UpdateProof(_ context.Context,
	   p *LimitDriftProof) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  existing, ok := m.proofs[p.ID]
	  if !ok {
		   return ErrNotFound
	   }
	  // 证明从 open 转出时，清理 proofByLease 索引。
	   if existing.State == ProofStateOpen && p.State != ProofStateOpen {
		   delete(m.proofByLease,
			  existing.LeaseID)
	  }
	   m.proofs[p.ID] = cloneProof(p)
	  return nil
   }

  func (m *MemStore) ListProofsByState(_ context.Context, state ProofState, limit int) ([]*LimitDriftProof, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  var out []*LimitDriftProof
	  for _, p := range m.proofs {
		   if p.State != state {
			  continue
		  }
		   out = append(out, cloneProof(p))
		  if limit > 0 && len(out) >= limit {
			   break
		  }
	  }
	   sort.Slice(out, func(i, j int) bool {
		  return out[i].OpenedAt.Before(out[j].OpenedAt)
	  })
	   return out, nil
  }

   func (m *MemStore) ListOpenProofsByUser(_ context.Context, userID string, limit int) ([]*LimitDriftProof, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	   var out []*LimitDriftProof
	   for _, p := range m.proofs {
		   if p.State != ProofStateOpen {
			  continue
		  }
		  if p.UserID != userID {
			   continue
		  }
		  out = append(out, cloneProof(p))
		  if limit > 0 && len(out) >= limit {
			   break
		  }
	  }
	   sort.Slice(out, func(i, j int) bool {
		  return out[i].OpenedAt.Before(out[j].OpenedAt)
	   })
	   return out, nil
  }
