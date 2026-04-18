package intxfergate

import (
	"context"
	"fmt"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/fundint"
)

// ─── 错误定义 ──────────────────────────────────────────────────────────────

var (
	ErrDuplicate = fmt.Errorf("intxfergate: duplicate key")
	ErrNotFound = fmt.Errorf("intxfergate: not found")
	ErrBadState = fmt.Errorf("intxfergate: bad state transition")
)

// ─── Store 接口 ────────────────────────────────────────────────────────────
// Store 站内转账闸完整持久化接口。
type Store interface {
	fundint.GateStore
	// DebitLease 扩展（按 ID 查询，供 CommitLease 使用）
	GetDebitLeaseByID(ctx context.Context, id string) (*fundint.DebitLease, error)
	// MisrouteCase CRUD
	InsertMisrouteCase(ctx context.Context, c *MisrouteCase) error
	GetMisrouteCaseByID(ctx context.Context, id string) (*MisrouteCase, error)
	GetMisrouteCaseBySnapLedger(ctx context.Context, snapID, ledgerID string) (*MisrouteCase, error)
	UpdateMisrouteCase(ctx context.Context,
		c *MisrouteCase) error
	ListMisrouteCases(ctx context.Context, state MisrouteCaseState, limit int) ([]*MisrouteCase, error)
	// XferEffectProof CRUD
	InsertEffectProof(ctx context.Context,
		p *XferEffectProof) error
	GetEffectProofByID(ctx context.Context, id string) (*XferEffectProof, error)
	GetEffectProofBySnapLedger(ctx context.Context, snapID, ledgerID string) (*XferEffectProof, error)
	ListEffectProofs(ctx context.Context, fromUserID string, limit int) ([]*XferEffectProof, error)
	// SendRecord CRUD
	InsertSendRecord(ctx context.Context, r *SendRecord) error
	GetSendRecordByIdem(ctx context.Context, idemKey string) (*SendRecord, error)
	GetSendRecordByID(ctx context.Context, id string) (*SendRecord, error)
	UpdateSendRecord(ctx context.Context, r *SendRecord) error
	ListSendRecordsByState(ctx context.Context,
		state SendState, limit int) ([]*SendRecord, error)
}

// ─── MemStore 内存实现 ─────────────────────────────────────────────────────
// MemStore 线程安全内存存储（供使用）。
type MemStore struct {
	mu sync.RWMutex
	snaps map[string]*fundint.ResolveSnap // id → snap
	snapIdx map[string]string // idemKey → id
	leases   map[string]*fundint.DebitLease // id → lease
	leaseIdx map[string]string              // idemKey → id
	misroutes   map[string]*MisrouteCase // id → case
	misrouteIdx map[string]string        // snapID+":"+ledgerID → id
	proofs   map[string]*XferEffectProof // id → proof
	proofIdx map[string]string           // snapID+":"+ledgerID → id
	records    map[string]*SendRecord // id → record
	recordIdem map[string]string      // idemKey → id
}

// NewMemStore 创建内存存储实例。

   func NewMemStore() *MemStore {
	   return &MemStore{
		  snaps:    map[string]*fundint.ResolveSnap{},
		  snapIdx:  map[string]string{},
		  leases:   map[string]*fundint.DebitLease{},
		   leaseIdx: map[string]string{},
		   misroutes:   map[string]*MisrouteCase{},
		   misrouteIdx: map[string]string{},
		   proofs:      map[string]*XferEffectProof{},
		   proofIdx:    map[string]string{},
		  records:     map[string]*SendRecord{},
		   recordIdem: map[string]string{},
	   }
   }

