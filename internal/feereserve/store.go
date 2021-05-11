package feereserve

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// ──────────────────────────────────────────
// Store 接口：面向领域仓储抽象
// ──────────────────────────────────────────

type Store interface {
	SaveLease(ctx context.Context, l ReserveLease) (ReserveLease, error)
	GetLease(ctx context.Context, id string) (ReserveLease, error)
	GetLeaseByWithdraw(ctx context.Context, withdrawID string) (ReserveLease, error)
	ListLeasesByState(ctx context.Context, state LeaseState, limit int) ([]ReserveLease, error)
	ListExpiredLeases(ctx context.Context, before time.Time, limit int) ([]ReserveLease, error)
	SaveTerminalBind(ctx context.Context, tb TerminalBind) error
	GetTerminalBind(ctx context.Context, withdrawID string) (TerminalBind, bool, error)
	SaveProof(ctx context.Context, dp DeltaProof) (DeltaProof, error)
	GetProof(ctx context.Context, id string) (DeltaProof, error)
	ListProofsByWithdraw(ctx context.Context, withdrawID string, limit int) ([]DeltaProof, error)
	ListProofsByLease(ctx context.Context, leaseID string, limit int) ([]DeltaProof, error)
	SaveCase(ctx context.Context, rc ReserveCase) (ReserveCase, error)
	GetCase(ctx context.Context, id string) (ReserveCase, error)
	ListCasesByState(ctx context.Context, state CaseState, limit int) ([]ReserveCase, error)
	ListCasesByWithdraw(ctx context.Context, withdrawID string, limit int) ([]ReserveCase, error)
}

// ──────────────────────────────────────────
// MemStore 内存实现（ / 单进程）
// ──────────────────────────────────────────

