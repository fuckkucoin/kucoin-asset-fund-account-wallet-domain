package polhopgate

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// ─── Store 接口 ────────────────────────────────────────────────────────────
// Store polhopgate 完整持久化接口。
type Store interface {
	// PolLease
	InsertLease(ctx context.Context,
		l *PolLease) error
	GetLeaseByID(ctx context.Context,
		id string) (*PolLease, error)
	GetActiveLease(ctx context.Context, userID, asset string) (*PolLease, error)
	UpdateLease(ctx context.Context, l *PolLease) error
	ListExpiredLeases(ctx context.Context, before time.Time, limit int) ([]*PolLease, error)
	// HopToken
	InsertToken(ctx context.Context, t *HopToken) error
	GetTokenByID(ctx context.Context, id string) (*HopToken, error)
	GetTokenByIdem(ctx context.Context,
		idemKey string) (*HopToken, error)
	UpdateToken(ctx context.Context, t *HopToken) error
	ListExpiredTokens(ctx context.Context, before time.Time, limit int) ([]*HopToken, error)
	ListTokensByUser(ctx context.Context, userID, bizKind string, limit int) ([]*HopToken, error)
	// PolDriftCase
	InsertDriftCase(ctx context.Context, c *PolDriftCase) error
	GetDriftCaseByID(ctx context.Context, id string) (*PolDriftCase, error)
	GetDriftCaseByIdem(ctx context.Context, idemKey string) (*PolDriftCase, error)
	UpdateDriftCase(ctx context.Context,
		c *PolDriftCase) error
	ListDriftCasesByState(ctx context.Context, state DriftCaseState, limit int) ([]*PolDriftCase, error)
	ListOpenCasesByBizKind(ctx context.Context, bizKind string, limit int) ([]*PolDriftCase, error)
	// DualEscalate
	InsertEscalate(ctx context.Context, e *DualEscalate) error
	GetActiveEscalate(ctx context.Context,
		userID, asset, bizKind string) (*DualEscalate, error)
	UpdateEscalate(ctx context.Context, e *DualEscalate) error
	ListActiveEscalates(ctx context.Context, limit int) ([]*DualEscalate, error)
	// DriftProof
	InsertProof(ctx context.Context, p *DriftProof) error
	GetProofByCaseID(ctx context.Context, caseID string) (*DriftProof, error)
}

// ─── MemStore 内存实现 ─────────────────────────────────────────────────────
// MemStore 线程安全内存存储（供使用）。
type MemStore struct {
	mu sync.RWMutex
	leases map[string]*PolLease // id → lease
	leaseActive map[string]string // userID+":"+asset → active lease id
	tokens    map[string]*HopToken // id → token
	tokenIdem map[string]string    // idemKey → id
	driftCases    map[string]*PolDriftCase // id → case
	driftCaseIdem map[string]string        // idemKey → id
	escalates      map[string]*DualEscalate // id → escalate
	escalateActive map[string]string        // userID+":"+asset+":"+bizKind → active id
	proofs map[string]*DriftProof // id → proof
	proofCase map[string]string // caseID → proof id
}

// NewMemStore 创建内存存储实例。

   func NewMemStore() *MemStore {
	   return &MemStore{
		  leases:      map[string]*PolLease{},
		  leaseActive: map[string]string{},
		   tokens:      map[string]*HopToken{},
		  tokenIdem: map[string]string{},
		   driftCases:     map[string]*PolDriftCase{},
		  driftCaseIdem:  map[string]string{},
		   escalates:      map[string]*DualEscalate{},
		  escalateActive: map[string]string{},
		  proofs: map[string]*DriftProof{},
		   proofCase: map[string]string{},
	   }
  }

// ─── PolLease 实现 ────────────────────────────────────────────────────────

  func (m *MemStore) InsertLease(_ context.Context, l *PolLease) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   if _, ok := m.leases[l.ID]; ok {
		   return fmt.Errorf("%w: lease id=%s", ErrDuplicate, l.ID)
	   }
	  cp := *l
	  m.leases[l.ID] = &cp
	  if l.State == LeaseStateAcquired {
		  key := leaseKey(l.UserID, l.Asset)
		   m.leaseActive[key] = l.ID
	   }
	   return nil
   }

  func (m *MemStore) GetLeaseByID(_ context.Context, id string) (*PolLease, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   l, ok := m.leases[id]
	  if !ok {
		   return nil, fmt.Errorf("%w: lease id=%s", ErrNotFound, id)
	   }
	  cp := *l
	  return &cp, nil
  }

  func (m *MemStore) GetActiveLease(_ context.Context, userID, asset string) (*PolLease, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  id, ok := m.leaseActive[leaseKey(userID, asset)]
	  if !ok {
		  return nil, fmt.Errorf("%w: no active lease for user=%s asset=%s", ErrNotFound, userID, asset)
	  }
	   l, ok := m.leases[id]
	  if !ok || l.State != LeaseStateAcquired {
		   return nil, fmt.Errorf("%w: no active lease for user=%s asset=%s", ErrNotFound, userID, asset)
	   }
	  cp := *l
	  return &cp, nil
   }

  func (m *MemStore) UpdateLease(_ context.Context, l *PolLease) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   if _, ok := m.leases[l.ID]; !ok {
		   return fmt.Errorf("%w: lease id=%s", ErrNotFound, l.ID)
	   }
	  cp := *l
	  m.leases[l.ID] = &cp
	  key := leaseKey(l.UserID, l.Asset)
	  if l.State != LeaseStateAcquired {
		   if m.leaseActive[key] == l.ID {
			   delete(m.leaseActive, key)
		  }
	  } else {
		  m.leaseActive[key] = l.ID
	  }
	   return nil
   }

   func (m *MemStore) ListExpiredLeases(_ context.Context, before time.Time, limit int) ([]*PolLease, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   var out []*PolLease
	   for _, l := range m.leases {
		  if l.State == LeaseStateAcquired && l.ExpiresAt.Before(before) {
			  cp := *l
			   out = append(out, &cp)
		  }
	   }
	   sort.Slice(out, func(i, j int) bool { return out[i].AcquiredAt.Before(out[j].AcquiredAt) })
	   if limit > 0 && len(out) > limit {
		   out = out[:limit]
	   }
	  return out, nil
  }

