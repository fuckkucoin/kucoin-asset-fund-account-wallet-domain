package ledseal

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

// Store 定义 ledseal 所需的持久化接口。
// 读写范围：snap_plans、snap_slices、diff_batches、drift_rows、seal_roots。
// 不得写入账本核心表（ledger_entries 等）。

type Store interface {
	// ──── SnapPlan ────
	PutPlan(ctx context.Context, p SnapPlan) error
	GetPlan(ctx context.Context,
		id string) (SnapPlan, error)
	GetPlanByName(ctx context.Context, name string) (SnapPlan, error)
	ListActivePlans(ctx context.Context, before time.Time) ([]SnapPlan, error)
	ListPlans(ctx context.Context, limit int) ([]SnapPlan, error)
	// ──── SnapSlice ────
	PutSlice(ctx context.Context, s SnapSlice) error
	GetSlice(ctx context.Context,
		id string) (SnapSlice, error)
	LatestSlice(ctx context.Context, planID string) (SnapSlice, error)
	ListSlices(ctx context.Context, planID string, limit int) ([]SnapSlice, error)
	// ──── DiffBatch ────
	PutBatch(ctx context.Context, b DiffBatch) error
	GetBatch(ctx context.Context,
		id string) (DiffBatch, error)
	ListBatches(ctx context.Context, planID string, state BatchState, limit int) ([]DiffBatch, error)
	// ──── DriftRow ────
	PutDriftRow(ctx context.Context, d DriftRow) error
	GetDriftRow(ctx context.Context, id string) (DriftRow, error)
	ListDriftRows(ctx context.Context,
		batchID string, limit int) ([]DriftRow, error)
	ConfirmDriftRow(ctx context.Context, id, actor, note string, at time.Time) error
	// ──── SealRoot ────
	PutSealRoot(ctx context.Context, r SealRoot) error
	GetSealRoot(ctx context.Context, id string) (SealRoot, error)
	GetSealRootByBatch(ctx context.Context, batchID string) (SealRoot, error)
	MarkExported(ctx context.Context,
		id, actor string, at time.Time) error
}

// ─────────────────────────────────────────────────────────
// MemStore 内存实现（ / 内存模式）
// ─────────────────────────────────────────────────────────

type MemStore struct {
	mu sync.Mutex
	plans map[string]*SnapPlan
	slices    map[string]*SnapSlice
	batches   map[string]*DiffBatch
	driftRows map[string]*DriftRow
	sealRoots map[string]*SealRoot
}

// NewMemStore 返回空的内存存储。

   func NewMemStore() *MemStore {
	  return &MemStore{
		  plans:  make(map[string]*SnapPlan),
		   slices: make(map[string]*SnapSlice),
		   batches: make(map[string]*DiffBatch),
		   driftRows: make(map[string]*DriftRow),
		  sealRoots: make(map[string]*SealRoot),
	   }
  }

// ──── SnapPlan ────

  func (m *MemStore) PutPlan(_ context.Context, p SnapPlan) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   cp := p.Copy()
	   m.plans[p.ID] = &cp
	  return nil
   }

  func (m *MemStore) GetPlan(_ context.Context, id string) (SnapPlan, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  p, ok := m.plans[strings.TrimSpace(id)]
	   if !ok {
		   return SnapPlan{}, ErrPlanNotFound
	  }
	   return p.Copy(), nil
   }

   func (m *MemStore) GetPlanByName(_ context.Context, name string) (SnapPlan, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  name = strings.TrimSpace(name)
	   for _, p := range m.plans {
		  if p.Name == name {
			  return p.Copy(), nil
		  }
	   }
	  return SnapPlan{}, ErrPlanNotFound
   }

   func (m *MemStore) ListActivePlans(_ context.Context, before time.Time) ([]SnapPlan, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   out := make([]SnapPlan, 0)
	  for _, p := range m.plans {
		   if p.State != PlanActive {
			  continue
		   }
		   if p.NextRunAt != nil && p.NextRunAt.After(before) {
			  continue
		   }
		  out = append(out, p.Copy())
	   }
	   return out, nil
  }

  func (m *MemStore) ListPlans(_ context.Context, limit int) ([]SnapPlan, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   if limit <= 0 {
		   limit = 50
	   }
	   out := make([]SnapPlan, 0, len(m.plans))
	  for _, p := range m.plans {
		   out = append(out, p.Copy())
	   }
	  sort.Slice(out, func(i, j int) bool {
		  return out[i].CreatedAt.Before(out[j].CreatedAt)
	   })
	  if len(out) > limit {
		  out = out[:limit]
	   }
	  return out, nil
  }

