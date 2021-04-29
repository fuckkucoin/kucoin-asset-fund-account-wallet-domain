package feebumpgate

import (
	"context"
	"strings"
	"sync"
	"time"
)

// MemStore 进程内 Store。

type MemStore struct {
	mu   sync.Mutex
	gens map[string]FeeBumpGen
	byIdem map[string]string
	occs map[string]BroadcastOccupancy
	proofs map[string]RbfOrphanProof
	audits map[string][]GenAudit
	epochs map[string]int64
}

// NewMemStore 构造空仓。

   func NewMemStore() *MemStore {
	  return &MemStore{
		  gens:   map[string]FeeBumpGen{},
		  byIdem: map[string]string{},
		   occs: map[string]BroadcastOccupancy{},
		  proofs: map[string]RbfOrphanProof{},
		   audits: map[string][]GenAudit{},
		  epochs: map[string]int64{},
	   }
   }

   func (m *MemStore) SaveGen(_ context.Context, g FeeBumpGen) error {
	   if m == nil {
		   return ErrUnavailable
	   }
	  id := strings.TrimSpace(g.GenID)
	  if id == "" {
		  return ErrGenNotFound
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if _, ok := m.gens[id]; ok {
		  return ErrIdem
	  }
	   if g.IdempotencyKey != "" {
		  if old, ok := m.byIdem[g.IdempotencyKey]; ok && old != id {
			  return ErrIdem
		   }
		   m.byIdem[g.IdempotencyKey] = id
	   }
	  m.gens[id] = g.Copy()
	   if g.Generation > m.epochs[g.TransferID] {
		  m.epochs[g.TransferID] = g.Generation
	  }
	  return nil
  }

   func (m *MemStore) GetGen(_ context.Context, genID string) (FeeBumpGen, error) {
	  if m == nil {
		  return FeeBumpGen{}, ErrUnavailable
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  cur, ok := m.gens[strings.TrimSpace(genID)]
	   if !ok {
		  return FeeBumpGen{},
			   ErrGenNotFound
	   }
	   return cur.Copy(), nil
   }

  func (m *MemStore) GetGenByIdem(_ context.Context, idem string) (FeeBumpGen, error) {
	   if m == nil {
		   return FeeBumpGen{},
			   ErrUnavailable
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   id, ok := m.byIdem[strings.TrimSpace(idem)]
	   if !ok {
		   return FeeBumpGen{}, ErrGenNotFound
	  }
	  return m.gens[id].Copy(), nil
  }

   func (m *MemStore) LatestGen(_ context.Context, transferID string) (FeeBumpGen, error) {
	   if m == nil {
		   return FeeBumpGen{}, ErrUnavailable
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   tid := strings.TrimSpace(transferID)
	  var best FeeBumpGen
	  found := false
	  for _, g := range m.gens {
		  if g.TransferID != tid {
			  continue
		   }
		   if !found || g.Generation > best.Generation {
			  best = g
			  found = true
		  }
	   }
	  if !found {
		  return FeeBumpGen{}, ErrGenNotFound
	  }
	  return best.Copy(), nil
  }

   func (m *MemStore) ListGens(_ context.Context, transferID string) ([]FeeBumpGen, error) {
	  if m == nil {
		  return nil, ErrUnavailable
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  tid := strings.TrimSpace(transferID)
	  out := make([]FeeBumpGen, 0)
	   for _, g := range m.gens {
		   if g.TransferID == tid {
			  out = append(out, g.Copy())
		   }
	   }
	   return out, nil
  }

   func (m *MemStore) ListActiveGens(_ context.Context, limit int) ([]FeeBumpGen, error) {
	  if m == nil {
		   return nil, ErrUnavailable
	   }
	   if limit <= 0 {
		   limit = 64
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  out := make([]FeeBumpGen, 0)
	   for _, g := range m.gens {
		   if g.State.Active() {
			   out = append(out, g.Copy())
			  if len(out) >= limit {
				  break
			  }
		  }
	   }
	   return out, nil
   }

  func (m *MemStore) CASGen(_ context.Context, genID string, from, to GenState, patch FeeBumpGen) error {
	   if m == nil {
		   return ErrUnavailable
	   }
	  if !AllowGenTransition(from, to) {
		   return ErrInvalidTransition
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   cur, ok := m.gens[strings.TrimSpace(genID)]
	  if !ok {
		   return ErrGenNotFound
	   }
	  if cur.State != from {
		   return ErrInvalidTransition
	   }
	   next := patch.Copy()
	  next.GenID = cur.GenID
	  next.State = to
	   m.gens[cur.GenID] = next
	   return nil
   }

   func (m *MemStore) SaveOcc(_ context.Context, o BroadcastOccupancy) error {
	  if m == nil {
		  return ErrUnavailable
	  }
	  id := strings.TrimSpace(o.OccID)
	   if id == "" {
		  return ErrOccNotFound
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  if _, ok := m.occs[id]; ok {
		  return ErrIdem
	   }
	  m.occs[id] = o.Copy()
	   return nil
  }

   func (m *MemStore) GetOcc(_ context.Context, occID string) (BroadcastOccupancy, error) {
	  if m == nil {
		  return BroadcastOccupancy{}, ErrUnavailable
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   cur, ok := m.occs[strings.TrimSpace(occID)]
	  if !ok {
		  return BroadcastOccupancy{},
			   ErrOccNotFound
	  }
	   return cur.Copy(), nil
  }

   func (m *MemStore) GetOccByGen(_ context.Context, genID string) (BroadcastOccupancy, error) {
	  if m == nil {
		  return BroadcastOccupancy{}, ErrUnavailable
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  gid := strings.TrimSpace(genID)
	   for _, o := range m.occs {
		   if o.GenID == gid {
			   return o.Copy(), nil
		   }
	   }
	  return BroadcastOccupancy{}, ErrOccNotFound
  }

  func (m *MemStore) ListHeldByTransfer(_ context.Context, transferID string) ([]BroadcastOccupancy, error) {
	  if m == nil {
		  return nil, ErrUnavailable
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  tid := strings.TrimSpace(transferID)
	   out := make([]BroadcastOccupancy, 0)
	  for _, o := range m.occs {
		  if o.TransferID == tid && o.State.Held() {
			   out = append(out, o.Copy())
		  }
	   }
	   return out, nil
   }

   func (m *MemStore) ListHeldExpired(_ context.Context, now time.Time, limit int) ([]BroadcastOccupancy, error) {
	   if m == nil {
		  return nil, ErrUnavailable
	   }
	   if limit <= 0 {
		   limit = 64
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   out := make([]BroadcastOccupancy, 0)
	  for _, o := range m.occs {
		   if o.State.Held() && !o.ExpireAt.IsZero() && !o.ExpireAt.After(now) {
			   out = append(out, o.Copy())
			  if len(out) >= limit {
				   break
			   }
		   }
	   }
	   return out, nil
  }

   func (m *MemStore) CASOcc(_ context.Context, occID string, from, to OccState, patch BroadcastOccupancy) error {
	   if m == nil {
		   return ErrUnavailable
	   }
	  if !AllowOccTransition(from, to) {
		   return ErrInvalidTransition
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  cur, ok := m.occs[strings.TrimSpace(occID)]
	  if !ok {
		   return ErrOccNotFound
	  }
	   if cur.State != from {
		   return ErrInvalidTransition
	  }
	  next := patch.Copy()
	  next.OccID = cur.OccID
	   next.State = to
	   m.occs[cur.OccID] = next
	   return nil
  }

  func (m *MemStore) CountHeld(_ context.Context, transferID string) (int, error) {
	  if m == nil {
		   return 0, ErrUnavailable
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  tid := strings.TrimSpace(transferID)
	  n := 0
	  for _, o := range m.occs {
		   if o.TransferID == tid && o.State.Held() {
			   n++
		   }
	   }
	  return n, nil
   }

  func (m *MemStore) SaveProof(_ context.Context, p RbfOrphanProof) error {
	   if m == nil {
		   return ErrUnavailable
	   }
	  id := strings.TrimSpace(p.ProofID)
	   if id == "" {
		   return ErrProofNotFound
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if _, ok := m.proofs[id]; ok {
		  return nil
	   }
	  m.proofs[id] = p.Copy()
	  return nil
   }

   func (m *MemStore) GetProof(_ context.Context, proofID string) (RbfOrphanProof, error) {
	   if m == nil {
		  return RbfOrphanProof{}, ErrUnavailable
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   cur, ok := m.proofs[strings.TrimSpace(proofID)]
	  if !ok {
		  return RbfOrphanProof{}, ErrProofNotFound
	  }
	   return cur.Copy(), nil
   }

   func (m *MemStore) ListProofsByTransfer(_ context.Context, transferID string) ([]RbfOrphanProof, error) {
	  if m == nil {
		   return nil, ErrUnavailable
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  tid := strings.TrimSpace(transferID)
	  out := make([]RbfOrphanProof, 0)
	  for _, p := range m.proofs {
		  if p.TransferID == tid {
			   out = append(out, p.Copy())
		  }
	  }
	   return out, nil
   }

  func (m *MemStore) ListOpenOrphans(_ context.Context, limit int) ([]RbfOrphanProof, error) {
	   if m == nil {
		  return nil, ErrUnavailable
	  }
	   if limit <= 0 {
		  limit = 64
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  out := make([]RbfOrphanProof, 0)
	  for _, p := range m.proofs {
		   if p.State.Open() {
			  out = append(out, p.Copy())
			   if len(out) >= limit {
				  break
			  }
		  }
	   }
	   return out, nil
   }

  func (m *MemStore) ListProofsByKind(_ context.Context, kind ProofKind, limit int) ([]RbfOrphanProof, error) {
	  if m == nil {
		   return nil, ErrUnavailable
	   }
	   if limit <= 0 {
		  limit = 64
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  out := make([]RbfOrphanProof, 0)
	   for _, p := range m.proofs {
		   if p.Kind == kind {
			  out = append(out, p.Copy())
			   if len(out) >= limit {
				   break
			   }
		   }
	   }
	   return out, nil
  }

   func (m *MemStore) CASProofState(_ context.Context, proofID string, from, to OrphanState, patch RbfOrphanProof) error {
	   if m == nil {
		   return ErrUnavailable
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  cur, ok := m.proofs[strings.TrimSpace(proofID)]
	   if !ok {
		  return ErrProofNotFound
	  }
	  if cur.State != from {
		  return ErrInvalidTransition
	   }
	   next := patch.Copy()
	   next.ProofID = cur.ProofID
	  next.State = to
	   m.proofs[cur.ProofID] = next
	  return nil
   }

   func (m *MemStore) AppendAudit(_ context.Context, a GenAudit) error {
	   if m == nil {
		   return ErrUnavailable
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  m.audits[a.GenID] = append(m.audits[a.GenID], a.Copy())
	   return nil
   }

   func (m *MemStore) ListAudit(_ context.Context, genID string) ([]GenAudit, error) {
	  if m == nil {
		   return nil, ErrUnavailable
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   src := m.audits[strings.TrimSpace(genID)]
	   out := make([]GenAudit, 0, len(src))
	  for _, a := range src {
		  out = append(out, a.Copy())
	   }
	   return out, nil
  }

  func (m *MemStore) NextGeneration(_ context.Context, transferID string) (int64, error) {
	  if m == nil {
		   return 0, ErrUnavailable
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  tid := strings.TrimSpace(transferID)
	   m.epochs[tid]++
	   return m.epochs[tid], nil
   }

  func (m *MemStore) CurrentGeneration(_ context.Context, transferID string) (int64, error) {
	  if m == nil {
		   return 0, ErrUnavailable
	  }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  return m.epochs[strings.TrimSpace(transferID)], nil
   }
