package stucksettle

import (
	"fmt"
	"sync"
	"time"
)

// MemStore Store 接口的内存实现。

type MemStore struct {
	mu   sync.Mutex
	gens map[string]*StuckGeneration // key = transfer_id
	leases map[string]*RecoverLease // key = lease_id
	cases  map[string]*DupRecoverCase // key = dup_case_id
	proofs map[string]*EffectProof    // key = proof_id
}

// NewMemStore 创建内存存储。

  func NewMemStore() *MemStore {
	   return &MemStore{
		   gens:   map[string]*StuckGeneration{},
		   leases: map[string]*RecoverLease{},
		  cases:  map[string]*DupRecoverCase{},
		   proofs: map[string]*EffectProof{},
	  }
   }

// ── StuckGeneration ──────────────────────────────────────────────────────────

  func (s *MemStore) GetOrCreateGen(transferID string, now time.Time) (*StuckGeneration, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  if g, ok := s.gens[transferID]; ok {
		  cp := *g
		  return &cp, nil
	  }
	  g := &StuckGeneration{
		   StuckGenID: fmt.Sprintf("gen:%s", transferID),
		  TransferID: transferID,
		   CurrentGen: 0,
		  CreatedAt:  now,
		  UpdatedAt:  now,
	  }
	  s.gens[transferID] = g
	  cp := *g
	  return &cp, nil
  }

   func (s *MemStore) BumpGen(transferID string, now time.Time) (*StuckGeneration, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  g, ok := s.gens[transferID]
	   if !ok {
		  g = &StuckGeneration{
			  StuckGenID: fmt.Sprintf("gen:%s", transferID),
			  TransferID: transferID,
			   CurrentGen: 0,
			  CreatedAt:  now,
		  }
		   s.gens[transferID] = g
	  }
	   g.CurrentGen++
	  g.LastActingAt = now
	   g.UpdatedAt = now
	  cp := *g
	   return &cp, nil
  }

  func (s *MemStore) GetGen(transferID string) (*StuckGeneration, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  g, ok := s.gens[transferID]
	   if !ok {
		   return nil, fmt.Errorf("stucksettle: gen not found for %s", transferID)
	  }
	  cp := *g
	   return &cp, nil
  }

// ── RecoverLease ─────────────────────────────────────────────────────────────

   func (s *MemStore) PutLease(l *RecoverLease) error {
	  if s == nil || l == nil {
		  return ErrUnavailable
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   cp := *l
	   s.leases[l.LeaseID] = &cp
	  return nil
  }

  func (s *MemStore) GetLease(leaseID string) (*RecoverLease, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  l, ok := s.leases[leaseID]
	  if !ok {
		  return nil, fmt.Errorf("stucksettle: lease %s not found", leaseID)
	  }
	  cp := *l
	  return &cp, nil
  }

// ActiveLeaseByTransfer 返回 transfer_id 当前 Acting 状态的租约（最新）。

   func (s *MemStore) ActiveLeaseByTransfer(transferID string) (*RecoverLease, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   var latest *RecoverLease
	   for _, l := range s.leases {
		  if l.TransferID != transferID {
			  continue
		   }
		   if l.State != LeaseActing {
			  continue
		   }
		   if latest == nil || l.ActingAt.After(latest.ActingAt) {
			   cp := *l
			   latest = &cp
		   }
	   }
	  if latest == nil {
		   return nil, nil // nolint
	   }
	   return latest, nil
   }

// ── DupRecoverCase ───────────────────────────────────────────────────────────

  func (s *MemStore) PutDupCase(c *DupRecoverCase) error {
	   if s == nil || c == nil {
		   return ErrUnavailable
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  cp := *c
	   s.cases[c.DupCaseID] = &cp
	   return nil
   }

  func (s *MemStore) GetDupCase(dupCaseID string) (*DupRecoverCase, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  c, ok := s.cases[dupCaseID]
	   if !ok {
		  return nil, ErrCaseNotFound
	   }
	  cp := *c
	   return &cp, nil
  }

  func (s *MemStore) DupCasesByTransfer(transferID string) ([]DupRecoverCase, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   out := make([]DupRecoverCase, 0)
	  for _, c := range s.cases {
		  if c.TransferID == transferID {
			  cp := *c
			   out = append(out, cp)
		  }
	  }
	   return out, nil
   }

// ── EffectProof (append-only) ─────────────────────────────────────────────────

   func (s *MemStore) AppendProof(p *EffectProof) error {
	   if s == nil || p == nil {
		  return ErrUnavailable
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  if _, exists := s.proofs[p.EffectProofID]; exists {
		   return ErrProofInvalid // append-only；重复写入拒绝
	   }
	   cp := *p
	   s.proofs[p.EffectProofID] = &cp
	   return nil
  }

   func (s *MemStore) GetProof(proofID string) (*EffectProof, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   p, ok := s.proofs[proofID]
	   if !ok {
		   return nil, ErrProofMissing
	  }
	  cp := *p
	  return &cp, nil
   }

   func (s *MemStore) ProofByLease(leaseID string) (*EffectProof, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  for _, p := range s.proofs {
		   if p.LeaseID == leaseID {
			   cp := *p
			  return &cp, nil
		   }
	  }
	  return nil, ErrProofMissing
   }