// ──── SnapSlice ────

  func (m *MemStore) PutSlice(_ context.Context, s SnapSlice) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  cp := s.Copy()
	  m.slices[s.ID] = &cp
	   return nil
  }

   func (m *MemStore) GetSlice(_ context.Context, id string) (SnapSlice, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  s, ok := m.slices[strings.TrimSpace(id)]
	   if !ok {
		  return SnapSlice{}, ErrSliceNotFound
	  }
	  return s.Copy(), nil
  }

  func (m *MemStore) LatestSlice(_ context.Context, planID string) (SnapSlice, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   var best *SnapSlice
	   for _, s := range m.slices {
		   if s.PlanID != planID {
			   continue
		   }
		   if best == nil || s.GenSeq > best.GenSeq {
			   best = s
		   }
	  }
	  if best == nil {
		   return SnapSlice{}, ErrSliceNotFound
	   }
	  return best.Copy(), nil
  }

  func (m *MemStore) ListSlices(_ context.Context, planID string, limit int) ([]SnapSlice, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  if limit <= 0 {
		  limit = 50
	   }
	  out := make([]SnapSlice, 0)
	   for _, s := range m.slices {
		   if planID != "" && s.PlanID != planID {
			  continue
		   }
		  out = append(out, s.Copy())
	  }
	  sort.Slice(out, func(i, j int) bool {
		   return out[i].GenSeq < out[j].GenSeq
	   })
	   if len(out) > limit {
		  out = out[:limit]
	  }
	   return out, nil
  }

// ──── DiffBatch ────

  func (m *MemStore) PutBatch(_ context.Context, b DiffBatch) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  cp := b.Copy()
	   m.batches[b.ID] = &cp
	   return nil
   }

   func (m *MemStore) GetBatch(_ context.Context, id string) (DiffBatch, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  b, ok := m.batches[strings.TrimSpace(id)]
	  if !ok {
		  return DiffBatch{}, ErrBatchNotFound
	   }
	  return b.Copy(), nil
  }

   func (m *MemStore) ListBatches(_ context.Context, planID string, state BatchState, limit int) ([]DiffBatch, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if limit <= 0 {
		  limit = 50
	   }
	  out := make([]DiffBatch, 0)
	  for _, b := range m.batches {
		   if planID != "" && b.PlanID != planID {
			   continue
		   }
		   if state != "" && b.State != state {
			   continue
		   }
		   out = append(out, b.Copy())
	  }
	   sort.Slice(out, func(i, j int) bool {
		   return out[i].CreatedAt.Before(out[j].CreatedAt)
	   })
	   if len(out) > limit {
		  out = out[:limit]
	  }
	   return out, nil
   }

// ──── DriftRow ────

   func (m *MemStore) PutDriftRow(_ context.Context, d DriftRow) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  cp := d.Copy()
	  m.driftRows[d.ID] = &cp
	   return nil
  }

  func (m *MemStore) GetDriftRow(_ context.Context, id string) (DriftRow, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   d, ok := m.driftRows[strings.TrimSpace(id)]
	  if !ok {
		   return DriftRow{}, ErrDriftNotFound
	   }
	  return d.Copy(), nil
   }

  func (m *MemStore) ListDriftRows(_ context.Context, batchID string, limit int) ([]DriftRow, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   if limit <= 0 {
		   limit = 200
	  }
	   out := make([]DriftRow, 0)
	   for _, d := range m.driftRows {
		  if batchID != "" && d.BatchID != batchID {
			  continue
		   }
		  out = append(out, d.Copy())
	   }
	  sort.Slice(out, func(i, j int) bool {
		  return out[i].CreatedAt.Before(out[j].CreatedAt)
	   })
	  if len(out) > limit {
		  out = out[:limit]
	   }
	   return out, nil
  }

   func (m *MemStore) ConfirmDriftRow(_ context.Context, id, actor, note string, at time.Time) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  d,
		  ok := m.driftRows[strings.TrimSpace(id)]
	   if !ok {
		   return ErrDriftNotFound
	  }
	   d.ConfirmedBy = actor
	  d.Note = note
	   t := at
	   d.ConfirmedAt = &t
	  return nil
   }

// ──── SealRoot ────

  func (m *MemStore) PutSealRoot(_ context.Context, r SealRoot) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   cp := r.Copy()
	   m.sealRoots[r.ID] = &cp
	  return nil
  }

   func (m *MemStore) GetSealRoot(_ context.Context, id string) (SealRoot, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  r, ok := m.sealRoots[strings.TrimSpace(id)]
	  if !ok {
		  return SealRoot{}, ErrSealRootNotFound
	   }
	   return r.Copy(), nil
  }

  func (m *MemStore) GetSealRootByBatch(_ context.Context, batchID string) (SealRoot, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  batchID = strings.TrimSpace(batchID)
	  for _, r := range m.sealRoots {
		  if r.BatchID == batchID {
			   return r.Copy(), nil
		   }
	  }
	  return SealRoot{}, ErrSealRootNotFound
  }

  func (m *MemStore) MarkExported(_ context.Context, id, actor string, at time.Time) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  r, ok := m.sealRoots[strings.TrimSpace(id)]
	   if !ok {
		   return ErrSealRootNotFound
	  }
	  r.ExportedBy = actor
	   t := at
	  r.ExportedAt = &t
	  return nil
   }
