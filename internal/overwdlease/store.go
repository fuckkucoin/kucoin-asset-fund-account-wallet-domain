package overwdlease

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// ─── Store 接口 ────────────────────────────────────────────────────────────
// Store overwdlease 完整持久化接口。
type Store interface {
	// PrecheckBind
	InsertBind(ctx context.Context, b *PrecheckBind) error
	GetBindByID(ctx context.Context, id string) (*PrecheckBind, error)
	GetBindByIdem(ctx context.Context, idemKey string) (*PrecheckBind, error)
	UpdateBind(ctx context.Context, b *PrecheckBind) error
	ListExpiredBinds(ctx context.Context,
		before time.Time, limit int) ([]*PrecheckBind, error)
	ListOpenBindsByUser(ctx context.Context, userID, asset string, limit int) ([]*PrecheckBind, error)
	// VarianceProof
	InsertProof(ctx context.Context, p *VarianceProof) error
	GetProofByID(ctx context.Context, id string) (*VarianceProof, error)
	GetProofByIdem(ctx context.Context, idemKey string) (*VarianceProof, error)
	GetOpenProofByUser(ctx context.Context, userID, asset string) (*VarianceProof, error)
	UpdateProof(ctx context.Context, p *VarianceProof) error
	ListProofsByState(ctx context.Context, state ProofState, limit int) ([]*VarianceProof, error)
	// OverwdDriftCase
	InsertDriftCase(ctx context.Context, c *OverwdDriftCase) error
	GetDriftCaseByID(ctx context.Context, id string) (*OverwdDriftCase, error)
	GetDriftCaseByIdem(ctx context.Context, idemKey string) (*OverwdDriftCase, error)
	UpdateDriftCase(ctx context.Context, c *OverwdDriftCase) error
	ListDriftCasesByState(ctx context.Context, state DriftCaseState, limit int) ([]*OverwdDriftCase, error)
	ListOpenCasesByUser(ctx context.Context, userID, asset string, limit int) ([]*OverwdDriftCase, error)
}

// ─── MemStore 内存实现 ─────────────────────────────────────────────────────
// MemStore 线程安全内存存储（供使用）。

type MemStore struct {
	mu sync.RWMutex
	binds    map[string]*PrecheckBind // id → bind
	bindIdem map[string]string        // idemKey → id
	proofs    map[string]*VarianceProof // id → proof
	proofIdem map[string]string         // idemKey → id
	proofOpenByUser map[string]string // userID+":"+asset → open proof id
	driftCases    map[string]*OverwdDriftCase // id → case
	driftCaseIdem map[string]string           // idemKey → id
}

// NewMemStore 创建内存存储实例。
   func NewMemStore() *MemStore {
	   return &MemStore{
		  binds:    map[string]*PrecheckBind{},
		  bindIdem: map[string]string{},
		  proofs: map[string]*VarianceProof{},
		   proofIdem:       map[string]string{},
		  proofOpenByUser: map[string]string{},
		  driftCases:      map[string]*OverwdDriftCase{},
		  driftCaseIdem: map[string]string{},
	   }
  }

// ─── PrecheckBind 实现 ────────────────────────────────────────────────────

   func (m *MemStore) InsertBind(_ context.Context, b *PrecheckBind) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   if _, ok := m.binds[b.ID]; ok {
		  return fmt.Errorf("%w: bind id=%s", ErrDuplicate, b.ID)
	   }
	   if b.IdemKey != "" {
		  if _, exists := m.bindIdem[b.IdemKey]; exists {
			   return fmt.Errorf("%w: bind idem=%s", ErrDuplicate, b.IdemKey)
		   }
		   m.bindIdem[b.IdemKey] = b.ID
	   }
	   cp := *b
	  m.binds[b.ID] = &cp
	   return nil
   }

  func (m *MemStore) GetBindByID(_ context.Context, id string) (*PrecheckBind, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   b, ok := m.binds[id]
	  if !ok {
		   return nil, fmt.Errorf("%w: bind id=%s", ErrNotFound, id)
	  }
	   cp := *b
	   return &cp, nil
   }

   func (m *MemStore) GetBindByIdem(_ context.Context, idemKey string) (*PrecheckBind, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	   id, ok := m.bindIdem[idemKey]
	   if !ok {
		  return nil, fmt.Errorf("%w: bind idem=%s", ErrNotFound, idemKey)
	  }
	  b := m.binds[id]
	  cp := *b
	   return &cp, nil
   }

  func (m *MemStore) UpdateBind(_ context.Context, b *PrecheckBind) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  if _, ok := m.binds[b.ID]; !ok {
		   return fmt.Errorf("%w: bind id=%s", ErrNotFound, b.ID)
	  }
	   cp := *b
	  m.binds[b.ID] = &cp
	   return nil
   }

   func (m *MemStore) ListExpiredBinds(_ context.Context, before time.Time, limit int) ([]*PrecheckBind, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  var out []*PrecheckBind
	  for _, b := range m.binds {
		   if b.State == BindStateOpen && b.ExpiresAt.Before(before) {
			  cp := *b
			  out = append(out, &cp)
		   }
	   }
	   sort.Slice(out, func(i, j int) bool { return out[i].PreCheckAt.Before(out[j].PreCheckAt) })
	  if limit > 0 && len(out) > limit {
		  out = out[:limit]
	  }
	  return out, nil
   }

   func (m *MemStore) ListOpenBindsByUser(_ context.Context, userID, asset string, limit int) ([]*PrecheckBind, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  var out []*PrecheckBind
	   for _, b := range m.binds {
		  if b.State != BindStateOpen {
			  continue
		  }
		   if b.UserID != userID {
			  continue
		   }
		   if asset != "" && b.Asset != asset {
			   continue
		   }
		   cp := *b
		   out = append(out, &cp)
	  }
	  sort.Slice(out, func(i, j int) bool { return out[i].PreCheckAt.After(out[j].PreCheckAt) })
	  if limit > 0 && len(out) > limit {
		  out = out[:limit]
	   }
	   return out, nil
  }

