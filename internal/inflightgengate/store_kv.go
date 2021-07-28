package inflightgengate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"
	"account-assets.kucoin.net/funding/internal/destlife"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
)

const (
	kindInflightLease = "destlife.inflight_gen_lease"
	kindRevokeGen = "destlife.revoke_gen"
	kindDriftCase = "destlife.drift_case"
	kindInflightProof = "destlife.inflight_proof"
)

// ─────────────────────────────────────────────────────────────────────────────
// KVGenLeaseStore
// ─────────────────────────────────────────────────────────────────────────────
// KVGenLeaseStore 基于 persist.KV 的在途世代租约存储。

type KVGenLeaseStore struct {
	kv persist.KV
}

// NewKVGenLeaseStore 构造 KV 版在途世代租约存储。
   func NewKVGenLeaseStore(kv persist.KV) *KVGenLeaseStore { return &KVGenLeaseStore{kv: kv} }

   func (s *KVGenLeaseStore) Insert(ctx context.Context, l destlife.InflightGenLease) (destlife.InflightGenLease, bool, error) {
	  existing, err := s.getByMovement(ctx, l.EntryID, l.MovementID)
	   if err == nil {
		  return existing, true, nil
	  }
	   if !errors.Is(err, destlife.ErrLeaseNotFound) {
		  return destlife.InflightGenLease{}, false, fmt.Errorf("%w: %v", destlife.ErrStore, err)
	   }
	   if l.LeaseID == "" {
		  l.LeaseID = uuid.NewString()
	   }
	   body, err := json.Marshal(l)
	  if err != nil {
		  return destlife.InflightGenLease{}, false, fmt.Errorf("marshal lease: %w", err)
	  }
	   if putErr := s.kv.Put(ctx, persist.Row{Kind: kindInflightLease, Key: l.LeaseID, Body: body}, persist.Fence{}); putErr != nil {
		   return destlife.InflightGenLease{}, false, fmt.Errorf("%w: %v", destlife.ErrStore, putErr)
	   }
	   return l, false, nil
  }

  func (s *KVGenLeaseStore) GetByID(ctx context.Context, leaseID string) (destlife.InflightGenLease, error) {
	   row, err := s.kv.Get(ctx, kindInflightLease, leaseID)
	   if err != nil {
		   if errors.Is(err, persist.ErrNotFound) {
			   return destlife.InflightGenLease{}, destlife.ErrLeaseNotFound
		  }
		  return destlife.InflightGenLease{}, fmt.Errorf("%w: %v", destlife.ErrStore, err)
	   }
	  var l destlife.InflightGenLease
	   if jsonErr := json.Unmarshal(row.Body, &l); jsonErr != nil {
		  return destlife.InflightGenLease{}, destlife.ErrLeaseNotFound
	  }
	   return l, nil
   }

  func (s *KVGenLeaseStore) GetByMovement(ctx context.Context, entryID, movementID string) (destlife.InflightGenLease, error) {
	  return s.getByMovement(ctx, entryID, movementID)
   }

  func (s *KVGenLeaseStore) getByMovement(ctx context.Context, entryID, movementID string) (destlife.InflightGenLease, error) {
	  rows, err := s.kv.List(ctx, kindInflightLease, 1000)
	  if err != nil {
		  return destlife.InflightGenLease{}, fmt.Errorf("%w: %v", destlife.ErrStore, err)
	  }
	   for _, row := range rows {
		   var l destlife.InflightGenLease
		   if json.Unmarshal(row.Body, &l) != nil {
			  continue
		  }
		  if l.EntryID == entryID && l.MovementID == movementID {
			  return l, nil
		  }
	  }
	   return destlife.InflightGenLease{}, destlife.ErrLeaseNotFound
   }

  func (s *KVGenLeaseStore) ListActiveByEntry(ctx context.Context, entryID string) ([]destlife.InflightGenLease, error) {
	  rows, err := s.kv.List(ctx, kindInflightLease, 1000)
	   if err != nil {
		  return nil, fmt.Errorf("%w: %v", destlife.ErrStore, err)
	   }
	   var out []destlife.InflightGenLease
	  for _, row := range rows {
		   var l destlife.InflightGenLease
		   if json.Unmarshal(row.Body, &l) != nil {
			  continue
		   }
		  if l.EntryID == entryID && (l.State == destlife.RefAcquired || l.State == destlife.RefBound) {
			  out = append(out, l)
		  }
	   }
	  sort.Slice(out, func(i, j int) bool { return out[i].IssuedAt.Before(out[j].IssuedAt) })
	  return out, nil
  }

  func (s *KVGenLeaseStore) CountActiveByEntry(ctx context.Context, entryID string) (int, error) {
	  list, err := s.ListActiveByEntry(ctx, entryID)
	  return len(list), err
  }

  func (s *KVGenLeaseStore) Transition(ctx context.Context, leaseID string, from, to destlife.RefState, updates destlife.InflightGenLease) error {
	   l, err := s.GetByID(ctx, leaseID)
	  if err != nil {
		  return err
	   }
	   if l.State != from {
		  return destlife.ErrInvalidState
	  }
	  l.State = to
	   if updates.BoundAt != nil {
		  l.BoundAt = updates.BoundAt
	   }
	   if updates.ReleasedAt != nil {
		  l.ReleasedAt = updates.ReleasedAt
	   }
	   if updates.BoundRevokeGen != 0 {
		  l.BoundRevokeGen = updates.BoundRevokeGen
	  }
	   if updates.Note != "" {
		   l.Note = updates.Note
	  }
	  body, err := json.Marshal(l)
	  if err != nil {
		  return fmt.Errorf("marshal lease: %w", err)
	  }
	  return s.kv.Put(ctx,
		  persist.Row{Kind: kindInflightLease, Key: leaseID, Body: body}, persist.Fence{})
   }

  func (s *KVGenLeaseStore) ListExpired(ctx context.Context, now time.Time, limit int) ([]destlife.InflightGenLease, error) {
	   rows, err := s.kv.List(ctx, kindInflightLease, 1000)
	   if err != nil {
		   return nil, fmt.Errorf("%w: %v", destlife.ErrStore, err)
	   }
	   var out []destlife.InflightGenLease
	  for _, row := range rows {
		   var l destlife.InflightGenLease
		   if json.Unmarshal(row.Body, &l) != nil {
			  continue
		   }
		  if l.State != destlife.RefReleased && l.ExpiresAt.Before(now) {
			  out = append(out, l)
			   if limit > 0 && len(out) >= limit {
				  break
			   }
		  }
	   }
	  return out, nil
   }

   func (s *KVGenLeaseStore) ListByEntry(ctx context.Context, entryID string) ([]destlife.InflightGenLease, error) {
	   rows, err := s.kv.List(ctx, kindInflightLease, 1000)
	  if err != nil {
		   return nil, fmt.Errorf("%w: %v", destlife.ErrStore, err)
	   }
	  var out []destlife.InflightGenLease
	  for _, row := range rows {
		   var l destlife.InflightGenLease
		   if json.Unmarshal(row.Body, &l) != nil {
			  continue
		  }
		   if l.EntryID == entryID {
			   out = append(out, l)
		   }
	   }
	   sort.Slice(out, func(i, j int) bool { return out[i].IssuedAt.Before(out[j].IssuedAt) })
	   return out, nil
   }

