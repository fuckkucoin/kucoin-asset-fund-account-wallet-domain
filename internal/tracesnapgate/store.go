package tracesnapgate

import (
	"context"
	"sort"
	"strings"
	"sync"
)

const defaultMaxItems = 10_000

// ─── Store 接口 ────────────────────────────────────────────────────────────
// Store tracesnapgate 持久化接口。
type Store interface {
	// PutSnapshot 写入或更新快照（以 ID 为主键）。
	PutSnapshot(ctx context.Context, s TraceSnapshot) error
	// GetSnapshot 按 ID 读取快照。
	GetSnapshot(ctx context.Context, id string) (TraceSnapshot, error)
	// SnapshotsByUser 返回用户所有快照（按 CreatedAt 降序）。
	SnapshotsByUser(ctx context.Context, userID string, limit int) ([]TraceSnapshot, error)
	// SnapshotByIdemKey 按幂等键查快照（防重）。
	SnapshotByIdemKey(ctx context.Context, idemKey string) (TraceSnapshot, bool, error)
	// LatestSealedByUserAsset 返回用户指定资产最新 sealed/exported 快照。
	LatestSealedByUserAsset(ctx context.Context, userID, asset string) (TraceSnapshot, bool, error)
	// PutProof 写入漂移证明（以 ID 为主键）。
	PutProof(ctx context.Context,
		p TraceDriftProof) error
	// GetProof 按 ID 读取漂移证明。
	GetProof(ctx context.Context, id string) (TraceDriftProof, error)
	// ProofsBySnapshot 返回指定快照的所有漂移证明。
	ProofsBySnapshot(ctx context.Context, snapshotID string) ([]TraceDriftProof, error)
	// ProofByHash 按指纹查证明（防重）。
	ProofByHash(ctx context.Context, hash string) (TraceDriftProof, bool, error)
	// PutBind 写入或更新调查案绑定（以 ID 为主键）。
	PutBind(ctx context.Context, b InvestigateCaseBind) error
	// GetBind 按 ID 读取绑定。
	GetBind(ctx context.Context, id string) (InvestigateCaseBind, error)
	// BindByCaseID 按调查案 ID 查绑定（一案最多一绑）。
	BindByCaseID(ctx context.Context, caseID string) (InvestigateCaseBind, bool, error)
	// BindsBySnapshot 返回指定快照的所有案件绑定。
	BindsBySnapshot(ctx context.Context, snapshotID string) ([]InvestigateCaseBind, error)
}

// ─── MemStore 内存实现 ──────────────────────────────────────────────────────
// MemStore 线程安全内存存储（ / 单机模式）。
type MemStore struct {
	mu sync.RWMutex
	maxItems int
	snapshots   map[string]TraceSnapshot // id → snap
	snapByIdem  map[string]string        // idem_key → id
	snapsByUser map[string][]string      // user_id → []id
	proofs map[string]TraceDriftProof // id → proof
	proofByHash map[string]string // hash → id
	proofsBySnap map[string][]string // snapshot_id → []id
	binds map[string]InvestigateCaseBind // id → bind
	bindByCase  map[string]string   // case_id → id
	bindsBySnap map[string][]string // snapshot_id → []id
}

// NewMemStore 创建新的内存存储（最多 10000 条）。

  func NewMemStore() *MemStore {
	  return NewMemStoreN(defaultMaxItems)
   }

// NewMemStoreN 创建指定容量的内存存储。
  func NewMemStoreN(max int) *MemStore {
	  return &MemStore{
		  maxItems:    max,
		   snapshots:   make(map[string]TraceSnapshot),
		   snapByIdem:  make(map[string]string),
		   snapsByUser: make(map[string][]string),
		  proofs: make(map[string]TraceDriftProof),
		   proofByHash: make(map[string]string),
		  proofsBySnap: make(map[string][]string),
		  binds: make(map[string]InvestigateCaseBind),
		   bindByCase:  make(map[string]string),
		  bindsBySnap: make(map[string][]string),
	   }
  }

