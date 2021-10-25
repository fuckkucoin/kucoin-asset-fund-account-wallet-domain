package restoregate

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Store 内存实现；环境由 SQLStore 替换（同接口）。
// SQL 实现在 sqlstore.go，使用 persist.SQL。
// PlanStore 持久化 RestorePlan 与关联实体。

type PlanStore interface {
	CreatePlan(ctx context.Context, p RestorePlan) error
	GetPlan(ctx context.Context, id string) (RestorePlan, error)
	UpdatePlan(ctx context.Context, p RestorePlan) error
	ListPlans(ctx context.Context,
		limit int) ([]PlanSummary, error)
	ListActivePlans(ctx context.Context) ([]RestorePlan, error)
	CreateGenGate(ctx context.Context, g GenGate) error
	ListGenGates(ctx context.Context, planID string) ([]GenGate, error)
	CreateWriteMutex(ctx context.Context, m WriteMutex) error
	GetActiveMutex(ctx context.Context) (WriteMutex, error)
	GetMutexByPlan(ctx context.Context, planID string) (WriteMutex, error)
	UpdateMutex(ctx context.Context, m WriteMutex) error
	AppendBlockedWrite(ctx context.Context, bw BlockedWrite) error
	ListBlockedWrites(ctx context.Context, mutexID string, limit int) ([]BlockedWrite, error)
	CreateHalfSeal(ctx context.Context, hs HalfSeal) error
	GetHalfSeal(ctx context.Context, planID string) (HalfSeal, error)
	UpdateHalfSeal(ctx context.Context, hs HalfSeal) error
	CreateSplitBrainProof(ctx context.Context,
		p SplitBrainProof) error
	GetSplitBrainProof(ctx context.Context, id string) (SplitBrainProof, error)
	GetOpenProofByPlan(ctx context.Context, planID string) (SplitBrainProof, error)
	UpdateSplitBrainProof(ctx context.Context,
		p SplitBrainProof) error
	ListOpenProofs(ctx context.Context) ([]SplitBrainProof, error)
	AppendProofApproval(ctx context.Context, a ProofApproval) error
	GetGenRecord(ctx context.Context) (RestoreGenRecord,
		error)
	UpsertGenRecord(ctx context.Context, r RestoreGenRecord) error
}