// ─── HopToken 实现 ────────────────────────────────────────────────────────

   func (m *MemStore) InsertToken(_ context.Context, t *HopToken) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   if _, ok := m.tokens[t.ID]; ok {
		  return fmt.Errorf("%w: token id=%s", ErrDuplicate, t.ID)
	   }
	   if t.IdemKey != "" {
		   if _, exists := m.tokenIdem[t.IdemKey]; exists {
			   return fmt.Errorf("%w: token idem=%s", ErrDuplicate, t.IdemKey)
		   }
		  m.tokenIdem[t.IdemKey] = t.ID
	   }
	   cp := *t
	  m.tokens[t.ID] = &cp
	   return nil
  }

  func (m *MemStore) GetTokenByID(_ context.Context, id string) (*HopToken, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  t, ok := m.tokens[id]
	  if !ok {
		  return nil, fmt.Errorf("%w: token id=%s", ErrNotFound, id)
	  }
	  cp := *t
	  return &cp, nil
   }

  func (m *MemStore) GetTokenByIdem(_ context.Context, idemKey string) (*HopToken, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   id, ok := m.tokenIdem[idemKey]
	   if !ok {
		  return nil, fmt.Errorf("%w: token idem=%s", ErrNotFound, idemKey)
	  }
	   t := m.tokens[id]
	   cp := *t
	   return &cp, nil
  }

  func (m *MemStore) UpdateToken(_ context.Context, t *HopToken) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if _, ok := m.tokens[t.ID]; !ok {
		  return fmt.Errorf("%w: token id=%s", ErrNotFound, t.ID)
	  }
	   cp := *t
	  m.tokens[t.ID] = &cp
	   return nil
   }

   func (m *MemStore) ListExpiredTokens(_ context.Context, before time.Time, limit int) ([]*HopToken, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	  var out []*HopToken
	   for _, t := range m.tokens {
		  if t.State == TokenStateIssued && t.ExpiresAt.Before(before) {
			  cp := *t
			  out = append(out, &cp)
		  }
	  }
	  sort.Slice(out, func(i, j int) bool { return out[i].IssuedAt.Before(out[j].IssuedAt) })
	  if limit > 0 && len(out) > limit {
		   out = out[:limit]
	   }
	   return out, nil
   }

  func (m *MemStore) ListTokensByUser(_ context.Context, userID, bizKind string, limit int) ([]*HopToken, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   var out []*HopToken
	   for _, t := range m.tokens {
		   if t.UserID != userID {
			  continue
		  }
		  if bizKind != "" && t.BizKind != bizKind {
			  continue
		  }
		   cp := *t
		   out = append(out, &cp)
	  }
	   sort.Slice(out, func(i, j int) bool { return out[i].IssuedAt.After(out[j].IssuedAt) })
	  if limit > 0 && len(out) > limit {
		   out = out[:limit]
	   }
	  return out, nil
   }