// ─────────────────────────────────────────────────────────────────────────────
// KVRevokeGenStore
// ─────────────────────────────────────────────────────────────────────────────
// KVRevokeGenStore 基于 persist.KV 的吊销世代存储。

type KVRevokeGenStore struct {
	kv persist.KV
}

// NewKVRevokeGenStore 构造 KV 版吊销世代存储。

  func NewKVRevokeGenStore(kv persist.KV) *KVRevokeGenStore { return &KVRevokeGenStore{kv: kv} }

  func (s *KVRevokeGenStore) GetOrInit(ctx context.Context, entryID, userID, chain, address string) (destlife.RevokeGeneration, error) {
	   r, err := s.Get(ctx, entryID)
	  if err == nil {
		   return r, nil
	   }
	   if !errors.Is(err, destlife.ErrEntryNotFound) {
		   return destlife.RevokeGeneration{}, err
	  }
	   r = destlife.RevokeGeneration{
		   EntryID: entryID,
		  UserID:  userID,
		  Chain: chain,
		  Address:    address,
		   Gen:        0,
		  State:      destlife.EntryActive,
		   ActiveRefs: 0,
		   InitiatedAt: time.Now().UTC(),
	  }
	   body, err := json.Marshal(r)
	  if err != nil {
		   return destlife.RevokeGeneration{}, fmt.Errorf("marshal revoke gen: %w", err)
	   }
	   if putErr := s.kv.Put(ctx, persist.Row{Kind: kindRevokeGen, Key: entryID, Body: body}, persist.Fence{}); putErr != nil {
		   return destlife.RevokeGeneration{}, fmt.Errorf("%w: %v", destlife.ErrStore, putErr)
	  }
	  return r, nil
  }

   func (s *KVRevokeGenStore) Get(ctx context.Context, entryID string) (destlife.RevokeGeneration, error) {
	   row, err := s.kv.Get(ctx, kindRevokeGen, entryID)
	  if err != nil {
		   if errors.Is(err, persist.ErrNotFound) {
			   return destlife.RevokeGeneration{}, destlife.ErrEntryNotFound
		   }
		  return destlife.RevokeGeneration{}, fmt.Errorf("%w: %v", destlife.ErrStore, err)
	  }
	  var r destlife.RevokeGeneration
	   if jsonErr := json.Unmarshal(row.Body, &r); jsonErr != nil {
		  return destlife.RevokeGeneration{}, destlife.ErrEntryNotFound
	  }
	  return r, nil
  }

   func (s *KVRevokeGenStore) BumpGen(ctx context.Context, entryID string, currentGen int64, newState destlife.EntryState, actor string) (destlife.RevokeGeneration, error) {
	   r, err := s.Get(ctx, entryID)
	   if err != nil {
		   return destlife.RevokeGeneration{},
			  err
	  }
	  if r.Gen != currentGen {
		   return destlife.RevokeGeneration{}, destlife.ErrInvalidState
	   }
	  r.Gen++
	   r.State = newState
	  r.Actor = actor
	  body, err := json.Marshal(r)
	   if err != nil {
		   return destlife.RevokeGeneration{}, fmt.Errorf("marshal revoke gen: %w", err)
	  }
	  if putErr := s.kv.Put(ctx, persist.Row{Kind: kindRevokeGen, Key: entryID, Body: body}, persist.Fence{}); putErr != nil {
		  return destlife.RevokeGeneration{}, fmt.Errorf("%w: %v", destlife.ErrStore, putErr)
	  }
	   return r, nil
  }

   func (s *KVRevokeGenStore) UpdateActiveRefs(ctx context.Context, entryID string, delta int) (destlife.RevokeGeneration, error) {
	  r, err := s.Get(ctx, entryID)
	  if err != nil {
		  return destlife.RevokeGeneration{}, err
	   }
	  r.ActiveRefs += delta
	   if r.ActiveRefs < 0 {
		   r.ActiveRefs = 0
	   }
	   body, err := json.Marshal(r)
	   if err != nil {
		   return destlife.RevokeGeneration{}, fmt.Errorf("marshal revoke gen: %w", err)
	   }
	   if putErr := s.kv.Put(ctx, persist.Row{Kind: kindRevokeGen, Key: entryID, Body: body}, persist.Fence{}); putErr != nil {
		   return destlife.RevokeGeneration{}, fmt.Errorf("%w: %v", destlife.ErrStore, putErr)
	  }
	   return r, nil
  }

   func (s *KVRevokeGenStore) SetRevoked(ctx context.Context, entryID string, actor string) (destlife.RevokeGeneration, error) {
	   r, err := s.Get(ctx, entryID)
	   if err != nil {
		  return destlife.RevokeGeneration{}, err
	  }
	  now := time.Now().UTC()
	   r.State = destlife.EntryRevoked
	   r.RevokedAt = &now
	   r.Actor = actor
	  body, err := json.Marshal(r)
	   if err != nil {
		  return destlife.RevokeGeneration{}, fmt.Errorf("marshal revoke gen: %w", err)
	   }
	  if putErr := s.kv.Put(ctx, persist.Row{Kind: kindRevokeGen, Key: entryID, Body: body}, persist.Fence{}); putErr != nil {
		   return destlife.RevokeGeneration{}, fmt.Errorf("%w: %v", destlife.ErrStore, putErr)
	   }
	   return r, nil
  }

  func (s *KVRevokeGenStore) ListRevoking(ctx context.Context, limit int) ([]destlife.RevokeGeneration, error) {
	  rows, err := s.kv.List(ctx, kindRevokeGen, 1000)
	   if err != nil {
		   return nil, fmt.Errorf("%w: %v", destlife.ErrStore, err)
	  }
	   var out []destlife.RevokeGeneration
	   for _, row := range rows {
		  var r destlife.RevokeGeneration
		   if json.Unmarshal(row.Body, &r) != nil {
			   continue
		   }
		   if r.State == destlife.EntryRevoking {
			  out = append(out, r)
			  if limit > 0 && len(out) >= limit {
				  break
			   }
		   }
	   }
	   return out, nil
  }

