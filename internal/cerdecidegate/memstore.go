package cerdecidegate

import (
	"context"
	"strings"
	"sync"
	"time"
)

// MemStore 进程内 Store。

type MemStore struct {
	mu       sync.Mutex
	leases   map[string]DecideLease
	byIdem   map[string]string
	proofs   map[string]CerDriftProof
	audits   map[string][]DecideAudit
	flips    map[string]FlipEvent
	flipDone map[string]bool
	epochs map[string]int64
}

// NewMemStore 构造空仓。
  func NewMemStore() *MemStore {
	  return &MemStore{
		  leases: map[string]DecideLease{},
		  byIdem: map[string]string{},
		   proofs:   map[string]CerDriftProof{},
		   audits:   map[string][]DecideAudit{},
		  flips:    map[string]FlipEvent{},
		   flipDone: map[string]bool{},
		   epochs:   map[string]int64{},
	   }
  }

   func (m *MemStore) SaveLease(_ context.Context, l DecideLease) error {
	  if m == nil {
		   return ErrUnavailable
	  }
	   id := strings.TrimSpace(l.LeaseID)
	   if id == "" {
		  return ErrLeaseNotFound
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  if _, ok := m.leases[id]; ok {
		  return ErrIdem
	  }
	  if l.Idem != "" {
		  if old, ok := m.byIdem[l.Idem]; ok && old != id {
			   return ErrIdem
		  }
		   m.byIdem[l.Idem] = id
	  }
	   m.leases[id] = l.Copy()
	  return nil
  }

   func (m *MemStore) GetLease(_ context.Context, leaseID string) (DecideLease, error) {
	   if m == nil {
		  return DecideLease{}, ErrUnavailable
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  cur, ok := m.leases[strings.TrimSpace(leaseID)]
	   if !ok {
		   return DecideLease{}, ErrLeaseNotFound
	   }
	   return cur.Copy(), nil
  }

   func (m *MemStore) GetLeaseByIdem(_ context.Context, idem string) (DecideLease, error) {
	  if m == nil {
		   return DecideLease{}, ErrUnavailable
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  id, ok := m.byIdem[strings.TrimSpace(idem)]
	   if !ok {
		  return DecideLease{}, ErrLeaseNotFound
	  }
	  return m.leases[id].Copy(), nil
  }

  func (m *MemStore) ListOpenByScope(_ context.Context, scope string) ([]DecideLease, error) {
	  if m == nil {
		  return nil, ErrUnavailable
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   out := make([]DecideLease, 0)
	  for _, l := range m.leases {
		   if !l.State.Open() {
			   continue
		  }
		   if scope == "" || matchScope(l, scope) {
			  out = append(out, l.Copy())
		   }
	   }
	  return out, nil
  }

  func (m *MemStore) ListOpenByPolicy(_ context.Context, policyID string) ([]DecideLease, error) {
	   if m == nil {
		   return nil, ErrUnavailable
	  }
	   id := strings.TrimSpace(policyID)
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   out := make([]DecideLease, 0)
	  for _, l := range m.leases {
		  if !l.State.Open() {
			   continue
		   }
		  if id == "" || l.PolicyID == id {
			  out = append(out, l.Copy())
		   }
	  }
	  return out, nil
   }

  func (m *MemStore) ListOpenByBatch(_ context.Context, batchID string) ([]DecideLease, error) {
	   if m == nil {
		   return nil, ErrUnavailable
	  }
	   id := strings.TrimSpace(batchID)
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  out := make([]DecideLease, 0)
	  for _, l := range m.leases {
		   if !l.State.Open() {
			  continue
		   }
		   if id == "" || l.BatchID == id {
			  out = append(out, l.Copy())
		  }
	   }
	  return out, nil
   }

  func (m *MemStore) ListIssuedOrBound(_ context.Context, now time.Time, limit int) ([]DecideLease, error) {
	  if m == nil {
		   return nil, ErrUnavailable
	  }
	   if limit <= 0 {
		   limit = 64
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   out := make([]DecideLease, 0)
	  for _, l := range m.leases {
		   if !l.State.Open() {
			   continue
		   }
		  deadline := l.IssuedAt.Add(time.Duration(l.TTLSec) * time.Second)
		   if !now.IsZero() && now.Before(deadline) && l.State == LeaseIssued {
			  // 仍在 TTL 内也返回，供巡检统计；过期扫描再过滤。
		  }
		   out = append(out, l.Copy())
		   if len(out) >= limit {
			   break
		  }
	   }
	  return out, nil
   }

  func (m *MemStore) CASState(_ context.Context, leaseID string, from, to LeaseState, patch DecideLease) error {
	   if m == nil {
		  return ErrUnavailable
	  }
	   if err := NextError(from,
		   to); err != nil && !Allow(from, to) {
		   return err
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   cur,
		  ok := m.leases[strings.TrimSpace(leaseID)]
	  if !ok {
		   return ErrLeaseNotFound
	  }
	  if cur.State != from {
		  return NextError(cur.State, to)
	  }
	   cur.State = to
	  if !patch.BoundAt.IsZero() {
		   cur.BoundAt = patch.BoundAt
	  }
	   if !patch.ConsumedAt.IsZero() {
		   cur.ConsumedAt = patch.ConsumedAt
	  }
	  if !patch.ExpiredAt.IsZero() {
		  cur.ExpiredAt = patch.ExpiredAt
	   }
	  if !patch.RevokedAt.IsZero() {
		   cur.RevokedAt = patch.RevokedAt
	   }
	   if patch.RevokeWhy != "" {
		  cur.RevokeWhy = patch.RevokeWhy
	   }
	   if patch.ExecRef != "" {
		   cur.ExecRef = patch.ExecRef
	   }
	  if !patch.UpdatedAt.IsZero() {
		  cur.UpdatedAt = patch.UpdatedAt
	   }
	   m.leases[cur.LeaseID] = cur
	  return nil
   }

  func (m *MemStore) SaveProof(_ context.Context, p CerDriftProof) error {
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

  func (m *MemStore) GetProof(_ context.Context, proofID string) (CerDriftProof, error) {
	  if m == nil {
		  return CerDriftProof{}, ErrUnavailable
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  p, ok := m.proofs[strings.TrimSpace(proofID)]
	  if !ok {
		  return CerDriftProof{}, ErrProofNotFound
	   }
	  return p.Copy(), nil
   }

   func (m *MemStore) ListProofsByLease(_ context.Context, leaseID string) ([]CerDriftProof, error) {
	  if m == nil {
		   return nil, ErrUnavailable
	  }
	  id := strings.TrimSpace(leaseID)
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  out := make([]CerDriftProof, 0)
	   for _, p := range m.proofs {
		   if p.LeaseID == id {
			  out = append(out, p.Copy())
		   }
	   }
	   return out, nil
   }

  func (m *MemStore) ListProofsByBatch(_ context.Context, batchID string, limit int) ([]CerDriftProof, error) {
	   if m == nil {
		   return nil, ErrUnavailable
	   }
	  if limit <= 0 {
		  limit = 64
	  }
	   id := strings.TrimSpace(batchID)
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   out := make([]CerDriftProof, 0)
	  for _, p := range m.proofs {
		  if id == "" || p.BatchID == id {
			   out = append(out, p.Copy())
			   if len(out) >= limit {
				  break
			  }
		  }
	  }
	  return out, nil
   }

   func (m *MemStore) ListProofsByKind(_ context.Context, kind ProofKind, limit int) ([]CerDriftProof, error) {
	   if m == nil {
		  return nil, ErrUnavailable
	   }
	  if limit <= 0 {
		   limit = 64
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  out := make([]CerDriftProof, 0)
	   for _, p := range m.proofs {
		  if kind == "" || p.Kind == kind {
			  out = append(out, p.Copy())
			  if len(out) >= limit {
				  break
			  }
		   }
	   }
	   return out, nil
  }

  func (m *MemStore) AppendAudit(_ context.Context, a DecideAudit) error {
	  if m == nil {
		  return ErrUnavailable
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  id := strings.TrimSpace(a.LeaseID)
	   m.audits[id] = append(m.audits[id], a.Copy())
	   return nil
  }

  func (m *MemStore) ListAudit(_ context.Context, leaseID string) ([]DecideAudit, error) {
	  if m == nil {
		   return nil, ErrUnavailable
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   src := m.audits[strings.TrimSpace(leaseID)]
	  out := make([]DecideAudit, len(src))
	   copy(out, src)
	   return out, nil
   }

   func (m *MemStore) SaveFlip(_ context.Context, e FlipEvent) error {
	  if m == nil {
		  return ErrUnavailable
	  }
	   id := strings.TrimSpace(e.EventID)
	   if id == "" {
		  return ErrStore
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   m.flips[id] = e.Copy()
	  return nil
   }

  func (m *MemStore) ListPendingFlips(_ context.Context, limit int) ([]FlipEvent, error) {
	  if m == nil {
		   return nil, ErrUnavailable
	   }
	   if limit <= 0 {
		   limit = 64
	  }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   out := make([]FlipEvent, 0)
	   for id, e := range m.flips {
		  if m.flipDone[id] || e.Done {
			   continue
		   }
		   out = append(out, e.Copy())
		   if len(out) >= limit {
			  break
		   }
	   }
	   return out, nil
   }

  func (m *MemStore) MarkFlipDone(_ context.Context, eventID string) error {
	  if m == nil {
		  return ErrUnavailable
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  id := strings.TrimSpace(eventID)
	  m.flipDone[id] = true
	  if ev, ok := m.flips[id]; ok {
		   ev.Done = true
		  m.flips[id] = ev
	  }
	   return nil
  }

  func (m *MemStore) NextGeneration(_ context.Context, scope string) (int64, error) {
	  if m == nil {
		  return 0, ErrUnavailable
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  k := strings.TrimSpace(scope)
	   if k == "" {
		   k = "*"
	  }
	   m.epochs[k]++
	  return m.epochs[k], nil
   }

   func (m *MemStore) CurrentGeneration(_ context.Context, scope string) (int64, error) {
	   if m == nil {
		   return 0, ErrUnavailable
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  k := strings.TrimSpace(scope)
	   if k == "" {
		  k = "*"
	  }
	  n := m.epochs[k]
	   if n == 0 {
		  n = 1
		   m.epochs[k] = 1
	  }
	   return n, nil
  }

  func matchScope(l DecideLease, scope string) bool {
	  if scope == "" {
		  return true
	   }
	   if strings.HasPrefix(scope, "batch:") {
		  return l.BatchID == strings.TrimPrefix(scope, "batch:")
	  }
	   if strings.HasPrefix(scope, "policy:") {
		   return l.PolicyID == strings.TrimPrefix(scope, "policy:")
	  }
	  if strings.HasPrefix(scope, "asset:") {
		  want := strings.TrimPrefix(scope, "asset:")
		  got := strings.ToUpper(l.Asset) + ":" + strings.ToUpper(l.Chain)
		   return got == want || strings.HasPrefix(want, strings.ToUpper(l.Asset))
	  }
	  return scope == l.BatchID || scope == l.PolicyID
   }
