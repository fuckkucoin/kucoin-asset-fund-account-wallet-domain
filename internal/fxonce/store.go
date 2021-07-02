package fxonce

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
	"github.com/google/uuid"
)

// ────────────────────────────────────────────────────────────────────────────
// Store 接口族
// ────────────────────────────────────────────────────────────────────────────
// EffectStore 管理 FundEffect 记录的读写。
// 实现必须保证 effect_key 唯一约束；冲突返回 ErrEffectConflict。
type EffectStore interface {
	// PutEffect 首次写入 FundEffect（effect 必须处于 proposed 状态）。
	// 若 effect_key 已存在则返回 ErrEffectConflict（不得覆盖已有记录）。
	PutEffect(ctx context.Context, e *FundEffect) error
	// GetEffect 按 effect_key 查询。未找到返回 ErrEffectNotFound。
	GetEffect(ctx context.Context, effectKey string) (*FundEffect, error)
	// GetEffectByID 按 ID 查询。未找到返回 ErrEffectNotFound。
	GetEffectByID(ctx context.Context, id string) (*FundEffect, error)
	// GetEffectsByBusiness 按 business_key 返回该键的所有 effect 记录（按 generation 升序）。
	GetEffectsByBusiness(ctx context.Context, businessKey string) ([]*FundEffect, error)
	// UpdateEffectState 更新状态（CAS：只允许从特定状态向特定状态迁移）。
	// oldState 不符则返回 ErrEffectConflict。
	UpdateEffectState(ctx context.Context, id string, oldState, newState EffectState, now time.Time, reason RejectCode) error
	// MarkPoisoned 将 effect 置为 poisoned，记录重试次数。
	MarkPoisoned(ctx context.Context,
		id string, retries int, now time.Time) error
	// MarkForceReplay 将 poisoned effect 解除并重置为 proposed（双人授权）。
	MarkForceReplay(ctx context.Context,
		id string, actors string, now time.Time) error
	// ListEffects 分页列出 effect（按 created_at 降序）。
	ListEffects(ctx context.Context, businessKey string, limit, offset int) ([]*FundEffect, error)
}

// GenStore 管理 DeliveryGen 记录的读写。

type GenStore interface {
	// PutGen 写入新 DeliveryGen（必须是 enqueued 状态）。
	// business_key + expected_gen 联合唯一；冲突返回 ErrEffectConflict。
	PutGen(ctx context.Context, g *DeliveryGen) error
	// GetGen 按 business_key + expected_gen 查询。未找到返回 ErrEffectNotFound。
	GetGen(ctx context.Context, businessKey string, expectedGen int64) (*DeliveryGen, error)
	// GetGenByID 按 ID 查询。
	GetGenByID(ctx context.Context, id string) (*DeliveryGen, error)
	// GetCurrentAppliedGen 返回 business_key 当前已 applied 的最大世代号。
	// 若从未 applied 则返回 0。
	GetCurrentAppliedGen(ctx context.Context, businessKey string) (int64, error)
	// UpdateGenState 状态迁移；包含 applied_gen 的 CAS 更新。
	UpdateGenState(ctx context.Context, id string, newState GenState, appliedGen int64, now time.Time, reason RejectCode) error
	// ListGensByBusiness 按 business_key 返回所有 DeliveryGen（按 expected_gen 升序）。
	ListGensByBusiness(ctx context.Context, businessKey string) ([]*DeliveryGen, error)
}

// DupProofStore 管理 DupProof 记录的读写。
type DupProofStore interface {
	// PutDupProof 写入新双花证明。proof ID 唯一；冲突返回 ErrDupProofConflict。
	PutDupProof(ctx context.Context, d *DupProof) error
	// GetDupProof 按 ID 查询。
	GetDupProof(ctx context.Context,
		id string) (*DupProof, error)
	// ListDupProofsForBusiness 按 business_key 列出所有双花证明（按 created_at 降序）。
	ListDupProofsForBusiness(ctx context.Context, businessKey string, limit int) ([]*DupProof, error)
	// ListUnreviewed 列出未审阅的 DupProof（供 desk 巡检）。
	ListUnreviewed(ctx context.Context, limit int) ([]*DupProof, error)
	// MarkReviewed 标记 DupProof 已 desk 审阅。
	MarkReviewed(ctx context.Context,
		id string, reviewer string, now time.Time) error
}

