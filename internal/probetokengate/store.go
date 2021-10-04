package probetokengate

import (
	"context"
	"sort"
	"sync"
	"time"
)

// ─── Store 接口 ────────────────────────────────────────────────────────────
// Store probetokengate 完整持久化接口。

type Store interface {
	// ─── ProbeTokenLease ────────────────────────────────────────────
	InsertLease(ctx context.Context, l *ProbeTokenLease) error
	GetLeaseByID(ctx context.Context, id string) (*ProbeTokenLease, error)
	GetLeaseByIdem(ctx context.Context, idemKey string) (*ProbeTokenLease, error)
	UpdateLease(ctx context.Context,
		l *ProbeTokenLease) error
	// ListExpiredLeases 列举已超 TTL 仍为非终态的租约（供 LeaseExpiryWorker）。
	ListExpiredLeases(ctx context.Context, before time.Time, limit int) ([]*ProbeTokenLease, error)
	// ListActiveLeasesByScope 列举指定 scope 的 checked/bound 租约。
	ListActiveLeasesByScope(ctx context.Context, scopeKey string, limit int) ([]*ProbeTokenLease, error)
	// ListActiveLeasesBySubject 列举指定主体的 checked/bound 租约。
	ListActiveLeasesBySubject(ctx context.Context, subjectID string, limit int) ([]*ProbeTokenLease, error)
	// ─── ProbeTokenDriftProof ────────────────────────────────────────
	InsertProof(ctx context.Context,
		p *ProbeTokenDriftProof) error
	GetProofByID(ctx context.Context, id string) (*ProbeTokenDriftProof, error)
	GetProofByIdem(ctx context.Context, idemKey string) (*ProbeTokenDriftProof, error)
	GetOpenProofByLease(ctx context.Context, leaseID string) (*ProbeTokenDriftProof, error)
	UpdateProof(ctx context.Context, p *ProbeTokenDriftProof) error
	ListProofsByState(ctx context.Context, state ProofState, limit int) ([]*ProbeTokenDriftProof, error)
	ListOpenProofsBySubject(ctx context.Context, subjectID string, limit int) ([]*ProbeTokenDriftProof, error)
	// ─── WriteScopeBind ─────────────────────────────────────────────
	InsertBind(ctx context.Context, b *WriteScopeBind) error
	GetBindByLeaseID(ctx context.Context, leaseID string) (*WriteScopeBind, error)
	GetBindByCommitRef(ctx context.Context, commitRef string) (*WriteScopeBind, error)
	UpdateBind(ctx context.Context, b *WriteScopeBind) error
}

// ─── MemStore 内存实现 ─────────────────────────────────────────────────────
// MemStore 线程安全内存存储（供使用）。
// 深拷贝策略：所有读写操作均对结构体做值拷贝，防止调用方持有存储内部引用。
type MemStore struct {
	mu sync.RWMutex
	leases map[string]*ProbeTokenLease // id → lease
	leaseIdem map[string]string // idemKey → id
	proofs map[string]*ProbeTokenDriftProof // id → proof
	proofIdem map[string]string // idemKey → id
	proofByLease map[string]string // leaseID → open proof id
	binds map[string]*WriteScopeBind // id → bind
	bindByLease  map[string]string // leaseID → bind id
	bindByCommit map[string]string // commitRef → bind id
}

// NewMemStore 创建内存存储实例。
   func NewMemStore() *MemStore {
	  return &MemStore{
		  leases: map[string]*ProbeTokenLease{},
		   leaseIdem: map[string]string{},
		  proofs:    map[string]*ProbeTokenDriftProof{},
		  proofIdem: map[string]string{},
		  proofByLease: map[string]string{},
		  binds: map[string]*WriteScopeBind{},
		  bindByLease: map[string]string{},
		  bindByCommit: map[string]string{},
	  }
  }

