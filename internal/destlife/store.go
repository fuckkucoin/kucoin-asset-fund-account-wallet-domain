package destlife

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
	"github.com/google/uuid"
)

// ─────────────────────────────────────────────────────────────────────────────
// GenLeaseStore — 在途世代租约存储
// ─────────────────────────────────────────────────────────────────────────────
// GenLeaseStore 在途世代租约存储接口。

type GenLeaseStore interface {
	// Insert 原子写入租约。(entry_id, movement_id) 冲突返回已有租约与 true（幂等信号）。
	Insert(ctx context.Context, l InflightGenLease) (InflightGenLease, bool, error)
	// GetByID 按 lease_id 读取。
	GetByID(ctx context.Context, leaseID string) (InflightGenLease, error)
	// GetByMovement 按 (entry_id, movement_id) 读取。
	GetByMovement(ctx context.Context, entryID, movementID string) (InflightGenLease, error)
	// ListActiveByEntry 返回 entry_id 下 state=acquired|bound 的活跃租约。
	ListActiveByEntry(ctx context.Context, entryID string) ([]InflightGenLease, error)
	// CountActiveByEntry 返回 entry_id 下活跃引用计数（state=acquired|bound）。
	CountActiveByEntry(ctx context.Context, entryID string) (int, error)
	// Transition CAS 状态迁移。
	Transition(ctx context.Context, leaseID string, from, to RefState, updates InflightGenLease) error
	// ListExpired 返回 expires_at<now 且 state 非 released 的泄漏引用，供泄漏扫描 worker。
	ListExpired(ctx context.Context,
		now time.Time, limit int) ([]InflightGenLease, error)
	// ListByEntry 返回 entry_id 下所有租约（含历史），按签发时间升序。
	ListByEntry(ctx context.Context, entryID string) ([]InflightGenLease, error)
}

// ─────────────────────────────────────────────────────────────────────────────
// RevokeGenStore — 吊销世代存储
// ─────────────────────────────────────────────────────────────────────────────
// RevokeGenStore 吊销世代记录存储接口。

type RevokeGenStore interface {
	// GetOrInit 按 entry_id 获取（不存在则以 active/gen=0 初始化）。
	GetOrInit(ctx context.Context, entryID, userID, chain, address string) (RevokeGeneration, error)
	// Get 按 entry_id 读取；不存在返回 ErrEntryNotFound。
	Get(ctx context.Context, entryID string) (RevokeGeneration, error)
	// BumpGen 原子抬升 Gen 并设置 State。须提供期望的 currentGen 作乐观锁。
	BumpGen(ctx context.Context, entryID string, currentGen int64, newState EntryState, actor string) (RevokeGeneration, error)
	// UpdateActiveRefs 原子更新 active_refs 计数（±delta）。
	UpdateActiveRefs(ctx context.Context, entryID string, delta int) (RevokeGeneration, error)
	// SetRevoked 标记 state=revoked，设置 revoked_at。
	SetRevoked(ctx context.Context, entryID string, actor string) (RevokeGeneration, error)
	// ListRevoking 返回 state=revoking 的所有记录，供完成吊销 worker。
	ListRevoking(ctx context.Context, limit int) ([]RevokeGeneration, error)
}

// ─────────────────────────────────────────────────────────────────────────────
// DriftCaseStore — 漂移案例存储
// ─────────────────────────────────────────────────────────────────────────────
// DriftCaseStore 漂移案例存储接口。

type DriftCaseStore interface {
	// Open 写入新漂移案例。同一 entry_id 已有 open 案例时返回已有（追加 Note）。
	Open(ctx context.Context, dc InflightDriftCase) (InflightDriftCase, error)
	// GetOpenByEntry 返回 entry_id 最新 open 案例；无则 ErrLeaseNotFound。
	GetOpenByEntry(ctx context.Context, entryID string) (InflightDriftCase, error)
	// GetByID 按 case_id 读取。
	GetByID(ctx context.Context, caseID string) (InflightDriftCase, error)
	// Close 四眼收口：state=open→closed。须提供 closedBy 与 resolution。
	Close(ctx context.Context,
		caseID, closedBy, resolution string) (InflightDriftCase, error)
	// ListOpen 返回所有 open 案例，按 opened_at 升序，供巡检 worker。
	ListOpen(ctx context.Context, limit int) ([]InflightDriftCase, error)
	// ListByEntry 返回 entry_id 所有案例，按时间升序。
	ListByEntry(ctx context.Context,
		entryID string) ([]InflightDriftCase, error)
}