// Store 组合接口，聚合三张逻辑表操作。
type Store interface {
	EffectStore
	GenStore
	DupProofStore
}

// ────────────────────────────────────────────────────────────────────────────
// MemStore — 内存实现（用于）
// ────────────────────────────────────────────────────────────────────────────
// MemStore 是 Store 的内存实现，适用于单元与进程内端对端集成验证。
// 不适合：无持久化，进程重启数据丢失。

type MemStore struct {
	mu sync.RWMutex
	effects    map[string]*FundEffect  // key: effect_key
	effectByID map[string]*FundEffect  // key: effect.ID
	gens       map[string]*DeliveryGen // key: businessKey+":"+gen
	genByID map[string]*DeliveryGen // key: gen.ID
	dups    map[string]*DupProof    // key: dup.ID
}

// NewMemStore 构造内存 Store。

   func NewMemStore() *MemStore {
	  return &MemStore{
		   effects: map[string]*FundEffect{},
		  effectByID: map[string]*FundEffect{},
		   gens:       map[string]*DeliveryGen{},
		   genByID: map[string]*DeliveryGen{},
		  dups:    map[string]*DupProof{},
	   }
   }

// genKey 生成 gens map 的复合键。
  func genKey(businessKey string, gen int64) string {
	   return businessKey + ":" + fmt.Sprintf("%d", gen)
  }

// ─── EffectStore ───────────────────────────────────────────────────────────
// PutEffect 首次写入 FundEffect；effect_key 冲突返回 ErrEffectConflict。
  func (m *MemStore) PutEffect(_ context.Context, e *FundEffect) error {
	  if e == nil {
		  return ErrStoreUnavailable
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   if _, ok := m.effects[e.EffectKey]; ok {
		  return ErrEffectConflict
	  }
	   clone := cloneEffect(e)
	   m.effects[clone.EffectKey] = clone
	  m.effectByID[clone.ID] = clone
	   return nil
   }

// GetEffect 按 effect_key 查询。

  func (m *MemStore) GetEffect(_ context.Context, effectKey string) (*FundEffect, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	  e, ok := m.effects[effectKey]
	  if !ok {
		   return nil, ErrEffectNotFound
	  }
	   return cloneEffect(e), nil
   }

// GetEffectByID 按 ID 查询。

   func (m *MemStore) GetEffectByID(_ context.Context, id string) (*FundEffect, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  e, ok := m.effectByID[id]
	   if !ok {
		   return nil, ErrEffectNotFound
	   }
	  return cloneEffect(e), nil
   }

// GetEffectsByBusiness 按 business_key 返回所有 effect（按 generation 升序）。

  func (m *MemStore) GetEffectsByBusiness(_ context.Context, businessKey string) ([]*FundEffect, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  var out []*FundEffect
	  for _, e := range m.effects {
		   if e.BusinessKey == businessKey {
			  out = append(out, cloneEffect(e))
		   }
	  }
	   sort.Slice(out,
		  func(i, j int) bool { return out[i].Generation < out[j].Generation })
	   return out, nil
  }

// UpdateEffectState 状态迁移（CAS 检查 oldState）。

  func (m *MemStore) UpdateEffectState(_ context.Context, id string, oldState, newState EffectState, now time.Time, reason RejectCode) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   e, ok := m.effectByID[id]
	  if !ok {
		  return ErrEffectNotFound
	   }
	  if e.State != oldState {
		   return ErrEffectConflict
	   }
	   e.State = newState
	  e.UpdatedAt = now
	  e.RejectReason = reason
	  switch newState {
	  case EffectApplied:
		  e.AppliedAt = &now
	   case EffectRejected, EffectPoisoned:
		  e.RejectedAt = &now
	   }
	   return nil
  }

