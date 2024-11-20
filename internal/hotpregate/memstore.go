package hotpregate

import (
	"context"
	"sort"
	"sync"
	"time"
)

// MemStore 内存存储；字段均为 map+slice，无 SQL 依赖。
type MemStore struct {
	mu sync.RWMutex
	leases map[string]*HotPrecheckLease // leaseID → lease
	byIdem map[string]string         // idemKey → leaseID
	proofs map[string]*InvDriftProof // proofID → proof
}

// NewMemStore 构造空 MemStore。
  func NewMemStore() *MemStore {
	  return &MemStore{
		   leases: map[string]*HotPrecheckLease{},
		  byIdem: map[string]string{},
		   proofs: map[string]*InvDriftProof{},
	  }
  }

// copyLease 安全复制，避免外部修改内部状态。
  func copyLease(l *HotPrecheckLease) *HotPrecheckLease {
	  if l == nil {
		  return nil
	  }
	   cp := *l
	  return &cp
  }

// copyProof 安全复制。
  func copyProof(p *InvDriftProof) *InvDriftProof {
	   if p == nil {
		  return nil
	   }
	   cp := *p
	  return &cp
  }

// ── HotPrecheckLease ─────────────────────────────────────────────────────────

  func (m *MemStore) GetLease(_ context.Context, leaseID string) (*HotPrecheckLease, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	   l, ok := m.leases[leaseID]
	   if !ok {
		  return nil, ErrLeaseNotFound
	   }
	  return copyLease(l), nil
  }

   func (m *MemStore) GetLeaseByIdem(_ context.Context, idemKey string) (*HotPrecheckLease, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   id, ok := m.byIdem[idemKey]
	   if !ok {
		   return nil, ErrLeaseNotFound
	  }
	   l, ok := m.leases[id]
	   if !ok {
		   return nil, ErrLeaseNotFound
	   }
	  return copyLease(l), nil
  }

   func (m *MemStore) InsertLease(_ context.Context, lease HotPrecheckLease) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   if existID, ok := m.byIdem[lease.IdemKey]; ok {
		   exist := m.leases[existID]
		   if exist != nil && !exist.IsExpired(time.Now().UTC()) {
			   return ErrIdemConflict
		   }
	  }
	   cp := lease
	   m.leases[lease.LeaseID] = &cp
	   m.byIdem[lease.IdemKey] = lease.LeaseID
	   return nil
   }

   func (m *MemStore) UpdateLeaseState(_ context.Context, leaseID string, expected, next LeaseState, now time.Time, note string) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  l, ok := m.leases[leaseID]
	   if !ok {
		   return ErrLeaseNotFound
	   }
	   if l.State != expected {
		  return ErrStateMismatch
	   }
	  l.State = next
	   l.Note = note
	  switch next {
	   case LeaseBound:
		  t := now
		  l.BoundAt = &t
	  case LeaseConsumed, LeaseDrifted:
		  t := now
		   l.ConsumedAt = &t
	  }
	  return nil
   }

   func (m *MemStore) SetLeaseDriftProof(_ context.Context, leaseID, proofID string, now time.Time) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  l, ok := m.leases[leaseID]
	  if !ok {
		   return ErrLeaseNotFound
	  }
	  l.State = LeaseDrifted
	  l.DriftProofID = proofID
	   t := now
	  l.ConsumedAt = &t
	  return nil
  }

   func (m *MemStore) ListLeases(_ context.Context, f LeaseListFilter) ([]HotPrecheckLease, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	   limit := f.Limit
	   if limit <= 0 {
		   limit = 200
	  }
	  var out []HotPrecheckLease
	   for _, l := range m.leases {
		  if f.Asset != "" && l.Asset != f.Asset {
			  continue
		  }
		  if f.State != "" && l.State != f.State {
			  continue
		   }
		   out = append(out, *l)
	  }
	  sort.Slice(out, func(i, j int) bool {
		   return out[i].IssuedAt.Before(out[j].IssuedAt)
	   })
	  if f.Offset < len(out) {
		  out = out[f.Offset:]
	   } else {
		   out = nil
	  }
	   if len(out) > limit {
		   out = out[:limit]
	   }
	   return out, nil
   }

   func (m *MemStore) ListExpiredLeases(_ context.Context, now time.Time, limit int) ([]HotPrecheckLease, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	   if limit <= 0 {
		  limit = 200
	   }
	   var out []HotPrecheckLease
	   for _, l := range m.leases {
		   if (l.State == LeaseIssued || l.State == LeaseBound) && now.After(l.ExpiresAt) {
			   out = append(out, *l)
		  }
	   }
	   sort.Slice(out, func(i, j int) bool {
		   return out[i].ExpiresAt.Before(out[j].ExpiresAt)
	  })
	  if len(out) > limit {
		   out = out[:limit]
	   }
	  return out, nil
  }

  func (m *MemStore) ListActiveLeasesByAsset(_ context.Context, asset string) ([]HotPrecheckLease, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	  var out []HotPrecheckLease
	   for _, l := range m.leases {
		  if l.Asset != asset {
			  continue
		   }
		  if l.State == LeaseIssued || l.State == LeaseBound {
			  out = append(out, *l)
		   }
	  }
	  sort.Slice(out, func(i, j int) bool {
		  return out[i].IssuedAt.Before(out[j].IssuedAt)
	  })
	  return out, nil
   }

// ── InvDriftProof ─────────────────────────────────────────────────────────────

  func (m *MemStore) GetProof(_ context.Context, proofID string) (*InvDriftProof, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  p, ok := m.proofs[proofID]
	   if !ok {
		  return nil, ErrProofNotFound
	  }
	  return copyProof(p), nil
   }

  func (m *MemStore) InsertProof(_ context.Context, proof InvDriftProof) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   cp := proof
	   m.proofs[proof.ProofID] = &cp
	  return nil
  }

  func (m *MemStore) ListProofs(_ context.Context, f DriftProofListFilter) ([]InvDriftProof, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  limit := f.Limit
	  if limit <= 0 {
		   limit = 200
	  }
	  var out []InvDriftProof
	  for _, p := range m.proofs {
		  if f.Asset != "" && p.Asset != f.Asset {
			  continue
		   }
		  if f.Severity != "" && p.Severity != f.Severity {
			  continue
		   }
		  out = append(out, *p)
	  }
	  sort.Slice(out,
		   func(i, j int) bool {
			   return out[i].CreatedAt.Before(out[j].CreatedAt)
		  })
	   if f.Offset < len(out) {
		  out = out[f.Offset:]
	  } else {
		  out = nil
	   }
	  if len(out) > limit {
		  out = out[:limit]
	   }
	   return out, nil
  }
