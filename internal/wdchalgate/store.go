package wdchalgate

import (
	"context"
	"sort"
	"sync"
	"time"
)

// ─── Store 接口 ────────────────────────────────────────────────────────────
// Store wdchalgate 完整持久化接口。

type Store interface {
	// ─── ChallengeLease ────────────────────────────────────────────
	InsertLease(ctx context.Context,
		l *ChallengeLease) error
	GetLeaseByID(ctx context.Context, id string) (*ChallengeLease, error)
	GetLeaseByIdem(ctx context.Context, idemKey string) (*ChallengeLease, error)
	UpdateLease(ctx context.Context, l *ChallengeLease) error
	// ListExpiredLeases 列举已超 TTL 仍为非终态的租约（供 LeaseExpiryWorker）。
	ListExpiredLeases(ctx context.Context,
		before time.Time, limit int) ([]*ChallengeLease, error)
	// ListOpenLeasesByUser 列举用户维度 issued/presented 租约。
	ListOpenLeasesByUser(ctx context.Context, userID string, limit int) ([]*ChallengeLease, error)
	// ListLeasesByUserFactor 列举指定用户+因子的非终态租约（因子变更监听）。
	ListLeasesByUserFactor(ctx context.Context, userID, factor string, limit int) ([]*ChallengeLease, error)
	// ─── StaleChalProof ────────────────────────────────────────────
	InsertProof(ctx context.Context, p *StaleChalProof) error
	GetProofByID(ctx context.Context, id string) (*StaleChalProof, error)
	GetProofByIdem(ctx context.Context,
		idemKey string) (*StaleChalProof, error)
	GetOpenProofByLease(ctx context.Context, leaseID string) (*StaleChalProof, error)
	UpdateProof(ctx context.Context, p *StaleChalProof) error
	ListProofsByState(ctx context.Context, state ProofState, limit int) ([]*StaleChalProof, error)
	ListOpenProofsByUser(ctx context.Context, userID string, limit int) ([]*StaleChalProof, error)
	// ─── ComboGeneration ────────────────────────────────────────────
	InsertComboGen(ctx context.Context,
		g *ComboGeneration) error
	GetCurrentGen(ctx context.Context, userID string) (int64, string, error)
	ListComboGens(ctx context.Context, userID string, limit int) ([]*ComboGeneration, error)
}

// ─── MemStore 内存实现 ─────────────────────────────────────────────────────
// MemStore 线程安全内存存储（供使用）。
// 深拷贝策略：所有读写操作均对结构体做值拷贝，防止调用方持有存储内部引用。

type MemStore struct {
	mu sync.RWMutex
	leases    map[string]*ChallengeLease // id → lease
	leaseIdem map[string]string          // idemKey → id
	proofs       map[string]*StaleChalProof // id → proof
	proofIdem    map[string]string          // idemKey → id
	proofByLease map[string]string          // leaseID → open proof id
	comboGens map[string][]*ComboGeneration // userID → ordered list (asc gen)
}

// NewMemStore 创建内存存储实例。
   func NewMemStore() *MemStore {
	  return &MemStore{
		  leases:    map[string]*ChallengeLease{},
		  leaseIdem: map[string]string{},
		  proofs:    map[string]*StaleChalProof{},
		  proofIdem: map[string]string{},
		  proofByLease: map[string]string{},
		  comboGens:    map[string][]*ComboGeneration{},
	   }
   }

// cloneLease 深拷贝 ChallengeLease。

   func cloneLease(l *ChallengeLease) *ChallengeLease {
	  if l == nil {
		  return nil
	   }
	  c := *l
	  if l.PresentedAt != nil {
		  t := *l.PresentedAt
		   c.PresentedAt = &t
	  }
	  if l.ConsumedAt != nil {
		   t := *l.ConsumedAt
		  c.ConsumedAt = &t
	   }
	  if l.StaleAt != nil {
		   t := *l.StaleAt
		  c.StaleAt = &t
	   }
	  return &c
  }