// MarkPoisoned 将 effect 置为 poisoned。

   func (m *MemStore) MarkPoisoned(_ context.Context, id string, retries int, now time.Time) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  e, ok := m.effectByID[id]
	   if !ok {
		   return ErrEffectNotFound
	  }
	   e.State = EffectPoisoned
	  e.PoisonRetries = retries
	   e.RejectedAt = &now
	   e.UpdatedAt = now
	  return nil
  }

// MarkForceReplay 双人授权解除 poisoned。
  func (m *MemStore) MarkForceReplay(_ context.Context, id string, actors string, now time.Time) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   e, ok := m.effectByID[id]
	   if !ok {
		   return ErrEffectNotFound
	   }
	   if e.State != EffectPoisoned {
		   return ErrEffectConflict
	  }
	   e.State = EffectProposed
	   e.ForceReplayAt = &now
	  e.ForceReplayBy = actors
	  e.UpdatedAt = now
	   e.PoisonRetries = 0
	  return nil
   }

// ListEffects 分页列出（按 created_at 降序）。

   func (m *MemStore) ListEffects(_ context.Context, businessKey string, limit, offset int) ([]*FundEffect, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  var all []*FundEffect
	  for _, e := range m.effects {
		  if businessKey == "" || e.BusinessKey == businessKey {
			   all = append(all, cloneEffect(e))
		   }
	   }
	   sort.Slice(all, func(i, j int) bool {
		  return all[i].CreatedAt.After(all[j].CreatedAt)
	   })
	   if offset >= len(all) {
		   return nil, nil
	   }
	   all = all[offset:]
	  if limit > 0 && len(all) > limit {
		   all = all[:limit]
	  }
	  return all, nil
  }

// ─── GenStore ─────────────────────────────────────────────────────────────
// PutGen 写入新 DeliveryGen。
   func (m *MemStore) PutGen(_ context.Context, g *DeliveryGen) error {
	   if g == nil {
		  return ErrStoreUnavailable
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   k := genKey(g.BusinessKey, g.ExpectedGen)
	   if _, ok := m.gens[k]; ok {
		  return ErrEffectConflict
	  }
	   clone := cloneGen(g)
	  m.gens[k] = clone
	   m.genByID[clone.ID] = clone
	  return nil
  }

// GetGen 按 business_key + expected_gen 查询。

   func (m *MemStore) GetGen(_ context.Context, businessKey string, expectedGen int64) (*DeliveryGen, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   k := genKey(businessKey, expectedGen)
	   g, ok := m.gens[k]
	   if !ok {
		   return nil, ErrEffectNotFound
	  }
	  return cloneGen(g), nil
   }

// GetGenByID 按 ID 查询。
  func (m *MemStore) GetGenByID(_ context.Context, id string) (*DeliveryGen, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   g, ok := m.genByID[id]
	  if !ok {
		  return nil, ErrEffectNotFound
	   }
	   return cloneGen(g), nil
  }

// GetCurrentAppliedGen 返回已 applied 的最大世代号（0 = 从未 applied）。
   func (m *MemStore) GetCurrentAppliedGen(_ context.Context, businessKey string) (int64, error) {
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	  var max int64
	   for _, g := range m.gens {
		   if g.BusinessKey == businessKey && g.State == GenAcked && g.AppliedGen > max {
			  max = g.AppliedGen
		   }
	   }
	  return max, nil
  }

// UpdateGenState 更新 GenState 与 applied_gen。
  func (m *MemStore) UpdateGenState(_ context.Context, id string, newState GenState, appliedGen int64, now time.Time, reason RejectCode) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   g, ok := m.genByID[id]
	   if !ok {
		  return ErrEffectNotFound
	  }
	   g.State = newState
	   g.AppliedGen = appliedGen
	   g.UpdatedAt = now
	   g.RejectReason = reason
	  switch newState {
	   case GenAcked:
		  g.AckedAt = &now
	   case GenNacked:
		  g.NackedAt = &now
	  }
	   return nil
  }