// ─────────────────────────────────────────────────────────────────────────────
// KVDriftCaseStore
// ─────────────────────────────────────────────────────────────────────────────
// KVDriftCaseStore 基于 persist.KV 的漂移案例存储。
type KVDriftCaseStore struct {
	kv persist.KV
}

// NewKVDriftCaseStore 构造 KV 版漂移案例存储。
  func NewKVDriftCaseStore(kv persist.KV) *KVDriftCaseStore { return &KVDriftCaseStore{kv: kv} }

   func (s *KVDriftCaseStore) Open(ctx context.Context, dc destlife.InflightDriftCase) (destlife.InflightDriftCase, error) {
	  existing, err := s.GetOpenByEntry(ctx, dc.EntryID)
	  if err == nil {
		   if dc.Note != "" {
			   existing.Note += "; " + dc.Note
			   body, _ := json.Marshal(existing)
			   _ = s.kv.Put(ctx, persist.Row{Kind: kindDriftCase, Key: existing.CaseID, Body: body}, persist.Fence{})
		   }
		  return existing, nil
	  }
	   if dc.CaseID == "" {
		   dc.CaseID = uuid.NewString()
	   }
	   body, err := json.Marshal(dc)
	   if err != nil {
		   return destlife.InflightDriftCase{}, fmt.Errorf("marshal drift case: %w", err)
	   }
	   if putErr := s.kv.Put(ctx, persist.Row{Kind: kindDriftCase, Key: dc.CaseID, Body: body}, persist.Fence{}); putErr != nil {
		   return destlife.InflightDriftCase{}, fmt.Errorf("%w: %v", destlife.ErrStore, putErr)
	   }
	  return dc, nil
   }

   func (s *KVDriftCaseStore) GetOpenByEntry(ctx context.Context, entryID string) (destlife.InflightDriftCase, error) {
	  rows, err := s.kv.List(ctx, kindDriftCase, 1000)
	  if err != nil {
		  return destlife.InflightDriftCase{}, fmt.Errorf("%w: %v", destlife.ErrStore, err)
	   }
	   for _, row := range rows {
		   var dc destlife.InflightDriftCase
		   if json.Unmarshal(row.Body, &dc) != nil {
			  continue
		  }
		  if dc.EntryID == entryID && dc.State == destlife.DriftCaseOpen {
			   return dc, nil
		  }
	  }
	   return destlife.InflightDriftCase{}, destlife.ErrLeaseNotFound
  }

  func (s *KVDriftCaseStore) GetByID(ctx context.Context, caseID string) (destlife.InflightDriftCase, error) {
	  row, err := s.kv.Get(ctx, kindDriftCase, caseID)
	  if err != nil {
		   if errors.Is(err, persist.ErrNotFound) {
			   return destlife.InflightDriftCase{}, destlife.ErrLeaseNotFound
		   }
		  return destlife.InflightDriftCase{}, fmt.Errorf("%w: %v", destlife.ErrStore, err)
	   }
	   var dc destlife.InflightDriftCase
	   if jsonErr := json.Unmarshal(row.Body, &dc); jsonErr != nil {
		   return destlife.InflightDriftCase{}, destlife.ErrLeaseNotFound
	  }
	   return dc, nil
  }

  func (s *KVDriftCaseStore) Close(ctx context.Context, caseID, closedBy, resolution string) (destlife.InflightDriftCase, error) {
	  dc, err := s.GetByID(ctx, caseID)
	   if err != nil {
		  return destlife.InflightDriftCase{}, err
	   }
	   if dc.State != destlife.DriftCaseOpen {
		  return dc, nil
	   }
	   now := time.Now().UTC()
	  dc.State = destlife.DriftCaseClosed
	  dc.ClosedAt = &now
	   dc.ClosedBy = closedBy
	   dc.Resolution = resolution
	   body, err := json.Marshal(dc)
	  if err != nil {
		   return destlife.InflightDriftCase{}, fmt.Errorf("marshal drift case: %w", err)
	  }
	   if putErr := s.kv.Put(ctx, persist.Row{Kind: kindDriftCase, Key: caseID, Body: body}, persist.Fence{}); putErr != nil {
		  return destlife.InflightDriftCase{}, fmt.Errorf("%w: %v", destlife.ErrStore, putErr)
	  }
	   return dc, nil
   }

   func (s *KVDriftCaseStore) ListOpen(ctx context.Context, limit int) ([]destlife.InflightDriftCase, error) {
	  rows, err := s.kv.List(ctx, kindDriftCase, 1000)
	   if err != nil {
		  return nil, fmt.Errorf("%w: %v", destlife.ErrStore, err)
	   }
	   var out []destlife.InflightDriftCase
	   for _, row := range rows {
		  var dc destlife.InflightDriftCase
		   if json.Unmarshal(row.Body, &dc) != nil {
			  continue
		  }
		   if dc.State == destlife.DriftCaseOpen {
			   out = append(out, dc)
			  if limit > 0 && len(out) >= limit {
				   break
			   }
		   }
	   }
	  sort.Slice(out, func(i, j int) bool { return out[i].OpenedAt.Before(out[j].OpenedAt) })
	   return out, nil
   }

  func (s *KVDriftCaseStore) ListByEntry(ctx context.Context, entryID string) ([]destlife.InflightDriftCase, error) {
	  rows, err := s.kv.List(ctx, kindDriftCase, 1000)
	  if err != nil {
		  return nil, fmt.Errorf("%w: %v", destlife.ErrStore, err)
	  }
	  var out []destlife.InflightDriftCase
	  for _, row := range rows {
		   var dc destlife.InflightDriftCase
		  if json.Unmarshal(row.Body, &dc) != nil {
			   continue
		   }
		   if dc.EntryID == entryID {
			  out = append(out, dc)
		  }
	  }
	   sort.Slice(out, func(i, j int) bool { return out[i].OpenedAt.Before(out[j].OpenedAt) })
	  return out, nil
   }

