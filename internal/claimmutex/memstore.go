package claimmutex

import (
	"context"
	"sync"
	"time"
)

// MemStore 内存实现，供和开发使用。

type MemStore struct {
	mu     sync.Mutex
	leases map[string]*OutputLease  // leaseID → lease
	byOut  map[string]string        // outputKey → leaseID
	cases  map[string]*DupClaimCase // caseID → case
	caseOut map[string]string // outputKey → caseID（只保留 open 案件）
	binds   map[string]*AutoCreditBind   // bindID → bind
	bindOut map[string]string            // outputKey → bindID（pending 绑定）
	proofs  map[string]*ClaimEffectProof // proofID → proof
	proofOut map[string]string // outputKey → proofID（最新证明）
}

// NewMemStore 创建内存存储实例。

   func NewMemStore() *MemStore {
	  return &MemStore{
		   leases: make(map[string]*OutputLease),
		  byOut: make(map[string]string),
		   cases: make(map[string]*DupClaimCase),
		  caseOut: make(map[string]string),
		   binds: make(map[string]*AutoCreditBind),
		   bindOut: make(map[string]string),
		   proofs: make(map[string]*ClaimEffectProof),
		  proofOut: make(map[string]string),
	   }
   }

// ── OutputLease ──────────────────────────────────────────────────────────────

   func (m *MemStore) GetLease(_ context.Context, key OutputKey) (*OutputLease, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   id, ok := m.byOut[key.String()]
	  if !ok {
		   return nil, ErrLeaseNotFound
	   }
	   l, ok := m.leases[id]
	  if !ok {
		  return nil, ErrLeaseNotFound
	  }
	  cp := *l
	   return &cp, nil
  }

  func (m *MemStore) GetLeaseByID(_ context.Context, leaseID string) (*OutputLease, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   l, ok := m.leases[leaseID]
	  if !ok {
		  return nil, ErrLeaseNotFound
	   }
	  cp := *l
	   return &cp, nil
   }

   func (m *MemStore) InsertLease(_ context.Context, lease OutputLease) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   k := lease.OutputKey.String()
	   if _, exists := m.byOut[k]; exists {
		  return ErrMutexConflict
	   }
	  cp := lease
	  m.leases[lease.LeaseID] = &cp
	   m.byOut[k] = lease.LeaseID
	  return nil
   }

  func (m *MemStore) UpdateLeaseState(_ context.Context, leaseID string, expected LeaseState, next LeaseState, now time.Time, blockReason string) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  l, ok := m.leases[leaseID]
	   if !ok {
		   return ErrLeaseNotFound
	  }
	  if l.State != expected {
		  return ErrGenerationMismatch
	  }
	  l.State = next
	  if next == LeaseBlocked {
		  t := now
		  l.BlockedAt = &t
		  l.BlockReason = blockReason
	  }
	   if next == LeaseClaimed || next == LeaseAutoCredited {
		  t := now
		  l.ClaimedAt = &t
	  }
	  return nil
  }

  func (m *MemStore) ListExpiredLeases(_ context.Context, now time.Time, limit int) ([]OutputLease, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   var out []OutputLease
	   for _, l := range m.leases {
		   if l.State != LeaseExpired && l.State != LeaseClaimed && l.State != LeaseAutoCredited && l.State != LeaseBlocked {
			  if now.After(l.ExpiresAt) {
				  out = append(out, *l)
			   }
		   }
		  if len(out) >= limit {
			  break
		  }
	  }
	  return out, nil
   }

// ── DupClaimCase ─────────────────────────────────────────────────────────────

  func (m *MemStore) GetOpenDupCase(_ context.Context, key OutputKey) (*DupClaimCase, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  id, ok := m.caseOut[key.String()]
	   if !ok {
		  return nil, ErrDupCaseNotFound
	   }
	  c, ok := m.cases[id]
	   if !ok {
		   return nil, ErrDupCaseNotFound
	   }
	  cp := *c
	  return &cp, nil
   }

   func (m *MemStore) GetDupCaseByID(_ context.Context, caseID string) (*DupClaimCase, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   c, ok := m.cases[caseID]
	  if !ok {
		   return nil, ErrDupCaseNotFound
	  }
	  cp := *c
	   return &cp, nil
   }

   func (m *MemStore) InsertDupCase(_ context.Context, c DupClaimCase) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   cp := c
	  m.cases[c.CaseID] = &cp
	   if c.State == DupCaseOpen {
		  m.caseOut[c.OutputKey.String()] = c.CaseID
	   }
	  return nil
  }

   func (m *MemStore) UpdateDupCaseState(_ context.Context, caseID string, expected DupCaseState, next DupCaseState, now time.Time, actor1, actor2, closeNote string) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  c, ok := m.cases[caseID]
	  if !ok {
		  return ErrDupCaseNotFound
	   }
	   if c.State != expected {
		  return ErrGenerationMismatch
	   }
	   c.State = next
	   if next == DupCaseProven {
		  t := now
		   c.ProvenAt = &t
		   c.Actor1 = actor1
		  c.Actor2 = actor2
	  }
	  if next == DupCaseClosed {
		   t := now
		   c.ClosedAt = &t
		  c.CloseNote = closeNote
		   // 关闭后清除 open 指针
		  delete(m.caseOut, c.OutputKey.String())
	  }
	  return nil
   }

  func (m *MemStore) ListOpenDupCases(_ context.Context, limit int) ([]DupClaimCase, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   var out []DupClaimCase
	  for _, c := range m.cases {
		  if c.State == DupCaseOpen {
			  out = append(out, *c)
		   }
		  if len(out) >= limit {
			   break
		  }
	   }
	  return out, nil
  }