type MemStore struct {
	mu sync.Mutex
	leases map[string]*ReserveLease
	byWdID map[string]string
	binds  map[string]*TerminalBind
	proofs map[string]*DeltaProof
	cases  map[string]*ReserveCase
	byIdem map[string]string
}

  func NewMemStore() *MemStore {
	  return &MemStore{
		   leases: make(map[string]*ReserveLease),
		   byWdID: make(map[string]string),
		   binds: make(map[string]*TerminalBind),
		  proofs: make(map[string]*DeltaProof),
		  cases: make(map[string]*ReserveCase),
		  byIdem: make(map[string]string),
	  }
  }

   func (m *MemStore) SaveLease(_ context.Context, l ReserveLease) (ReserveLease, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  cp := l.Copy()
	   m.leases[cp.ID] = &cp
	   m.byWdID[cp.WithdrawID] = cp.ID
	   return cp, nil
  }

  func (m *MemStore) GetLease(_ context.Context, id string) (ReserveLease, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   l, ok := m.leases[strings.TrimSpace(id)]
	   if !ok {
		  return ReserveLease{}, ErrLeaseNotFound
	   }
	   return l.Copy(), nil
  }

  func (m *MemStore) GetLeaseByWithdraw(_ context.Context, withdrawID string) (ReserveLease, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   lid, ok := m.byWdID[strings.TrimSpace(withdrawID)]
	   if !ok {
		   return ReserveLease{}, ErrLeaseNotFound
	   }
	  l, ok := m.leases[lid]
	   if !ok {
		   return ReserveLease{}, ErrLeaseNotFound
	  }
	  return l.Copy(), nil
   }

  func (m *MemStore) ListLeasesByState(_ context.Context, state LeaseState, limit int) ([]ReserveLease, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   if limit <= 0 {
		  limit = 100
	   }
	   out := make([]ReserveLease, 0, limit)
	   for _, l := range m.leases {
		  if l.State == state {
			  out = append(out, l.Copy())
			  if len(out) >= limit {
				   break
			   }
		   }
	  }
	  return out, nil
   }

   func (m *MemStore) ListExpiredLeases(_ context.Context, before time.Time, limit int) ([]ReserveLease, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   if limit <= 0 {
		   limit = 100
	  }
	   out := make([]ReserveLease, 0, limit)
	   for _, l := range m.leases {
		  if l.State == LeaseReserved && l.LeaseExpiry.Before(before) {
			  out = append(out, l.Copy())
			  if len(out) >= limit {
				  break
			  }
		   }
	  }
	   return out, nil
  }

   func (m *MemStore) SaveTerminalBind(_ context.Context, tb TerminalBind) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  cp := tb
	   m.binds[cp.WithdrawID] = &cp
	  return nil
  }

   func (m *MemStore) GetTerminalBind(_ context.Context, withdrawID string) (TerminalBind, bool, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   tb, ok := m.binds[strings.TrimSpace(withdrawID)]
	  if !ok {
		   return TerminalBind{}, false, nil
	  }
	  return *tb, true, nil
  }

  func (m *MemStore) SaveProof(_ context.Context, dp DeltaProof) (DeltaProof, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  cp := dp
	  m.proofs[cp.ID] = &cp
	  return cp, nil
   }

  func (m *MemStore) GetProof(_ context.Context, id string) (DeltaProof, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  p, ok := m.proofs[strings.TrimSpace(id)]
	   if !ok {
		   return DeltaProof{}, ErrDeltaProof
	   }
	  return *p, nil
  }

   func (m *MemStore) ListProofsByWithdraw(_ context.Context, withdrawID string, limit int) ([]DeltaProof, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  if limit <= 0 {
		  limit = 100
	  }
	   wid := strings.TrimSpace(withdrawID)
	  out := make([]DeltaProof,
		  0, limit)
	  for _, p := range m.proofs {
		  if p.WithdrawID == wid {
			  out = append(out, *p)
			  if len(out) >= limit {
				   break
			   }
		   }
	  }
	  return out, nil
  }

  func (m *MemStore) ListProofsByLease(_ context.Context, leaseID string, limit int) ([]DeltaProof, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  if limit <= 0 {
		   limit = 100
	  }
	   lid := strings.TrimSpace(leaseID)
	   out := make([]DeltaProof, 0, limit)
	   for _, p := range m.proofs {
		  if p.LeaseID == lid {
			   out = append(out, *p)
			   if len(out) >= limit {
				  break
			  }
		  }
	   }
	  return out, nil
   }

  func (m *MemStore) SaveCase(_ context.Context, rc ReserveCase) (ReserveCase, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  cp := rc.Copy()
	   m.cases[cp.ID] = &cp
	   return cp, nil
  }

  func (m *MemStore) GetCase(_ context.Context, id string) (ReserveCase, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  c, ok := m.cases[strings.TrimSpace(id)]
	   if !ok {
		   return ReserveCase{}, ErrCaseNotFound
	   }
	  return c.Copy(), nil
   }

  func (m *MemStore) ListCasesByState(_ context.Context, state CaseState, limit int) ([]ReserveCase, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  if limit <= 0 {
		   limit = 100
	  }
	  out := make([]ReserveCase, 0, limit)
	  for _, c := range m.cases {
		  if c.State == state {
			   out = append(out, c.Copy())
			  if len(out) >= limit {
				  break
			  }
		   }
	  }
	   return out, nil
  }

   func (m *MemStore) ListCasesByWithdraw(_ context.Context, withdrawID string, limit int) ([]ReserveCase, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  if limit <= 0 {
		  limit = 100
	   }
	   wid := strings.TrimSpace(withdrawID)
	   out := make([]ReserveCase, 0, limit)
	  for _, c := range m.cases {
		  if c.WithdrawID == wid {
			  out = append(out, c.Copy())
			   if len(out) >= limit {
				  break
			   }
		  }
	  }
	   return out, nil
  }

// ──────────────────────────────────────────
// KVStore 基于 persist.KV 的实现
// ──────────────────────────────────────────

const (
	kvKindLease = "fr_lease"
	kvKindBind  = "fr_bind"
	kvKindProof = "fr_proof"
	kvKindCase = "fr_case"
)