// ─── ResolveSnap ───────────────────────────────────────────────────────────

  func (m *MemStore) InsertResolveSnap(_ context.Context, s *fundint.ResolveSnap) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  if _, ok := m.snaps[s.ID]; ok {
		   return ErrDuplicate
	   }
	   cp := *s
	  m.snaps[s.ID] = &cp
	   m.snapIdx[s.IdemKey] = s.ID
	  return nil
  }

  func (m *MemStore) GetResolveSnapByIdem(_ context.Context, idemKey string) (*fundint.ResolveSnap, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	   id, ok := m.snapIdx[idemKey]
	   if !ok {
		  return nil,
			   fundint.ErrSnapNotFound
	   }
	   s, ok := m.snaps[id]
	  if !ok {
		   return nil, fundint.ErrSnapNotFound
	  }
	  cp := *s
	   return &cp, nil
   }

   func (m *MemStore) UpdateResolveSnap(_ context.Context, s *fundint.ResolveSnap) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  if _, ok := m.snaps[s.ID]; !ok {
		  return ErrNotFound
	   }
	   cp := *s
	  m.snaps[s.ID] = &cp
	   return nil
  }

  func (m *MemStore) ListFreshSnapsByState(_ context.Context, state fundint.ResolveSnapState, limit int) ([]*fundint.ResolveSnap, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	  var out []*fundint.ResolveSnap
	   for _, s := range m.snaps {
		   if s.State == state {
			   cp := *s
			   out = append(out, &cp)
		   }
		   if limit > 0 && len(out) >= limit {
			  break
		   }
	   }
	  return out, nil
   }

// ─── DebitLease ────────────────────────────────────────────────────────────

  func (m *MemStore) InsertDebitLease(_ context.Context, l *fundint.DebitLease) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   if _, ok := m.leases[l.ID]; ok {
		  return ErrDuplicate
	   }
	  // IdemKey 全局唯一约束
	  if _, ok := m.leaseIdx[l.IdemKey]; ok {
		  return ErrDuplicate
	  }
	   cp := *l
	  m.leases[l.ID] = &cp
	  m.leaseIdx[l.IdemKey] = l.ID
	  return nil
  }

  func (m *MemStore) GetDebitLeaseByIdem(_ context.Context, idemKey string) (*fundint.DebitLease, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  id, ok := m.leaseIdx[idemKey]
	   if !ok {
		   return nil, fundint.ErrLeaseNotFound
	  }
	  l, ok := m.leases[id]
	  if !ok {
		   return nil, fundint.ErrLeaseNotFound
	  }
	   cp := *l
	   return &cp, nil
  }

   func (m *MemStore) GetDebitLeaseByID(_ context.Context, id string) (*fundint.DebitLease, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  l, ok := m.leases[id]
	   if !ok {
		   return nil, fundint.ErrLeaseNotFound
	  }
	   cp := *l
	  return &cp, nil
  }

   func (m *MemStore) UpdateDebitLease(_ context.Context, l *fundint.DebitLease) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   if _, ok := m.leases[l.ID]; !ok {
		  return ErrNotFound
	   }
	  cp := *l
	  m.leases[l.ID] = &cp
	  return nil
   }

  func (m *MemStore) ListExpiredLeases(_ context.Context, before time.Time, limit int) ([]*fundint.DebitLease, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	   var out []*fundint.DebitLease
	  for _, l := range m.leases {
		   if l.State == fundint.DebitLeaseHeld && l.ExpiresAt.Before(before) {
			   cp := *l
			  out = append(out, &cp)
		  }
		   if limit > 0 && len(out) >= limit {
			   break
		   }
	   }
	  return out, nil
   }

// ─── MisrouteCase ──────────────────────────────────────────────────────────

   func (m *MemStore) InsertMisrouteCase(_ context.Context, c *MisrouteCase) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  if _, ok := m.misroutes[c.ID]; ok {
		   return ErrDuplicate
	   }
	  key := c.SnapID + ":" + c.LedgerID
	  if _, ok := m.misrouteIdx[key]; ok {
		  return ErrDuplicate
	  }
	  cp := *c
	   m.misroutes[c.ID] = &cp
	   m.misrouteIdx[key] = c.ID
	  return nil
  }

   func (m *MemStore) GetMisrouteCaseByID(_ context.Context, id string) (*MisrouteCase, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  c, ok := m.misroutes[id]
	  if !ok {
		  return nil, ErrNotFound
	  }
	   cp := *c
	  return &cp, nil
  }

   func (m *MemStore) GetMisrouteCaseBySnapLedger(_ context.Context, snapID, ledgerID string) (*MisrouteCase, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  id, ok := m.misrouteIdx[snapID+":"+ledgerID]
	  if !ok {
		   return nil, ErrNotFound
	   }
	   c, ok := m.misroutes[id]
	  if !ok {
		  return nil, ErrNotFound
	   }
	  cp := *c
	  return &cp, nil
  }

   func (m *MemStore) UpdateMisrouteCase(_ context.Context, c *MisrouteCase) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  if _, ok := m.misroutes[c.ID]; !ok {
		   return ErrNotFound
	  }
	  cp := *c
	  m.misroutes[c.ID] = &cp
	  return nil
  }

  func (m *MemStore) ListMisrouteCases(_ context.Context, state MisrouteCaseState, limit int) ([]*MisrouteCase, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  var out []*MisrouteCase
	  for _, c := range m.misroutes {
		  if c.State == state {
			   cp := *c
			   out = append(out, &cp)
		   }
		  if limit > 0 && len(out) >= limit {
			  break
		  }
	   }
	  return out, nil
   }

