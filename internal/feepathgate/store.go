package feepathgate

import (
	"context"
	"strings"
	"sync"
	"time"
)

// Store 存储接口；按业务对象分组，不暴露 SQL 细节。
type Store interface {
	// PathLease CRUD
	InsertLease(ctx context.Context, l *PathLease, expectedGen int64) error
	UpdateLease(ctx context.Context,
		l *PathLease) error
	GetLease(ctx context.Context, leaseID string) (*PathLease, error)
	ListLeasesByBizKey(ctx context.Context,
		bizKey string) ([]*PathLease, error)
	ListLeasesByStatus(ctx context.Context, status LeaseStatus, limit int) ([]*PathLease, error)
	// PathGeneration CAS
	GetGeneration(ctx context.Context, bizKey string) (*PathGeneration, error)
	UpsertGeneration(ctx context.Context, gen *PathGeneration) error
	// DualHit CRUD
	InsertDualHit(ctx context.Context, h *DualHit) error
	UpdateDualHit(ctx context.Context, h *DualHit) error
	GetDualHit(ctx context.Context,
		hitID string) (*DualHit, error)
	GetDualHitByBizKey(ctx context.Context, bizKey string) (*DualHit, error)
	ListDualHitsByStatus(ctx context.Context, status DualHitStatus) ([]*DualHit, error)
	// HedgeProof CRUD
	InsertHedgeProof(ctx context.Context, p *HedgeProof) error
	UpdateHedgeProof(ctx context.Context, p *HedgeProof) error
	GetHedgeProof(ctx context.Context, proofID string) (*HedgeProof, error)
	GetHedgeProofByPair(ctx context.Context, takeLeaseID, rebateLeaseID string) (*HedgeProof, error)
	ListHedgeProofsByStatus(ctx context.Context, status HedgeStatus) ([]*HedgeProof, error)
	// ReplayBlock
	InsertReplayBlock(ctx context.Context, b *ReplayBlock) error
	GetReplayBlock(ctx context.Context, idemKey string) (*ReplayBlock, error)
}

// MemStore 内存实现，线程安全，仅用于和单机场景。
// 多实例部署必须替换为 SQL Store（sql CAS 保证 generation 原子递增）。

type MemStore struct {
	mu sync.Mutex
	leases      map[string]*PathLease      // lease_id → lease
	generations map[string]*PathGeneration // biz_key → generation
	dualHits    map[string]*DualHit        // hit_id → DualHit
	hitByBiz map[string]string // biz_key → hit_id (open/proven only)
	proofs      map[string]*HedgeProof // proof_id → HedgeProof
	proofByPair map[string]string      // "take:rebate" → proof_id
	replayBlocks map[string]*ReplayBlock // idem_key → block
}

  func NewMemStore() *MemStore {
	  return &MemStore{
		   leases: make(map[string]*PathLease),
		   generations: make(map[string]*PathGeneration),
		  dualHits: make(map[string]*DualHit),
		  hitByBiz: make(map[string]string),
		   proofs:       make(map[string]*HedgeProof),
		   proofByPair:  make(map[string]string),
		  replayBlocks: make(map[string]*ReplayBlock),
	  }
   }

