package holdopsgate

import (
	"context"
	"sync"
	"time"
)

type Store interface {
	GetGeneration(ctx context.Context) (HoldOpsGeneration,
		error)
	CASBumpOps(ctx context.Context, expect int64, fence bool, note string, at time.Time) (HoldOpsGeneration, error)
	PutLease(ctx context.Context, lease HoldOpsLease) error
	GetLease(ctx context.Context, id string) (HoldOpsLease, error)
	FindLeaseByNonce(ctx context.Context, actor, nonce string) (HoldOpsLease, bool, error)
	ListOpenLeases(ctx context.Context) ([]HoldOpsLease, error)
	ListTokenOpen(ctx context.Context, tokenID string) ([]HoldOpsLease, error)
	ListActorOpen(ctx context.Context, actor string) ([]HoldOpsLease, error)
	UpdateLease(ctx context.Context, lease HoldOpsLease) error
	PutProof(ctx context.Context, p HoldOpsRaceProof) error
	GetProof(ctx context.Context, id string) (HoldOpsRaceProof, error)
	FindProof(ctx context.Context,
		tokenID string, kind ProofKind) (HoldOpsRaceProof, bool, error)
	ListOpenProofs(ctx context.Context) ([]HoldOpsRaceProof, error)
	ListProofsByToken(ctx context.Context, tokenID string) ([]HoldOpsRaceProof, error)
	ListProofsByUser(ctx context.Context, userID string) ([]HoldOpsRaceProof, error)
	UpdateProof(ctx context.Context,
		p HoldOpsRaceProof) error
	PutAudit(ctx context.Context, a HoldOpsAudit) error
	GetAuditByReplay(ctx context.Context,
		key string) (HoldOpsAudit, bool, error)
	ListAuditByOps(ctx context.Context, opsID string) ([]HoldOpsAudit, error)
	PutToken(ctx context.Context, t ShadowToken) error
	GetToken(ctx context.Context, id string) (ShadowToken, error)
	ListOpenTokensByGap(ctx context.Context, gapID string) ([]ShadowToken, error)
	ListOpenTokensByUser(ctx context.Context, userID string) ([]ShadowToken, error)
	PutGap(ctx context.Context, g ShadowGap) error
	GetGap(ctx context.Context, id string) (ShadowGap, error)
}

