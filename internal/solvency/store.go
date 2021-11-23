package solvency

import (
	"context"
	"strings"
	"sync"
)

// Store 证明单、叶、准备金快照、漂移与调度的持久化。
type Store interface {
	InsertAttestation(ctx context.Context, a Attestation) error
	GetAttestation(ctx context.Context, id string) (Attestation, error)
	ListAttestations(ctx context.Context, scheduleID string, limit int) ([]Attestation, error)
	SaveCollection(ctx context.Context, a Attestation, leaves []LiabilityLeaf) error
	ListLeaves(ctx context.Context,
		attestationID string) ([]LiabilityLeaf, error)
	SaveCompare(ctx context.Context, a Attestation, snaps []ReserveSnapshot, drifts []DriftCase) error
	ListReserve(ctx context.Context,
		attestationID string) ([]ReserveSnapshot, error)
	CasAttestation(ctx context.Context, id string, from Status, patch AttestPatch) (Attestation, error)
	NoteAudit(ctx context.Context,
		id, ref string) error
	GetDrift(ctx context.Context, id string) (DriftCase, error)
	ListDrifts(ctx context.Context, status DriftStatus, limit int) ([]DriftCase, error)
	CasDrift(ctx context.Context, id string, from DriftStatus, next DriftCase) (DriftCase, error)
	PutSchedule(ctx context.Context, s Schedule) error
	GetSchedule(ctx context.Context, id string) (Schedule, error)
	ListSchedules(ctx context.Context) ([]Schedule, error)
	MarkSignaled(ctx context.Context, id string) error
}

