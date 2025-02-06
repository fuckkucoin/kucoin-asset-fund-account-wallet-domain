package stmtexpiry

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// MemStore 纯内存 Store 实现，供单元使用。不做 SQL 编解码。
type MemStore struct {
	mu sync.Mutex
	leases      map[string]ExportLease
	drifts      map[string]RootDriftCase
	revocations map[string]Revocation
	proofs      map[string]IssueProof
	mutexes     map[string]string
}

  func NewMemStore() *MemStore {
	  return &MemStore{
		   leases: make(map[string]ExportLease),
		  drifts: make(map[string]RootDriftCase),
		   revocations: make(map[string]Revocation),
		   proofs:  make(map[string]IssueProof),
		   mutexes: make(map[string]string),
	   }
  }

// ──────────────────────────── 租约 ────────────────────────────

  func (m *MemStore) InsertLease(_ context.Context, l ExportLease) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  if _, ok := m.leases[l.ID]; ok {
		  return fmt.Errorf("lease %s already exists", l.ID)
	   }
	   m.leases[l.ID] = l
	   return nil
   }

   func (m *MemStore) GetLease(_ context.Context, id string) (ExportLease, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   l, ok := m.leases[id]
	   if !ok {
		  return l, fmt.Errorf("lease %s not found", id)
	   }
	   return l, nil
  }

  func (m *MemStore) UpdateLeaseState(_ context.Context, id string, from, to LeaseState, updatedAt time.Time) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   l, ok := m.leases[id]
	   if !ok {
		   return fmt.Errorf("lease %s not found", id)
	  }
	   if l.State != from {
		   return fmt.Errorf("lease %s: expected state %s, got %s", id, from, l.State)
	   }
	  l.State = to
	   l.UpdatedAt = updatedAt
	   m.leases[id] = l
	   return nil
  }

   func (m *MemStore) SetLeaseIssued(_ context.Context, id, proofID string, issuedAt, expiresAt time.Time) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   l, ok := m.leases[id]
	   if !ok {
		   return fmt.Errorf("lease %s not found", id)
	  }
	  if l.State != LeaseBuilt {
		   return fmt.Errorf("lease %s: expected built, got %s", id, l.State)
	   }
	  l.State = LeaseIssued
	  l.ProofID = proofID
	   l.IssuedAt = issuedAt
	  l.ExpiresAt = expiresAt
	   l.UpdatedAt = time.Now().UTC()
	  m.leases[id] = l
	   return nil
  }

  func (m *MemStore) SetLeaseRevoked(_ context.Context, id, revokedBy string) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   l, ok := m.leases[id]
	   if !ok {
		   return fmt.Errorf("lease %s not found", id)
	   }
	  if l.State != LeaseBuilt && l.State != LeaseIssued {
		  return fmt.Errorf("lease %s: cannot revoke in state %s", id, l.State)
	  }
	  l.State = LeaseRevoked
	   l.RevokedBy = revokedBy
	  l.UpdatedAt = time.Now().UTC()
	  m.leases[id] = l
	  return nil
  }

   func (m *MemStore) ListExpiredLeases(_ context.Context, before time.Time, limit int) ([]ExportLease, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   var out []ExportLease
	   for _, l := range m.leases {
		   if l.State == LeaseIssued && !l.ExpiresAt.IsZero() && l.ExpiresAt.Before(before) {
			  out = append(out, l)
		  }
	   }
	  sort.Slice(out, func(i, j int) bool { return out[i].ExpiresAt.Before(out[j].ExpiresAt) })
	  if limit > 0 && len(out) > limit {
		  out = out[:limit]
	   }
	  return out, nil
   }

   func (m *MemStore) ListActiveLeasesByUser(_ context.Context, userID string) ([]ExportLease, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   var out []ExportLease
	   for _, l := range m.leases {
		  if l.UserID == userID && (l.State == LeaseBuilt || l.State == LeaseIssued) {
			  out = append(out, l)
		   }
	   }
	   sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	   return out, nil
  }

  func (m *MemStore) ListIssuedLeases(_ context.Context, limit int) ([]ExportLease, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   var out []ExportLease
	  for _, l := range m.leases {
		  if l.State == LeaseIssued {
			   out = append(out, l)
		   }
	   }
	   sort.Slice(out, func(i, j int) bool { return out[i].IssuedAt.Before(out[j].IssuedAt) })
	  if limit > 0 && len(out) > limit {
		   out = out[:limit]
	   }
	  return out, nil
  }

