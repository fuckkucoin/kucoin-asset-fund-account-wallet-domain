package oracletickgate

import (
	"context"
	"strings"
	"sync"
	"time"
)

// MemStore 内存实现。

type MemStore struct {
	mu sync.Mutex
	lease map[string]TickLease
	gens map[string]TickGeneration
	proof map[string]TickDriftProof
	audit []AuditEvent
	write map[string]FundWriteRecord
	idem  map[string]string
}

// NewMemStore 空仓。
   func NewMemStore() *MemStore {
	  return &MemStore{
		  lease: map[string]TickLease{},
		  gens:  map[string]TickGeneration{},
		   proof: map[string]TickDriftProof{},
		   write: map[string]FundWriteRecord{},
		   idem: map[string]string{},
	  }
  }

   func (m *MemStore) InsertLease(_ context.Context, l TickLease) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   if _, ok := m.lease[l.LeaseID]; ok {
		   return ErrIdem
	   }
	   m.lease[l.LeaseID] = l
	   return nil
  }

   func (m *MemStore) GetLease(_ context.Context, leaseID string) (*TickLease, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  l, ok := m.lease[strings.TrimSpace(leaseID)]
	   if !ok {
		   return nil, ErrLeaseNotFound
	  }
	   cp := l
	  return &cp, nil
  }

   func (m *MemStore) CASLease(_ context.Context, leaseID string, expectedGen int64, expectedState LeaseState, next TickLease) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   cur, ok := m.lease[strings.TrimSpace(leaseID)]
	   if !ok {
		   return ErrLeaseNotFound
	  }
	   if cur.Generation != expectedGen || cur.State != expectedState {
		   return ErrCAS
	  }
	  if err := CanTransit(cur.State, next.State); err != nil {
		  return err
	   }
	   m.lease[leaseID] = next
	  return nil
  }

   func (m *MemStore) ListLeasesByState(_ context.Context, state LeaseState, limit int) ([]TickLease, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  out := make([]TickLease, 0)
	   for _, l := range m.lease {
		  if l.State == state {
			  out = append(out, l)
			  if limit > 0 && len(out) >= limit {
				   break
			  }
		   }
	  }
	  return out, nil
  }

  func (m *MemStore) ListOpenLeasesByPair(_ context.Context, pair string) ([]TickLease, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  pair = strings.TrimSpace(pair)
	   out := make([]TickLease, 0)
	  for _, l := range m.lease {
		  if l.Pair == pair && !l.State.Terminal() {
			  out = append(out, l)
		   }
	   }
	  return out, nil
   }

  func (m *MemStore) ListExpiredOpen(_ context.Context, now time.Time, limit int) ([]TickLease, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  out := make([]TickLease, 0)
	   for _, l := range m.lease {
		  if l.State.Terminal() {
			   continue
		   }
		   if !l.ExpiresAt.IsZero() && !now.Before(l.ExpiresAt) {
			  out = append(out, l)
			   if limit > 0 && len(out) >= limit {
				   break
			  }
		  }
	  }
	   return out, nil
   }

   func (m *MemStore) ListOpenLeases(_ context.Context, limit int) ([]TickLease, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   out := make([]TickLease, 0)
	  for _, l := range m.lease {
		   if !l.State.Terminal() {
			   out = append(out, l)
			  if limit > 0 && len(out) >= limit {
				  break
			   }
		   }
	  }
	  return out, nil
  }

  func (m *MemStore) UpsertGeneration(_ context.Context, g TickGeneration) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   m.gens[g.Pair] = g
	   return nil
  }

   func (m *MemStore) GetGeneration(_ context.Context, pair string) (*TickGeneration, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   g, ok := m.gens[strings.TrimSpace(pair)]
	  if !ok {
		   return nil, ErrTickMissing
	   }
	  cp := g
	  return &cp, nil
  }

   func (m *MemStore) ListGenerations(_ context.Context, limit int) ([]TickGeneration, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  out := make([]TickGeneration,
		  0, len(m.gens))
	  for _, g := range m.gens {
		  out = append(out, g)
		   if limit > 0 && len(out) >= limit {
			   break
		   }
	   }
	  return out, nil
  }

  func (m *MemStore) InsertProof(_ context.Context, p TickDriftProof) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   if _, ok := m.proof[p.ProofID]; ok {
		   return ErrIdem
	   }
	  m.proof[p.ProofID] = p
	   return nil
  }

  func (m *MemStore) GetProof(_ context.Context, id string) (*TickDriftProof, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  p, ok := m.proof[strings.TrimSpace(id)]
	   if !ok {
		  return nil, ErrProofNotFound
	   }
	  cp := p
	  return &cp, nil
   }

   func (m *MemStore) ListProofs(_ context.Context, limit int) ([]TickDriftProof, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   out := make([]TickDriftProof, 0, len(m.proof))
	   for _, p := range m.proof {
		   out = append(out, p)
		   if limit > 0 && len(out) >= limit {
			   break
		  }
	  }
	  return out, nil
  }

   func (m *MemStore) ListProofsByLease(_ context.Context, leaseID string) ([]TickDriftProof, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   out := make([]TickDriftProof, 0)
	  for _, p := range m.proof {
		   if p.LeaseID == leaseID {
			   out = append(out, p)
		  }
	  }
	  return out, nil
   }

  func (m *MemStore) MarkProofExported(_ context.Context, id string, now time.Time) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  p, ok := m.proof[strings.TrimSpace(id)]
	   if !ok {
		  return ErrProofNotFound
	   }
	  p.ExportedAt = &now
	   m.proof[id] = p
	  return nil
  }

  func (m *MemStore) InsertAudit(_ context.Context, a AuditEvent) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   m.audit = append(m.audit, a)
	  return nil
  }

   func (m *MemStore) ListAuditsByLease(_ context.Context, leaseID string, limit int) ([]AuditEvent, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  out := make([]AuditEvent, 0)
	  for _, a := range m.audit {
		   if a.LeaseID == leaseID {
			  out = append(out, a)
			  if limit > 0 && len(out) >= limit {
				  break
			   }
		  }
	   }
	  return out, nil
  }

  func (m *MemStore) ListAuditsByPair(_ context.Context, pair string, limit int) ([]AuditEvent, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   out := make([]AuditEvent, 0)
	   for _, a := range m.audit {
		  if a.Pair == pair {
			   out = append(out, a)
			  if limit > 0 && len(out) >= limit {
				   break
			  }
		   }
	  }
	   return out, nil
   }

  func (m *MemStore) InsertWrite(_ context.Context, w FundWriteRecord) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   if _, ok := m.write[w.LeaseID]; ok {
		   return ErrDoubleSpend
	  }
	   m.write[w.LeaseID] = w
	   return nil
  }

   func (m *MemStore) GetWriteByLease(_ context.Context, leaseID string) (*FundWriteRecord, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   w,
		  ok := m.write[strings.TrimSpace(leaseID)]
	   if !ok {
		  return nil, ErrNoLease
	  }
	   cp := w
	   return &cp, nil
  }

   func (m *MemStore) PutIdem(_ context.Context, key, resultID string) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if old, ok := m.idem[key]; ok && old != resultID {
		   return ErrIdem
	  }
	   m.idem[key] = resultID
	  return nil
  }

  func (m *MemStore) GetIdem(_ context.Context, key string) (string, bool, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   id, ok := m.idem[key]
	   return id, ok, nil
  }

  func (m *MemStore) Snapshot() StoreSnapshot {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   s := StoreSnapshot{Idem: map[string]string{}}
	   for _, l := range m.lease {
		  s.Leases = append(s.Leases, l)
	  }
	  for _, g := range m.gens {
		   s.Generations = append(s.Generations, g)
	   }
	   for _, p := range m.proof {
		  s.Proofs = append(s.Proofs, p)
	   }
	  s.Audits = append(s.Audits, m.audit...)
	  for _, w := range m.write {
		  s.Writes = append(s.Writes, w)
	   }
	   for k, v := range m.idem {
		   s.Idem[k] = v
	  }
	  return s
   }

  func (m *MemStore) Restore(s StoreSnapshot) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   m.lease = map[string]TickLease{}
	   m.gens = map[string]TickGeneration{}
	  m.proof = map[string]TickDriftProof{}
	  m.write = map[string]FundWriteRecord{}
	  m.idem = map[string]string{}
	  m.audit = nil
	   for _, l := range s.Leases {
		   m.lease[l.LeaseID] = l
	  }
	   for _, g := range s.Generations {
		  m.gens[g.Pair] = g
	   }
	   for _, p := range s.Proofs {
		   m.proof[p.ProofID] = p
	  }
	  m.audit = append(m.audit, s.Audits...)
	  for _, w := range s.Writes {
		   m.write[w.LeaseID] = w
	  }
	  for k, v := range s.Idem {
		  m.idem[k] = v
	  }
   }