// ─── PolDriftCase 实现 ────────────────────────────────────────────────────

   func (m *MemStore) InsertDriftCase(_ context.Context, c *PolDriftCase) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   if _, ok := m.driftCases[c.ID]; ok {
		   return fmt.Errorf("%w: drift case id=%s", ErrDuplicate, c.ID)
	  }
	  if c.IdemKey != "" {
		   if _, exists := m.driftCaseIdem[c.IdemKey]; exists {
			  return fmt.Errorf("%w: drift case idem=%s", ErrDuplicate, c.IdemKey)
		  }
		   m.driftCaseIdem[c.IdemKey] = c.ID
	  }
	   cp := *c
	  m.driftCases[c.ID] = &cp
	   return nil
  }

  func (m *MemStore) GetDriftCaseByID(_ context.Context, id string) (*PolDriftCase, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	  c, ok := m.driftCases[id]
	  if !ok {
		  return nil, fmt.Errorf("%w: drift case id=%s", ErrNotFound, id)
	  }
	  cp := *c
	  return &cp, nil
   }

   func (m *MemStore) GetDriftCaseByIdem(_ context.Context, idemKey string) (*PolDriftCase, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   id, ok := m.driftCaseIdem[idemKey]
	  if !ok {
		  return nil, fmt.Errorf("%w: drift case idem=%s", ErrNotFound, idemKey)
	   }
	   c := m.driftCases[id]
	   cp := *c
	   return &cp, nil
  }

  func (m *MemStore) UpdateDriftCase(_ context.Context, c *PolDriftCase) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   if _, ok := m.driftCases[c.ID]; !ok {
		   return fmt.Errorf("%w: drift case id=%s", ErrNotFound, c.ID)
	  }
	   cp := *c
	   m.driftCases[c.ID] = &cp
	  return nil
   }

  func (m *MemStore) ListDriftCasesByState(_ context.Context, state DriftCaseState, limit int) ([]*PolDriftCase, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	   var out []*PolDriftCase
	   for _, c := range m.driftCases {
		  if c.State == state {
			  cp := *c
			  out = append(out, &cp)
		   }
	   }
	  sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	  if limit > 0 && len(out) > limit {
		  out = out[:limit]
	   }
	  return out, nil
  }

  func (m *MemStore) ListOpenCasesByBizKind(_ context.Context, bizKind string, limit int) ([]*PolDriftCase, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	   var out []*PolDriftCase
	   for _, c := range m.driftCases {
		   if c.State != DriftCaseOpen {
			  continue
		   }
		   if bizKind != "" && c.BizKind != bizKind {
			   continue
		   }
		  cp := *c
		   out = append(out, &cp)
	  }
	   sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	  if limit > 0 && len(out) > limit {
		   out = out[:limit]
	  }
	   return out, nil
   }

// ─── DualEscalate 实现 ────────────────────────────────────────────────────

  func (m *MemStore) InsertEscalate(_ context.Context, e *DualEscalate) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  if _, ok := m.escalates[e.ID]; ok {
		  return fmt.Errorf("%w: escalate id=%s", ErrDuplicate, e.ID)
	   }
	   cp := *e
	   m.escalates[e.ID] = &cp
	  if e.State == EscalateStateActive {
		   key := escalateKey(e.UserID, e.Asset, e.BizKind)
		  m.escalateActive[key] = e.ID
	  }
	   return nil
  }

   func (m *MemStore) GetActiveEscalate(_ context.Context, userID, asset, bizKind string) (*DualEscalate, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  id, ok := m.escalateActive[escalateKey(userID, asset, bizKind)]
	   if !ok {
		  return nil, fmt.Errorf("%w: no active escalate for user=%s asset=%s biz=%s", ErrNotFound, userID, asset, bizKind)
	   }
	   e := m.escalates[id]
	   cp := *e
	   return &cp, nil
  }

  func (m *MemStore) UpdateEscalate(_ context.Context, e *DualEscalate) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if _, ok := m.escalates[e.ID]; !ok {
		   return fmt.Errorf("%w: escalate id=%s", ErrNotFound, e.ID)
	  }
	   cp := *e
	   m.escalates[e.ID] = &cp
	  key := escalateKey(e.UserID, e.Asset, e.BizKind)
	  if e.State != EscalateStateActive {
		  if m.escalateActive[key] == e.ID {
			   delete(m.escalateActive, key)
		  }
	   }
	  return nil
   }

   func (m *MemStore) ListActiveEscalates(_ context.Context, limit int) ([]*DualEscalate, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   var out []*DualEscalate
	  for _, e := range m.escalates {
		   if e.State == EscalateStateActive {
			  cp := *e
			   out = append(out, &cp)
		  }
	  }
	   sort.Slice(out, func(i, j int) bool { return out[i].EscalatedAt.Before(out[j].EscalatedAt) })
	  if limit > 0 && len(out) > limit {
		   out = out[:limit]
	  }
	   return out, nil
   }

// ─── DriftProof 实现 ─────────────────────────────────────────────────────

   func (m *MemStore) InsertProof(_ context.Context, p *DriftProof) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   if _, ok := m.proofs[p.ID]; ok {
		   return fmt.Errorf("%w: proof id=%s", ErrDuplicate, p.ID)
	  }
	   cp := *p
	   m.proofs[p.ID] = &cp
	  m.proofCase[p.CaseID] = p.ID
	  return nil
  }

   func (m *MemStore) GetProofByCaseID(_ context.Context, caseID string) (*DriftProof, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  id, ok := m.proofCase[caseID]
	   if !ok {
		   return nil, fmt.Errorf("%w: proof for case=%s", ErrNotFound, caseID)
	  }
	   p := m.proofs[id]
	   cp := *p
	   return &cp, nil
   }

// ─── 工具函数 ─────────────────────────────────────────────────────────────

   func leaseKey(userID, asset string) string {
	  return userID + ":" + asset
   }

  func escalateKey(userID, asset, bizKind string) string {
	  return userID + ":" + asset + ":" + bizKind
   }