type MemStore struct {
	mu sync.Mutex
	atts   map[string]Attestation
	leaves map[string][]LiabilityLeaf
	snaps  map[string][]ReserveSnapshot
	drifts map[string]DriftCase
	byAtt  map[string][]string
	scheds map[string]Schedule
}

  func NewMem() *MemStore {
	   return &MemStore{
		  atts:   map[string]Attestation{},
		   leaves: map[string][]LiabilityLeaf{},
		   snaps:  map[string][]ReserveSnapshot{},
		   drifts: map[string]DriftCase{},
		   byAtt:  map[string][]string{},
		  scheds: map[string]Schedule{},
	  }
  }

  func (m *MemStore) InsertAttestation(_ context.Context, a Attestation) error {
	  if m == nil {
		  return ErrUnavailable
	  }
	   if a.ID == "" || a.Status != StatusBuilding || a.Root != "" || a.SealDigest != "" || a.Signature != "" {
		   return ErrTransition
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   if _, ok := m.atts[a.ID]; ok {
		   return ErrDuplicate
	  }
	  a.Solvency, a.Deprecated = deriveSolvency(a.Class)
	   m.atts[a.ID] = a
	   return nil
  }

  func (m *MemStore) GetAttestation(_ context.Context, id string) (Attestation, error) {
	   if m == nil {
		   return Attestation{}, ErrUnavailable
	  }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   a, ok := m.atts[strings.TrimSpace(id)]
	   if !ok {
		  return Attestation{}, ErrNotFound
	  }
	  return a, nil
  }

  func (m *MemStore) ListAttestations(_ context.Context, scheduleID string, limit int) ([]Attestation, error) {
	  if m == nil {
		  return nil, ErrUnavailable
	  }
	   if limit <= 0 || limit > 500 {
		   limit = 100
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  out := make([]Attestation, 0)
	  for _, a := range m.atts {
		   if scheduleID != "" && a.ScheduleID != scheduleID {
			   continue
		   }
		   out = append(out, a)
	   }
	   sortAttest(out)
	  if len(out) > limit {
		  out = out[:limit]
	   }
	   return out, nil
   }

  func sortAttest(in []Attestation) {
	   for i := 1; i < len(in); i++ {
		   j := i
		   for j > 0 && attestNewer(in[j], in[j-1]) {
			  in[j], in[j-1] = in[j-1], in[j]
			  j--
		  }
	  }
   }

  func attestNewer(a, b Attestation) bool {
	  if a.CreatedAt.Equal(b.CreatedAt) {
		   return a.ID > b.ID
	   }
	   return a.CreatedAt.After(b.CreatedAt)
  }

  func (m *MemStore) SaveCollection(_ context.Context, a Attestation, leaves []LiabilityLeaf) error {
	  if m == nil {
		   return ErrUnavailable
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  cur, ok := m.atts[a.ID]
	  if !ok {
		  return ErrNotFound
	  }
	   if cur.Status != StatusBuilding {
		   return ErrImmutable
	  }
	   if cur.Root != "" {
		  return ErrAlreadyCollected
	  }
	   if a.Root == "" {
		   return ErrRoot
	  }
	   cp := append([]LiabilityLeaf(nil),
		   leaves...)
	  for i := range cp {
		   cp[i].AttestationID = a.ID
	  }
	   cur.Root = a.Root
	  cur.LeafCount = a.LeafCount
	  cur.BookDigest = a.BookDigest
	   cur.FenceEpoch = a.FenceEpoch
	  cur.UpdatedAt = a.UpdatedAt
	  cur.Solvency = true
	   cur.Deprecated = false
	   m.atts[a.ID] = cur
	  m.leaves[a.ID] = cp
	  return nil
  }

  func (m *MemStore) ListLeaves(_ context.Context, attestationID string) ([]LiabilityLeaf, error) {
	   if m == nil {
		  return nil, ErrUnavailable
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   if _, ok := m.atts[attestationID]; !ok {
		  return nil, ErrNotFound
	   }
	   src := m.leaves[attestationID]
	  out := append([]LiabilityLeaf(nil), src...)
	   return out, nil
  }

  func (m *MemStore) SaveCompare(_ context.Context, a Attestation, snaps []ReserveSnapshot, drifts []DriftCase) error {
	  if m == nil {
		  return ErrUnavailable
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   cur, ok := m.atts[a.ID]
	   if !ok {
		   return ErrNotFound
	  }
	  if cur.Status != StatusBuilding {
		  return ErrImmutable
	   }
	  if cur.Root == "" {
		  return ErrNotCompared
	  }
	  for _, id := range m.byAtt[a.ID] {
		  d := m.drifts[id]
		   if d.Status != DriftOpen {
			  return ErrDrift
		   }
	  }
	   for _, id := range m.byAtt[a.ID] {
		  delete(m.drifts, id)
	  }
	   delete(m.byAtt, a.ID)
	  cp := append([]ReserveSnapshot(nil), snaps...)
	  m.snaps[a.ID] = cp
	  ids := make([]string, 0, len(drifts))
	   for _, d := range drifts {
		   d.AttestationID = a.ID
		   if d.Status == "" {
			   d.Status = DriftOpen
		   }
		  m.drifts[d.ID] = d
		   ids = append(ids, d.ID)
	  }
	   m.byAtt[a.ID] = ids
	  cur.Coverage = a.Coverage
	   cur.Class = a.Class
	   cur.Disclaimer = a.Disclaimer
	   cur.UpdatedAt = a.UpdatedAt
	  cur.Solvency, cur.Deprecated = deriveSolvency(cur.Class)
	  m.atts[a.ID] = cur
	   return nil
  }

   func (m *MemStore) ListReserve(_ context.Context, attestationID string) ([]ReserveSnapshot, error) {
	   if m == nil {
		  return nil, ErrUnavailable
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   if _, ok := m.atts[attestationID]; !ok {
		  return nil, ErrNotFound
	  }
	  return append([]ReserveSnapshot(nil), m.snaps[attestationID]...), nil
  }

   func applyPatch(cur Attestation, patch AttestPatch) Attestation {
	  cur.Status = patch.Status
	  if !patch.UpdatedAt.IsZero() {
		  cur.UpdatedAt = patch.UpdatedAt
	  }
	   if patch.Signature != "" {
		  cur.Signature = patch.Signature
	  }
	   if patch.Signer != "" {
		  cur.Signer = patch.Signer
	   }
	   if patch.SealDigest != "" {
		   cur.SealDigest = patch.SealDigest
	   }
	  if patch.Class != "" {
		   cur.Class = patch.Class
	   }
	  if patch.Disclaimer != "" {
		  cur.Disclaimer = patch.Disclaimer
	   }
	  if patch.Coverage != "" {
		  cur.Coverage = patch.Coverage
	  }
	   if patch.Reason != "" {
		  cur.Reason = patch.Reason
	  }
	  if patch.AuditRef != "" {
		   cur.AuditRef = patch.AuditRef
	   }
	  if patch.SealedAt != nil {
		   t := utc(*patch.SealedAt)
		  cur.SealedAt = &t
	  }
	   if patch.PublishedAt != nil {
		   t := utc(*patch.PublishedAt)
		  cur.PublishedAt = &t
	  }
	  if patch.RevokedAt != nil {
		   t := utc(*patch.RevokedAt)
		  cur.RevokedAt = &t
	   }
	  cur.Solvency, cur.Deprecated = deriveSolvency(cur.Class)
	   return cur
   }

  func (m *MemStore) CasAttestation(_ context.Context, id string, from Status, patch AttestPatch) (Attestation, error) {
	  if m == nil {
		   return Attestation{}, ErrUnavailable
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  cur, ok := m.atts[strings.TrimSpace(id)]
	  if !ok {
		  return Attestation{}, ErrNotFound
	  }
	   if cur.Status != from {
		  return cur, ErrTransition
	   }
	  if !AllowAttest(cur.Status, patch.Status) {
		  return cur, ErrTransition
	  }
	  if patch.Status == StatusPublished {
		  if cur.Root == "" || cur.SealDigest == "" || cur.Signature == "" {
			   return cur, ErrPartial
		  }
		   if patch.SealDigest != "" && patch.SealDigest != cur.SealDigest {
			   return cur, ErrSeal
		   }
	   }
	   if patch.Status == StatusSealed {
		  if cur.Root == "" || patch.SealDigest == "" || patch.Signature == "" || patch.SealedAt == nil {
			  return cur, ErrPartial
		  }
	  }
	   next := applyPatch(cur, patch)
	  if next.Root != cur.Root || next.LeafCount != cur.LeafCount || next.BookDigest != cur.BookDigest || next.FenceEpoch != cur.FenceEpoch || next.FenceToken != cur.FenceToken {
		   return cur, ErrImmutable
	   }
	   m.atts[id] = next
	  return next, nil
  }

  func (m *MemStore) NoteAudit(_ context.Context, id, ref string) error {
	   if m == nil {
		   return ErrUnavailable
	   }
	  ref = strings.TrimSpace(ref)
	  if ref == "" {
		   return ErrAudit
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   cur, ok := m.atts[id]
	  if !ok {
		   return ErrNotFound
	  }
	  if cur.Status != StatusPublished && cur.Status != StatusSealed {
		   return ErrTransition
	   }
	   if cur.Root == "" {
		  return ErrPartial
	   }
	   cur.AuditRef = ref
	  m.atts[id] = cur
	  return nil
  }

   func (m *MemStore) GetDrift(_ context.Context, id string) (DriftCase, error) {
	  if m == nil {
		   return DriftCase{}, ErrUnavailable
	  }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  d, ok := m.drifts[strings.TrimSpace(id)]
	   if !ok {
		   return DriftCase{}, ErrDriftNotFound
	   }
	  return d, nil
   }

   func (m *MemStore) ListDrifts(_ context.Context, status DriftStatus, limit int) ([]DriftCase, error) {
	   if m == nil {
		  return nil, ErrUnavailable
	  }
	  if limit <= 0 || limit > 500 {
		   limit = 100
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  out := make([]DriftCase, 0)
	  for _, d := range m.drifts {
		  if status != "" && d.Status != status {
			  continue
		   }
		  out = append(out, d)
	   }
	  for i := 1; i < len(out); i++ {
		   j := i
		   for j > 0 && (out[j].OpenedAt.After(out[j-1].OpenedAt) || (out[j].OpenedAt.Equal(out[j-1].OpenedAt) && out[j].Asset < out[j-1].Asset)) {
			  out[j], out[j-1] = out[j-1], out[j]
			   j--
		   }
	  }
	  if len(out) > limit {
		   out = out[:limit]
	   }
	  return out, nil
   }

  func (m *MemStore) CasDrift(_ context.Context, id string, from DriftStatus, next DriftCase) (DriftCase, error) {
	   if m == nil {
		  return DriftCase{}, ErrUnavailable
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   cur, ok := m.drifts[id]
	  if !ok {
		  return DriftCase{}, ErrDriftNotFound
	   }
	  if cur.Status != from || !AllowDrift(from, next.Status) {
		  return cur, ErrDrift
	   }
	   if cur.Asset != next.Asset || cur.AttestationID != next.AttestationID || cur.Deficit != next.Deficit || cur.Liability != next.Liability {
		  return cur, ErrImmutable
	   }
	   m.drifts[id] = next
	  return next, nil
   }

   func (m *MemStore) PutSchedule(_ context.Context, s Schedule) error {
	  if m == nil {
		   return ErrUnavailable
	  }
	  if s.ID == "" || s.Name == "" || s.Window <= 0 {
		  return ErrSchedule
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   m.scheds[s.ID] = s
	   return nil
   }

   func (m *MemStore) GetSchedule(_ context.Context, id string) (Schedule, error) {
	  if m == nil {
		  return Schedule{}, ErrUnavailable
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  s, ok := m.scheds[strings.TrimSpace(id)]
	   if !ok {
		  return Schedule{}, ErrScheduleNotFound
	   }
	  return s, nil
   }

   func (m *MemStore) ListSchedules(_ context.Context) ([]Schedule, error) {
	  if m == nil {
		   return nil, ErrUnavailable
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  out := make([]Schedule, 0, len(m.scheds))
	  for _, s := range m.scheds {
		   out = append(out, s)
	   }
	   for i := 1; i < len(out); i++ {
		   j := i
		   for j > 0 && out[j].ID < out[j-1].ID {
			  out[j], out[j-1] = out[j-1], out[j]
			  j--
		  }
	  }
	  return out, nil
   }

   func (m *MemStore) MarkSignaled(_ context.Context, id string) error {
	   if m == nil {
		  return ErrUnavailable
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  id = strings.TrimSpace(id)
	   d, ok := m.drifts[id]
	   if !ok {
		  return ErrDriftNotFound
	   }
	  d.Signaled = true
	  m.drifts[id] = d
	  return nil
  }
