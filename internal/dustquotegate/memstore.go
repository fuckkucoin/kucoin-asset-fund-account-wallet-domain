package dustquotegate

import (
	"context"
	"strings"
	"sync"
	"time"
)

// MemStore 进程内 Store。
type MemStore struct {
	mu sync.Mutex
	leases map[string]DustQuoteLease
	byIdem map[string]string
	proofs map[string]DustQuoteDriftProof
	audits map[string][]DustQuoteAudit
	cursors map[string]ScanCursor
	flips    map[string]FlipEvent
	flipDone map[string]bool
	epochs map[string]int64
}

// NewMemStore 构造空仓。

   func NewMemStore() *MemStore {
	   return &MemStore{
		  leases: map[string]DustQuoteLease{},
		   byIdem: map[string]string{},
		  proofs: map[string]DustQuoteDriftProof{},
		  audits: map[string][]DustQuoteAudit{},
		   cursors:  map[string]ScanCursor{},
		  flips:    map[string]FlipEvent{},
		  flipDone: map[string]bool{},
		  epochs:   map[string]int64{},
	   }
   }

   func (m *MemStore) SaveLease(_ context.Context, l DustQuoteLease) error {
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

   func (m *MemStore) GetLease(_ context.Context, leaseID string) (DustQuoteLease, error) {
	   if m == nil {
		   return DustQuoteLease{}, ErrUnavailable
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   cur, ok := m.leases[strings.TrimSpace(leaseID)]
	   if !ok {
		  return DustQuoteLease{}, ErrLeaseNotFound
	  }
	   return cur.Copy(), nil
  }

  func (m *MemStore) GetLeaseByIdem(_ context.Context, idem string) (DustQuoteLease, error) {
	  if m == nil {
		  return DustQuoteLease{},
			  ErrUnavailable
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   id, ok := m.byIdem[strings.TrimSpace(idem)]
	  if !ok {
		  return DustQuoteLease{}, ErrLeaseNotFound
	   }
	  return m.leases[id].Copy(), nil
   }

   func (m *MemStore) ListLeasesByUserAsset(_ context.Context, user, asset string) ([]DustQuoteLease, error) {
	  if m == nil {
		  return nil, ErrUnavailable
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  out := make([]DustQuoteLease, 0)
	  for _, l := range m.leases {
		  if user != "" && l.UserID != user {
			  continue
		   }
		  if asset != "" && l.Asset != asset {
			   continue
		   }
		  out = append(out, l.Copy())
	   }
	   return out, nil
  }

   func (m *MemStore) ListOpenByAsset(_ context.Context, asset string) ([]DustQuoteLease, error) {
	   if m == nil {
		   return nil, ErrUnavailable
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   out := make([]DustQuoteLease, 0)
	  for _, l := range m.leases {
		   if asset != "" && l.Asset != asset {
			   continue
		   }
		  if l.State.Terminal() {
			   continue
		  }
		  out = append(out, l.Copy())
	  }
	  return out, nil
   }

  func (m *MemStore) ListQuotedOrBound(_ context.Context, now time.Time, limit int) ([]DustQuoteLease, error) {
	   if m == nil {
		  return nil, ErrUnavailable
	   }
	  if limit <= 0 {
		  limit = 200
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  out := make([]DustQuoteLease, 0, limit)
	  for _, l := range m.leases {
		   if l.State != LeaseQuoted && l.State != LeaseBound {
			   continue
		  }
		   _ = now
		  out = append(out, l.Copy())
		   if len(out) >= limit {
			  break
		   }
	   }
	  return out, nil
   }

  func (m *MemStore) CASState(_ context.Context, leaseID string, from, to LeaseState, patch DustQuoteLease) error {
	   if m == nil {
		  return ErrUnavailable
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  cur, ok := m.leases[strings.TrimSpace(leaseID)]
	   if !ok {
		  return ErrLeaseNotFound
	   }
	   if cur.State != from {
		   return ErrInvalidTransition
	   }
	   if !legalTransit(from, to) {
		  return ErrInvalidTransition
	   }
	   cur.State = to
	   if !patch.BoundAt.IsZero() {
		  cur.BoundAt = patch.BoundAt
	  }
	   if !patch.SettledAt.IsZero() {
		   cur.SettledAt = patch.SettledAt
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
	   if patch.ConvertID != "" {
		   cur.ConvertID = patch.ConvertID
	  }
	   if !patch.UpdatedAt.IsZero() {
		   cur.UpdatedAt = patch.UpdatedAt
	  }
	   m.leases[cur.LeaseID] = cur
	  return nil
  }

  func legalTransit(from, to LeaseState) bool {
	   switch from {
	  case LeaseQuoted:
		  return to == LeaseBound || to == LeaseExpired || to == LeaseRevoked
	   case LeaseBound:
		   return to == LeaseSettled || to == LeaseExpired || to == LeaseRevoked
	   default:
		  return false
	  }
   }

   func (m *MemStore) SaveProof(_ context.Context, p DustQuoteDriftProof) error {
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
		  return ErrAlreadyProven
	   }
	   m.proofs[id] = p.Copy()
	  return nil
  }

   func (m *MemStore) GetProof(_ context.Context, proofID string) (DustQuoteDriftProof, error) {
	   if m == nil {
		  return DustQuoteDriftProof{}, ErrUnavailable
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  p, ok := m.proofs[strings.TrimSpace(proofID)]
	   if !ok {
		   return DustQuoteDriftProof{},
			  ErrProofNotFound
	   }
	  return p.Copy(), nil
  }

   func (m *MemStore) ListProofsByLease(_ context.Context, leaseID string) ([]DustQuoteDriftProof, error) {
	  if m == nil {
		  return nil, ErrUnavailable
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  out := make([]DustQuoteDriftProof, 0)
	  for _, p := range m.proofs {
		  if p.LeaseID == leaseID {
			  out = append(out, p.Copy())
		   }
	   }
	  return out, nil
   }

  func (m *MemStore) ListProofsByUser(_ context.Context, user string, limit int) ([]DustQuoteDriftProof, error) {
	   if m == nil {
		  return nil, ErrUnavailable
	  }
	  if limit <= 0 {
		  limit = 50
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  out := make([]DustQuoteDriftProof, 0, limit)
	   for _, p := range m.proofs {
		  if user != "" && p.UserID != user {
			  continue
		   }
		   out = append(out, p.Copy())
		   if len(out) >= limit {
			   break
		  }
	   }
	   return out, nil
   }

  func (m *MemStore) ListProofsByKind(_ context.Context, kind ProofKind, limit int) ([]DustQuoteDriftProof, error) {
	   if m == nil {
		  return nil, ErrUnavailable
	  }
	   if limit <= 0 {
		   limit = 50
	  }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   out := make([]DustQuoteDriftProof, 0, limit)
	   for _, p := range m.proofs {
		   if kind != "" && p.Kind != kind {
			  continue
		  }
		   out = append(out, p.Copy())
		   if len(out) >= limit {
			   break
		   }
	   }
	  return out, nil
  }

   func (m *MemStore) AppendAudit(_ context.Context, a DustQuoteAudit) error {
	  if m == nil {
		  return ErrUnavailable
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  key := strings.TrimSpace(a.LeaseID)
	  m.audits[key] = append(m.audits[key], a.Copy())
	   return nil
  }

  func (m *MemStore) ListAudit(_ context.Context, leaseID string) ([]DustQuoteAudit, error) {
	   if m == nil {
		  return nil, ErrUnavailable
	  }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   src := m.audits[strings.TrimSpace(leaseID)]
	   out := make([]DustQuoteAudit, len(src))
	   copy(out, src)
	   return out, nil
   }

  func (m *MemStore) SaveCursor(_ context.Context, c ScanCursor) error {
	  if m == nil {
		   return ErrUnavailable
	   }
	   if strings.TrimSpace(c.BatchID) == "" {
		   return ErrScanCursor
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  m.cursors[c.BatchID] = c.Copy()
	   return nil
  }

  func (m *MemStore) GetCursor(_ context.Context, batchID string) (ScanCursor, error) {
	   if m == nil {
		  return ScanCursor{}, ErrUnavailable
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   c,
		   ok := m.cursors[strings.TrimSpace(batchID)]
	  if !ok {
		  return ScanCursor{}, ErrScanCursor
	  }
	  return c.Copy(), nil
   }

  func (m *MemStore) ListHaltedCursors(_ context.Context) ([]ScanCursor, error) {
	   if m == nil {
		  return nil, ErrUnavailable
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  out := make([]ScanCursor, 0)
	   for _, c := range m.cursors {
		  if c.Halted {
			   out = append(out, c.Copy())
		   }
	   }
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
		   limit = 100
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   out := make([]FlipEvent, 0, limit)
	   for id, e := range m.flips {
		  if m.flipDone[id] {
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
	  m.flipDone[strings.TrimSpace(eventID)] = true
	  return nil
  }

   func (m *MemStore) NextFlipEpoch(_ context.Context, asset string) (int64, error) {
	  if m == nil {
		  return 0, ErrUnavailable
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   as := strings.ToUpper(strings.TrimSpace(asset))
	   m.epochs[as]++
	   return m.epochs[as], nil
   }

   func (m *MemStore) CurrentFlipEpoch(_ context.Context, asset string) (int64, error) {
	   if m == nil {
		   return 0, ErrUnavailable
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  return m.epochs[strings.ToUpper(strings.TrimSpace(asset))],
		  nil
  }