// InsertLease 原子 CAS：检查 generation == expectedGen，递增后写入 lease。
// 若 expectedGen 与当前值不符，返回 ErrStaleGen（ SQL UPDATE WHERE gen=?）。

  func (m *MemStore) InsertLease(ctx context.Context, l *PathLease, expectedGen int64) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   gen := m.generations[l.BusinessKey]
	   currentGen := int64(0)
	   if gen != nil {
		   currentGen = gen.Generation
	   }
	  if currentGen != expectedGen {
		  return ErrStaleGen
	  }
	  // 检查同 biz_key 是否已有反 Kind 活跃租约。
	  for _, existing := range m.leases {
		  if existing.BusinessKey == l.BusinessKey &&
			  (existing.Status == LeaseOpen || existing.Status == LeaseBound) &&
			  existing.Kind != l.Kind {
			  return ErrPathConflict
		  }
	   }
	   // 递增世代。
	  now := time.Now().UTC()
	   if gen == nil {
		   gen = &PathGeneration{
			   ID:          l.ID + "-gen",
			  BusinessKey: l.BusinessKey,
		  }
		  m.generations[l.BusinessKey] = gen
	   }
	   gen.Generation = expectedGen + 1
	  gen.LockedBy = l.ID
	  gen.LockedKind = l.Kind
	  gen.UpdatedAt = now
	   cp := *l
	  cp.Generation = expectedGen + 1
	  m.leases[l.ID] = &cp
	  return nil
  }

   func (m *MemStore) UpdateLease(_ context.Context, l *PathLease) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  if _, ok := m.leases[l.ID]; !ok {
		  return ErrLeaseNotFound
	  }
	   cp := *l
	   m.leases[l.ID] = &cp
	   return nil
   }

   func (m *MemStore) GetLease(_ context.Context, leaseID string) (*PathLease, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  l := m.leases[strings.TrimSpace(leaseID)]
	  if l == nil {
		  return nil, ErrLeaseNotFound
	  }
	  cp := *l
	  return &cp, nil
  }

  func (m *MemStore) ListLeasesByBizKey(_ context.Context, bizKey string) ([]*PathLease, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  var out []*PathLease
	   for _, l := range m.leases {
		   if l.BusinessKey == bizKey {
			  cp := *l
			   out = append(out, &cp)
		   }
	   }
	  return out, nil
  }

  func (m *MemStore) ListLeasesByStatus(_ context.Context, status LeaseStatus, limit int) ([]*PathLease, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  var out []*PathLease
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

  func (m *MemStore) GetGeneration(_ context.Context, bizKey string) (*PathGeneration, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   g := m.generations[bizKey]
	   if g == nil {
		   return nil, ErrLeaseNotFound
	   }
	   cp := *g
	  return &cp, nil
  }

   func (m *MemStore) UpsertGeneration(_ context.Context, gen *PathGeneration) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  cp := *gen
	  m.generations[gen.BusinessKey] = &cp
	   return nil
   }

   func (m *MemStore) InsertDualHit(_ context.Context, h *DualHit) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if _, ok := m.hitByBiz[h.BusinessKey]; ok {
		  return ErrDualHitExists
	   }
	   cp := *h
	   m.dualHits[h.ID] = &cp
	   m.hitByBiz[h.BusinessKey] = h.ID
	   return nil
   }

   func (m *MemStore) UpdateDualHit(_ context.Context, h *DualHit) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  if _, ok := m.dualHits[h.ID]; !ok {
		  return ErrLeaseNotFound
	   }
	   cp := *h
	   m.dualHits[h.ID] = &cp
	   if h.Status == DualHitClosed {
		   delete(m.hitByBiz, h.BusinessKey)
	   }
	  return nil
  }

   func (m *MemStore) GetDualHit(_ context.Context, hitID string) (*DualHit, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  h := m.dualHits[strings.TrimSpace(hitID)]
	  if h == nil {
		   return nil, ErrLeaseNotFound
	   }
	  cp := *h
	  return &cp, nil
   }

   func (m *MemStore) GetDualHitByBizKey(_ context.Context, bizKey string) (*DualHit, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  id := m.hitByBiz[bizKey]
	  if id == "" {
		   return nil, nil
	  }
	  h := m.dualHits[id]
	   if h == nil {
		   return nil, nil
	   }
	   cp := *h
	  return &cp, nil
   }

   func (m *MemStore) ListDualHitsByStatus(_ context.Context, status DualHitStatus) ([]*DualHit, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   var out []*DualHit
	  for _, h := range m.dualHits {
		  if h.Status == status {
			  cp := *h
			  out = append(out, &cp)
		  }
	   }
	  return out, nil
  }

  func (m *MemStore) InsertHedgeProof(_ context.Context, p *HedgeProof) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  cp := *p
	  if len(p.LedgerIDs) > 0 {
		   cp.LedgerIDs = append([]string(nil),
			   p.LedgerIDs...)
	   }
	  m.proofs[p.ID] = &cp
	   m.proofByPair[pairKey(p.TakeLeaseID,
		  p.RebateLeaseID)] = p.ID
	  return nil
   }

   func (m *MemStore) UpdateHedgeProof(_ context.Context, p *HedgeProof) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  if _, ok := m.proofs[p.ID]; !ok {
		   return ErrProofMissing
	  }
	  cp := *p
	  if len(p.LedgerIDs) > 0 {
		  cp.LedgerIDs = append([]string(nil), p.LedgerIDs...)
	   }
	  m.proofs[p.ID] = &cp
	  return nil
   }

  func (m *MemStore) GetHedgeProof(_ context.Context, proofID string) (*HedgeProof, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   p := m.proofs[strings.TrimSpace(proofID)]
	  if p == nil {
		   return nil, ErrProofMissing
	  }
	   return cloneProof(p), nil
   }

   func (m *MemStore) GetHedgeProofByPair(_ context.Context, takeLeaseID, rebateLeaseID string) (*HedgeProof, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  id := m.proofByPair[pairKey(takeLeaseID,
		   rebateLeaseID)]
	  if id == "" {
		   return nil, ErrProofMissing
	   }
	  p := m.proofs[id]
	   if p == nil {
		   return nil, ErrProofMissing
	   }
	  return cloneProof(p), nil
   }

   func (m *MemStore) ListHedgeProofsByStatus(_ context.Context, status HedgeStatus) ([]*HedgeProof, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  var out []*HedgeProof
	   for _, p := range m.proofs {
		  if p.Status == status {
			   out = append(out, cloneProof(p))
		   }
	  }
	  return out, nil
  }

   func (m *MemStore) InsertReplayBlock(_ context.Context, b *ReplayBlock) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  cp := *b
	   m.replayBlocks[b.IdemKey] = &cp
	  return nil
  }

  func (m *MemStore) GetReplayBlock(_ context.Context, idemKey string) (*ReplayBlock, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  b := m.replayBlocks[strings.TrimSpace(idemKey)]
	   if b == nil {
		  return nil, ErrLeaseNotFound
	   }
	   cp := *b
	  return &cp, nil
  }