// ─── 深拷贝辅助 ────────────────────────────────────────────────────────────

   func cloneLease(l *ProbeTokenLease) *ProbeTokenLease {
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

  func cloneProof(p *ProbeTokenDriftProof) *ProbeTokenDriftProof {
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

  func cloneBind(b *WriteScopeBind) *WriteScopeBind {
	  if b == nil {
		  return nil
	   }
	  c := *b
	   if b.ConsumedAt != nil {
		   t := *b.ConsumedAt
		  c.ConsumedAt = &t
	   }
	   return &c
   }

// ─── ProbeTokenLease 实现 ─────────────────────────────────────────────────

   func (m *MemStore) InsertLease(_ context.Context, l *ProbeTokenLease) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   if _, ok := m.leases[l.ID]; ok {
		  return ErrDuplicate
	   }
	   if l.IdemKey != "" {
		   if _, ok := m.leaseIdem[l.IdemKey]; ok {
			   return ErrDuplicate
		  }
	   }
	  m.leases[l.ID] = cloneLease(l)
	  if l.IdemKey != "" {
		  m.leaseIdem[l.IdemKey] = l.ID
	  }
	  return nil
  }

   func (m *MemStore) GetLeaseByID(_ context.Context, id string) (*ProbeTokenLease, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  l, ok := m.leases[id]
	   if !ok {
		  return nil, ErrNotFound
	  }
	  return cloneLease(l), nil
  }

   func (m *MemStore) GetLeaseByIdem(_ context.Context, idemKey string) (*ProbeTokenLease, error) {
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

   func (m *MemStore) UpdateLease(_ context.Context, l *ProbeTokenLease) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  if _, ok := m.leases[l.ID]; !ok {
		  return ErrNotFound
	  }
	  m.leases[l.ID] = cloneLease(l)
	  return nil
  }

  func (m *MemStore) ListExpiredLeases(_ context.Context, before time.Time, limit int) ([]*ProbeTokenLease, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  var out []*ProbeTokenLease
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

   func (m *MemStore) ListActiveLeasesByScope(_ context.Context, scopeKey string, limit int) ([]*ProbeTokenLease, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	   var out []*ProbeTokenLease
	  for _, l := range m.leases {
		   if l.State.Closed() {
			  continue
		  }
		  if l.ScopeKey == scopeKey {
			   out = append(out, cloneLease(l))
		  }
		  if limit > 0 && len(out) >= limit {
			  break
		  }
	   }
	  return out, nil
   }

  func (m *MemStore) ListActiveLeasesBySubject(_ context.Context, subjectID string, limit int) ([]*ProbeTokenLease, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	   var out []*ProbeTokenLease
	   for _, l := range m.leases {
		   if l.State.Closed() {
			  continue
		  }
		   if l.SubjectID == subjectID {
			   out = append(out, cloneLease(l))
		  }
		  if limit > 0 && len(out) >= limit {
			  break
		   }
	   }
	  return out, nil
   }

// ─── ProbeTokenDriftProof 实现 ────────────────────────────────────────────

  func (m *MemStore) InsertProof(_ context.Context, p *ProbeTokenDriftProof) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   if _, ok := m.proofs[p.ID]; ok {
		  return ErrDuplicate
	  }
	   if p.IdemKey != "" {
		   if _, ok := m.proofIdem[p.IdemKey]; ok {
			   return ErrDuplicate
		   }
	   }
	  m.proofs[p.ID] = cloneProof(p)
	  if p.IdemKey != "" {
		  m.proofIdem[p.IdemKey] = p.ID
	   }
	  if p.State == ProofStateOpen {
		  m.proofByLease[p.LeaseID] = p.ID
	  }
	   return nil
  }

   func (m *MemStore) GetProofByID(_ context.Context, id string) (*ProbeTokenDriftProof, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	   p, ok := m.proofs[id]
	   if !ok {
		   return nil, ErrNotFound
	   }
	   return cloneProof(p), nil
  }

   func (m *MemStore) GetProofByIdem(_ context.Context, idemKey string) (*ProbeTokenDriftProof, error) {
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

  func (m *MemStore) GetOpenProofByLease(_ context.Context, leaseID string) (*ProbeTokenDriftProof, error) {
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

  func (m *MemStore) UpdateProof(_ context.Context, p *ProbeTokenDriftProof) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if _, ok := m.proofs[p.ID]; !ok {
		  return ErrNotFound
	   }
	  m.proofs[p.ID] = cloneProof(p)
	   if p.State != ProofStateOpen {
		  delete(m.proofByLease, p.LeaseID)
	  }
	   return nil
   }

   func (m *MemStore) ListProofsByState(_ context.Context, state ProofState, limit int) ([]*ProbeTokenDriftProof, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   var out []*ProbeTokenDriftProof
	  for _, p := range m.proofs {
		  if p.State == state {
			  out = append(out, cloneProof(p))
		   }
		  if limit > 0 && len(out) >= limit {
			   break
		   }
	   }
	   sort.Slice(out, func(i, j int) bool {
		  return out[i].OpenedAt.Before(out[j].OpenedAt)
	   })
	   return out, nil
  }

  func (m *MemStore) ListOpenProofsBySubject(_ context.Context, subjectID string, limit int) ([]*ProbeTokenDriftProof, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	   var out []*ProbeTokenDriftProof
	  for _, p := range m.proofs {
		  if p.State == ProofStateOpen && p.SubjectID == subjectID {
			   out = append(out, cloneProof(p))
		   }
		  if limit > 0 && len(out) >= limit {
			   break
		   }
	  }
	  return out, nil
  }

// ─── WriteScopeBind 实现 ──────────────────────────────────────────────────

  func (m *MemStore) InsertBind(_ context.Context, b *WriteScopeBind) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   if _, ok := m.binds[b.ID]; ok {
		  return ErrDuplicate
	   }
	   if _, ok := m.bindByLease[b.LeaseID]; ok {
		  return ErrDuplicate
	  }
	  if b.CommitRef != "" {
		  if _, ok := m.bindByCommit[b.CommitRef]; ok {
			   return ErrDuplicate
		  }
	   }
	   m.binds[b.ID] = cloneBind(b)
	  m.bindByLease[b.LeaseID] = b.ID
	   if b.CommitRef != "" {
		  m.bindByCommit[b.CommitRef] = b.ID
	  }
	   return nil
  }

   func (m *MemStore) GetBindByLeaseID(_ context.Context, leaseID string) (*WriteScopeBind, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  id, ok := m.bindByLease[leaseID]
	  if !ok {
		  return nil, ErrNotFound
	  }
	   b, ok := m.binds[id]
	   if !ok {
		  return nil, ErrNotFound
	   }
	  return cloneBind(b), nil
   }

  func (m *MemStore) GetBindByCommitRef(_ context.Context, commitRef string) (*WriteScopeBind, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  id, ok := m.bindByCommit[commitRef]
	  if !ok {
		  return nil, ErrNotFound
	   }
	  b, ok := m.binds[id]
	   if !ok {
		  return nil, ErrNotFound
	   }
	  return cloneBind(b), nil
   }

  func (m *MemStore) UpdateBind(_ context.Context, b *WriteScopeBind) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  if _, ok := m.binds[b.ID]; !ok {
		   return ErrNotFound
	  }
	   m.binds[b.ID] = cloneBind(b)
	  return nil
   }
