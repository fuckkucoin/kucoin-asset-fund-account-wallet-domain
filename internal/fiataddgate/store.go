package fiataddgate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
)

// ErrLeaseConflict 同 Session 已有活跃租约（confirming 中），禁止重复签发。
var ErrLeaseConflict = errors.New("fiataddgate: confirm lease conflict")

// ErrEffectDup 同 session_id+confirm_gen 已存在贷记效果，幂等回放信号。

var ErrEffectDup = errors.New("fiataddgate: credit effect duplicate")

// ErrForkBlocked 该 Session 存在未关闭分叉案例，禁止 Confirm。
var ErrForkBlocked = errors.New("fiataddgate: session blocked by open fork case")

// ErrNotFound 记录不存在。
var ErrNotFound = errors.New("fiataddgate: not found")

// LeaseStore 确认租约存储接口。
type LeaseStore interface {
	// Insert 原子写入租约。若同 SessionID 已有 state=issued 的租约则返回 ErrLeaseConflict。
	Insert(ctx context.Context, l ConfirmLease) error
	// GetBySessionGen 按 session_id + confirm_gen 读取唯一租约。
	GetBySessionGen(ctx context.Context, sessionID string, gen int64) (ConfirmLease, error)
	// GetByID 按 lease_id 读取。
	GetByID(ctx context.Context, leaseID string) (ConfirmLease, error)
	// GetActiveBySession 返回同 Session 中 state=issued 的活跃租约（若无则 ErrNotFound）。
	GetActiveBySession(ctx context.Context, sessionID string) (ConfirmLease, error)
	// Transition CAS 状态迁移。oldState→newState；若当前 state 不符则 ErrLeaseConflict。
	Transition(ctx context.Context, leaseID string, from, to LeaseState, updates ConfirmLease) error
	// ListExpired 返回已过期（expires_at<now）且仍为 issued 状态的租约，供 recycle worker。
	ListExpired(ctx context.Context, now time.Time, limit int) ([]ConfirmLease, error)
	// ListBySession 返回某 Session 的所有历史租约，按签发时间升序。
	ListBySession(ctx context.Context, sessionID string) ([]ConfirmLease, error)
}

// EffectStore 贷记效果存储接口。
type EffectStore interface {
	// Insert 写入不可变贷记证明。(session_id, confirm_gen) 唯一冲突返回 ErrEffectDup。
	Insert(ctx context.Context, e CreditEffect) error
	// GetBySessionGen 读取唯一贷记证明。
	GetBySessionGen(ctx context.Context,
		sessionID string, gen int64) (CreditEffect, error)
	// ListBySession 返回某 Session 的所有贷记证明，按时间升序。
	ListBySession(ctx context.Context, sessionID string) ([]CreditEffect, error)
}

// ForkCaseStore 分叉案例存储接口。
type ForkCaseStore interface {
	// Open 写入新的分叉案例。若同 session_id 已有 state=open 的案例则追加 Note 并返回已有。
	Open(ctx context.Context, fc FiatAddForkCase) (FiatAddForkCase, error)
	// GetOpenCase 返回同 Session 最新的 open 案例；无则 ErrNotFound。
	GetOpenCase(ctx context.Context, sessionID string) (FiatAddForkCase, error)
	// Close 四眼收口：state=open→closed。须提供 closedBy 与 resolution。
	Close(ctx context.Context, caseID, closedBy, resolution string) (FiatAddForkCase, error)
	// GetByID 按 case_id 读取。
	GetByID(ctx context.Context, caseID string) (FiatAddForkCase, error)
	// ListOpen 返回所有 state=open 的案例，供巡检 worker，按 opened_at 升序。
	ListOpen(ctx context.Context, limit int) ([]FiatAddForkCase, error)
	// ListBySession 返回某 Session 所有案例，按 opened_at 升序。
	ListBySession(ctx context.Context, sessionID string) ([]FiatAddForkCase, error)
}

// ─────────────────────────────────────────────────────────────────────────────
// 内存实现（单元与内存模式进程使用）
// ─────────────────────────────────────────────────────────────────────────────

const (
	kindLease  = "aa_fiat_confirm_lease"
	kindEffect = "aa_fiat_credit_effect"
	kindFork   = "aa_fiat_fork_case"
)