// ── AutoCreditBind ───────────────────────────────────────────────────────────

   func (m *MemStore) GetPendingAutoBind(_ context.Context, key OutputKey) (*AutoCreditBind, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  id, ok := m.bindOut[key.String()]
	  if !ok {
		   return nil, ErrBindNotFound
	  }
	  b, ok := m.binds[id]
	   if !ok {
		  return nil, ErrBindNotFound
	   }
	  cp := *b
	  return &cp, nil
   }

  func (m *MemStore) GetAutoBindByID(_ context.Context, bindID string) (*AutoCreditBind, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  b, ok := m.binds[bindID]
	   if !ok {
		  return nil, ErrBindNotFound
	   }
	   cp := *b
	   return &cp, nil
   }

  func (m *MemStore) InsertAutoBind(_ context.Context, b AutoCreditBind) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   cp := b
	   m.binds[b.BindID] = &cp
	  if b.State == BindPending {
		   m.bindOut[b.OutputKey.String()] = b.BindID
	   }
	  return nil
  }

   func (m *MemStore) UpdateAutoBindState(_ context.Context,
	  bindID string, expected BindState, next BindState, now time.Time, rejectNote string) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   b, ok := m.binds[bindID]
	  if !ok {
		   return ErrBindNotFound
	  }
	  if b.State != expected {
		  return ErrGenerationMismatch
	   }
	  b.State = next
	  t := now
	   b.ResolvedAt = &t
	   b.RejectNote = rejectNote
	   if next != BindPending {
		   delete(m.bindOut, b.OutputKey.String())
	   }
	   return nil
   }

// ── ClaimEffectProof ─────────────────────────────────────────────────────────

  func (m *MemStore) GetProofByOutput(_ context.Context, key OutputKey) (*ClaimEffectProof, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  id, ok := m.proofOut[key.String()]
	   if !ok {
		   return nil, ErrProofNotFound
	  }
	  p, ok := m.proofs[id]
	   if !ok {
		  return nil, ErrProofNotFound
	  }
	   cp := *p
	   return &cp, nil
   }

  func (m *MemStore) GetProofByID(_ context.Context, proofID string) (*ClaimEffectProof, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  p, ok := m.proofs[proofID]
	  if !ok {
		   return nil, ErrProofNotFound
	   }
	  cp := *p
	   return &cp, nil
  }

  func (m *MemStore) InsertProof(_ context.Context, p ClaimEffectProof) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   cp := p
	   m.proofs[p.ProofID] = &cp
	  m.proofOut[p.OutputKey.String()] = p.ProofID
	   return nil
   }

   func (m *MemStore) UpdateProofState(_ context.Context, proofID string, expected ProofState, next ProofState, now time.Time, failReason, sealedBy string) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  p, ok := m.proofs[proofID]
	  if !ok {
		   return ErrProofNotFound
	  }
	   if p.State != expected {
		  return ErrGenerationMismatch
	   }
	  p.State = next
	   if next == ProofVerified {
		   t := now
		   p.VerifiedAt = &t
	   }
	  if next == ProofSealed {
		   t := now
		  p.SealedAt = &t
		  p.SealedBy = sealedBy
	  }
	  if next == ProofFailed {
		  p.FailReason = failReason
	  }
	   return nil
  }

   func (m *MemStore) ListPendingProofs(_ context.Context, limit int) ([]ClaimEffectProof, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  var out []ClaimEffectProof
	  for _, p := range m.proofs {
		   if p.State == ProofPending {
			  out = append(out, *p)
		   }
		  if len(out) >= limit {
			  break
		   }
	  }
	  return out, nil
   }