// ──────────────────────────── 漂移 ────────────────────────────

   func (m *MemStore) InsertDrift(_ context.Context, d RootDriftCase) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   if _, ok := m.drifts[d.ID]; ok {
		   return fmt.Errorf("drift %s already exists", d.ID)
	   }
	  m.drifts[d.ID] = d
	   return nil
   }

  func (m *MemStore) GetDrift(_ context.Context, id string) (RootDriftCase, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   d, ok := m.drifts[id]
	   if !ok {
		   return d, fmt.Errorf("drift %s not found", id)
	   }
	   return d, nil
  }

  func (m *MemStore) UpdateDriftState(_ context.Context, id string, from, to DriftState) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  d, ok := m.drifts[id]
	  if !ok {
		  return fmt.Errorf("drift %s not found", id)
	   }
	  if d.State != from {
		   return fmt.Errorf("drift %s: expected %s, got %s", id, from, d.State)
	   }
	  d.State = to
	   now := time.Now().UTC()
	   switch to {
	  case DriftProven:
		  d.ProvenAt = now
	   case DriftClosed:
		   d.ClosedAt = now
	  }
	   m.drifts[id] = d
	   return nil
  }

  func (m *MemStore) ListOpenDrifts(_ context.Context, limit int) ([]RootDriftCase, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   var out []RootDriftCase
	  for _, d := range m.drifts {
		   if d.State == DriftOpen || d.State == DriftProven {
			   out = append(out, d)
		  }
	  }
	   sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	  if limit > 0 && len(out) > limit {
		   out = out[:limit]
	  }
	   return out, nil
  }

  func (m *MemStore) ListDriftsByLease(_ context.Context, leaseID string) ([]RootDriftCase, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   var out []RootDriftCase
	   for _, d := range m.drifts {
		   if d.LeaseID == leaseID {
			   out = append(out, d)
		  }
	  }
	   sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	   return out, nil
  }

// ──────────────────────────── 撤销 ────────────────────────────

   func (m *MemStore) InsertRevocation(_ context.Context, r Revocation) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  if _, ok := m.revocations[r.ID]; ok {
		  return fmt.Errorf("revocation %s already exists", r.ID)
	   }
	  m.revocations[r.ID] = r
	   return nil
  }

  func (m *MemStore) GetRevocation(_ context.Context, id string) (Revocation, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  r, ok := m.revocations[id]
	  if !ok {
		   return r, fmt.Errorf("revocation %s not found", id)
	   }
	   return r, nil
   }

   func (m *MemStore) ListRevocationsByLease(_ context.Context, leaseID string) ([]Revocation, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   var out []Revocation
	   for _, r := range m.revocations {
		  if r.LeaseID == leaseID {
			  out = append(out, r)
		   }
	  }
	   sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	  return out, nil
   }

// ──────────────────────────── 证明 ────────────────────────────

   func (m *MemStore) InsertProof(_ context.Context, p IssueProof) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if _, ok := m.proofs[p.ID]; ok {
		  return fmt.Errorf("proof %s already exists", p.ID)
	  }
	  m.proofs[p.ID] = p
	  return nil
   }

  func (m *MemStore) GetProof(_ context.Context, id string) (IssueProof, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   p, ok := m.proofs[id]
	  if !ok {
		  return p, fmt.Errorf("proof %s not found", id)
	  }
	  return p, nil
  }

  func (m *MemStore) GetProofByLease(_ context.Context, leaseID string) (IssueProof, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   var best IssueProof
	  found := false
	   for _, p := range m.proofs {
		  if p.LeaseID == leaseID {
			   if !found || p.IssuedAt.After(best.IssuedAt) {
				  best = p
				   found = true
			   }
		  }
	   }
	   if !found {
		  return best, fmt.Errorf("proof for lease %s not found", leaseID)
	   }
	   return best, nil
  }

// ──────────────────────────── 互斥 ────────────────────────────

  func (m *MemStore) TryAcquireExportMutex(_ context.Context, userID, leaseID string, ttl time.Duration) (bool, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   key := "export:" + userID
	   if holder, ok := m.mutexes[key]; ok && holder != leaseID {
		   return false, nil
	   }
	  m.mutexes[key] = leaseID
	  go func() {
		  <-time.After(ttl)
		   m.mu.Lock()
		  if m.mutexes[key] == leaseID {
			  delete(m.mutexes, key)
		   }
		  m.mu.Unlock()
	  }()
	   return true, nil
  }

  func (m *MemStore) ReleaseExportMutex(_ context.Context, userID, leaseID string) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   key := "export:" + userID
	   if m.mutexes[key] == leaseID {
		  delete(m.mutexes, key)
	  }
	   return nil
   }
