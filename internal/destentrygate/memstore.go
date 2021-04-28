package destentrygate

import (
	"context"
	"sync"
	"time"
)

// MemStore 进程内仓储。
type MemStore struct {
	mu sync.RWMutex
	leases map[string]DestEntryLease
	idem map[string]string
	gens map[string]EntryGeneration
	proofs map[string]DestEntryDriftProof
	audits []DestEntryAudit
}

// NewMemStore 构造空仓储。

   func NewMemStore() *MemStore {
	  return &MemStore{
		  leases: make(map[string]DestEntryLease),
		  idem: make(map[string]string),
		   gens: make(map[string]EntryGeneration),
		  proofs: make(map[string]DestEntryDriftProof),
	  }
  }

   func (m *MemStore) SaveLease(_ context.Context, lease DestEntryLease) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  if lease.LeaseID == "" {
		   return ErrInvalidReq
	  }
	  m.leases[lease.LeaseID] = lease
	  if lease.IdempotencyKey != "" {
		   m.idem[lease.IdempotencyKey] = lease.LeaseID
	   }
	   return nil
  }

  func (m *MemStore) GetLease(_ context.Context, leaseID string) (DestEntryLease, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   l, ok := m.leases[leaseID]
	   if !ok {
		  return DestEntryLease{}, ErrLeaseNotFound
	   }
	  return l, nil
   }

   func (m *MemStore) GetLeaseByIdempotencyKey(_ context.Context, key string) (DestEntryLease, bool, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	   id, ok := m.idem[key]
	  if !ok {
		  return DestEntryLease{}, false, nil
	   }
	  l, ok := m.leases[id]
	   if !ok {
		   return DestEntryLease{}, false, nil
	   }
	   return l, true, nil
  }

   func (m *MemStore) UpdateLease(_ context.Context, lease DestEntryLease) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  if _, ok := m.leases[lease.LeaseID]; !ok {
		  return ErrLeaseNotFound
	  }
	  m.leases[lease.LeaseID] = lease
	  return nil
   }

  func (m *MemStore) ListLeasesByState(_ context.Context, state LeaseState, limit int) ([]DestEntryLease, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	  if limit <= 0 {
		   limit = 100
	   }
	   out := make([]DestEntryLease, 0, limit)
	  for _, l := range m.leases {
		  if l.State == state {
			  out = append(out, l)
			   if len(out) >= limit {
				   break
			  }
		   }
	  }
	   return out, nil
  }

   func (m *MemStore) ListExpiredIssued(_ context.Context, now time.Time, limit int) ([]DestEntryLease, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  if limit <= 0 {
		  limit = 200
	   }
	  out := make([]DestEntryLease, 0, limit)
	  for _, l := range m.leases {
		   if l.State != LeaseIssued && l.State != LeaseBound {
			  continue
		   }
		  if !now.After(l.ExpiresAt()) {
			   continue
		   }
		   out = append(out, l)
		   if len(out) >= limit {
			  break
		  }
	   }
	  return out, nil
  }

  func (m *MemStore) ListUnconsumed(_ context.Context, limit int) ([]DestEntryLease, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   if limit <= 0 {
		   limit = 100
	  }
	   out := make([]DestEntryLease, 0, limit)
	   for _, l := range m.leases {
		   if l.State == LeaseIssued || l.State == LeaseBound {
			   out = append(out, l)
			  if len(out) >= limit {
				  break
			   }
		  }
	  }
	  return out, nil
  }

  func (m *MemStore) ListByEntry(_ context.Context, entryID string, limit int) ([]DestEntryLease, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  if limit <= 0 {
		   limit = 200
	  }
	  out := make([]DestEntryLease, 0, limit)
	  for _, l := range m.leases {
		  if l.EntryID == entryID {
			  out = append(out, l)
			  if len(out) >= limit {
				  break
			   }
		   }
	  }
	   return out, nil
  }

  func (m *MemStore) ListRevocableByEntryGen(_ context.Context, entryID string, maxGen int64, limit int) ([]DestEntryLease, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  if limit <= 0 {
		   limit = 200
	   }
	  out := make([]DestEntryLease, 0, limit)
	  for _, l := range m.leases {
		   if l.EntryID != entryID {
			  continue
		  }
		   if l.State != LeaseIssued && l.State != LeaseBound {
			   continue
		   }
		  if maxGen > 0 && l.EntryGen >= maxGen {
			   continue
		   }
		  out = append(out, l)
		  if len(out) >= limit {
			  break
		  }
	   }
	   return out, nil
   }

   func (m *MemStore) SaveGen(_ context.Context, gen EntryGeneration) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  if gen.EntryID == "" {
		   return ErrInvalidReq
	  }
	   m.gens[gen.EntryID] = gen
	  return nil
  }

   func (m *MemStore) GetGen(_ context.Context, entryID string) (EntryGeneration, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  g, ok := m.gens[entryID]
	  if !ok {
		   return EntryGeneration{}, ErrGenNotFound
	   }
	   return g, nil
  }

  func (m *MemStore) SaveProof(_ context.Context, proof DestEntryDriftProof) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   if proof.ProofID == "" {
		   return ErrInvalidReq
	  }
	   m.proofs[proof.ProofID] = proof
	  return nil
  }

   func (m *MemStore) GetProof(_ context.Context, proofID string) (DestEntryDriftProof, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  p, ok := m.proofs[proofID]
	  if !ok {
		  return DestEntryDriftProof{}, ErrProofNotFound
	   }
	  return p, nil
  }

  func (m *MemStore) ListProofs(_ context.Context, filter ExportProofFilter) ([]DestEntryDriftProof, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	   limit := filter.Limit
	   if limit <= 0 {
		  limit = 100
	  }
	   out := make([]DestEntryDriftProof, 0, limit)
	   for _, p := range m.proofs {
		  if filter.Kind != "" && p.Kind != filter.Kind {
			  continue
		  }
		  if filter.EntryID != "" && p.EntryID != filter.EntryID {
			  continue
		  }
		  if filter.UserID != "" && p.UserID != filter.UserID {
			   continue
		   }
		  if filter.UnexportedOnly && p.ExportedAt != nil {
			   continue
		  }
		   out = append(out, p)
		  if len(out) >= limit {
			   break
		  }
	   }
	   return out, nil
   }

   func (m *MemStore) MarkProofsExported(_ context.Context, ids []string, at time.Time) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  for _, id := range ids {
		  p, ok := m.proofs[id]
		   if !ok {
			  continue
		  }
		  t := at.UTC()
		   p.ExportedAt = &t
		   m.proofs[id] = p
	   }
	  return nil
   }

  func (m *MemStore) SaveAudit(_ context.Context, row DestEntryAudit) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  m.audits = append(m.audits, row)
	   return nil
  }

   func (m *MemStore) ListAuditByLease(_ context.Context, leaseID string, limit int) ([]DestEntryAudit, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	  if limit <= 0 {
		  limit = 100
	   }
	   out := make([]DestEntryAudit, 0, limit)
	   for i := len(m.audits) - 1; i >= 0; i-- {
		   if m.audits[i].LeaseID == leaseID || leaseID == "" {
			  out = append(out, m.audits[i])
			   if len(out) >= limit {
				  break
			  }
		  }
	  }
	   return out, nil
   }

// CountProofs 辅助。
  func (m *MemStore) CountProofs() int {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   return len(m.proofs)
  }