// ─────────────────────────────────────────────────────────────────────────────
// KVProofStore
// ─────────────────────────────────────────────────────────────────────────────
// KVProofStore 基于 persist.KV 的在途证明存储。
type KVProofStore struct {
	kv persist.KV
}

// NewKVProofStore 构造 KV 版在途证明存储。

  func NewKVProofStore(kv persist.KV) *KVProofStore { return &KVProofStore{kv: kv} }

   func (s *KVProofStore) Insert(ctx context.Context, p destlife.InflightProof) (destlife.InflightProof, bool, error) {
	   existing, err := s.GetByLease(ctx, p.LeaseID, p.MovementID)
	   if err == nil {
		  return existing, true, nil
	   }
	  if !errors.Is(err, destlife.ErrProofMissing) {
		   return destlife.InflightProof{}, false, fmt.Errorf("%w: %v", destlife.ErrStore, err)
	  }
	   if p.ProofID == "" {
		  p.ProofID = uuid.NewString()
	   }
	  body, err := json.Marshal(p)
	   if err != nil {
		   return destlife.InflightProof{}, false, fmt.Errorf("marshal proof: %w", err)
	   }
	  if putErr := s.kv.Put(ctx, persist.Row{Kind: kindInflightProof, Key: p.ProofID, Body: body}, persist.Fence{}); putErr != nil {
		  return destlife.InflightProof{}, false, fmt.Errorf("%w: %v", destlife.ErrStore, putErr)
	  }
	   return p, false, nil
  }

   func (s *KVProofStore) GetByLease(ctx context.Context, leaseID, movementID string) (destlife.InflightProof, error) {
	   rows, err := s.kv.List(ctx, kindInflightProof, 1000)
	   if err != nil {
		  return destlife.InflightProof{}, fmt.Errorf("%w: %v", destlife.ErrStore, err)
	   }
	  for _, row := range rows {
		  var p destlife.InflightProof
		  if json.Unmarshal(row.Body, &p) != nil {
			   continue
		  }
		   if p.LeaseID == leaseID && p.MovementID == movementID {
			   return p, nil
		   }
	   }
	   return destlife.InflightProof{}, destlife.ErrProofMissing
   }

  func (s *KVProofStore) GetByID(ctx context.Context, proofID string) (destlife.InflightProof, error) {
	  row, err := s.kv.Get(ctx, kindInflightProof, proofID)
	   if err != nil {
		  if errors.Is(err, persist.ErrNotFound) {
			   return destlife.InflightProof{},
				   destlife.ErrProofMissing
		  }
		  return destlife.InflightProof{}, fmt.Errorf("%w: %v", destlife.ErrStore, err)
	  }
	  var p destlife.InflightProof
	   if jsonErr := json.Unmarshal(row.Body, &p); jsonErr != nil {
		  return destlife.InflightProof{},
			  destlife.ErrProofMissing
	   }
	  return p, nil
   }

   func (s *KVProofStore) ListByEntry(ctx context.Context, entryID string) ([]destlife.InflightProof, error) {
	   rows, err := s.kv.List(ctx, kindInflightProof, 1000)
	  if err != nil {
		  return nil, fmt.Errorf("%w: %v", destlife.ErrStore, err)
	  }
	  var out []destlife.InflightProof
	   for _, row := range rows {
		  var p destlife.InflightProof
		  if json.Unmarshal(row.Body, &p) != nil {
			   continue
		   }
		   if p.EntryID == entryID {
			   out = append(out, p)
		   }
	  }
	   sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	  return out, nil
   }

// NewKVGate 使用 KV Store 构造闸实例（适用于嵌入式/模式）。
  func NewKVGate(kv persist.KV, cfg GateConfig) *Gate {
	  return NewGate(
		   NewKVGenLeaseStore(kv),
		   NewKVRevokeGenStore(kv),
		   NewKVDriftCaseStore(kv),
		  NewKVProofStore(kv),
		   cfg,
	  )
  }