// MemLeaseStore 基于 persist.KV 的内存租约存储。

type MemLeaseStore struct {
	mu    sync.Mutex
	items map[string]ConfirmLease // leaseID → lease
}

   func NewMemLeaseStore() *MemLeaseStore {
	  return &MemLeaseStore{items: map[string]ConfirmLease{}}
   }

   func (s *MemLeaseStore) Insert(ctx context.Context, l ConfirmLease) error {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  for _, v := range s.items {
		   if v.SessionID == l.SessionID && v.State == LeaseIssued {
			  return ErrLeaseConflict
		  }
	   }
	  s.items[l.LeaseID] = l
	   return nil
  }

   func (s *MemLeaseStore) GetBySessionGen(ctx context.Context, sessionID string, gen int64) (ConfirmLease, error) {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   for _, v := range s.items {
		  if v.SessionID == sessionID && v.ConfirmGen == gen {
			  return v, nil
		   }
	   }
	  return ConfirmLease{}, ErrNotFound
   }

   func (s *MemLeaseStore) GetByID(ctx context.Context, leaseID string) (ConfirmLease, error) {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   v, ok := s.items[leaseID]
	   if !ok {
		  return ConfirmLease{}, ErrNotFound
	   }
	  return v, nil
  }

   func (s *MemLeaseStore) GetActiveBySession(ctx context.Context, sessionID string) (ConfirmLease, error) {
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  for _, v := range s.items {
		  if v.SessionID == sessionID && v.State == LeaseIssued {
			   return v, nil
		   }
	   }
	  return ConfirmLease{}, ErrNotFound
   }

  func (s *MemLeaseStore) Transition(ctx context.Context, leaseID string, from, to LeaseState, updates ConfirmLease) error {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  v, ok := s.items[leaseID]
	   if !ok {
		   return ErrNotFound
	  }
	  if v.State != from {
		   return ErrLeaseConflict
	   }
	   v.State = to
	   if updates.CreditID != "" {
		  v.CreditID = updates.CreditID
	  }
	  if updates.CreditedAt != nil {
		  v.CreditedAt = updates.CreditedAt
	  }
	  if updates.ForkedAt != nil {
		   v.ForkedAt = updates.ForkedAt
	  }
	  if updates.ExpiredAt != nil {
		   v.ExpiredAt = updates.ExpiredAt
	   }
	  if updates.Note != "" {
		   v.Note = updates.Note
	  }
	   s.items[leaseID] = v
	   return nil
  }

   func (s *MemLeaseStore) ListExpired(ctx context.Context, now time.Time, limit int) ([]ConfirmLease, error) {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   var out []ConfirmLease
	  for _, v := range s.items {
		   if v.State == LeaseIssued && v.ExpiresAt.Before(now) {
			  out = append(out, v)
		  }
	   }
	   sort.Slice(out,
		  func(i, j int) bool { return out[i].IssuedAt.Before(out[j].IssuedAt) })
	   if limit > 0 && len(out) > limit {
		   out = out[:limit]
	  }
	   return out, nil
   }

  func (s *MemLeaseStore) ListBySession(ctx context.Context, sessionID string) ([]ConfirmLease, error) {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   var out []ConfirmLease
	   for _, v := range s.items {
		   if v.SessionID == sessionID {
			  out = append(out, v)
		  }
	   }
	  sort.Slice(out, func(i, j int) bool { return out[i].IssuedAt.Before(out[j].IssuedAt) })
	   return out, nil
   }

