package cvrollback

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

const (
	kindPlan = "aa_cv_rollback"
	kindLeg = "aa_cv_rollback_leg"
)

// Store 回滚计划存储接口。

type Store interface {
	GetPlan(ctx context.Context, id string) (*RollbackPlan, error)
	GetPlanByOcc(ctx context.Context, occID string) (*RollbackPlan, error)
	SavePlan(ctx context.Context, plan *RollbackPlan) error
	ListPendingPlans(ctx context.Context,
		limit int) ([]*RollbackPlan, error)
	ListLegs(ctx context.Context, planID string) ([]*RollbackLeg, error)
	SaveLeg(ctx context.Context, leg *RollbackLeg) error
}

// KVStore 基于 persist.KV 的实现。
type KVStore struct {
	kv persist.KV
}

// NewKVStore 构造 KV 存储。
   func NewKVStore(kv persist.KV) *KVStore {
	  return &KVStore{kv: kv}
  }

   func (s *KVStore) GetPlan(ctx context.Context, id string) (*RollbackPlan, error) {
	  if s == nil || s.kv == nil {
		   return nil, ErrPlanStore
	   }
	   row, err := s.kv.Get(ctx, kindPlan, id)
	   if err != nil {
		  if errors.Is(err, persist.ErrNotFound) {
			   return nil, ErrPlanNotFound
		   }
		  return nil, err
	  }
	  var p RollbackPlan
	   if json.Unmarshal(row.Body, &p) != nil || p.ID == "" {
		  return nil, ErrPlanNotFound
	   }
	   return &p, nil
   }

   func (s *KVStore) GetPlanByOcc(ctx context.Context, occID string) (*RollbackPlan, error) {
	   if s == nil || s.kv == nil {
		  return nil, ErrPlanStore
	  }
	   row, err := s.kv.Get(ctx, kindPlan, "idx|occ|"+occID)
	   if err != nil {
		   if errors.Is(err, persist.ErrNotFound) {
			  return nil, ErrPlanNotFound
		   }
		   return nil, err
	   }
	   return s.GetPlan(ctx, string(row.Body))
   }

   func (s *KVStore) SavePlan(ctx context.Context, plan *RollbackPlan) error {
	  if s == nil || s.kv == nil {
		  return ErrPlanStore
	   }
	   plan.UpdatedAt = time.Now().UTC()
	   body, err := json.Marshal(plan)
	  if err != nil {
		   return err
	  }
	  exp := persist.Fence{}
	  if cur, err := s.kv.Get(ctx, kindPlan, plan.ID); err == nil {
		  exp = cur.Fence
	  }
	   if err := s.kv.Put(ctx, persist.Row{Kind: kindPlan, Key: plan.ID, Body: body}, exp); err != nil {
		  return err
	  }
	   if plan.OccID != "" {
		  _ = s.kv.Put(ctx, persist.Row{Kind: kindPlan, Key: "idx|occ|" + plan.OccID, Body: []byte(plan.ID)}, persist.Fence{})
	  }
	  return nil
  }

  func (s *KVStore) ListPendingPlans(ctx context.Context, limit int) ([]*RollbackPlan, error) {
	  if s == nil || s.kv == nil {
		  return nil, ErrPlanStore
	   }
	   if limit <= 0 {
		  limit = 50
	   }
	  rows, err := s.kv.List(ctx, kindPlan, limit*4)
	   if err != nil {
		  return nil, err
	   }
	  out := make([]*RollbackPlan,
		   0, limit)
	   for _, r := range rows {
		   if len(r.Body) < 10 {
			   continue
		  }
		   var p RollbackPlan
		  if json.Unmarshal(r.Body, &p) != nil || p.ID == "" {
			   continue
		  }
		  if p.Status != RollbackPlanned && p.Status != RollbackExecuting {
			   continue
		  }
		   out = append(out, &p)
		  if len(out) >= limit {
			   break
		  }
	  }
	  return out, nil
  }

   func (s *KVStore) ListLegs(ctx context.Context, planID string) ([]*RollbackLeg, error) {
	  if s == nil || s.kv == nil {
		  return nil, ErrPlanStore
	  }
	  rows, err := s.kv.List(ctx, kindLeg, 30)
	   if err != nil {
		  return nil, err
	   }
	  out := make([]*RollbackLeg, 0)
	  for _, r := range rows {
		   var l RollbackLeg
		  if json.Unmarshal(r.Body, &l) != nil || l.ID == "" {
			   continue
		   }
		   if l.PlanID == planID {
			  out = append(out, &l)
		   }
	  }
	  return out, nil
   }

  func (s *KVStore) SaveLeg(ctx context.Context, leg *RollbackLeg) error {
	   if s == nil || s.kv == nil {
		  return ErrPlanStore
	   }
	   leg.UpdatedAt = time.Now().UTC()
	   body, err := json.Marshal(leg)
	  if err != nil {
		   return err
	  }
	   exp := persist.Fence{}
	   if cur, err := s.kv.Get(ctx, kindLeg, leg.ID); err == nil {
		   exp = cur.Fence
	  }
	  return s.kv.Put(ctx,
		  persist.Row{Kind: kindLeg, Key: leg.ID, Body: body}, exp)
  }