// cloneProof 深拷贝 StaleChalProof。
   func cloneProof(p *StaleChalProof) *StaleChalProof {
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

// ─── ChallengeLease 实现 ──────────────────────────────────────────────────

   func (m *MemStore) InsertLease(_ context.Context, l *ChallengeLease) error {
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

  func (m *MemStore) GetLeaseByID(_ context.Context, id string) (*ChallengeLease, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	   l, ok := m.leases[id]
	   if !ok {
		  return nil, ErrNotFound
	  }
	  return cloneLease(l), nil
   }

   func (m *MemStore) GetLeaseByIdem(_ context.Context, idemKey string) (*ChallengeLease, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	   id, ok := m.leaseIdem[idemKey]
	  if !ok {
		   return nil, ErrNotFound
	  }
	   return cloneLease(m.leases[id]), nil
   }

  func (m *MemStore) UpdateLease(_ context.Context, l *ChallengeLease) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   if _, ok := m.leases[l.ID]; !ok {
		  return ErrNotFound
	  }
	   m.leases[l.ID] = cloneLease(l)
	  return nil
  }

   func (m *MemStore) ListExpiredLeases(_ context.Context, before time.Time, limit int) ([]*ChallengeLease, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	  var out []*ChallengeLease
	  for _, l := range m.leases {
		  if l.State.Closed() {
			  continue
		  }
		  if l.ExpiresAt.Before(before) {
			  out = append(out, cloneLease(l))
			  if limit > 0 && len(out) >= limit {
				  break
			  }
		  }
	   }
	  return out, nil
   }

   func (m *MemStore) ListOpenLeasesByUser(_ context.Context, userID string, limit int) ([]*ChallengeLease, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	   var out []*ChallengeLease
	   for _, l := range m.leases {
		  if l.UserID != userID || l.State.Closed() {
			   continue
		  }
		  out = append(out, cloneLease(l))
		  if limit > 0 && len(out) >= limit {
			   break
		   }
	  }
	  return out, nil
   }

   func (m *MemStore) ListLeasesByUserFactor(_ context.Context, userID, factor string, limit int) ([]*ChallengeLease, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  var out []*ChallengeLease
	  for _, l := range m.leases {
		   if l.UserID != userID || l.Factor != factor || l.State.Closed() {
			   continue
		   }
		  out = append(out, cloneLease(l))
		  if limit > 0 && len(out) >= limit {
			  break
		  }
	   }
	   return out, nil
   }

// ─── StaleChalProof 实现 ──────────────────────────────────────────────────

  func (m *MemStore) InsertProof(_ context.Context, p *StaleChalProof) error {
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
	   if p.State == ProofStateOpen && p.LeaseID != "" {
		   m.proofByLease[p.LeaseID] = p.ID
	  }
	   return nil
  }

  func (m *MemStore) GetProofByID(_ context.Context, id string) (*StaleChalProof, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   p, ok := m.proofs[id]
	  if !ok {
		   return nil, ErrNotFound
	   }
	   return cloneProof(p), nil
  }

   func (m *MemStore) GetProofByIdem(_ context.Context, idemKey string) (*StaleChalProof, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   id, ok := m.proofIdem[idemKey]
	   if !ok {
		   return nil, ErrNotFound
	  }
	   return cloneProof(m.proofs[id]), nil
   }

   func (m *MemStore) GetOpenProofByLease(_ context.Context, leaseID string) (*StaleChalProof, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  id, ok := m.proofByLease[leaseID]
	  if !ok {
		  return nil, ErrNotFound
	   }
	   p := m.proofs[id]
	   if p == nil || p.State != ProofStateOpen {
		   return nil, ErrNotFound
	   }
	   return cloneProof(p), nil
   }

   func (m *MemStore) UpdateProof(_ context.Context, p *StaleChalProof) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  old, ok := m.proofs[p.ID]
	  if !ok {
		  return ErrNotFound
	  }
	  // 若从 open 转出，清理 proofByLease 索引。
	   if old.State == ProofStateOpen && p.State != ProofStateOpen {
		   delete(m.proofByLease, old.LeaseID)
	  }
	  m.proofs[p.ID] = cloneProof(p)
	   return nil
   }

   func (m *MemStore) ListProofsByState(_ context.Context, state ProofState, limit int) ([]*StaleChalProof, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   var out []*StaleChalProof
	   for _, p := range m.proofs {
		   if p.State != state {
			   continue
		  }
		  out = append(out, cloneProof(p))
		  if limit > 0 && len(out) >= limit {
			  break
		   }
	   }
	  return out, nil
  }

   func (m *MemStore) ListOpenProofsByUser(_ context.Context, userID string, limit int) ([]*StaleChalProof, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   var out []*StaleChalProof
	   for _, p := range m.proofs {
		  if p.UserID != userID || p.State != ProofStateOpen {
			   continue
		  }
		  out = append(out, cloneProof(p))
		   if limit > 0 && len(out) >= limit {
			   break
		   }
	  }
	   return out, nil
   }

// ─── ComboGeneration 实现 ─────────────────────────────────────────────────

  func (m *MemStore) InsertComboGen(_ context.Context, g *ComboGeneration) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   list := m.comboGens[g.UserID]
	   // 检查 gen 单调性。
	  if len(list) > 0 && list[len(list)-1].Gen >= g.Gen {
		  return ErrDuplicate
	  }
	   cp := *g
	  m.comboGens[g.UserID] = append(list, &cp)
	  return nil
   }

  func (m *MemStore) GetCurrentGen(_ context.Context, userID string) (int64, string, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   list := m.comboGens[userID]
	   if len(list) == 0 {
		  return 0, "", nil
	  }
	  // 排序确保正确性（写入已保证有序，此处兜底）。
	  sort.Slice(list, func(i, j int) bool { return list[i].Gen < list[j].Gen })
	  last := list[len(list)-1]
	  return last.Gen, last.FactorSig, nil
   }

  func (m *MemStore) ListComboGens(_ context.Context, userID string, limit int) ([]*ComboGeneration, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   list := m.comboGens[userID]
	   var out []*ComboGeneration
	  for i := len(list) - 1; i >= 0 && (limit <= 0 || len(out) < limit); i-- {
		  cp := *list[i]
		   out = append(out, &cp)
	   }
	  return out, nil
  }