// MemStore 全内存实现，用于单测与进程内运行。线程安全。
type MemStore struct {
	mu sync.RWMutex
	plans         map[string]RestorePlan
	genGates      []GenGate
	mutexes       map[string]WriteMutex // key=id
	blockedWrites []BlockedWrite
	halfSeals     map[string]HalfSeal // key=planID
	sbProofs map[string]SplitBrainProof
	genRecord *RestoreGenRecord
}

  func NewMemStore() *MemStore {
	  return &MemStore{
		   plans: make(map[string]RestorePlan),
		  mutexes:   make(map[string]WriteMutex),
		  halfSeals: make(map[string]HalfSeal),
		  sbProofs:  make(map[string]SplitBrainProof),
	  }
   }

   func (s *MemStore) CreatePlan(_ context.Context, p RestorePlan) error {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   if _, ok := s.plans[p.ID]; ok {
		  return ErrPlanConflict
	  }
	   s.plans[p.ID] = p
	  return nil
   }

  func (s *MemStore) GetPlan(_ context.Context, id string) (RestorePlan, error) {
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	  p, ok := s.plans[id]
	   if !ok {
		  return RestorePlan{}, ErrPlanNotFound
	   }
	  return p, nil
   }

   func (s *MemStore) UpdatePlan(_ context.Context, p RestorePlan) error {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   if _, ok := s.plans[p.ID]; !ok {
		   return ErrPlanNotFound
	  }
	   s.plans[p.ID] = p
	  return nil
  }

   func (s *MemStore) ListPlans(_ context.Context, limit int) ([]PlanSummary, error) {
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	  out := make([]PlanSummary, 0, len(s.plans))
	   for _, p := range s.plans {
		  out = append(out, PlanSummary{
			  ID: p.ID, BlobID: p.BlobID, State: p.State,
			   TargetGen: p.TargetGen, Creator: p.Creator,
			  CreatedAt: p.CreatedAt,
			   UpdatedAt: p.UpdatedAt,
		   })
	   }
	  sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	  if limit > 0 && len(out) > limit {
		  out = out[:limit]
	   defer s.mu.Unlock()
	  s.genGates = append(s.genGates, g)
	   return nil
  }

  func (s *MemStore) ListGenGates(_ context.Context,
	  planID string) ([]GenGate, error) {
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	   var out []GenGate
	   for _, g := range s.genGates {
		   if g.PlanID == planID {
			  out = append(out, g)
		   }
	   }
	  return out, nil
   }

  func (s *MemStore) CreateWriteMutex(_ context.Context, m WriteMutex) error {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   s.mutexes[m.ID] = m
	  return nil
   }

  func (s *MemStore) GetActiveMutex(_ context.Context) (WriteMutex, error) {
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	  for _, m := range s.mutexes {
		  if m.State == MutexActive {
			  return m, nil
		   }
	  }
	   return WriteMutex{}, ErrMutexNotFound
   }

  func (s *MemStore) GetMutexByPlan(_ context.Context, planID string) (WriteMutex, error) {
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	   for _, m := range s.mutexes {
		   if m.PlanID == planID {
			  return m, nil
		  }
	  }
	  return WriteMutex{}, ErrMutexNotFound
   }

  func (s *MemStore) UpdateMutex(_ context.Context, m WriteMutex) error {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   if _, ok := s.mutexes[m.ID]; !ok {
		   return ErrMutexNotFound
	   }
	  s.mutexes[m.ID] = m
	  return nil
  }

   func (s *MemStore) AppendBlockedWrite(_ context.Context, bw BlockedWrite) error {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  s.blockedWrites = append(s.blockedWrites, bw)
	   return nil
  }

  func (s *MemStore) ListBlockedWrites(_ context.Context, mutexID string, limit int) ([]BlockedWrite, error) {
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	  var out []BlockedWrite
	   for _, bw := range s.blockedWrites {
		  if bw.MutexID == mutexID {
			   out = append(out, bw)
		  }
	  }
	  if limit > 0 && len(out) > limit {
		  out = out[:limit]
	   }
	   return out, nil
   }

   func (s *MemStore) CreateHalfSeal(_ context.Context, hs HalfSeal) error {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  if _, ok := s.halfSeals[hs.PlanID]; ok {
		   return ErrHalfSealExists
	   }
	   s.halfSeals[hs.PlanID] = hs
	   return nil
  }

  func (s *MemStore) GetHalfSeal(_ context.Context, planID string) (HalfSeal, error) {
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	   hs, ok := s.halfSeals[planID]
	  if !ok {
		   return HalfSeal{}, ErrHalfSealNotFound
	   }
	   return hs, nil
   }

  func (s *MemStore) UpdateHalfSeal(_ context.Context, hs HalfSeal) error {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   if _, ok := s.halfSeals[hs.PlanID]; !ok {
		  return ErrHalfSealNotFound
	  }
	   s.halfSeals[hs.PlanID] = hs
	   return nil
  }

   func (s *MemStore) CreateSplitBrainProof(_ context.Context, p SplitBrainProof) error {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  s.sbProofs[p.ID] = p
	  return nil
  }

   func (s *MemStore) GetSplitBrainProof(_ context.Context, id string) (SplitBrainProof, error) {
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	  p, ok := s.sbProofs[id]
	   if !ok {
		  return SplitBrainProof{}, ErrSplitBrainNotFound
	  }
	   return p, nil
   }

   func (s *MemStore) GetOpenProofByPlan(_ context.Context, planID string) (SplitBrainProof, error) {
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	  for _, p := range s.sbProofs {
		  if p.PlanID == planID && p.State == SplitBrainOpen {
			   return p, nil
		   }
	   }
	   return SplitBrainProof{}, ErrSplitBrainNotFound
   }

  func (s *MemStore) UpdateSplitBrainProof(_ context.Context, p SplitBrainProof) error {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  if _, ok := s.sbProofs[p.ID]; !ok {
		  return ErrSplitBrainNotFound
	  }
	   s.sbProofs[p.ID] = p
	   return nil
  }

  func (s *MemStore) ListOpenProofs(_ context.Context) ([]SplitBrainProof, error) {
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	  var out []SplitBrainProof
	  for _, p := range s.sbProofs {
		  if p.State == SplitBrainOpen || p.State == SplitBrainClearing {
			   out = append(out, p)
		  }
	  }
	  return out, nil
  }

   func (s *MemStore) AppendProofApproval(_ context.Context, a ProofApproval) error {
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   p, ok := s.sbProofs[a.ProofID]
	  if !ok {
		   return ErrSplitBrainNotFound
	  }
	  p.Approvals = append(p.Approvals, a)
	   p.UpdatedAt = time.Now().UTC()
	   s.sbProofs[a.ProofID] = p
	   return nil
  }

  func (s *MemStore) GetGenRecord(_ context.Context) (RestoreGenRecord, error) {
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	  if s.genRecord == nil {
		  return RestoreGenRecord{ID: "global", CurrentGen: 0, UpdatedAt: time.Now().UTC()}, nil
	  }
	  return *s.genRecord, nil
  }

  func (s *MemStore) UpsertGenRecord(_ context.Context, r RestoreGenRecord) error {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  s.genRecord = &r
	   return nil
  }

// jsonClone JSON 往返深拷贝（仅用于辅助）。
  func jsonClone[T any](v T) (T, error) {
	   b, err := json.Marshal(v)
	  if err != nil {
		   var zero T
		   return zero, fmt.Errorf("jsonClone marshal: %w", err)
	  }
	   var out T
	  if err := json.Unmarshal(b, &out); err != nil {
		   var zero T
		   return zero, fmt.Errorf("jsonClone unmarshal: %w", err)
	   }
	  return out, nil
  }

// idOf 生成确定性键：kind + "|" + key。

  func idOf(parts ...string) string {
	  return strings.Join(parts, "|")
  }