// ─────────────────────────────────────────────────────────────────────────────
// ProofStore — 在途证明存储
// ─────────────────────────────────────────────────────────────────────────────
// ProofStore 在途广播证明存储接口。

type ProofStore interface {
	// Insert 写入不可变证明。(lease_id, movement_id) 唯一冲突返回已有证明（幂等）。
	Insert(ctx context.Context, p InflightProof) (InflightProof, bool, error)
	// GetByLease 按 (lease_id, movement_id) 读取唯一证明。
	GetByLease(ctx context.Context, leaseID, movementID string) (InflightProof, error)
	// GetByID 按 proof_id 读取。
	GetByID(ctx context.Context,
		proofID string) (InflightProof, error)
	// ListByEntry 返回 entry_id 下所有证明，按时间升序。
	ListByEntry(ctx context.Context, entryID string) ([]InflightProof, error)
}

// ─────────────────────────────────────────────────────────────────────────────
// 内存实现（单元与非持久化模式）
// ─────────────────────────────────────────────────────────────────────────────
// MemGenLeaseStore 内存版在途世代租约存储。
type MemGenLeaseStore struct {
	mu   sync.Mutex
	byID map[string]InflightGenLease
	byMove map[string]string // "entryID|movementID" → leaseID
}

// NewMemGenLeaseStore 构造内存版租约存储。

   func NewMemGenLeaseStore() *MemGenLeaseStore {
	   return &MemGenLeaseStore{
		  byID:   make(map[string]InflightGenLease),
		   byMove: make(map[string]string),
	   }
  }

   func moveKey(entryID, movementID string) string { return entryID + "|" + movementID }

   func (s *MemGenLeaseStore) Insert(_ context.Context, l InflightGenLease) (InflightGenLease, bool, error) {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  k := moveKey(l.EntryID, l.MovementID)
	  if id, ok := s.byMove[k]; ok {
		  return s.byID[id], true, nil
	   }
	   if l.LeaseID == "" {
		   l.LeaseID = uuid.NewString()
	   }
	  s.byID[l.LeaseID] = l
	  s.byMove[k] = l.LeaseID
	   return l, false, nil
   }

   func (s *MemGenLeaseStore) GetByID(_ context.Context, leaseID string) (InflightGenLease, error) {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  l, ok := s.byID[leaseID]
	   if !ok {
		   return InflightGenLease{},
			  ErrLeaseNotFound
	   }
	  return l, nil
  }

  func (s *MemGenLeaseStore) GetByMovement(_ context.Context, entryID, movementID string) (InflightGenLease, error) {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  id, ok := s.byMove[moveKey(entryID, movementID)]
	  if !ok {
		   return InflightGenLease{}, ErrLeaseNotFound
	   }
	   return s.byID[id], nil
  }

   func (s *MemGenLeaseStore) ListActiveByEntry(_ context.Context, entryID string) ([]InflightGenLease, error) {
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   var out []InflightGenLease
	  for _, l := range s.byID {
		   if l.EntryID == entryID && (l.State == RefAcquired || l.State == RefBound) {
			  out = append(out, l)
		  }
	  }
	  sort.Slice(out, func(i, j int) bool { return out[i].IssuedAt.Before(out[j].IssuedAt) })
	   return out, nil
  }

  func (s *MemGenLeaseStore) CountActiveByEntry(ctx context.Context, entryID string) (int, error) {
	  list, err := s.ListActiveByEntry(ctx, entryID)
	   return len(list), err
  }

  func (s *MemGenLeaseStore) Transition(_ context.Context, leaseID string, from, to RefState, updates InflightGenLease) error {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   l, ok := s.byID[leaseID]
	  if !ok {
		  return ErrLeaseNotFound
	   }
	  if l.State != from {
		  return ErrInvalidState
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
	   s.byID[leaseID] = l
	  return nil
  }

  func (s *MemGenLeaseStore) ListExpired(_ context.Context, now time.Time, limit int) ([]InflightGenLease, error) {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   var out []InflightGenLease
	   for _, l := range s.byID {
		  if l.State != RefReleased && l.ExpiresAt.Before(now) {
			   out = append(out, l)
			  if limit > 0 && len(out) >= limit {
				   break
			  }
		  }
	  }
	  return out, nil
  }

   func (s *MemGenLeaseStore) ListByEntry(_ context.Context, entryID string) ([]InflightGenLease, error) {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  var out []InflightGenLease
	  for _, l := range s.byID {
		   if l.EntryID == entryID {
			   out = append(out, l)
		   }
	   }
	  sort.Slice(out, func(i, j int) bool { return out[i].IssuedAt.Before(out[j].IssuedAt) })
	   return out, nil
  }

// ─────────────────────────────────────────────────────────────────────────────
// MemRevokeGenStore 内存版吊销世代存储。

type MemRevokeGenStore struct {
	mu sync.Mutex
	rows map[string]RevokeGeneration
}

// NewMemRevokeGenStore 构造内存版吊销世代存储。
  func NewMemRevokeGenStore() *MemRevokeGenStore {
	  return &MemRevokeGenStore{rows: make(map[string]RevokeGeneration)}
  }

  func (s *MemRevokeGenStore) GetOrInit(_ context.Context, entryID, userID, chain, address string) (RevokeGeneration, error) {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  if r, ok := s.rows[entryID]; ok {
		  return r, nil
	  }
	   r := RevokeGeneration{
		   EntryID: entryID,
		   UserID: userID,
		  Chain: chain,
		  Address:    address,
		   Gen:        0,
		  State:      EntryActive,
		  ActiveRefs: 0,
		   InitiatedAt: time.Now().UTC(),
	  }
	   s.rows[entryID] = r
	   return r, nil
  }

  func (s *MemRevokeGenStore) Get(_ context.Context, entryID string) (RevokeGeneration, error) {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   r, ok := s.rows[entryID]
	   if !ok {
		   return RevokeGeneration{}, ErrEntryNotFound
	  }
	  return r, nil
  }

   func (s *MemRevokeGenStore) BumpGen(_ context.Context, entryID string, currentGen int64, newState EntryState, actor string) (RevokeGeneration, error) {
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   r, ok := s.rows[entryID]
	   if !ok {
		  return RevokeGeneration{}, ErrEntryNotFound
	   }
	   if r.Gen != currentGen {
		  return RevokeGeneration{},
			   ErrInvalidState
	  }
	  r.Gen++
	   r.State = newState
	  r.Actor = actor
	   s.rows[entryID] = r
	   return r, nil
  }

  func (s *MemRevokeGenStore) UpdateActiveRefs(_ context.Context, entryID string, delta int) (RevokeGeneration, error) {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  r, ok := s.rows[entryID]
	   if !ok {
		  return RevokeGeneration{},
			   ErrEntryNotFound
	   }
	  r.ActiveRefs += delta
	  if r.ActiveRefs < 0 {
		   r.ActiveRefs = 0
	   }
	  s.rows[entryID] = r
	   return r, nil
  }

  func (s *MemRevokeGenStore) SetRevoked(_ context.Context, entryID string, actor string) (RevokeGeneration, error) {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  r, ok := s.rows[entryID]
	  if !ok {
		  return RevokeGeneration{}, ErrEntryNotFound
	  }
	  now := time.Now().UTC()
	  r.State = EntryRevoked
	  r.RevokedAt = &now
	   r.Actor = actor
	   s.rows[entryID] = r
	   return r, nil
   }

   func (s *MemRevokeGenStore) ListRevoking(_ context.Context, limit int) ([]RevokeGeneration, error) {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   var out []RevokeGeneration
	  for _, r := range s.rows {
		   if r.State == EntryRevoking {
			   out = append(out, r)
			  if limit > 0 && len(out) >= limit {
				   break
			   }
		   }
	  }
	   return out, nil
   }

// ─────────────────────────────────────────────────────────────────────────────
// MemDriftCaseStore 内存版漂移案例存储。

type MemDriftCaseStore struct {
	mu sync.Mutex
	byID map[string]InflightDriftCase
}

// NewMemDriftCaseStore 构造内存版漂移案例存储。

  func NewMemDriftCaseStore() *MemDriftCaseStore {
	  return &MemDriftCaseStore{byID: make(map[string]InflightDriftCase)}
  }

   func (s *MemDriftCaseStore) Open(_ context.Context, dc InflightDriftCase) (InflightDriftCase, error) {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  for _, c := range s.byID {
		  if c.EntryID == dc.EntryID && c.State == DriftCaseOpen {
			   if dc.Note != "" {
				   c.Note += "; " + dc.Note
				   s.byID[c.CaseID] = c
			   }
			  return c, nil
		  }
	  }
	   if dc.CaseID == "" {
		   dc.CaseID = uuid.NewString()
	  }
	   s.byID[dc.CaseID] = dc
	   return dc, nil
   }

   func (s *MemDriftCaseStore) GetOpenByEntry(_ context.Context, entryID string) (InflightDriftCase, error) {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   for _, c := range s.byID {
		  if c.EntryID == entryID && c.State == DriftCaseOpen {
			  return c, nil
		  }
	   }
	  return InflightDriftCase{}, ErrLeaseNotFound
  }

   func (s *MemDriftCaseStore) GetByID(_ context.Context, caseID string) (InflightDriftCase, error) {
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   c, ok := s.byID[caseID]
	   if !ok {
		  return InflightDriftCase{},
			  ErrLeaseNotFound
	  }
	  return c, nil
   }

  func (s *MemDriftCaseStore) Close(_ context.Context, caseID, closedBy, resolution string) (InflightDriftCase, error) {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  c, ok := s.byID[caseID]
	   if !ok {
		   return InflightDriftCase{}, ErrLeaseNotFound
	   }
	   if c.State != DriftCaseOpen {
		  return c, nil
	   }
	  now := time.Now().UTC()
	   c.State = DriftCaseClosed
	   c.ClosedAt = &now
	   c.ClosedBy = closedBy
	  c.Resolution = resolution
	  s.byID[caseID] = c
	  return c, nil
   }

  func (s *MemDriftCaseStore) ListOpen(_ context.Context, limit int) ([]InflightDriftCase, error) {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   var out []InflightDriftCase
	  for _, c := range s.byID {
		  if c.State == DriftCaseOpen {
			   out = append(out, c)
			  if limit > 0 && len(out) >= limit {
				  break
			   }
		  }
	   }
	   sort.Slice(out, func(i, j int) bool { return out[i].OpenedAt.Before(out[j].OpenedAt) })
	   return out, nil
   }

   func (s *MemDriftCaseStore) ListByEntry(_ context.Context, entryID string) ([]InflightDriftCase, error) {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  var out []InflightDriftCase
	  for _, c := range s.byID {
		   if c.EntryID == entryID {
			  out = append(out, c)
		   }
	   }
	  sort.Slice(out, func(i, j int) bool { return out[i].OpenedAt.Before(out[j].OpenedAt) })
	   return out, nil
  }

// ─────────────────────────────────────────────────────────────────────────────
// MemProofStore 内存版在途证明存储。
type MemProofStore struct {
	mu sync.Mutex
	byID   map[string]InflightProof
	byMove map[string]string // "leaseID|movementID" → proofID
}

// NewMemProofStore 构造内存版证明存储。
  func NewMemProofStore() *MemProofStore {
	   return &MemProofStore{
		  byID: make(map[string]InflightProof),
		   byMove: make(map[string]string),
	  }
  }

   func proofMoveKey(leaseID, movementID string) string { return leaseID + "|" + movementID }

   func (s *MemProofStore) Insert(_ context.Context, p InflightProof) (InflightProof, bool, error) {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  k := proofMoveKey(p.LeaseID, p.MovementID)
	   if id, ok := s.byMove[k]; ok {
		  return s.byID[id], true, nil
	  }
	   if p.ProofID == "" {
		  p.ProofID = uuid.NewString()
	  }
	  s.byID[p.ProofID] = p
	  s.byMove[k] = p.ProofID
	   return p, false, nil
   }

   func (s *MemProofStore) GetByLease(_ context.Context, leaseID, movementID string) (InflightProof, error) {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   id, ok := s.byMove[proofMoveKey(leaseID, movementID)]
	  if !ok {
		  return InflightProof{}, ErrProofMissing
	   }
	  return s.byID[id], nil
   }

   func (s *MemProofStore) GetByID(_ context.Context, proofID string) (InflightProof, error) {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  p, ok := s.byID[proofID]
	   if !ok {
		  return InflightProof{}, ErrProofMissing
	   }
	  return p, nil
  }

  func (s *MemProofStore) ListByEntry(_ context.Context, entryID string) ([]InflightProof, error) {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  var out []InflightProof
	   for _, p := range s.byID {
		   if p.EntryID == entryID {
			  out = append(out, p)
		  }
	  }
	   sort.Slice(out,
		   func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	  return out, nil
   }

// EnsureEntryExists 在 RevokeGenStore 中初始化条目（若不存在）。
// 供 Gate 调用前置确认目的地条目，不写 destallow 主链。
   func EnsureEntryExists(ctx context.Context, store RevokeGenStore, entryID, userID, chain, address string) (RevokeGeneration, error) {
	   r, err := store.GetOrInit(ctx, entryID, userID, chain, address)
	   if err != nil {
		  return RevokeGeneration{}, errors.Join(ErrStore, err)
	  }
	  return r, nil
  }