// MemStore 进程内存 Store（用）。
type MemStore struct {
	mu     sync.RWMutex
	plans  map[string]*RollbackPlan
	legs   map[string]*RollbackLeg
	occIdx map[string]string
}

// NewMemStore 构造内存 Store。
  func NewMemStore() *MemStore {
	  return &MemStore{
		   plans: map[string]*RollbackPlan{},
		   legs: map[string]*RollbackLeg{},
		   occIdx: map[string]string{},
	  }
   }

  func (m *MemStore) GetPlan(_ context.Context, id string) (*RollbackPlan, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	   p, ok := m.plans[id]
	  if !ok {
		   return nil, ErrPlanNotFound
	  }
	   cp := *p
	  return &cp, nil
  }

  func (m *MemStore) GetPlanByOcc(_ context.Context, occID string) (*RollbackPlan, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   id, ok := m.occIdx[occID]
	  if !ok {
		  return nil, ErrPlanNotFound
	  }
	  p, ok := m.plans[id]
	  if !ok {
		  return nil, ErrPlanNotFound
	  }
	  cp := *p
	  return &cp, nil
  }

  func (m *MemStore) SavePlan(_ context.Context, plan *RollbackPlan) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  plan.UpdatedAt = time.Now().UTC()
	   cp := *plan
	  m.plans[plan.ID] = &cp
	   if plan.OccID != "" {
		   m.occIdx[plan.OccID] = plan.ID
	   }
	  return nil
  }

   func (m *MemStore) ListPendingPlans(_ context.Context, limit int) ([]*RollbackPlan, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	   if limit <= 0 {
		   limit = 50
	  }
	   out := make([]*RollbackPlan, 0)
	   for _, p := range m.plans {
		  if p.Status != RollbackPlanned && p.Status != RollbackExecuting {
			   continue
		   }
		  cp := *p
		   out = append(out, &cp)
		  if len(out) >= limit {
			   break
		  }
	  }
	  sort.Slice(out,
		   func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	  return out, nil
   }

  func (m *MemStore) ListLegs(_ context.Context, planID string) ([]*RollbackLeg, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  out := make([]*RollbackLeg, 0)
	   for _, l := range m.legs {
		  if l.PlanID == planID {
			  cp := *l
			   out = append(out, &cp)
		   }
	   }
	  return out, nil
   }

   func (m *MemStore) SaveLeg(_ context.Context, leg *RollbackLeg) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   leg.UpdatedAt = time.Now().UTC()
	   cp := *leg
	  m.legs[leg.ID] = &cp
	  return nil
  }
