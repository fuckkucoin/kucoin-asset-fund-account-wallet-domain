package destlife

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// MemCalendarStore 内存 CalendarStore 桩。
type MemCalendarStore struct {
	mu     sync.RWMutex
	byID   map[string]*AllowEntryLife // entry_life_id → entry
	byAddr map[string]*AllowEntryLife // "user|chain|addr" → latest non-revoked
}

  func NewMemCalendarStore() *MemCalendarStore {
	   return &MemCalendarStore{
		   byID: make(map[string]*AllowEntryLife),
		  byAddr: make(map[string]*AllowEntryLife),
	   }
  }

   func addrKey(userID, chain, addr string) string {
	   return userID + "|" + chain + "|" + addr
   }

   func (m *MemCalendarStore) GetByAddr(_ context.Context, userID, chain, addr string) (AllowEntryLife, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	  e,
		  ok := m.byAddr[addrKey(userID, chain, addr)]
	   if !ok || e == nil {
		   return AllowEntryLife{}, ErrEntryNotFound
	   }
	  return *e, nil
  }

   func (m *MemCalendarStore) GetByID(_ context.Context, id string) (AllowEntryLife, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  e, ok := m.byID[id]
	  if !ok || e == nil {
		   return AllowEntryLife{},
			  ErrEntryNotFound
	  }
	  return *e, nil
  }

   func (m *MemCalendarStore) Insert(_ context.Context, e AllowEntryLife) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   cp := e
	  m.byID[e.EntryLifeID] = &cp
	   key := addrKey(e.UserID, e.Chain, e.Address)
	  if prev, ok := m.byAddr[key]; ok && prev.State != StateRevoked {
		  return fmt.Errorf("destlife: entry already exists for %s/%s/%s", e.UserID, e.Chain, e.Address)
	  }
	  m.byAddr[key] = &cp
	   return nil
   }

  func (m *MemCalendarStore) UpdateState(_ context.Context, entryLifeID string, to EntryState, at time.Time, expectedGen, newGen int64) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  e, ok := m.byID[entryLifeID]
	  if !ok {
		   return ErrEntryNotFound
	   }
	   if e.Generation != expectedGen {
		  return fmt.Errorf("destlife: generation mismatch expected=%d got=%d", expectedGen, e.Generation)
	   }
	   e.State = to
	  e.Generation = newGen
	   e.UpdatedAt = at
	   switch to {
	  case StateActive:
		  e.ActiveAt = at
	  case StateRevoking:
		   e.RevokingAt = at
	  case StateRevoked:
		   e.RevokedAt = at
		  // 从 byAddr 中移除（条目已撤销）。
		   delete(m.byAddr, addrKey(e.UserID, e.Chain, e.Address))
	   }
	   return nil
  }

   func (m *MemCalendarStore) ListByUser(_ context.Context, userID string) ([]AllowEntryLife, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	  var out []AllowEntryLife
	   for _, e := range m.byID {
		  if e.UserID == userID && e.State != StateRevoked {
			   out = append(out, *e)
		  }
	   }
	  return out, nil
   }

   func (m *MemCalendarStore) ListCooling(_ context.Context, before time.Time, limit int) ([]AllowEntryLife, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   var out []AllowEntryLife
	  for _, e := range m.byID {
		   if e.State == StateCooling && !e.CoolingFrom.IsZero() && e.CoolingFrom.Before(before) {
			   out = append(out, *e)
			   if limit > 0 && len(out) >= limit {
				   break
			   }
		  }
	  }
	  return out, nil
  }

   func (m *MemCalendarStore) ListRevoking(_ context.Context, limit int) ([]AllowEntryLife, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  var out []AllowEntryLife
	   for _, e := range m.byID {
		  if e.State == StateRevoking {
			   out = append(out, *e)
			   if limit > 0 && len(out) >= limit {
				  break
			  }
		  }
	  }
	   return out, nil
  }

// MemAuditStore 内存 AuditStore 桩。
type MemAuditStore struct {
	mu sync.RWMutex
	rows []LifecycleAuditRow
}

  func NewMemAuditStore() *MemAuditStore {
	   return &MemAuditStore{}
  }

   func (m *MemAuditStore) AppendAudit(_ context.Context, row LifecycleAuditRow) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   m.rows = append(m.rows, row)
	   return nil
   }

  func (m *MemAuditStore) ListAudit(_ context.Context, entryLifeID string, limit int) ([]LifecycleAuditRow, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  var out []LifecycleAuditRow
	  for _, r := range m.rows {
		  if r.EntryLifeID == entryLifeID {
			  out = append(out, r)
			   if limit > 0 && len(out) >= limit {
				   break
			  }
		   }
	  }
	  return out, nil
   }

   func (m *MemAuditStore) ListAuditByUser(_ context.Context, userID string, limit int) ([]LifecycleAuditRow, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	  var out []LifecycleAuditRow
	   for _, r := range m.rows {
		   if r.UserID == userID {
			   out = append(out, r)
			  if limit > 0 && len(out) >= limit {
				   break
			   }
		   }
	  }
	   return out, nil
  }

// MemInflightStore 内存 InflightStore 桩。

