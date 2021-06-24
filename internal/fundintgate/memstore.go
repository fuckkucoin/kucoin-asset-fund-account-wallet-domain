package fundintgate

import (
	"context"
	"sync"
	"time"
)

// MemStore 进程内仓储，与单机开发使用。
type MemStore struct {
	mu sync.RWMutex
	leases map[string]IntGateLease
	idem map[string]string
	gens map[string]GateGeneration
	proofs map[string]IntGateDriftProof
	audits []IntGateAudit
}

// NewMemStore 构造空仓储。
  func NewMemStore() *MemStore {
	   return &MemStore{
		   leases: make(map[string]IntGateLease),
		   idem: make(map[string]string),
		   gens: make(map[string]GateGeneration),
		  proofs: make(map[string]IntGateDriftProof),
	  }
   }

   func (m *MemStore) SaveLease(_ context.Context, lease IntGateLease) error {
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

  func (m *MemStore) GetLease(_ context.Context, leaseID string) (IntGateLease, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   l, ok := m.leases[leaseID]
	   if !ok {
		  return IntGateLease{}, ErrLeaseNotFound
	  }
	  return l, nil
  }

   func (m *MemStore) GetLeaseByIdempotencyKey(_ context.Context, key string) (IntGateLease, bool, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	  id, ok := m.idem[key]
	   if !ok {
		  return IntGateLease{}, false, nil
	   }
	  l, ok := m.leases[id]
	   if !ok {
		  return IntGateLease{}, false, nil
	   }
	   return l, true, nil
   }

   func (m *MemStore) UpdateLease(_ context.Context, lease IntGateLease) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   if _, ok := m.leases[lease.LeaseID]; !ok {
		  return ErrLeaseNotFound
	   }
	   m.leases[lease.LeaseID] = lease
	   return nil
   }

  func (m *MemStore) ListLeasesByState(_ context.Context, state LeaseState, limit int) ([]IntGateLease, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   if limit <= 0 {
		   limit = 100
	  }
	   out := make([]IntGateLease, 0, limit)
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

  func (m *MemStore) ListExpiredGated(_ context.Context, now time.Time, limit int) ([]IntGateLease, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   if limit <= 0 {
		  limit = 100
	   }
	  out := make([]IntGateLease,
		   0, limit)
	  for _, l := range m.leases {
		  if (l.State == LeaseGated || l.State == LeaseBound) && now.After(l.ExpiresAt()) {
			  out = append(out, l)
			  if len(out) >= limit {
				  break
			   }
		  }
	   }
	   return out, nil
   }

   func (m *MemStore) ListUnconsumed(_ context.Context, limit int) ([]IntGateLease, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   if limit <= 0 {
		  limit = 100
	   }
	   out := make([]IntGateLease, 0, limit)
	   for _, l := range m.leases {
		  if l.State == LeaseGated || l.State == LeaseBound {
			  out = append(out, l)
			  if len(out) >= limit {
				  break
			  }
		  }
	   }
	  return out, nil
   }

  func (m *MemStore) ListRevocableByGen(_ context.Context, maxGen int64, limit int) ([]IntGateLease, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	  if limit <= 0 {
		   limit = 200
	   }
	  out := make([]IntGateLease, 0, limit)
	   for _, l := range m.leases {
		   if (l.State == LeaseGated || l.State == LeaseBound) && l.GateGen < maxGen {
			  out = append(out, l)
			  if len(out) >= limit {
				  break
			   }
		   }
	   }
	  return out, nil
  }

  func (m *MemStore) SaveGen(_ context.Context, gen GateGeneration) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  if gen.Scope == "" {
		   gen.Scope = ScopeGlobal
	  }
	   m.gens[gen.Scope] = gen
	   return nil
  }

   func (m *MemStore) GetGen(_ context.Context, scope string) (GateGeneration, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  if scope == "" {
		   scope = ScopeGlobal
	  }
	  g, ok := m.gens[scope]
	  if !ok {
		  return GateGeneration{},
			   ErrGenNotFound
	   }
	  return g, nil
  }

   func (m *MemStore) SaveProof(_ context.Context, proof IntGateDriftProof) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  if proof.ProofID == "" {
		  return ErrInvalidReq
	   }
	  m.proofs[proof.ProofID] = proof
	  return nil
  }

   func (m *MemStore) GetProof(_ context.Context, proofID string) (IntGateDriftProof, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  p, ok := m.proofs[proofID]
	   if !ok {
		  return IntGateDriftProof{}, ErrProofNotFound
	   }
	  return p, nil
  }

  func (m *MemStore) ListProofs(_ context.Context, filter ExportProofFilter) ([]IntGateDriftProof, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  limit := filter.Limit
	   if limit <= 0 {
		   limit = 100
	  }
	   out := make([]IntGateDriftProof, 0, limit)
	   for _, p := range m.proofs {
		   if filter.Kind != "" && p.Kind != filter.Kind {
			  continue
		  }
		  if filter.LeaseID != "" && p.LeaseID != filter.LeaseID {
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

  func (m *MemStore) SaveAudit(_ context.Context, row IntGateAudit) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   m.audits = append(m.audits, row)
	  return nil
   }

  func (m *MemStore) ListAuditByLease(_ context.Context, leaseID string, limit int) ([]IntGateAudit, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  if limit <= 0 {
		   limit = 100
	   }
	   out := make([]IntGateAudit, 0)
	  for _, a := range m.audits {
		   if a.LeaseID == leaseID {
			  out = append(out, a)
			  if len(out) >= limit {
				  break
			   }
		  }
	  }
	  return out, nil
   }