type MemStore struct {
	mu  sync.Mutex
	gen HoldOpsGeneration
	lease map[string]HoldOpsLease
	proof map[string]HoldOpsRaceProof
	audit map[string]HoldOpsAudit
	tok map[string]ShadowToken
	gap map[string]ShadowGap
}

  func NewMemStore() *MemStore {
	   return &MemStore{
		  gen: HoldOpsGeneration{
			  ID:        "singleton",
			   OpsGen:    1,
			   FenceOn:   false,
			   UpdatedAt: time.Unix(0, 0).UTC(),
		  },
		  lease: map[string]HoldOpsLease{},
		   proof: map[string]HoldOpsRaceProof{},
		  audit: map[string]HoldOpsAudit{},
		   tok:   map[string]ShadowToken{},
		   gap: map[string]ShadowGap{},
	   }
  }

  func (m *MemStore) GetGeneration(ctx context.Context) (HoldOpsGeneration, error) {
	   _ = ctx
	   if m == nil {
		  return HoldOpsGeneration{}, ErrUnavailable
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  return m.gen.Copy(), nil
  }

   func (m *MemStore) CASBumpOps(ctx context.Context, expect int64, fence bool, note string, at time.Time) (HoldOpsGeneration, error) {
	   _ = ctx
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   if m.gen.OpsGen != expect {
		   return m.gen.Copy(), ErrStaleGen
	  }
	  m.gen.OpsGen++
	  m.gen.FenceOn = fence
	   m.gen.Note = note
	   m.gen.UpdatedAt = at
	   return m.gen.Copy(), nil
   }

   func (m *MemStore) PutLease(ctx context.Context, lease HoldOpsLease) error {
	   _ = ctx
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if _, ok := m.lease[lease.ID]; ok {
		  return ErrIdempotentNonce
	  }
	   m.lease[lease.ID] = lease.Copy()
	  return nil
   }

  func (m *MemStore) GetLease(ctx context.Context, id string) (HoldOpsLease, error) {
	   _ = ctx
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   got, ok := m.lease[id]
	  if !ok {
		  return HoldOpsLease{}, ErrLeaseNotFound
	   }
	   return got.Copy(), nil
   }

   func (m *MemStore) FindLeaseByNonce(ctx context.Context, actor, nonce string) (HoldOpsLease, bool, error) {
	   _ = ctx
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   for _, l := range m.lease {
		  if l.Actor == actor && l.Nonce == nonce {
			  return l.Copy(), true, nil
		   }
	   }
	  return HoldOpsLease{}, false, nil
   }

   func (m *MemStore) ListOpenLeases(ctx context.Context) ([]HoldOpsLease, error) {
	   _ = ctx
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  out := make([]HoldOpsLease, 0)
	   for _, l := range m.lease {
		  if !l.Status.Terminal() {
			   out = append(out, l.Copy())
		   }
	  }
	   return out, nil
   }

   func (m *MemStore) ListTokenOpen(ctx context.Context, tokenID string) ([]HoldOpsLease, error) {
	  _ = ctx
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   out := make([]HoldOpsLease, 0)
	   for _, l := range m.lease {
		  if l.TokenID == tokenID && !l.Status.Terminal() {
			  out = append(out, l.Copy())
		   }
	  }
	  return out, nil
   }

   func (m *MemStore) ListActorOpen(ctx context.Context, actor string) ([]HoldOpsLease, error) {
	   _ = ctx
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   out := make([]HoldOpsLease, 0)
	   for _, l := range m.lease {
		  if l.Actor == actor && !l.Status.Terminal() {
			   out = append(out, l.Copy())
		   }
	  }
	  return out, nil
  }

  func (m *MemStore) UpdateLease(ctx context.Context, lease HoldOpsLease) error {
	   _ = ctx
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  if _, ok := m.lease[lease.ID]; !ok {
		  return ErrLeaseNotFound
	  }
	   m.lease[lease.ID] = lease.Copy()
	  return nil
   }

  func (m *MemStore) PutProof(ctx context.Context, p HoldOpsRaceProof) error {
	  _ = ctx
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   m.proof[p.ID] = p.Copy()
	  return nil
  }

  func (m *MemStore) GetProof(ctx context.Context, id string) (HoldOpsRaceProof, error) {
	   _ = ctx
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  got, ok := m.proof[id]
	  if !ok {
		  return HoldOpsRaceProof{}, ErrProofNotFound
	  }
	   return got.Copy(), nil
   }

  func (m *MemStore) FindProof(ctx context.Context, tokenID string, kind ProofKind) (HoldOpsRaceProof, bool, error) {
	   _ = ctx
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   for _, p := range m.proof {
		   if p.TokenID == tokenID && p.Kind == kind && p.BlocksWrites() {
			  return p.Copy(), true, nil
		  }
	   }
	   return HoldOpsRaceProof{},
		  false, nil
   }

   func (m *MemStore) ListOpenProofs(ctx context.Context) ([]HoldOpsRaceProof, error) {
	  _ = ctx
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   out := make([]HoldOpsRaceProof, 0)
	   for _, p := range m.proof {
		  if p.BlocksWrites() {
			   out = append(out, p.Copy())
		   }
	  }
	  return out, nil
   }

   func (m *MemStore) ListProofsByToken(ctx context.Context, tokenID string) ([]HoldOpsRaceProof, error) {
	   _ = ctx
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  out := make([]HoldOpsRaceProof, 0)
	   for _, p := range m.proof {
		  if p.TokenID == tokenID {
			   out = append(out, p.Copy())
		   }
	   }
	  return out, nil
  }

  func (m *MemStore) ListProofsByUser(ctx context.Context, userID string) ([]HoldOpsRaceProof, error) {
	  _ = ctx
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   out := make([]HoldOpsRaceProof, 0)
	   for _, p := range m.proof {
		   if p.UserID == userID {
			  out = append(out, p.Copy())
		  }
	  }
	   return out, nil
   }

   func (m *MemStore) UpdateProof(ctx context.Context, p HoldOpsRaceProof) error {
	  _ = ctx
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  if _, ok := m.proof[p.ID]; !ok {
		   return ErrProofNotFound
	  }
	  m.proof[p.ID] = p.Copy()
	   return nil
  }

  func (m *MemStore) PutAudit(ctx context.Context, a HoldOpsAudit) error {
	  _ = ctx
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   if _, ok := m.audit[a.ReplayKey]; ok {
		  return ErrReplayAudit
	  }
	   m.audit[a.ReplayKey] = a.Copy()
	   return nil
   }

  func (m *MemStore) GetAuditByReplay(ctx context.Context, key string) (HoldOpsAudit, bool, error) {
	   _ = ctx
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  got, ok := m.audit[key]
	   if !ok {
		   return HoldOpsAudit{}, false, nil
	  }
	   return got.Copy(), true, nil
  }

  func (m *MemStore) ListAuditByOps(ctx context.Context, opsID string) ([]HoldOpsAudit, error) {
	   _ = ctx
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  out := make([]HoldOpsAudit, 0)
	  for _, a := range m.audit {
		  if a.OpsID == opsID {
			   out = append(out, a.Copy())
		   }
	  }
	  return out, nil
   }

  func (m *MemStore) PutToken(ctx context.Context, t ShadowToken) error {
	   _ = ctx
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   m.tok[t.ID] = t
	   return nil
  }

   func (m *MemStore) GetToken(ctx context.Context, id string) (ShadowToken, error) {
	   _ = ctx
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   got, ok := m.tok[id]
	   if !ok {
		   return ShadowToken{}, ErrTokenGone
	   }
	   return got, nil
  }

   func (m *MemStore) ListOpenTokensByGap(ctx context.Context, gapID string) ([]ShadowToken, error) {
	  _ = ctx
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   out := make([]ShadowToken, 0)
	  for _, t := range m.tok {
		   if t.GapID == gapID && t.Open {
			  out = append(out, t)
		  }
	   }
	   return out, nil
  }

   func (m *MemStore) ListOpenTokensByUser(ctx context.Context, userID string) ([]ShadowToken, error) {
	  _ = ctx
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  out := make([]ShadowToken, 0)
	   for _, t := range m.tok {
		  if t.UserID == userID && t.Open {
			   out = append(out, t)
		   }
	   }
	   return out, nil
  }

  func (m *MemStore) PutGap(ctx context.Context, g ShadowGap) error {
	   _ = ctx
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  m.gap[g.ID] = g
	   return nil
   }

   func (m *MemStore) GetGap(ctx context.Context, id string) (ShadowGap, error) {
	   _ = ctx
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  got, ok := m.gap[id]
	  if !ok {
		  return ShadowGap{}, ErrGapGone
	   }
	   return got, nil
   }