// ─── XferEffectProof ───────────────────────────────────────────────────────

   func (m *MemStore) InsertEffectProof(_ context.Context, p *XferEffectProof) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   if _, ok := m.proofs[p.ID]; ok {
		   return ErrDuplicate
	   }
	  key := p.SnapID + ":" + p.LedgerID
	   if _, ok := m.proofIdx[key]; ok {
		   return ErrDuplicate
	  }
	  cp := *p
	   m.proofs[p.ID] = &cp
	  m.proofIdx[key] = p.ID
	   return nil
   }

  func (m *MemStore) GetEffectProofByID(_ context.Context, id string) (*XferEffectProof, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  p, ok := m.proofs[id]
	  if !ok {
		  return nil, ErrNotFound
	  }
	   cp := *p
	   return &cp, nil
  }

   func (m *MemStore) GetEffectProofBySnapLedger(_ context.Context, snapID, ledgerID string) (*XferEffectProof, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  id, ok := m.proofIdx[snapID+":"+ledgerID]
	   if !ok {
		  return nil, ErrNotFound
	  }
	  p, ok := m.proofs[id]
	   if !ok {
		  return nil, ErrNotFound
	  }
	   cp := *p
	   return &cp, nil
  }

  func (m *MemStore) ListEffectProofs(_ context.Context, fromUserID string, limit int) ([]*XferEffectProof, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  var out []*XferEffectProof
	   for _, p := range m.proofs {
		   if fromUserID == "" || p.FromUserID == fromUserID {
			   cp := *p
			  out = append(out, &cp)
		   }
		  if limit > 0 && len(out) >= limit {
			   break
		  }
	  }
	   return out, nil
  }

// ─── SendRecord ────────────────────────────────────────────────────────────

  func (m *MemStore) InsertSendRecord(_ context.Context, r *SendRecord) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   if _, ok := m.records[r.ID]; ok {
		  return ErrDuplicate
	   }
	  // IdemKey 全局唯一约束（ DB UNIQUE KEY）
	   if _, ok := m.recordIdem[r.IdemKey]; ok {
		   return ErrDuplicate
	  }
	   cp := *r
	   m.records[r.ID] = &cp
	   m.recordIdem[r.IdemKey] = r.ID
	  return nil
   }

   func (m *MemStore) GetSendRecordByIdem(_ context.Context, idemKey string) (*SendRecord, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   id, ok := m.recordIdem[idemKey]
	  if !ok {
		  return nil, ErrNotFound
	  }
	  r, ok := m.records[id]
	  if !ok {
		   return nil, ErrNotFound
	   }
	   cp := *r
	  return &cp, nil
  }

  func (m *MemStore) GetSendRecordByID(_ context.Context, id string) (*SendRecord, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  r, ok := m.records[id]
	  if !ok {
		  return nil, ErrNotFound
	  }
	   cp := *r
	   return &cp, nil
   }

  func (m *MemStore) UpdateSendRecord(_ context.Context, r *SendRecord) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   if _, ok := m.records[r.ID]; !ok {
		  return ErrNotFound
	  }
	  cp := *r
	  m.records[r.ID] = &cp
	   return nil
   }

   func (m *MemStore) ListSendRecordsByState(_ context.Context, state SendState, limit int) ([]*SendRecord, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   var out []*SendRecord
	   for _, r := range m.records {
		   if r.State == state {
			  cp := *r
			  out = append(out, &cp)
		   }
		   if limit > 0 && len(out) >= limit {
			  break
		  }
	  }
	  return out, nil
   }