// ─── VarianceProof 实现 ───────────────────────────────────────────────────

   func (m *MemStore) InsertProof(_ context.Context, p *VarianceProof) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if _, ok := m.proofs[p.ID]; ok {
		   return fmt.Errorf("%w: proof id=%s", ErrDuplicate, p.ID)
	  }
	  if p.IdemKey != "" {
		  if _, exists := m.proofIdem[p.IdemKey]; exists {
			  return fmt.Errorf("%w: proof idem=%s", ErrDuplicate, p.IdemKey)
		   }
		  m.proofIdem[p.IdemKey] = p.ID
	   }
	   cp := *p
	  m.proofs[p.ID] = &cp
	  if p.State == ProofStateOpen {
		  key := userAssetKey(p.UserID, p.Asset)
		  m.proofOpenByUser[key] = p.ID
	  }
	  return nil
  }

  func (m *MemStore) GetProofByID(_ context.Context, id string) (*VarianceProof, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  p, ok := m.proofs[id]
	   if !ok {
		  return nil, fmt.Errorf("%w: proof id=%s", ErrNotFound, id)
	  }
	   cp := *p
	  return &cp, nil
   }

   func (m *MemStore) GetProofByIdem(_ context.Context, idemKey string) (*VarianceProof, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  id, ok := m.proofIdem[idemKey]
	   if !ok {
		  return nil, fmt.Errorf("%w: proof idem=%s", ErrNotFound, idemKey)
	   }
	  p := m.proofs[id]
	  cp := *p
	   return &cp, nil
  }

  func (m *MemStore) GetOpenProofByUser(_ context.Context, userID, asset string) (*VarianceProof, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  id, ok := m.proofOpenByUser[userAssetKey(userID, asset)]
	   if !ok {
		   return nil, fmt.Errorf("%w: no open proof for user=%s asset=%s", ErrNotFound, userID, asset)
	  }
	  p, ok := m.proofs[id]
	  if !ok || p.State != ProofStateOpen {
		  return nil, fmt.Errorf("%w: no open proof for user=%s asset=%s", ErrNotFound, userID, asset)
	   }
	  cp := *p
	  return &cp, nil
   }

   func (m *MemStore) UpdateProof(_ context.Context, p *VarianceProof) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   if _, ok := m.proofs[p.ID]; !ok {
		   return fmt.Errorf("%w: proof id=%s", ErrNotFound, p.ID)
	  }
	   cp := *p
	  m.proofs[p.ID] = &cp
	  key := userAssetKey(p.UserID,
		  p.Asset)
	   if p.State != ProofStateOpen {
		   if m.proofOpenByUser[key] == p.ID {
			   delete(m.proofOpenByUser, key)
		   }
	  } else {
		  m.proofOpenByUser[key] = p.ID
	   }
	  return nil
   }

  func (m *MemStore) ListProofsByState(_ context.Context,
	   state ProofState, limit int) ([]*VarianceProof, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	   var out []*VarianceProof
	   for _, p := range m.proofs {
		  if p.State == state {
			   cp := *p
			  out = append(out, &cp)
		  }
	  }
	  sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	   if limit > 0 && len(out) > limit {
		   out = out[:limit]
	   }
	  return out, nil
  }

// ─── OverwdDriftCase 实现 ─────────────────────────────────────────────────

  func (m *MemStore) InsertDriftCase(_ context.Context, c *OverwdDriftCase) error {
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

   func (m *MemStore) GetDriftCaseByID(_ context.Context, id string) (*OverwdDriftCase, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  c, ok := m.driftCases[id]
	  if !ok {
		  return nil, fmt.Errorf("%w: drift case id=%s", ErrNotFound, id)
	  }
	  cp := *c
	  return &cp, nil
   }

  func (m *MemStore) GetDriftCaseByIdem(_ context.Context, idemKey string) (*OverwdDriftCase, error) {
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

  func (m *MemStore) UpdateDriftCase(_ context.Context, c *OverwdDriftCase) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   if _, ok := m.driftCases[c.ID]; !ok {
		   return fmt.Errorf("%w: drift case id=%s", ErrNotFound, c.ID)
	  }
	  cp := *c
	  m.driftCases[c.ID] = &cp
	   return nil
   }

   func (m *MemStore) ListDriftCasesByState(_ context.Context, state DriftCaseState, limit int) ([]*OverwdDriftCase, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	   var out []*OverwdDriftCase
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

  func (m *MemStore) ListOpenCasesByUser(_ context.Context, userID, asset string, limit int) ([]*OverwdDriftCase, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   var out []*OverwdDriftCase
	   for _, c := range m.driftCases {
		  if c.State != DriftCaseOpen {
			  continue
		  }
		  if c.UserID != userID {
			  continue
		  }
		  if asset != "" && c.Asset != asset {
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

// ─── 工具函数 ─────────────────────────────────────────────────────────────

   func userAssetKey(userID, asset string) string {
	   return userID + ":" + asset
   }