type MemInflightStore struct {
	mu   sync.RWMutex
	refs map[string]*InflightRef // ref_id → ref
	mvs map[string]string // movement_id → ref_id
}

  func NewMemInflightStore() *MemInflightStore {
	  return &MemInflightStore{
		  refs: make(map[string]*InflightRef),
		   mvs:  make(map[string]string),
	   }
  }

  func (m *MemInflightStore) InsertRef(_ context.Context, ref InflightRef) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  if _, ok := m.mvs[ref.MovementID]; ok {
		  return ErrDuplicateRef
	   }
	  cp := ref
	  m.refs[ref.RefID] = &cp
	  m.mvs[ref.MovementID] = ref.RefID
	   return nil
   }

   func (m *MemInflightStore) ReleaseRef(_ context.Context, refID string, releasedAt time.Time) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  ref, ok := m.refs[refID]
	  if !ok {
		  return errors.New("destlife: ref not found")
	  }
	  ref.Released = true
	   ref.ReleasedAt = releasedAt
	   return nil
   }

  func (m *MemInflightStore) GetRef(_ context.Context, refID string) (InflightRef, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   ref, ok := m.refs[refID]
	   if !ok {
		  return InflightRef{}, errors.New("destlife: ref not found")
	   }
	  return *ref, nil
   }

  func (m *MemInflightStore) GetRefByMovement(_ context.Context, movementID string) (InflightRef, bool, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	   refID, ok := m.mvs[movementID]
	  if !ok {
		   return InflightRef{}, false, nil
	  }
	   ref, ok := m.refs[refID]
	   if !ok {
		  return InflightRef{}, false, nil
	  }
	   return *ref, true, nil
   }

  func (m *MemInflightStore) ActiveCount(_ context.Context, entryLifeID string) (int, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   cnt := 0
	   for _, ref := range m.refs {
		  if ref.EntryLifeID == entryLifeID && !ref.Released {
			   cnt++
		  }
	  }
	  return cnt, nil
  }

   func (m *MemInflightStore) ListByEntry(_ context.Context, entryLifeID string, limit int) ([]InflightRef, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	  var out []InflightRef
	   for _, ref := range m.refs {
		   if ref.EntryLifeID == entryLifeID {
			   out = append(out, *ref)
			   if limit > 0 && len(out) >= limit {
				   break
			   }
		   }
	  }
	   return out, nil
   }

   func (m *MemInflightStore) ActiveSummary(_ context.Context,
	  entryLifeID string) (InflightCount, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  ic := InflightCount{EntryLifeID: entryLifeID}
	  for _, ref := range m.refs {
		  if ref.EntryLifeID != entryLifeID {
			   continue
		   }
		  ic.Total++
		  if !ref.Released {
			   ic.Active++
		  }
		  if ic.UserID == "" {
			  ic.UserID = ref.UserID
			  ic.Chain = ref.Chain
			   ic.Address = ref.Address
		  }
	   }
	   return ic, nil
   }

// MemWindowStore 内存 WindowStore 桩。

type MemWindowStore struct {
	mu      sync.RWMutex
	windows map[string]CooldownWindow // scope → window
}

   func NewMemWindowStore() *MemWindowStore {
	   return &MemWindowStore{windows: make(map[string]CooldownWindow)}
  }

  func (m *MemWindowStore) GetWindow(_ context.Context, scope string) (CooldownWindow, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	   w, ok := m.windows[scope]
	  if !ok {
		  return CooldownWindow{}, fmt.Errorf("destlife: window not found for scope %q", scope)
	  }
	   return w, nil
  }

  func (m *MemWindowStore) SetWindow(_ context.Context, w CooldownWindow) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   m.windows[w.Scope] = w
	   return nil
   }

// MemBypassStore 内存 BypassStore 桩。

type MemBypassStore struct {
	mu sync.RWMutex
	proofs map[string]*BypassProof // proof_id → proof
}

   func NewMemBypassStore() *MemBypassStore {
	   return &MemBypassStore{proofs: make(map[string]*BypassProof)}
  }

  func (m *MemBypassStore) InsertProof(_ context.Context, proof BypassProof) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   cp := proof
	  m.proofs[proof.ProofID] = &cp
	   return nil
  }

   func (m *MemBypassStore) BlockProof(_ context.Context, proofID, blockedBy string, at time.Time) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  p, ok := m.proofs[proofID]
	  if !ok {
		   return fmt.Errorf("destlife: proof not found: %s", proofID)
	   }
	   p.Blocked = true
	  p.BlockedBy = blockedBy
	  p.BlockedAt = at
	  return nil
  }

  func (m *MemBypassStore) ListByEntry(_ context.Context,
	  entryLifeID string, limit int) ([]BypassProof, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  var out []BypassProof
	   for _, p := range m.proofs {
		  if p.EntryLifeID == entryLifeID {
			  out = append(out, *p)
			   if limit > 0 && len(out) >= limit {
				   break
			  }
		  }
	  }
	  return out, nil
  }

   func (m *MemBypassStore) ListUnblocked(_ context.Context, limit int) ([]BypassProof, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	   var out []BypassProof
	   for _, p := range m.proofs {
		  if !p.Blocked {
			  out = append(out, *p)
			  if limit > 0 && len(out) >= limit {
				   break
			   }
		  }
	   }
	   return out, nil
  }