// MemEffectStore 基于内存的贷记效果存储。
type MemEffectStore struct {
	mu sync.Mutex
	items map[string]CreditEffect // effectID → effect
}

   func NewMemEffectStore() *MemEffectStore {
	  return &MemEffectStore{items: map[string]CreditEffect{}}
  }

  func (s *MemEffectStore) Insert(ctx context.Context, e CreditEffect) error {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   for _, v := range s.items {
		  if v.SessionID == e.SessionID && v.ConfirmGen == e.ConfirmGen {
			  return ErrEffectDup
		  }
	   }
	   s.items[e.EffectID] = e
	  return nil
  }

  func (s *MemEffectStore) GetBySessionGen(ctx context.Context, sessionID string, gen int64) (CreditEffect, error) {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  for _, v := range s.items {
		  if v.SessionID == sessionID && v.ConfirmGen == gen {
			  return v, nil
		   }
	   }
	  return CreditEffect{}, ErrNotFound
   }

   func (s *MemEffectStore) ListBySession(ctx context.Context, sessionID string) ([]CreditEffect, error) {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  var out []CreditEffect
	   for _, v := range s.items {
		   if v.SessionID == sessionID {
			   out = append(out, v)
		  }
	   }
	   sort.Slice(out,
		  func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	   return out, nil
   }

// MemForkCaseStore 基于内存的分叉案例存储。
type MemForkCaseStore struct {
	mu sync.Mutex
	items map[string]FiatAddForkCase
}

  func NewMemForkCaseStore() *MemForkCaseStore {
	  return &MemForkCaseStore{items: map[string]FiatAddForkCase{}}
   }

  func (s *MemForkCaseStore) Open(ctx context.Context, fc FiatAddForkCase) (FiatAddForkCase, error) {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  for _, v := range s.items {
		   if v.SessionID == fc.SessionID && v.State == ForkCaseOpen {
			  return v, nil
		   }
	  }
	   s.items[fc.CaseID] = fc
	  return fc, nil
  }

  func (s *MemForkCaseStore) GetOpenCase(ctx context.Context, sessionID string) (FiatAddForkCase, error) {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  for _, v := range s.items {
		  if v.SessionID == sessionID && v.State == ForkCaseOpen {
			   return v, nil
		  }
	   }
	   return FiatAddForkCase{}, ErrNotFound
  }

   func (s *MemForkCaseStore) Close(ctx context.Context, caseID, closedBy, resolution string) (FiatAddForkCase, error) {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   v, ok := s.items[caseID]
	  if !ok {
		  return FiatAddForkCase{}, ErrNotFound
	  }
	  if v.State == ForkCaseClosed {
		  return v, nil
	  }
	  now := time.Now().UTC()
	  v.State = ForkCaseClosed
	   v.ClosedAt = &now
	  v.ClosedBy = closedBy
	   v.Resolution = resolution
	   s.items[caseID] = v
	   return v, nil
  }

  func (s *MemForkCaseStore) GetByID(ctx context.Context, caseID string) (FiatAddForkCase, error) {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   v, ok := s.items[caseID]
	   if !ok {
		  return FiatAddForkCase{}, ErrNotFound
	  }
	   return v, nil
  }

  func (s *MemForkCaseStore) ListOpen(ctx context.Context, limit int) ([]FiatAddForkCase, error) {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  var out []FiatAddForkCase
	   for _, v := range s.items {
		  if v.State == ForkCaseOpen {
			   out = append(out, v)
		  }
	   }
	   sort.Slice(out, func(i, j int) bool { return out[i].OpenedAt.Before(out[j].OpenedAt) })
	   if limit > 0 && len(out) > limit {
		  out = out[:limit]
	  }
	   return out, nil
   }

  func (s *MemForkCaseStore) ListBySession(ctx context.Context, sessionID string) ([]FiatAddForkCase, error) {
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  var out []FiatAddForkCase
	  for _, v := range s.items {
		   if v.SessionID == sessionID {
			  out = append(out, v)
		  }
	   }
	  sort.Slice(out, func(i, j int) bool { return out[i].OpenedAt.Before(out[j].OpenedAt) })
	  return out, nil
   }

// ─────────────────────────────────────────────────────────────────────────────
// KV 后端实现（用于环境 KV 存储）
// ─────────────────────────────────────────────────────────────────────────────
// KVLeaseStore 基于 persist.KV 的租约存储。
type KVLeaseStore struct {
	kv persist.KV
}

  func NewKVLeaseStore(kv persist.KV) *KVLeaseStore { return &KVLeaseStore{kv: kv} }

   func (s *KVLeaseStore) Insert(ctx context.Context, l ConfirmLease) error {
	   body, err := json.Marshal(l)
	   if err != nil {
		   return fmt.Errorf("fiataddgate lease marshal: %w", err)
	   }
	   return s.kv.InTx(ctx, func(ctx context.Context) error {
		   rows, err := s.kv.List(ctx, kindLease, 200)
		  if err != nil {
			   return err
		  }
		  for _, row := range rows {
			  var v ConfirmLease
			   if json.Unmarshal(row.Body, &v) != nil {
				  continue
			   }
			   if v.SessionID == l.SessionID && v.State == LeaseIssued {
				  return ErrLeaseConflict
			  }
		   }
		   return s.kv.Put(ctx, persist.Row{Kind: kindLease, Key: l.LeaseID, Body: body}, persist.Fence{})
	   })
  }

   func (s *KVLeaseStore) GetBySessionGen(ctx context.Context, sessionID string, gen int64) (ConfirmLease, error) {
	   rows, err := s.kv.List(ctx, kindLease, 500)
	   if err != nil {
		   return ConfirmLease{}, err
	   }
	   for _, row := range rows {
		   var v ConfirmLease
		  if json.Unmarshal(row.Body, &v) != nil {
			  continue
		   }
		  if v.SessionID == sessionID && v.ConfirmGen == gen {
			   return v, nil
		  }
	  }
	  return ConfirmLease{}, ErrNotFound
  }

   func (s *KVLeaseStore) GetByID(ctx context.Context, leaseID string) (ConfirmLease, error) {
	  row,
		   err := s.kv.Get(ctx, kindLease, leaseID)
	   if err != nil {
		  if errors.Is(err, persist.ErrNotFound) {
			  return ConfirmLease{}, ErrNotFound
		   }
		  return ConfirmLease{}, err
	  }
	  var v ConfirmLease
	  if json.Unmarshal(row.Body, &v) != nil || v.LeaseID == "" {
		   return ConfirmLease{}, ErrNotFound
	   }
	   return v, nil
   }

   func (s *KVLeaseStore) GetActiveBySession(ctx context.Context, sessionID string) (ConfirmLease, error) {
	  rows, err := s.kv.List(ctx, kindLease, 500)
	  if err != nil {
		  return ConfirmLease{}, err
	  }
	   for _, row := range rows {
		  var v ConfirmLease
		   if json.Unmarshal(row.Body, &v) != nil {
			   continue
		  }
		  if v.SessionID == sessionID && v.State == LeaseIssued {
			  return v, nil
		   }
	  }
	   return ConfirmLease{},
		  ErrNotFound
   }

  func (s *KVLeaseStore) Transition(ctx context.Context, leaseID string, from, to LeaseState, updates ConfirmLease) error {
	   return s.kv.InTx(ctx, func(ctx context.Context) error {
		  row, err := s.kv.Get(ctx, kindLease, leaseID)
		   if err != nil {
			   if errors.Is(err, persist.ErrNotFound) {
				   return ErrNotFound
			   }
			   return err
		   }
		  var v ConfirmLease
		  if json.Unmarshal(row.Body, &v) != nil {
			  return ErrNotFound
		   }
		   if v.State != from {
			  return ErrLeaseConflict
		  }
		  v.State = to
		   if updates.CreditID != "" {
			  v.CreditID = updates.CreditID
		   }
		  if updates.CreditedAt != nil {
			  v.CreditedAt = updates.CreditedAt
		  }
		  if updates.ForkedAt != nil {
			   v.ForkedAt = updates.ForkedAt
		  }
		  if updates.ExpiredAt != nil {
			  v.ExpiredAt = updates.ExpiredAt
		   }
		   if updates.Note != "" {
			  v.Note = updates.Note
		   }
		  body, err := json.Marshal(v)
		  if err != nil {
			   return err
		  }
		   return s.kv.Put(ctx,
			   persist.Row{Kind: kindLease, Key: leaseID, Body: body}, row.Fence)
	   })
  }

  func (s *KVLeaseStore) ListExpired(ctx context.Context, now time.Time, limit int) ([]ConfirmLease, error) {
	   rows, err := s.kv.List(ctx, kindLease, limit*4)
	   if err != nil {
		   return nil, err
	   }
	   var out []ConfirmLease
	  for _, row := range rows {
		  var v ConfirmLease
		  if json.Unmarshal(row.Body, &v) != nil {
			   continue
		  }
		   if v.State == LeaseIssued && v.ExpiresAt.Before(now) {
			  out = append(out, v)
		   }
	  }
	   sort.Slice(out, func(i, j int) bool { return out[i].IssuedAt.Before(out[j].IssuedAt) })
	   if limit > 0 && len(out) > limit {
		  out = out[:limit]
	   }
	   return out, nil
  }

   func (s *KVLeaseStore) ListBySession(ctx context.Context, sessionID string) ([]ConfirmLease, error) {
	  rows,
		  err := s.kv.List(ctx, kindLease, 500)
	   if err != nil {
		   return nil, err
	   }
	  var out []ConfirmLease
	  for _, row := range rows {
		  var v ConfirmLease
		   if json.Unmarshal(row.Body, &v) != nil {
			   continue
		  }
		  if v.SessionID == sessionID {
			  out = append(out, v)
		  }
	  }
	   sort.Slice(out, func(i, j int) bool { return out[i].IssuedAt.Before(out[j].IssuedAt) })
	  return out, nil
  }

// KVEffectStore 基于 persist.KV 的贷记效果存储。
type KVEffectStore struct {
	kv persist.KV
}

   func NewKVEffectStore(kv persist.KV) *KVEffectStore { return &KVEffectStore{kv: kv} }

   func (s *KVEffectStore) Insert(ctx context.Context, e CreditEffect) error {
	  return s.kv.InTx(ctx, func(ctx context.Context) error {
		  rows, err := s.kv.List(ctx, kindEffect, 200)
		   if err != nil {
			   return err
		   }
		  for _, row := range rows {
			   var v CreditEffect
			   if json.Unmarshal(row.Body, &v) != nil {
				   continue
			  }
			  if v.SessionID == e.SessionID && v.ConfirmGen == e.ConfirmGen {
				  return ErrEffectDup
			   }
		  }
		   body, err := json.Marshal(e)
		  if err != nil {
			   return err
		   }
		   return s.kv.Put(ctx, persist.Row{Kind: kindEffect, Key: e.EffectID, Body: body}, persist.Fence{})
	  })
   }

  func (s *KVEffectStore) GetBySessionGen(ctx context.Context, sessionID string, gen int64) (CreditEffect, error) {
	   rows, err := s.kv.List(ctx, kindEffect, 500)
	  if err != nil {
		   return CreditEffect{}, err
	  }
	   for _, row := range rows {
		   var v CreditEffect
		  if json.Unmarshal(row.Body, &v) != nil {
			  continue
		   }
		  if v.SessionID == sessionID && v.ConfirmGen == gen {
			  return v, nil
		  }
	  }
	   return CreditEffect{}, ErrNotFound
   }

  func (s *KVEffectStore) ListBySession(ctx context.Context, sessionID string) ([]CreditEffect, error) {
	  rows, err := s.kv.List(ctx, kindEffect, 500)
	  if err != nil {
		   return nil, err
	   }
	  var out []CreditEffect
	  for _, row := range rows {
		   var v CreditEffect
		   if json.Unmarshal(row.Body, &v) != nil {
			   continue
		   }
		  if v.SessionID == sessionID {
			  out = append(out, v)
		   }
	   }
	  sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	  return out, nil
  }

// KVForkCaseStore 基于 persist.KV 的分叉案例存储。
type KVForkCaseStore struct {
	kv persist.KV
}

   func NewKVForkCaseStore(kv persist.KV) *KVForkCaseStore { return &KVForkCaseStore{kv: kv} }

   func (s *KVForkCaseStore) Open(ctx context.Context, fc FiatAddForkCase) (FiatAddForkCase, error) {
	   var out FiatAddForkCase
	  err := s.kv.InTx(ctx, func(ctx context.Context) error {
		  rows, err := s.kv.List(ctx, kindFork, 200)
		  if err != nil {
			   return err
		  }
		  for _, row := range rows {
			   var v FiatAddForkCase
			   if json.Unmarshal(row.Body, &v) != nil {
				  continue
			  }
			  if v.SessionID == fc.SessionID && v.State == ForkCaseOpen {
				   out = v
				   return nil
			  }
		   }
		  body, err := json.Marshal(fc)
		  if err != nil {
			   return err
		   }
		  if err := s.kv.Put(ctx, persist.Row{Kind: kindFork, Key: fc.CaseID, Body: body}, persist.Fence{}); err != nil {
			   return err
		  }
		   out = fc
		   return nil
	   })
	  return out, err
   }

  func (s *KVForkCaseStore) GetOpenCase(ctx context.Context, sessionID string) (FiatAddForkCase, error) {
	  rows, err := s.kv.List(ctx, kindFork, 500)
	  if err != nil {
		   return FiatAddForkCase{}, err
	   }
	   for _, row := range rows {
		  var v FiatAddForkCase
		   if json.Unmarshal(row.Body, &v) != nil {
			   continue
		  }
		   if v.SessionID == sessionID && v.State == ForkCaseOpen {
			  return v, nil
		   }
	   }
	  return FiatAddForkCase{}, ErrNotFound
  }

   func (s *KVForkCaseStore) Close(ctx context.Context, caseID, closedBy, resolution string) (FiatAddForkCase, error) {
	   var out FiatAddForkCase
	   err := s.kv.InTx(ctx, func(ctx context.Context) error {
		  row, err := s.kv.Get(ctx, kindFork, caseID)
		   if err != nil {
			  if errors.Is(err, persist.ErrNotFound) {
				  return ErrNotFound
			  }
			  return err
		   }
		  var v FiatAddForkCase
		  if json.Unmarshal(row.Body, &v) != nil {
			  return ErrNotFound
		  }
		   if v.State == ForkCaseClosed {
			   out = v
			   return nil
		   }
		   now := time.Now().UTC()
		   v.State = ForkCaseClosed
		   v.ClosedAt = &now
		   v.ClosedBy = strings.TrimSpace(closedBy)
		  v.Resolution = strings.TrimSpace(resolution)
		   body, err := json.Marshal(v)
		  if err != nil {
			   return err
		  }
		   if err := s.kv.Put(ctx, persist.Row{Kind: kindFork, Key: caseID, Body: body}, row.Fence); err != nil {
			  return err
		   }
		   out = v
		  return nil
	   })
	  return out, err
   }

  func (s *KVForkCaseStore) GetByID(ctx context.Context, caseID string) (FiatAddForkCase, error) {
	   row,
		   err := s.kv.Get(ctx, kindFork, caseID)
	  if err != nil {
		   if errors.Is(err, persist.ErrNotFound) {
			   return FiatAddForkCase{},
				  ErrNotFound
		  }
		  return FiatAddForkCase{}, err
	  }
	  var v FiatAddForkCase
	   if json.Unmarshal(row.Body, &v) != nil || v.CaseID == "" {
		  return FiatAddForkCase{}, ErrNotFound
	   }
	   return v, nil
  }

  func (s *KVForkCaseStore) ListOpen(ctx context.Context, limit int) ([]FiatAddForkCase, error) {
	  rows, err := s.kv.List(ctx, kindFork, limit*4)
	  if err != nil {
		  return nil, err
	   }
	  var out []FiatAddForkCase
	   for _, row := range rows {
		  var v FiatAddForkCase
		  if json.Unmarshal(row.Body, &v) != nil {
			  continue
		  }
		  if v.State == ForkCaseOpen {
			  out = append(out, v)
		   }
	   }
	  sort.Slice(out, func(i, j int) bool { return out[i].OpenedAt.Before(out[j].OpenedAt) })
	   if limit > 0 && len(out) > limit {
		   out = out[:limit]
	  }
	  return out, nil
  }

   func (s *KVForkCaseStore) ListBySession(ctx context.Context, sessionID string) ([]FiatAddForkCase, error) {
	   rows, err := s.kv.List(ctx, kindFork, 500)
	   if err != nil {
		   return nil, err
	  }
	  var out []FiatAddForkCase
	  for _, row := range rows {
		  var v FiatAddForkCase
		   if json.Unmarshal(row.Body, &v) != nil {
			   continue
		  }
		  if v.SessionID == sessionID {
			   out = append(out, v)
		  }
	  }
	  sort.Slice(out, func(i, j int) bool { return out[i].OpenedAt.Before(out[j].OpenedAt) })
	  return out, nil
   }

// newLeaseID 生成唯一租约 ID。
   func newLeaseID() string { return "cl-" + uuid.NewString() }

// newEffectID 生成唯一效果 ID。
   func newEffectID() string { return "ce-" + uuid.NewString() }

// newCaseID 生成唯一案例 ID。

   func newCaseID() string { return "fc-" + uuid.NewString() }
