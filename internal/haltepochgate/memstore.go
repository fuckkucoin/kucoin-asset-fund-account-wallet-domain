package haltepochgate

import (
	"context"
	"sync"
)

type MemStore struct {
	mu sync.RWMutex
	epochs  map[string]HaltEpoch
	intents map[string]FundIntent
	idem    map[string]string
	proofs  map[string]SlipProof
	cases  map[string]ClearWindowCase
	quorum map[string]QuorumBundle
	qepoch map[string]string
	audits []AuditEvent
}

   func NewMemStore() *MemStore {
	   return &MemStore{
		   epochs: map[string]HaltEpoch{},
		  intents: map[string]FundIntent{},
		   idem:    map[string]string{},
		   proofs: map[string]SlipProof{},
		  cases:  map[string]ClearWindowCase{},
		   quorum: map[string]QuorumBundle{},
		  qepoch: map[string]string{},
	   }
  }

   func (m *MemStore) GetEpoch(_ context.Context, scope ScopeKey) (*HaltEpoch, error) {
	   if m == nil {
		  return nil, ErrStoreUnavailable
	   }
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  e, ok := m.epochs[scope.String()]
	  if !ok {
		   return nil, ErrEpochNotFound
	   }
	  cp := e
	  return &cp, nil
   }

   func (m *MemStore) PutEpoch(_ context.Context, e HaltEpoch) error {
	   if m == nil {
		  return ErrStoreUnavailable
	  }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   m.epochs[e.Scope.String()] = e
	   return nil
  }

   func (m *MemStore) ListEpochs(_ context.Context, limit int) ([]HaltEpoch, error) {
	  if m == nil {
		  return nil, ErrStoreUnavailable
	   }
	   if limit <= 0 || limit > 500 {
		   limit = 100
	  }
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	  out := make([]HaltEpoch, 0, len(m.epochs))
	  for _, e := range m.epochs {
		  out = append(out, e)
		  if len(out) >= limit {
			   break
		  }
	  }
	  return out, nil
  }

  func (m *MemStore) GetIntent(_ context.Context, id string) (*FundIntent, error) {
	  if m == nil {
		  return nil, ErrStoreUnavailable
	  }
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  in, ok := m.intents[id]
	   if !ok {
		   return nil, ErrIntentNotFound
	  }
	  cp := in
	   return &cp, nil
   }

  func (m *MemStore) GetIntentByIdem(_ context.Context, idem string) (*FundIntent, error) {
	   if m == nil {
		   return nil, ErrStoreUnavailable
	  }
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  id, ok := m.idem[idem]
	   if !ok {
		  return nil, ErrIntentNotFound
	   }
	   in, ok := m.intents[id]
	  if !ok {
		   return nil, ErrIntentNotFound
	  }
	   cp := in
	   return &cp, nil
  }

   func (m *MemStore) PutIntent(_ context.Context, in FundIntent) error {
	   if m == nil {
		   return ErrStoreUnavailable
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   if in.IdemKey != "" {
		  if exist, ok := m.idem[in.IdemKey]; ok && exist != in.IntentID {
			  return ErrIdemConflict
		  }
		   m.idem[in.IdemKey] = in.IntentID
	   }
	  m.intents[in.IntentID] = in
	  return nil
   }

   func (m *MemStore) ListIntentsByScope(_ context.Context, scope ScopeKey, states []IntentState, limit int) ([]FundIntent, error) {
	   if m == nil {
		   return nil, ErrStoreUnavailable
	  }
	  if limit <= 0 {
		  limit = 200
	  }
	  allow := map[IntentState]bool{}
	   for _, s := range states {
		   allow[s] = true
	  }
	  key := scope.String()
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	   out := make([]FundIntent, 0)
	   for _, in := range m.intents {
		  if in.Scope.String() != key {
			  continue
		  }
		  if len(allow) > 0 && !allow[in.State] {
			   continue
		  }
		  out = append(out, in)
		  if len(out) >= limit {
			   break
		   }
	   }
	  return out, nil
   }

   func (m *MemStore) ListAllowedIntents(_ context.Context, limit int) ([]FundIntent, error) {
	  if m == nil {
		  return nil, ErrStoreUnavailable
	  }
	  if limit <= 0 {
		   limit = 200
	  }
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	  out := make([]FundIntent, 0)
	   for _, in := range m.intents {
		  if in.State != IntentAllowed {
			   continue
		   }
		   out = append(out, in)
		  if len(out) >= limit {
			   break
		   }
	  }
	   return out, nil
   }

  func (m *MemStore) GetProof(_ context.Context, id string) (*SlipProof, error) {
	   if m == nil {
		  return nil, ErrStoreUnavailable
	  }
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	  p, ok := m.proofs[id]
	   if !ok {
		  return nil, ErrProofNotFound
	  }
	   cp := p
	   return &cp, nil
   }

   func (m *MemStore) PutProof(_ context.Context, p SlipProof) error {
	  if m == nil {
		  return ErrStoreUnavailable
	  }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   m.proofs[p.ProofID] = p
	  return nil
   }

  func (m *MemStore) ListProofs(_ context.Context, state SlipState, limit int) ([]SlipProof, error) {
	   if m == nil {
		   return nil, ErrStoreUnavailable
	   }
	  if limit <= 0 {
		  limit = 200
	  }
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  out := make([]SlipProof, 0)
	  for _, p := range m.proofs {
		   if state != "" && p.State != state {
			  continue
		  }
		   out = append(out, p)
		  if len(out) >= limit {
			   break
		  }
	  }
	  return out, nil
  }

  func (m *MemStore) ListProofsByScope(_ context.Context, scope ScopeKey, limit int) ([]SlipProof, error) {
	   if m == nil {
		   return nil, ErrStoreUnavailable
	  }
	   if limit <= 0 {
		   limit = 200
	  }
	   key := scope.String()
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  out := make([]SlipProof, 0)
	   for _, p := range m.proofs {
		   if p.Scope.String() != key {
			   continue
		  }
		  out = append(out, p)
		  if len(out) >= limit {
			   break
		  }
	   }
	   return out, nil
   }

  func (m *MemStore) GetCase(_ context.Context, id string) (*ClearWindowCase, error) {
	  if m == nil {
		  return nil, ErrStoreUnavailable
	   }
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  c, ok := m.cases[id]
	   if !ok {
		  return nil, ErrCaseNotFound
	  }
	  cp := c
	  return &cp, nil
   }

   func (m *MemStore) PutCase(_ context.Context, c ClearWindowCase) error {
	  if m == nil {
		  return ErrStoreUnavailable
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  m.cases[c.CaseID] = c
	  return nil
  }

  func (m *MemStore) ListOpenCases(_ context.Context, limit int) ([]ClearWindowCase, error) {
	  if m == nil {
		  return nil, ErrStoreUnavailable
	   }
	  if limit <= 0 {
		   limit = 100
	  }
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	   out := make([]ClearWindowCase, 0)
	   for _, c := range m.cases {
		  if c.State == WindowSealed {
			   continue
		   }
		   out = append(out, c)
		  if len(out) >= limit {
			  break
		  }
	   }
	  return out, nil
  }

  func (m *MemStore) GetQuorum(_ context.Context, id string) (*QuorumBundle, error) {
	  if m == nil {
		   return nil, ErrStoreUnavailable
	  }
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  q, ok := m.quorum[id]
	   if !ok {
		  return nil, ErrQuorumDenied
	   }
	  cp := q
	  return &cp, nil
   }

  func (m *MemStore) PutQuorum(_ context.Context, q QuorumBundle) error {
	  if m == nil {
		   return ErrStoreUnavailable
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   m.quorum[q.QuorumID] = q
	  m.qepoch[q.EpochID] = q.QuorumID
	  return nil
  }

  func (m *MemStore) GetQuorumByEpoch(_ context.Context, epochID string) (*QuorumBundle, error) {
	  if m == nil {
		   return nil, ErrStoreUnavailable
	   }
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   id, ok := m.qepoch[epochID]
	   if !ok {
		  return nil, ErrQuorumDenied
	  }
	   q, ok := m.quorum[id]
	  if !ok {
		  return nil, ErrQuorumDenied
	   }
	   cp := q
	  return &cp, nil
   }

  func (m *MemStore) AppendAudit(_ context.Context, e AuditEvent) error {
	  if m == nil {
		  return ErrStoreUnavailable
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   m.audits = append(m.audits, e)
	  if len(m.audits) > 2000 {
		   m.audits = m.audits[len(m.audits)-2000:]
	   }
	  return nil
  }

   func (m *MemStore) ListAudit(_ context.Context, limit int) ([]AuditEvent, error) {
	   if m == nil {
		   return nil, ErrStoreUnavailable
	   }
	  if limit <= 0 || limit > 500 {
		   limit = 50
	   }
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	  n := len(m.audits)
	  if n == 0 {
		   return nil, nil
	  }
	  if limit < n {
		   return append([]AuditEvent(nil), m.audits[n-limit:]...), nil
	  }
	  return append([]AuditEvent(nil), m.audits...), nil
   }