type KVStore struct {
	kv persist.KV
}

   func NewKVStore(kv persist.KV) *KVStore {
	   return &KVStore{kv: kv}
   }

   func (s *KVStore) SaveLease(ctx context.Context, l ReserveLease) (ReserveLease, error) {
	  body, err := json.Marshal(l)
	   if err != nil {
		   return ReserveLease{}, err
	   }
	   row, existed, err := persist.GetOrInit(ctx, s.kv, kvKindLease, l.ID, body)
	  if err != nil {
		   return ReserveLease{}, err
	  }
	  if existed {
		   if err := persist.Replace(ctx, s.kv, kvKindLease, l.ID, body); err != nil {
			   if errors.Is(err, persist.ErrStaleFence) {
				   return ReserveLease{}, ErrFence
			   }
			  return ReserveLease{}, err
		  }
		   row, err = s.kv.Get(ctx, kvKindLease, l.ID)
		  if err != nil {
			   return ReserveLease{}, err
		  }
	   }
	   var out ReserveLease
	  if err := json.Unmarshal(row.Body, &out); err != nil {
		   return ReserveLease{}, err
	   }
	  return out, nil
  }

   func (s *KVStore) GetLease(ctx context.Context, id string) (ReserveLease, error) {
	  row, err := s.kv.Get(ctx, kvKindLease, strings.TrimSpace(id))
	  if err != nil {
		   if errors.Is(err, persist.ErrNotFound) {
			  return ReserveLease{}, ErrLeaseNotFound
		   }
		  return ReserveLease{}, err
	   }
	  var out ReserveLease
	  if err := json.Unmarshal(row.Body, &out); err != nil {
		   return ReserveLease{}, err
	  }
	  return out, nil
   }

  func (s *KVStore) GetLeaseByWithdraw(ctx context.Context, withdrawID string) (ReserveLease, error) {
	  row, err := s.kv.Get(ctx, kvKindLease, "wd:"+strings.TrimSpace(withdrawID))
	   if err != nil {
		  if errors.Is(err, persist.ErrNotFound) {
			  return ReserveLease{}, ErrLeaseNotFound
		   }
		   return ReserveLease{}, err
	   }
	   var out ReserveLease
	  if err := json.Unmarshal(row.Body, &out); err != nil {
		  return ReserveLease{}, err
	  }
	   return out, nil
   }

   func (s *KVStore) ListLeasesByState(ctx context.Context, state LeaseState, limit int) ([]ReserveLease, error) {
	  if limit <= 0 {
		  limit = 100
	  }
	   rows, err := s.kv.List(ctx, kvKindLease, 500)
	  if err != nil {
		  return nil, err
	  }
	   out := make([]ReserveLease, 0, limit)
	   for _, r := range rows {
		   var l ReserveLease
		   if err := json.Unmarshal(r.Body, &l); err != nil {
			  continue
		  }
		   if l.State == state {
			  out = append(out, l)
			   if len(out) >= limit {
				  break
			  }
		  }
	   }
	  return out, nil
  }

  func (s *KVStore) ListExpiredLeases(ctx context.Context, before time.Time, limit int) ([]ReserveLease, error) {
	  if limit <= 0 {
		  limit = 100
	   }
	   rows, err := s.kv.List(ctx, kvKindLease, 500)
	   if err != nil {
		  return nil, err
	   }
	  out := make([]ReserveLease, 0, limit)
	   for _, r := range rows {
		   var l ReserveLease
		   if err := json.Unmarshal(r.Body, &l); err != nil {
			   continue
		   }
		   if l.State == LeaseReserved && l.LeaseExpiry.Before(before) {
			   out = append(out, l)
			   if len(out) >= limit {
				   break
			  }
		  }
	  }
	   return out, nil
  }

  func (s *KVStore) SaveTerminalBind(ctx context.Context, tb TerminalBind) error {
	  body, err := json.Marshal(tb)
	   if err != nil {
		   return err
	  }
	   return persist.PutNew(ctx, s.kv, persist.Row{Kind: kvKindBind, Key: tb.WithdrawID, Body: body})
  }

  func (s *KVStore) GetTerminalBind(ctx context.Context, withdrawID string) (TerminalBind, bool, error) {
	  row, err := s.kv.Get(ctx, kvKindBind, strings.TrimSpace(withdrawID))
	   if err != nil {
		   if errors.Is(err, persist.ErrNotFound) {
			  return TerminalBind{}, false, nil
		  }
		   return TerminalBind{}, false, err
	  }
	   var out TerminalBind
	   if err := json.Unmarshal(row.Body, &out); err != nil {
		  return TerminalBind{}, false, err
	   }
	   return out, true, nil
  }

   func (s *KVStore) SaveProof(ctx context.Context, dp DeltaProof) (DeltaProof, error) {
	  body, err := json.Marshal(dp)
	  if err != nil {
		   return DeltaProof{}, err
	  }
	  _, _, err = persist.GetOrInit(ctx, s.kv, kvKindProof, dp.ID, body)
	  if err != nil {
		  return DeltaProof{}, err
	  }
	  return dp, nil
   }

  func (s *KVStore) GetProof(ctx context.Context, id string) (DeltaProof, error) {
	   row,
		  err := s.kv.Get(ctx, kvKindProof, strings.TrimSpace(id))
	   if err != nil {
		   if errors.Is(err, persist.ErrNotFound) {
			  return DeltaProof{}, ErrDeltaProof
		   }
		   return DeltaProof{}, err
	  }
	   var out DeltaProof
	   if err := json.Unmarshal(row.Body, &out); err != nil {
		  return DeltaProof{}, err
	  }
	   return out, nil
   }

   func (s *KVStore) ListProofsByWithdraw(ctx context.Context, withdrawID string, limit int) ([]DeltaProof, error) {
	   if limit <= 0 {
		   limit = 100
	  }
	  rows,
		  err := s.kv.List(ctx, kvKindProof, 500)
	  if err != nil {
		  return nil, err
	  }
	   wid := strings.TrimSpace(withdrawID)
	  out := make([]DeltaProof, 0, limit)
	  for _, r := range rows {
		   var p DeltaProof
		  if err := json.Unmarshal(r.Body, &p); err != nil {
			   continue
		  }
		   if p.WithdrawID == wid {
			  out = append(out, p)
			   if len(out) >= limit {
				   break
			  }
		  }
	  }
	   return out, nil
  }

   func (s *KVStore) ListProofsByLease(ctx context.Context, leaseID string, limit int) ([]DeltaProof, error) {
	   if limit <= 0 {
		   limit = 100
	   }
	   rows, err := s.kv.List(ctx, kvKindProof, 500)
	  if err != nil {
		   return nil, err
	  }
	   lid := strings.TrimSpace(leaseID)
	   out := make([]DeltaProof,
		  0, limit)
	  for _, r := range rows {
		  var p DeltaProof
		  if err := json.Unmarshal(r.Body, &p); err != nil {
			  continue
		   }
		  if p.LeaseID == lid {
			   out = append(out, p)
			   if len(out) >= limit {
				  break
			  }
		   }
	  }
	   return out, nil
   }

   func (s *KVStore) SaveCase(ctx context.Context, rc ReserveCase) (ReserveCase, error) {
	   body, err := json.Marshal(rc)
	  if err != nil {
		  return ReserveCase{}, err
	  }
	  row, existed, err := persist.GetOrInit(ctx, s.kv, kvKindCase, rc.ID, body)
	  if err != nil {
		   return ReserveCase{}, err
	  }
	   if existed {
		   if err := persist.Replace(ctx, s.kv, kvKindCase, rc.ID, body); err != nil {
			   if errors.Is(err, persist.ErrStaleFence) {
				  return ReserveCase{}, ErrFence
			   }
			   return ReserveCase{}, err
		  }
		   row, err = s.kv.Get(ctx, kvKindCase, rc.ID)
		   if err != nil {
			  return ReserveCase{}, err
		   }
	   }
	  var out ReserveCase
	  if err := json.Unmarshal(row.Body, &out); err != nil {
		   return ReserveCase{}, err
	  }
	   return out, nil
   }

  func (s *KVStore) GetCase(ctx context.Context, id string) (ReserveCase, error) {
	  row, err := s.kv.Get(ctx, kvKindCase, strings.TrimSpace(id))
	   if err != nil {
		  if errors.Is(err, persist.ErrNotFound) {
			  return ReserveCase{}, ErrCaseNotFound
		   }
		   return ReserveCase{}, err
	  }
	   var out ReserveCase
	   if err := json.Unmarshal(row.Body, &out); err != nil {
		   return ReserveCase{}, err
	   }
	   return out, nil
   }

   func (s *KVStore) ListCasesByState(ctx context.Context, state CaseState, limit int) ([]ReserveCase, error) {
	   if limit <= 0 {
		   limit = 100
	   }
	   rows, err := s.kv.List(ctx, kvKindCase, 500)
	   if err != nil {
		   return nil, err
	   }
	  out := make([]ReserveCase,
		   0, limit)
	   for _, r := range rows {
		   var c ReserveCase
		  if err := json.Unmarshal(r.Body, &c); err != nil {
			  continue
		   }
		  if c.State == state {
			   out = append(out, c)
			   if len(out) >= limit {
				   break
			  }
		  }
	  }
	   return out, nil
   }

  func (s *KVStore) ListCasesByWithdraw(ctx context.Context, withdrawID string, limit int) ([]ReserveCase, error) {
	  if limit <= 0 {
		  limit = 100
	   }
	  rows,
		   err := s.kv.List(ctx, kvKindCase, 500)
	   if err != nil {
		   return nil, err
	   }
	   wid := strings.TrimSpace(withdrawID)
	   out := make([]ReserveCase, 0, limit)
	  for _, r := range rows {
		  var c ReserveCase
		   if err := json.Unmarshal(r.Body, &c); err != nil {
			  continue
		   }
		  if c.WithdrawID == wid {
			  out = append(out, c)
			   if len(out) >= limit {
				   break
			   }
		   }
	   }
	  return out, nil
   }
