package keyrotgate

import (
	"context"
	"sync"
	"time"
)

// MemStore 基于内存的 Store 实现。

type MemStore struct {
	mu     sync.RWMutex
	leases map[string]*RotateLease
	binds  map[string]*SignBind
	drifts map[string]*KeyRotDriftCase
	proofs map[string]*FingerprintProof
	clock  func() time.Time
}

// NewMemStore 构造内存 Store。

  func NewMemStore() *MemStore {
	  return &MemStore{
		   leases: make(map[string]*RotateLease),
		  binds:  make(map[string]*SignBind),
		   drifts: make(map[string]*KeyRotDriftCase),
		   proofs: make(map[string]*FingerprintProof),
		   clock:  func() time.Time { return time.Now().UTC() },
	  }
  }

// WithClock 注入时钟（用）。

  func (m *MemStore) WithClock(fn func() time.Time) *MemStore {
	  if fn != nil {
		  m.clock = fn
	   }
	   return m
   }

   // ── RotateLease ──────────────────────────────────────────────────────────────

  func (m *MemStore) SaveLease(_ context.Context, l *RotateLease) error {
	  if m == nil || l == nil {
		  return ErrUnavailable
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  cp := *l
	  m.leases[l.RotateLeaseID] = &cp
	  return nil
  }

   func (m *MemStore) GetLease(_ context.Context, id string) (*RotateLease, error) {
	   if m == nil {
		   return nil, ErrUnavailable
	   }
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  l, ok := m.leases[id]
	   if !ok {
		  return nil, ErrLeaseNotFound
	  }
	   cp := *l
	   return &cp, nil
   }

   func (m *MemStore) GetLeaseByTicket(_ context.Context, ticketID string) (*RotateLease, error) {
	  if m == nil {
		   return nil, ErrUnavailable
	  }
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  for _, l := range m.leases {
		   if l.TicketID == ticketID {
			   cp := *l
			  return &cp, nil
		  }
	   }
	  return nil, ErrLeaseNotFound
  }

   func (m *MemStore) ListActiveLeasesBySlot(_ context.Context, srcSlotID string) ([]*RotateLease, error) {
	   if m == nil {
		   return nil, ErrUnavailable
	   }
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	   var out []*RotateLease
	   for _, l := range m.leases {
		   if l.SrcSlotID == srcSlotID && !LeaseIsTerminal(l.State) {
			   cp := *l
			   out = append(out, &cp)
		  }
	  }
	  return out, nil
   }

  func (m *MemStore) UpdateLeaseState(_ context.Context, id string, from, to LeaseState, update *RotateLease) error {
	   if m == nil {
		   return ErrUnavailable
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  l, ok := m.leases[id]
	  if !ok {
		  return ErrLeaseNotFound
	   }
	  if l.State != from {
		  return ErrInvalidTransition
	   }
	  cp := *update
	   cp.State = to
	  m.leases[id] = &cp
	   return nil
  }

  func (m *MemStore) ListExpiredLeases(_ context.Context, limit int) ([]*RotateLease, error) {
	   if m == nil {
		  return nil, ErrUnavailable
	  }
	  now := m.clock()
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   var out []*RotateLease
	   for _, l := range m.leases {
		  if !LeaseIsTerminal(l.State) && now.After(l.ExpiresAt) {
			   cp := *l
			  out = append(out, &cp)
		  }
		  if limit > 0 && len(out) >= limit {
			   break
		   }
	  }
	   return out, nil
   }

// ── SignBind ──────────────────────────────────────────────────────────────────

  func (m *MemStore) SaveSignBind(_ context.Context, b *SignBind) error {
	  if m == nil || b == nil {
		   return ErrUnavailable
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   cp := *b
	  m.binds[b.SignBindID] = &cp
	   return nil
  }

   func (m *MemStore) GetSignBind(_ context.Context, id string) (*SignBind, error) {
	   if m == nil {
		  return nil, ErrUnavailable
	   }
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	   b, ok := m.binds[id]
	  if !ok {
		   return nil, ErrSignBindNotFound
	  }
	   cp := *b
	   return &cp, nil
   }

   func (m *MemStore) GetSignBindByOperation(_ context.Context, operationID string) (*SignBind, error) {
	   if m == nil {
		  return nil, ErrUnavailable
	  }
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   for _, b := range m.binds {
		  if b.OperationID == operationID {
			   cp := *b
			  return &cp, nil
		  }
	   }
	   return nil, ErrSignBindNotFound
  }

   func (m *MemStore) ListActiveSignBinds(_ context.Context, srcSlotID string) ([]*SignBind, error) {
	  if m == nil {
		  return nil, ErrUnavailable
	   }
	   now := m.clock()
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  var out []*SignBind
	  for _, b := range m.binds {
		  if b.SrcSlotID == srcSlotID && b.IsActive(now) {
			  cp := *b
			  out = append(out, &cp)
		  }
	   }
	  return out, nil
  }

  func (m *MemStore) UpdateSignBindState(_ context.Context, id string, from, to SignBindState, update *SignBind) error {
	   if m == nil {
		  return ErrUnavailable
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   b, ok := m.binds[id]
	   if !ok {
		  return ErrSignBindNotFound
	   }
	   if b.State != from {
		   return ErrInvalidTransition
	  }
	   cp := *update
	   cp.State = to
	   m.binds[id] = &cp
	  return nil
   }

  func (m *MemStore) ListExpiredSignBinds(_ context.Context, limit int) ([]*SignBind, error) {
	   if m == nil {
		  return nil, ErrUnavailable
	   }
	  now := m.clock()
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   var out []*SignBind
	   for _, b := range m.binds {
		   if b.State == SignBindActive && now.After(b.ExpiresAt) {
			   cp := *b
			  out = append(out, &cp)
		   }
		  if limit > 0 && len(out) >= limit {
			   break
		  }
	  }
	  return out, nil
   }

// ── KeyRotDriftCase ───────────────────────────────────────────────────────────

  func (m *MemStore) SaveDriftCase(_ context.Context, d *KeyRotDriftCase) error {
	   if m == nil || d == nil {
		  return ErrUnavailable
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  cp := *d
	  m.drifts[d.DriftCaseID] = &cp
	  return nil
   }

  func (m *MemStore) GetDriftCase(_ context.Context, id string) (*KeyRotDriftCase, error) {
	  if m == nil {
		  return nil, ErrUnavailable
	  }
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   d, ok := m.drifts[id]
	   if !ok {
		  return nil, ErrDriftCaseNotFound
	   }
	  cp := *d
	  return &cp, nil
  }

   func (m *MemStore) ListOpenDriftCases(_ context.Context, srcSlotID string) ([]*KeyRotDriftCase, error) {
	  if m == nil {
		  return nil, ErrUnavailable
	   }
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  var out []*KeyRotDriftCase
	  for _, d := range m.drifts {
		   if (srcSlotID == "" || d.SrcSlotID == srcSlotID) && !DriftCaseIsTerminal(d.State) {
			   cp := *d
			  out = append(out, &cp)
		  }
	  }
	   return out, nil
   }

  func (m *MemStore) UpdateDriftCaseState(_ context.Context, id string, from, to DriftCaseState, update *KeyRotDriftCase) error {
	  if m == nil {
		  return ErrUnavailable
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   d, ok := m.drifts[id]
	  if !ok {
		   return ErrDriftCaseNotFound
	   }
	  if d.State != from {
		   return ErrInvalidTransition
	  }
	  cp := *update
	  cp.State = to
	   m.drifts[id] = &cp
	   return nil
   }

// ── FingerprintProof ──────────────────────────────────────────────────────────

  func (m *MemStore) SaveProof(_ context.Context, p *FingerprintProof) error {
	  if m == nil || p == nil {
		  return ErrUnavailable
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  cp := *p
	   m.proofs[p.ProofID] = &cp
	  return nil
   }

  func (m *MemStore) GetProof(_ context.Context, id string) (*FingerprintProof, error) {
	  if m == nil {
		  return nil, ErrUnavailable
	   }
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	   p, ok := m.proofs[id]
	   if !ok {
		  return nil, ErrProofNotFound
	   }
	   cp := *p
	  return &cp, nil
  }

   func (m *MemStore) GetProofByLease(_ context.Context, leaseID string) (*FingerprintProof, error) {
	  if m == nil {
		   return nil, ErrUnavailable
	  }
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	   for _, p := range m.proofs {
		  if p.RotateLeaseID == leaseID {
			  cp := *p
			   return &cp, nil
		   }
	  }
	   return nil, ErrProofNotFound
  }

   func (m *MemStore) GetLatestProofBySlot(_ context.Context, srcSlotID string) (*FingerprintProof, error) {
	   if m == nil {
		  return nil, ErrUnavailable
	  }
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	   var latest *FingerprintProof
	   for _, p := range m.proofs {
		   if p.SrcSlotID == srcSlotID {
			  if latest == nil || p.IssuedAt.After(latest.IssuedAt) {
				   cp := *p
				  latest = &cp
			  }
		   }
	  }
	   if latest == nil {
		   return nil, ErrProofNotFound
	  }
	   return latest, nil
   }