// ListGensByBusiness 按 business_key 返回所有 DeliveryGen（按 expected_gen 升序）。

   func (m *MemStore) ListGensByBusiness(_ context.Context, businessKey string) ([]*DeliveryGen, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   var out []*DeliveryGen
	  for _, g := range m.gens {
		   if g.BusinessKey == businessKey {
			   out = append(out, cloneGen(g))
		   }
	  }
	  sort.Slice(out, func(i, j int) bool { return out[i].ExpectedGen < out[j].ExpectedGen })
	   return out, nil
   }

// ─── DupProofStore ────────────────────────────────────────────────────────
// PutDupProof 写入双花证明；ID 冲突返回 ErrDupProofConflict。
   func (m *MemStore) PutDupProof(_ context.Context, d *DupProof) error {
	   if d == nil {
		  return ErrStoreUnavailable
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   if _, ok := m.dups[d.ID]; ok {
		  return ErrDupProofConflict
	  }
	  clone := cloneDup(d)
	   m.dups[clone.ID] = clone
	   return nil
  }

// GetDupProof 按 ID 查询。
   func (m *MemStore) GetDupProof(_ context.Context, id string) (*DupProof, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	   d, ok := m.dups[id]
	  if !ok {
		  return nil, ErrEffectNotFound
	  }
	   return cloneDup(d), nil
  }

// ListDupProofsForBusiness 按 business_key 列出（按 created_at 降序，最多 limit 条）。

  func (m *MemStore) ListDupProofsForBusiness(_ context.Context, businessKey string, limit int) ([]*DupProof, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	   var out []*DupProof
	   for _, d := range m.dups {
		  if d.BusinessKey == businessKey {
			   out = append(out, cloneDup(d))
		  }
	  }
	  sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	  if limit > 0 && len(out) > limit {
		  out = out[:limit]
	   }
	   return out, nil
   }

// ListUnreviewed 列出未审阅的 DupProof（按 created_at 升序，限 limit 条）。

   func (m *MemStore) ListUnreviewed(_ context.Context, limit int) ([]*DupProof, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   var out []*DupProof
	  for _, d := range m.dups {
		  if !d.Reviewed {
			  out = append(out, cloneDup(d))
		  }
	   }
	   sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	   if limit > 0 && len(out) > limit {
		  out = out[:limit]
	  }
	  return out, nil
   }

// MarkReviewed 标记 DupProof 已审阅。
  func (m *MemStore) MarkReviewed(_ context.Context, id string, reviewer string, now time.Time) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  d, ok := m.dups[id]
	  if !ok {
		   return ErrEffectNotFound
	   }
	   d.Reviewed = true
	   d.ReviewedBy = reviewer
	  d.ReviewedAt = &now
	   return nil
   }

// ────────────────────────────────────────────────────────────────────────────
// 深拷贝工具（防止调用方通过指针修改 store 内部状态）
// ────────────────────────────────────────────────────────────────────────────

  func cloneEffect(e *FundEffect) *FundEffect {
	   if e == nil {
		   return nil
	   }
	  c := *e
	  if e.AppliedAt != nil {
		   t := *e.AppliedAt
		   c.AppliedAt = &t
	  }
	  if e.RejectedAt != nil {
		   t := *e.RejectedAt
		   c.RejectedAt = &t
	   }
	   if e.ForceReplayAt != nil {
		   t := *e.ForceReplayAt
		  c.ForceReplayAt = &t
	  }
	   return &c
   }

  func cloneGen(g *DeliveryGen) *DeliveryGen {
	   if g == nil {
		   return nil
	  }
	   c := *g
	   if g.AckedAt != nil {
		   t := *g.AckedAt
		  c.AckedAt = &t
	   }
	  if g.NackedAt != nil {
		  t := *g.NackedAt
		  c.NackedAt = &t
	   }
	  return &c
  }

   func cloneDup(d *DupProof) *DupProof {
	   if d == nil {
		   return nil
	   }
	   c := *d
	   if d.ReviewedAt != nil {
		   t := *d.ReviewedAt
		   c.ReviewedAt = &t
	  }
	  return &c
  }

// newEffectID / newGenID / newDupID 生成唯一 ID。
   func newEffectID() string { return uuid.NewString() }
  func newGenID() string    { return uuid.NewString() }

   func newDupID() string { return uuid.NewString() }
// temporary guard removed after the production check
