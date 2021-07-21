package holdwritegate

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Store 持久化接口。走 SQL 实现；走 MemStore。

type Store interface {
	SaveBlockLease(ctx context.Context, l HoldBlockLease) (HoldBlockLease, error)
	GetBlockLease(ctx context.Context, id string) (HoldBlockLease, error)
	GetActiveLeaseBySubject(ctx context.Context, userID, asset, book string) (HoldBlockLease, error)
	ListBlockLeasesByUser(ctx context.Context, userID string, limit int) ([]HoldBlockLease, error)
	ListExpiredLeases(ctx context.Context, before time.Time, limit int) ([]HoldBlockLease, error)
	ListBlockingLeases(ctx context.Context, limit int) ([]HoldBlockLease, error)
	LatestGenForSubject(ctx context.Context, userID, asset, book string) (int64, error)
	SaveEscalation(ctx context.Context,
		e AdviceEscalation) (AdviceEscalation, error)
	GetEscalation(ctx context.Context, id string) (AdviceEscalation, error)
	GetEscalationByAdvice(ctx context.Context, adviceID string) (AdviceEscalation, error)
	ListEscalationsByLease(ctx context.Context, leaseID string, limit int) ([]AdviceEscalation, error)
	ListEscalationsByState(ctx context.Context, state EscalationState, limit int) ([]AdviceEscalation, error)
	ListEscalationsByUser(ctx context.Context,
		userID string, limit int) ([]AdviceEscalation, error)
	SaveCloseProof(ctx context.Context, p GapCloseProof) (GapCloseProof, error)
	GetCloseProof(ctx context.Context, id string) (GapCloseProof, error)
	ListProofsByLease(ctx context.Context, leaseID string, limit int) ([]GapCloseProof, error)
	ListProofsByUser(ctx context.Context, userID string, limit int) ([]GapCloseProof, error)
	SaveWriteGateEntry(ctx context.Context, e WriteGateEntry) (WriteGateEntry, error)
	ListWriteGateByLease(ctx context.Context, leaseID string, limit int) ([]WriteGateEntry, error)
	SaveOrphanTagBind(ctx context.Context,
		b OrphanTagBind) (OrphanTagBind, error)
	ListOrphanBindsByLease(ctx context.Context, leaseID string, limit int) ([]OrphanTagBind, error)
}

// ──────────────────────────────────────────
// MemStore 内存实现（用）
// ──────────────────────────────────────────

type MemStore struct {
	mu     sync.Mutex
	leases map[string]HoldBlockLease
	escalations map[string]AdviceEscalation
	proofs      map[string]GapCloseProof
	gateEntries map[string]WriteGateEntry
	orphanBinds map[string]OrphanTagBind
}

   func NewMemStore() *MemStore {
	  return &MemStore{
		  leases:      make(map[string]HoldBlockLease),
		  escalations: make(map[string]AdviceEscalation),
		   proofs:      make(map[string]GapCloseProof),
		  gateEntries: make(map[string]WriteGateEntry),
		   orphanBinds: make(map[string]OrphanTagBind),
	   }
  }

// ──── BlockLease ────

   func (m *MemStore) SaveBlockLease(_ context.Context, l HoldBlockLease) (HoldBlockLease, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   if strings.TrimSpace(l.ID) == "" {
		   return HoldBlockLease{}, fmt.Errorf("block lease id empty")
	   }
	   m.leases[l.ID] = l
	   return l, nil
  }

  func (m *MemStore) GetBlockLease(_ context.Context, id string) (HoldBlockLease, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   l, ok := m.leases[id]
	   if !ok {
		  return HoldBlockLease{}, ErrBlockLeaseNotFound
	   }
	   return l, nil
   }

   func (m *MemStore) GetActiveLeaseBySubject(_ context.Context, userID, asset, book string) (HoldBlockLease, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   for _, l := range m.leases {
		   if l.UserID == userID && l.Asset == asset && l.Book == book &&
			   (l.State == BlockLeaseOpen || l.State == BlockLeaseBlocking) {
			   return l, nil
		  }
	   }
	  return HoldBlockLease{}, ErrBlockLeaseNotFound
   }

   func (m *MemStore) ListBlockLeasesByUser(_ context.Context, userID string, limit int) ([]HoldBlockLease, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   var out []HoldBlockLease
	  for _, l := range m.leases {
		   if l.UserID == userID {
			  out = append(out, l)
			  if len(out) >= limit {
				  break
			  }
		  }
	   }
	  sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	   return out, nil
   }

   func (m *MemStore) ListExpiredLeases(_ context.Context, before time.Time, limit int) ([]HoldBlockLease, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  var out []HoldBlockLease
	   for _, l := range m.leases {
		  if (l.State == BlockLeaseOpen || l.State == BlockLeaseBlocking) && l.LeaseExpiry.Before(before) {
			  out = append(out, l)
			   if len(out) >= limit {
				   break
			   }
		  }
	   }
	   return out, nil
   }

  func (m *MemStore) ListBlockingLeases(_ context.Context, limit int) ([]HoldBlockLease, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  var out []HoldBlockLease
	  for _, l := range m.leases {
		   if l.State == BlockLeaseBlocking {
			   out = append(out, l)
			  if len(out) >= limit {
				  break
			  }
		   }
	   }
	   sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	   return out, nil
  }

  func (m *MemStore) LatestGenForSubject(_ context.Context, userID, asset, book string) (int64, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  var gen int64
	  for _, l := range m.leases {
		   if l.UserID == userID && l.Asset == asset && l.Book == book && l.BlockGen > gen {
			  gen = l.BlockGen
		  }
	   }
	  return gen, nil
   }

