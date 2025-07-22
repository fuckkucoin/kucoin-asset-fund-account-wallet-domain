package recycleepoch

import (
	"context"
	"sync"
	"time"
)

// MemStore 内存实现。
type MemStore struct {
	mu     sync.Mutex
	epochs map[string]*RecycleEpoch // epochID
	bySlot map[string]string
	binds   map[string]*ObserveBind // slot
	bindsID map[string]*ObserveBind
	proofs   map[string]*MiscreditProof
	observes map[string]*ObserveRecord // observeID
	byIdem map[string]string
	audits []AuditEvent
}

  func NewMemStore() *MemStore {
	   return &MemStore{
		   epochs: make(map[string]*RecycleEpoch),
		   bySlot: make(map[string]string),
		  binds: make(map[string]*ObserveBind),
		  bindsID: make(map[string]*ObserveBind),
		  proofs:   make(map[string]*MiscreditProof),
		   observes: make(map[string]*ObserveRecord),
		   byIdem: make(map[string]string),
	   }
   }

  func (m *MemStore) Snapshot() *MemStore {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   out := NewMemStore()
	   for k, v := range m.epochs {
		   cp := cloneEpoch(v)
		   out.epochs[k] = cp
	   }
	  for k, v := range m.bySlot {
		  out.bySlot[k] = v
	  }
	   for k, v := range m.binds {
		  cp := cloneBind(v)
		   out.binds[k] = cp
	  }
	   for k, v := range m.bindsID {
		   cp := cloneBind(v)
		  out.bindsID[k] = cp
	   }
	  for k, v := range m.proofs {
		  cp := cloneProof(v)
		   out.proofs[k] = cp
	   }
	  for k, v := range m.observes {
		   cp := *v
		   out.observes[k] = &cp
	   }
	  for k, v := range m.byIdem {
		  out.byIdem[k] = v
	   }
	  out.audits = append(out.audits, m.audits...)
	  return out
  }

   func cloneEpoch(e *RecycleEpoch) *RecycleEpoch {
	  if e == nil {
		   return nil
	   }
	   cp := *e
	  if e.ReadyAt != nil {
		  t := *e.ReadyAt
		   cp.ReadyAt = &t
	   }
	   if e.AssignedAt != nil {
		   t := *e.AssignedAt
		   cp.AssignedAt = &t
	   }
	  return &cp
  }

  func cloneBind(b *ObserveBind) *ObserveBind {
	  if b == nil {
		   return nil
	   }
	  cp := *b
	   if b.RevokedAt != nil {
		   t := *b.RevokedAt
		  cp.RevokedAt = &t
	   }
	   return &cp
   }

  func cloneProof(p *MiscreditProof) *MiscreditProof {
	   if p == nil {
		   return nil
	  }
	  cp := *p
	   if p.AckedAt != nil {
		  t := *p.AckedAt
		  cp.AckedAt = &t
	  }
	   if p.ExportedAt != nil {
		  t := *p.ExportedAt
		  cp.ExportedAt = &t
	   }
	   return &cp
  }

   func (m *MemStore) GetEpoch(_ context.Context, slot SlotKey) (*RecycleEpoch, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   id, ok := m.bySlot[slot.String()]
	   if !ok {
		  return nil, ErrEpochNotFound
	   }
	   e, ok := m.epochs[id]
	  if !ok {
		  return nil, ErrEpochNotFound
	  }
	  return cloneEpoch(e), nil
   }

  func (m *MemStore) GetEpochByID(_ context.Context, epochID string) (*RecycleEpoch, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   e, ok := m.epochs[epochID]
	  if !ok {
		   return nil, ErrEpochNotFound
	   }
	   return cloneEpoch(e), nil
  }

  func (m *MemStore) InsertEpoch(_ context.Context, e RecycleEpoch) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   k := e.Slot.String()
	   if _, exists := m.bySlot[k]; exists {
		   return ErrEpochMismatch
	  }
	   m.epochs[e.EpochID] = cloneEpoch(&e)
	  m.bySlot[k] = e.EpochID
	  return nil
   }

   func (m *MemStore) CASUpdateEpoch(_ context.Context, epochID string, expectedGen int64, expectedState EpochState, next RecycleEpoch) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   e, ok := m.epochs[epochID]
	  if !ok {
		   return ErrEpochNotFound
	   }
	  if e.Generation != expectedGen || e.State != expectedState {
		   return ErrEpochMismatch
	   }
	  if next.Generation < e.Generation {
		   return ErrEpochRollback
	   }
	   cp := cloneEpoch(&next)
	  cp.EpochID = e.EpochID
	   cp.Slot = e.Slot
	   m.epochs[epochID] = cp
	  return nil
   }

  func (m *MemStore) ListEpochsByState(_ context.Context, state EpochState, limit int) ([]RecycleEpoch, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  var out []RecycleEpoch
	  for _, e := range m.epochs {
		  if e.State == state {
			   out = append(out, *cloneEpoch(e))
		  }
		   if limit > 0 && len(out) >= limit {
			  break
		   }
	  }
	  return out, nil
  }

  func (m *MemStore) ListCoolingElapsed(_ context.Context, now time.Time, limit int) ([]RecycleEpoch, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  var out []RecycleEpoch
	   for _, e := range m.epochs {
		   if e.State == EpochCooling && e.CoolElapsed(now) {
			  out = append(out, *cloneEpoch(e))
		   }
		  if limit > 0 && len(out) >= limit {
			   break
		   }
	  }
	   return out, nil
   }

   func (m *MemStore) GetBind(_ context.Context, slot SlotKey) (*ObserveBind, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  b, ok := m.binds[slot.String()]
	  if !ok {
		  return nil, ErrBindNotFound
	  }
	  return cloneBind(b), nil
  }

   func (m *MemStore) GetBindByID(_ context.Context, bindID string) (*ObserveBind, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  b, ok := m.bindsID[bindID]
	  if !ok {
		   return nil, ErrBindNotFound
	   }
	   return cloneBind(b), nil
   }

   func (m *MemStore) UpsertBind(_ context.Context, b ObserveBind) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   cp := cloneBind(&b)
	   m.binds[b.Slot.String()] = cp
	   m.bindsID[b.BindID] = cp
	  return nil
   }

  func (m *MemStore) RevokeBind(_ context.Context, slot SlotKey, reason string, now time.Time) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  b, ok := m.binds[slot.String()]
	  if !ok {
		  return nil
	  }
	  b.State = BindRevoked
	   t := now
	   b.RevokedAt = &t
	  b.RevokeReason = reason
	   return nil
  }

   func (m *MemStore) ListBindsByUser(_ context.Context, userID string, limit int) ([]ObserveBind, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   var out []ObserveBind
	   for _, b := range m.binds {
		   if b.UserID == userID {
			   out = append(out, *cloneBind(b))
		   }
		  if limit > 0 && len(out) >= limit {
			  break
		   }
	  }
	   return out, nil
   }

  func (m *MemStore) InsertProof(_ context.Context, p MiscreditProof) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   m.proofs[p.ProofID] = cloneProof(&p)
	   return nil
   }

  func (m *MemStore) GetProof(_ context.Context, proofID string) (*MiscreditProof, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   p, ok := m.proofs[proofID]
	   if !ok {
		   return nil, ErrProofNotFound
	  }
	  return cloneProof(p), nil
   }

  func (m *MemStore) ListOpenProofs(_ context.Context, limit int) ([]MiscreditProof, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   var out []MiscreditProof
	  for _, p := range m.proofs {
		   if p.State == ProofOpen {
			   out = append(out, *cloneProof(p))
		   }
		   if limit > 0 && len(out) >= limit {
			  break
		   }
	  }
	   return out, nil
  }

   func (m *MemStore) ListProofsBySlot(_ context.Context, slot SlotKey) ([]MiscreditProof, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   var out []MiscreditProof
	   ks := slot.String()
	  for _, p := range m.proofs {
		  if p.Slot.String() == ks {
			   out = append(out, *cloneProof(p))
		   }
	  }
	   return out, nil
  }

   func (m *MemStore) AckProof(_ context.Context, proofID string, now time.Time) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  p, ok := m.proofs[proofID]
	  if !ok {
		  return ErrProofNotFound
	  }
	   if p.State == ProofAcked {
		   return ErrProofAlreadyAcked
	  }
	   p.State = ProofAcked
	   t := now
	  p.AckedAt = &t
	   return nil
   }

   func (m *MemStore) MarkProofExported(_ context.Context, proofID string, now time.Time) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  p, ok := m.proofs[proofID]
	   if !ok {
		   return ErrProofNotFound
	   }
	   t := now
	  p.ExportedAt = &t
	  return nil
  }

   func (m *MemStore) InsertObserve(_ context.Context, r ObserveRecord) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  if r.IdemKey != "" {
		   if id, ok := m.byIdem[r.IdemKey]; ok {
			   prev := m.observes[id]
			  if prev != nil && (prev.UserID != r.UserID || prev.TxHash != r.TxHash || prev.Outcome != r.Outcome) {
				  return ErrObserveIdemConflict
			   }
			  return nil
		   }
		   m.byIdem[r.IdemKey] = r.ObserveID
	  }
	   cp := r
	   m.observes[r.ObserveID] = &cp
	  return nil
  }

  func (m *MemStore) GetObserveByIdem(_ context.Context, idemKey string) (*ObserveRecord, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   id, ok := m.byIdem[idemKey]
	  if !ok {
		   return nil, ErrBindNotFound
	  }
	  r, ok := m.observes[id]
	  if !ok {
		  return nil, ErrBindNotFound
	  }
	  cp := *r
	  return &cp, nil
  }

   func (m *MemStore) ListObservesBySlot(_ context.Context, slot SlotKey, limit int) ([]ObserveRecord, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  var out []ObserveRecord
	  ks := slot.String()
	   for _, r := range m.observes {
		  if r.Slot.String() == ks {
			   out = append(out, *r)
		  }
		  if limit > 0 && len(out) >= limit {
			   break
		  }
	  }
	   return out, nil
  }

  func (m *MemStore) InsertAudit(_ context.Context, ev AuditEvent) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  m.audits = append(m.audits, ev)
	   return nil
   }

  func (m *MemStore) ListAudit(_ context.Context, slot SlotKey, limit int) ([]AuditEvent, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  var out []AuditEvent
	  ks := slot.String()
	  for _, ev := range m.audits {
		   if ev.Slot.String() == ks {
			   out = append(out, ev)
		  }
		  if limit > 0 && len(out) >= limit {
			  break
		  }
	  }
	   return out, nil
   }