// ─── Snapshot 操作 ─────────────────────────────────────────────────────────

   func (m *MemStore) PutSnapshot(_ context.Context, s TraceSnapshot) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  _, existing := m.snapshots[s.ID]
	  if !existing && len(m.snapshots) >= m.maxItems {
		  return ErrStoreFull
	  }
	   m.snapshots[s.ID] = s
	  if s.IdemKey != "" {
		  m.snapByIdem[s.IdemKey] = s.ID
	  }
	   if !existing {
		  m.snapsByUser[s.UserID] = append(m.snapsByUser[s.UserID], s.ID)
	  }
	  return nil
  }

   func (m *MemStore) GetSnapshot(_ context.Context, id string) (TraceSnapshot, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   s,
		  ok := m.snapshots[strings.TrimSpace(id)]
	  if !ok {
		  return TraceSnapshot{}, ErrNotFound
	   }
	  return s, nil
  }

   func (m *MemStore) SnapshotsByUser(_ context.Context, userID string, limit int) ([]TraceSnapshot, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   ids := m.snapsByUser[strings.TrimSpace(userID)]
	  out := make([]TraceSnapshot, 0, len(ids))
	  for _, id := range ids {
		   if s, ok := m.snapshots[id]; ok {
			  out = append(out, s)
		  }
	  }
	   sort.Slice(out, func(i, j int) bool {
		  return out[i].CreatedAt.After(out[j].CreatedAt)
	   })
	   if limit > 0 && len(out) > limit {
		  out = out[:limit]
	  }
	  return out, nil
   }

  func (m *MemStore) SnapshotByIdemKey(_ context.Context, idemKey string) (TraceSnapshot, bool, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  id, ok := m.snapByIdem[strings.TrimSpace(idemKey)]
	   if !ok {
		  return TraceSnapshot{}, false, nil
	   }
	  s, ok2 := m.snapshots[id]
	   return s, ok2, nil
   }

  func (m *MemStore) LatestSealedByUserAsset(_ context.Context, userID, asset string) (TraceSnapshot, bool, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   userID = strings.TrimSpace(userID)
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	  ids := m.snapsByUser[userID]
	  var best *TraceSnapshot
	  for _, id := range ids {
		  s := m.snapshots[id]
		   if s.Asset != asset {
			   continue
		   }
		  if s.Status != StatusSealed && s.Status != StatusExported {
			  continue
		   }
		  if best == nil || s.CreatedAt.After(best.CreatedAt) {
			  cp := s
			  best = &cp
		  }
	  }
	  if best == nil {
		  return TraceSnapshot{},
			   false, nil
	  }
	  return *best, true, nil
  }

// ─── Proof 操作 ────────────────────────────────────────────────────────────

  func (m *MemStore) PutProof(_ context.Context, p TraceDriftProof) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  _, existing := m.proofs[p.ID]
	   if !existing && len(m.proofs) >= m.maxItems {
		  return ErrStoreFull
	   }
	   m.proofs[p.ID] = p
	   if p.ProofHash != "" {
		  m.proofByHash[p.ProofHash] = p.ID
	   }
	  if !existing {
		  m.proofsBySnap[p.SnapshotID] = append(m.proofsBySnap[p.SnapshotID], p.ID)
	  }
	   return nil
   }

   func (m *MemStore) GetProof(_ context.Context, id string) (TraceDriftProof, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   p, ok := m.proofs[strings.TrimSpace(id)]
	   if !ok {
		  return TraceDriftProof{},
			  ErrNotFound
	  }
	   return p, nil
   }

  func (m *MemStore) ProofsBySnapshot(_ context.Context, snapshotID string) ([]TraceDriftProof, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  ids := m.proofsBySnap[strings.TrimSpace(snapshotID)]
	   out := make([]TraceDriftProof,
		   0, len(ids))
	  for _, id := range ids {
		  if p, ok := m.proofs[id]; ok {
			   out = append(out, p)
		  }
	  }
	   return out, nil
  }

   func (m *MemStore) ProofByHash(_ context.Context, hash string) (TraceDriftProof, bool, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   id, ok := m.proofByHash[strings.TrimSpace(hash)]
	  if !ok {
		  return TraceDriftProof{}, false, nil
	  }
	   p, ok2 := m.proofs[id]
	   return p, ok2, nil
  }

// ─── Bind 操作 ─────────────────────────────────────────────────────────────

   func (m *MemStore) PutBind(_ context.Context, b InvestigateCaseBind) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  _, existing := m.binds[b.ID]
	  if !existing && len(m.binds) >= m.maxItems {
		  return ErrStoreFull
	   }
	  m.binds[b.ID] = b
	   if !existing {
		   m.bindByCase[b.CaseID] = b.ID
		   m.bindsBySnap[b.SnapshotID] = append(m.bindsBySnap[b.SnapshotID],
			   b.ID)
	  }
	   return nil
   }

  func (m *MemStore) GetBind(_ context.Context, id string) (InvestigateCaseBind, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   b, ok := m.binds[strings.TrimSpace(id)]
	  if !ok {
		  return InvestigateCaseBind{}, ErrNotFound
	  }
	   return b, nil
  }

   func (m *MemStore) BindByCaseID(_ context.Context, caseID string) (InvestigateCaseBind, bool, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   id,
		   ok := m.bindByCase[strings.TrimSpace(caseID)]
	   if !ok {
		  return InvestigateCaseBind{}, false, nil
	  }
	  b, ok2 := m.binds[id]
	  return b, ok2, nil
  }

  func (m *MemStore) BindsBySnapshot(_ context.Context, snapshotID string) ([]InvestigateCaseBind, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	   ids := m.bindsBySnap[strings.TrimSpace(snapshotID)]
	   out := make([]InvestigateCaseBind, 0, len(ids))
	  for _, id := range ids {
		  if b, ok := m.binds[id]; ok {
			  out = append(out, b)
		   }
	   }
	   return out, nil
   }

// ─── 统计接口（辅助）──────────────────────────────────────────────────────
// Stats 返回内存存储当前行数统计（辅助）。
  func (m *MemStore) Stats() map[string]int {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  return map[string]int{
		  "snapshots": len(m.snapshots),
		   "proofs":    len(m.proofs),
		   "binds":     len(m.binds),
	  }
   }
