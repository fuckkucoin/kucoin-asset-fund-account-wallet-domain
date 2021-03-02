package batchresiduefence

import (
	"context"
	"strings"
	"sync"
)

type MemStore struct {
	mu     sync.Mutex
	fences map[string]ResidueFence
	proofs map[string]BatchDupFreezeProof
	halves map[string]HalfSubmitCase
	slots map[string]FreezeSlot
	mutex map[string]MutexHold
	audits []AuditEvent
}

  func NewMemStore() *MemStore {
	   return &MemStore{
		  fences: map[string]ResidueFence{},
		  proofs: map[string]BatchDupFreezeProof{},
		  halves: map[string]HalfSubmitCase{},
		  slots:  map[string]FreezeSlot{},
		  mutex:  map[string]MutexHold{},
	  }
  }

  func (m *MemStore) SaveFence(_ context.Context, f ResidueFence) error {
	   if m == nil {
		   return ErrStoreUnavailable
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   m.fences[f.ID] = f.Copy()
	   return nil
  }

   func (m *MemStore) GetFence(_ context.Context, id string) (ResidueFence, bool, error) {
	   if m == nil {
		   return ResidueFence{}, false, ErrStoreUnavailable
	  }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  f, ok := m.fences[strings.TrimSpace(id)]
	   return f.Copy(), ok, nil
   }

  func (m *MemStore) ListFencesByItem(_ context.Context, batchID, itemIdem string) ([]ResidueFence, error) {
	  if m == nil {
		   return nil, ErrStoreUnavailable
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   out := make([]ResidueFence, 0)
	  for _, f := range m.fences {
		  if f.BatchID == batchID && f.ItemIdem == itemIdem {
			  out = append(out, f.Copy())
		   }
	  }
	   return out, nil
  }

  func (m *MemStore) ListFencesByBatch(_ context.Context, batchID string) ([]ResidueFence, error) {
	  if m == nil {
		   return nil, ErrStoreUnavailable
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  out := make([]ResidueFence, 0)
	   for _, f := range m.fences {
		   if f.BatchID == batchID {
			  out = append(out, f.Copy())
		  }
	  }
	  return out, nil
  }

  func (m *MemStore) ListLiveFences(_ context.Context, limit int) ([]ResidueFence, error) {
	   if m == nil {
		   return nil, ErrStoreUnavailable
	  }
	   if limit <= 0 {
		   limit = DefaultPatrolLimit
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  out := make([]ResidueFence, 0)
	  for _, f := range m.fences {
		   if f.State == FenceIssued || f.State == FenceFrozen {
			   out = append(out, f.Copy())
			  if len(out) >= limit {
				  break
			   }
		   }
	  }
	  return out, nil
  }

  func (m *MemStore) SaveProof(_ context.Context, p BatchDupFreezeProof) error {
	  if m == nil {
		   return ErrStoreUnavailable
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  m.proofs[p.ID] = p.Copy()
	  return nil
   }

   func (m *MemStore) ListProofs(_ context.Context, batchID string) ([]BatchDupFreezeProof, error) {
	   if m == nil {
		  return nil, ErrStoreUnavailable
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  out := make([]BatchDupFreezeProof, 0)
	  for _, p := range m.proofs {
		  if batchID == "" || p.BatchID == batchID {
			   out = append(out, p.Copy())
		   }
	  }
	  return out, nil
  }

   func (m *MemStore) ListUnexportedProofs(_ context.Context, limit int) ([]BatchDupFreezeProof, error) {
	   if m == nil {
		   return nil, ErrStoreUnavailable
	  }
	   if limit <= 0 {
		   limit = DefaultPatrolLimit
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   out := make([]BatchDupFreezeProof, 0)
	  for _, p := range m.proofs {
		  if !p.Exported {
			  out = append(out, p.Copy())
			   if len(out) >= limit {
				  break
			  }
		   }
	   }
	  return out, nil
   }

   func (m *MemStore) SaveHalf(_ context.Context, h HalfSubmitCase) error {
	   if m == nil {
		  return ErrStoreUnavailable
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  m.halves[h.ID] = h.Copy()
	   return nil
   }

  func (m *MemStore) GetOpenHalf(_ context.Context, batchID, itemIdem string) (HalfSubmitCase, bool, error) {
	   if m == nil {
		   return HalfSubmitCase{}, false, ErrStoreUnavailable
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  for _, h := range m.halves {
		   if h.BatchID == batchID && h.ItemIdem == itemIdem && h.State == HalfOpen {
			   return h.Copy(), true, nil
		   }
	  }
	  return HalfSubmitCase{}, false, nil
  }

   func (m *MemStore) ListOpenHalves(_ context.Context, limit int) ([]HalfSubmitCase, error) {
	  if m == nil {
		  return nil, ErrStoreUnavailable
	  }
	   if limit <= 0 {
		  limit = DefaultPatrolLimit
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  out := make([]HalfSubmitCase, 0)
	  for _, h := range m.halves {
		  if h.State == HalfOpen {
			   out = append(out, h.Copy())
			   if len(out) >= limit {
				  break
			   }
		  }
	  }
	  return out, nil
  }

   func (m *MemStore) ListHalvesByBatch(_ context.Context, batchID string) ([]HalfSubmitCase, error) {
	   if m == nil {
		  return nil, ErrStoreUnavailable
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  out := make([]HalfSubmitCase, 0)
	  for _, h := range m.halves {
		  if h.BatchID == batchID {
			  out = append(out, h.Copy())
		   }
	  }
	  return out, nil
   }

   func (m *MemStore) SaveSlot(_ context.Context, s FreezeSlot) error {
	  if m == nil {
		   return ErrStoreUnavailable
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   m.slots[s.ItemIdem] = s
	   return nil
  }

   func (m *MemStore) GetSlot(_ context.Context, itemIdem string) (FreezeSlot, bool, error) {
	  if m == nil {
		   return FreezeSlot{}, false, ErrStoreUnavailable
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  s, ok := m.slots[itemIdem]
	  return s, ok, nil
  }

  func (m *MemStore) SaveMutex(_ context.Context, hold MutexHold) error {
	  if m == nil {
		   return ErrStoreUnavailable
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   m.mutex[hold.BatchID] = hold
	  return nil
  }

  func (m *MemStore) GetMutex(_ context.Context, batchID string) (MutexHold, bool, error) {
	   if m == nil {
		  return MutexHold{}, false, ErrStoreUnavailable
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  h, ok := m.mutex[batchID]
	   return h, ok, nil
  }

  func (m *MemStore) DeleteMutex(_ context.Context, batchID string) error {
	  if m == nil {
		  return ErrStoreUnavailable
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   delete(m.mutex, batchID)
	   return nil
   }

   func (m *MemStore) SaveAudit(_ context.Context, a AuditEvent) error {
	  if m == nil {
		  return ErrStoreUnavailable
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  m.audits = append(m.audits, a)
	   return nil
   }

  func (m *MemStore) ListAudits(_ context.Context, batchID string, limit int) ([]AuditEvent, error) {
	   if m == nil {
		   return nil, ErrStoreUnavailable
	   }
	   if limit <= 0 {
		   limit = 100
	  }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  out := make([]AuditEvent, 0)
	  for i := len(m.audits) - 1; i >= 0 && len(out) < limit; i-- {
		   if batchID == "" || m.audits[i].BatchID == batchID {
			   out = append(out, m.audits[i])
		  }
	   }
	  return out, nil
  }
