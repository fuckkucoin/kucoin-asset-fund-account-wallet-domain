package airimportgate

import (
	"context"
	"sync"
	"time"
)

// MemStore 基于内存的 Store 实现（用）。
type MemStore struct {
	mu      sync.RWMutex
	leases  map[string]*ImportLease
	digests map[string]*SealedBlobDigest // key=DigestHex
	digestIDs map[string]*SealedBlobDigest // key=DigestID
	conflicts map[string]*PhaseConflictProof
	binds     map[string]*QuorumBind
	bindByLease map[string]*QuorumBind // key=LeaseID
	clock       func() time.Time
}

// NewMemStore 构造内存 Store。

   func NewMemStore() *MemStore {
	   return &MemStore{
		   leases:    make(map[string]*ImportLease),
		   digests:   make(map[string]*SealedBlobDigest),
		   digestIDs: make(map[string]*SealedBlobDigest),
		   conflicts:   make(map[string]*PhaseConflictProof),
		   binds:       make(map[string]*QuorumBind),
		  bindByLease: make(map[string]*QuorumBind),
		  clock:       func() time.Time { return time.Now().UTC() },
	  }
   }

// WithClock 注入时钟（用）。
  func (m *MemStore) WithClock(fn func() time.Time) *MemStore {
	   if fn != nil {
		   m.clock = fn
	  }
	  return m
   }

   // ── ImportLease ───────────────────────────────────────────────────────────────

   func (m *MemStore) SaveLease(_ context.Context, l *ImportLease) error {
	   if m == nil || l == nil {
		   return ErrUnavailable
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  cp := l.Copy()
	  m.leases[l.LeaseID] = &cp
	   return nil
  }

  func (m *MemStore) GetLease(_ context.Context, id string) (*ImportLease, error) {
	  if m == nil {
		   return nil, ErrUnavailable
	   }
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  l, ok := m.leases[id]
	  if !ok {
		  return nil, ErrLeaseNotFound
	  }
	  cp := l.Copy()
	   return &cp, nil
  }

   func (m *MemStore) UpdateLeaseState(_ context.Context, id string, from, to LeaseState, update *ImportLease) error {
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
	   if update != nil {
		   cp := update.Copy()
		  cp.State = to
		   m.leases[id] = &cp
	   } else {
		  l.State = to
		   l.UpdatedAt = m.clock()
	   }
	  return nil
  }

   func (m *MemStore) ListActiveLeasesForSession(_ context.Context, sessionID string) ([]*ImportLease, error) {
	   if m == nil {
		  return nil, ErrUnavailable
	   }
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	  var out []*ImportLease
	  for _, l := range m.leases {
		   if l.SessionID == sessionID && !l.State.Terminal() {
			   cp := l.Copy()
			   out = append(out, &cp)
		   }
	  }
	  return out, nil
   }

  func (m *MemStore) ListExpiredLeases(_ context.Context, limit int) ([]*ImportLease, error) {
	  if m == nil {
		  return nil, ErrUnavailable
	   }
	  now := m.clock()
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	  var out []*ImportLease
	   for _, l := range m.leases {
		   if !l.State.Terminal() && l.ExpiresAt.Before(now) {
			   cp := l.Copy()
			  out = append(out, &cp)
			  if limit > 0 && len(out) >= limit {
				   break
			   }
		  }
	  }
	  return out, nil
  }

   func (m *MemStore) ListLeasesBySession(_ context.Context, sessionID string, limit int) ([]*ImportLease, error) {
	   if m == nil {
		  return nil, ErrUnavailable
	   }
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  var out []*ImportLease
	  for _, l := range m.leases {
		   if l.SessionID == sessionID {
			  cp := l.Copy()
			  out = append(out, &cp)
			   if limit > 0 && len(out) >= limit {
				   break
			  }
		   }
	  }
	   return out, nil
  }

// ── SealedBlobDigest ──────────────────────────────────────────────────────────

  func (m *MemStore) SaveDigest(_ context.Context, d *SealedBlobDigest) error {
	  if m == nil || d == nil {
		   return ErrUnavailable
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  if _, exists := m.digests[d.DigestHex]; exists {
		  return ErrBlobReplay
	  }
	   cp := d.Copy()
	  m.digests[d.DigestHex] = &cp
	   m.digestIDs[d.DigestID] = &cp
	  return nil
   }

  func (m *MemStore) GetDigestByHex(_ context.Context, hex string) (*SealedBlobDigest, error) {
	  if m == nil {
		  return nil, ErrUnavailable
	   }
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   d, ok := m.digests[hex]
	   if !ok {
		  return nil, ErrDigestNotFound
	   }
	   cp := d.Copy()
	  return &cp, nil
  }

   func (m *MemStore) ListDigestsBySession(_ context.Context, sessionID string) ([]*SealedBlobDigest, error) {
	   if m == nil {
		  return nil, ErrUnavailable
	   }
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	   var out []*SealedBlobDigest
	   for _, d := range m.digestIDs {
		  if d.SessionID == sessionID {
			   cp := d.Copy()
			   out = append(out, &cp)
		  }
	   }
	   return out, nil
  }

// ── PhaseConflictProof ────────────────────────────────────────────────────────

  func (m *MemStore) SaveConflict(_ context.Context, p *PhaseConflictProof) error {
	   if m == nil || p == nil {
		   return ErrUnavailable
	  }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   cp := p.Copy()
	   m.conflicts[p.ConflictID] = &cp
	  return nil
   }

   func (m *MemStore) GetConflict(_ context.Context, id string) (*PhaseConflictProof, error) {
	  if m == nil {
		  return nil, ErrUnavailable
	   }
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   p, ok := m.conflicts[id]
	  if !ok {
		  return nil, ErrConflictNotFound
	   }
	   cp := p.Copy()
	   return &cp, nil
  }

  func (m *MemStore) UpdateConflictState(_ context.Context, id string, from, to ConflictState, update *PhaseConflictProof) error {
	  if m == nil {
		  return ErrUnavailable
	  }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   p, ok := m.conflicts[id]
	   if !ok {
		  return ErrConflictNotFound
	  }
	  if p.State != from {
		  return ErrInvalidTransition
	  }
	   if update != nil {
		  cp := update.Copy()
		  cp.State = to
		   m.conflicts[id] = &cp
	  } else {
		   p.State = to
		   p.UpdatedAt = m.clock()
	   }
	  return nil
  }

  func (m *MemStore) ListOpenConflictsBySession(_ context.Context, sessionID string) ([]*PhaseConflictProof, error) {
	  if m == nil {
		   return nil, ErrUnavailable
	   }
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	   var out []*PhaseConflictProof
	   for _, p := range m.conflicts {
		  if p.SessionID == sessionID && (p.State == ConflictOpen || p.State == ConflictProven) {
			   cp := p.Copy()
			   out = append(out, &cp)
		  }
	  }
	  return out, nil
   }

// ── QuorumBind ────────────────────────────────────────────────────────────────

  func (m *MemStore) SaveBind(_ context.Context, b *QuorumBind) error {
	  if m == nil || b == nil {
		   return ErrUnavailable
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  cp := b.Copy()
	   m.binds[b.BindID] = &cp
	   m.bindByLease[b.LeaseID] = &cp
	   return nil
   }

  func (m *MemStore) GetBind(_ context.Context, id string) (*QuorumBind, error) {
	   if m == nil {
		  return nil, ErrUnavailable
	  }
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  b, ok := m.binds[id]
	   if !ok {
		  return nil, ErrBindNotFound
	   }
	   cp := b.Copy()
	  return &cp, nil
   }

  func (m *MemStore) GetBindByLease(_ context.Context, leaseID string) (*QuorumBind, error) {
	   if m == nil {
		   return nil, ErrUnavailable
	   }
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  b, ok := m.bindByLease[leaseID]
	   if !ok {
		   return nil, ErrBindNotFound
	  }
	  cp := b.Copy()
	   return &cp, nil
  }