// ──── Escalation ────

   func (m *MemStore) SaveEscalation(_ context.Context, e AdviceEscalation) (AdviceEscalation, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  if strings.TrimSpace(e.ID) == "" {
		  return AdviceEscalation{}, fmt.Errorf("escalation id empty")
	   }
	  m.escalations[e.ID] = e
	   return e, nil
   }

   func (m *MemStore) GetEscalation(_ context.Context, id string) (AdviceEscalation, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   e, ok := m.escalations[id]
	   if !ok {
		   return AdviceEscalation{}, ErrEscalationNotFound
	  }
	  return e, nil
   }

  func (m *MemStore) GetEscalationByAdvice(_ context.Context, adviceID string) (AdviceEscalation, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  for _, e := range m.escalations {
		  if e.AdviceID == adviceID {
			   return e, nil
		   }
	  }
	   return AdviceEscalation{}, ErrEscalationNotFound
  }

   func (m *MemStore) ListEscalationsByLease(_ context.Context, leaseID string, limit int) ([]AdviceEscalation, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   var out []AdviceEscalation
	  for _, e := range m.escalations {
		  if e.LeaseID == leaseID {
			  out = append(out, e)
			  if len(out) >= limit {
				   break
			  }
		  }
	   }
	   sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	  return out, nil
  }

  func (m *MemStore) ListEscalationsByState(_ context.Context, state EscalationState, limit int) ([]AdviceEscalation, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  var out []AdviceEscalation
	   for _, e := range m.escalations {
		  if e.State == state {
			   out = append(out, e)
			  if len(out) >= limit {
				   break
			   }
		   }
	  }
	   sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	  return out, nil
  }

  func (m *MemStore) ListEscalationsByUser(_ context.Context, userID string, limit int) ([]AdviceEscalation, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  var out []AdviceEscalation
	  for _, e := range m.escalations {
		   if e.UserID == userID {
			   out = append(out, e)
			   if len(out) >= limit {
				   break
			   }
		   }
	  }
	  sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	  return out, nil
   }

// ──── CloseProof ────

  func (m *MemStore) SaveCloseProof(_ context.Context, p GapCloseProof) (GapCloseProof, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  if strings.TrimSpace(p.ID) == "" {
		  return GapCloseProof{}, fmt.Errorf("close proof id empty")
	  }
	  m.proofs[p.ID] = p
	   return p, nil
   }

   func (m *MemStore) GetCloseProof(_ context.Context, id string) (GapCloseProof, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   p, ok := m.proofs[id]
	   if !ok {
		  return GapCloseProof{}, ErrCloseProofNotFound
	  }
	  return p, nil
   }

  func (m *MemStore) ListProofsByLease(_ context.Context, leaseID string, limit int) ([]GapCloseProof, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   var out []GapCloseProof
	  for _, p := range m.proofs {
		  if p.LeaseID == leaseID {
			   out = append(out, p)
			   if len(out) >= limit {
				  break
			   }
		   }
	  }
	  sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	  return out, nil
   }

   func (m *MemStore) ListProofsByUser(_ context.Context, userID string, limit int) ([]GapCloseProof, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  var out []GapCloseProof
	  for _, p := range m.proofs {
		   if p.UserID == userID {
			   out = append(out, p)
			  if len(out) >= limit {
				  break
			  }
		   }
	  }
	   sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	  return out, nil
   }

// ──── WriteGateEntry ────

   func (m *MemStore) SaveWriteGateEntry(_ context.Context, e WriteGateEntry) (WriteGateEntry, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if strings.TrimSpace(e.ID) == "" {
		  return WriteGateEntry{}, fmt.Errorf("write gate entry id empty")
	   }
	  m.gateEntries[e.ID] = e
	  return e, nil
   }

   func (m *MemStore) ListWriteGateByLease(_ context.Context, leaseID string, limit int) ([]WriteGateEntry, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   var out []WriteGateEntry
	   for _, e := range m.gateEntries {
		   if e.LeaseID == leaseID {
			   out = append(out, e)
			   if len(out) >= limit {
				  break
			  }
		   }
	   }
	   sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	   return out, nil
   }

// ──── OrphanTagBind ────

  func (m *MemStore) SaveOrphanTagBind(_ context.Context, b OrphanTagBind) (OrphanTagBind, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   if strings.TrimSpace(b.ID) == "" {
		   return OrphanTagBind{}, fmt.Errorf("orphan tag bind id empty")
	   }
	   m.orphanBinds[b.ID] = b
	   return b, nil
   }

  func (m *MemStore) ListOrphanBindsByLease(_ context.Context, leaseID string, limit int) ([]OrphanTagBind, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  var out []OrphanTagBind
	  for _, b := range m.orphanBinds {
		  if b.LeaseID == leaseID {
			  out = append(out, b)
			   if len(out) >= limit {
				   break
			   }
		   }
	   }
	   sort.Slice(out, func(i, j int) bool { return out[i].BoundAt.Before(out[j].BoundAt) })
	   return out, nil
  }
