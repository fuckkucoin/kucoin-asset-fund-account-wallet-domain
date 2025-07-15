package xferclose

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

// HalfHopStore HalfHop 工单持久化接口。
type HalfHopStore interface {
	PutHop(ctx context.Context, h HalfHop) error
	UpdateHop(ctx context.Context, h HalfHop) error
	GetHop(ctx context.Context, id string) (HalfHop, error)
	GetByHopID(ctx context.Context, hopID string) (HalfHop, error)
	ListActive(ctx context.Context, offset, limit int) ([]HalfHop, error)
	ListByState(ctx context.Context,
		state HopState, offset, limit int) ([]HalfHop, error)
}

// PlanStore CloseoutPlan 持久化接口。
type PlanStore interface {
	PutPlan(ctx context.Context, p CloseoutPlan) error
	UpdatePlan(ctx context.Context, p CloseoutPlan) error
	GetPlan(ctx context.Context, id string) (CloseoutPlan, error)
	GetPlanByHopID(ctx context.Context, hopID string) (CloseoutPlan, bool, error)
	ListPlans(ctx context.Context, state PlanState, offset, limit int) ([]CloseoutPlan, error)
}

// LegStore CloseoutLeg 持久化接口。
type LegStore interface {
	PutLeg(ctx context.Context,
		l CloseoutLeg) error
	UpdateLeg(ctx context.Context, l CloseoutLeg) error
	GetLeg(ctx context.Context, id string) (CloseoutLeg, error)
	GetLegByIdem(ctx context.Context, idemKey string) (CloseoutLeg, error)
	ListLegs(ctx context.Context, planID string) ([]CloseoutLeg, error)
}

// SLAStore XferSLA 规则持久化接口。

type SLAStore interface {
	PutSLA(ctx context.Context, s XferSLA) error
	UpdateSLA(ctx context.Context,
		s XferSLA) error
	GetSLA(ctx context.Context,
		id string) (XferSLA, error)
	ListSLAs(ctx context.Context) ([]XferSLA, error)
	DeleteSLA(ctx context.Context, id string) error
}

// Store 聚合所有子仓储接口，方便依赖注入。

type Store interface {
	HalfHopStore
	PlanStore
	LegStore
	SLAStore
}

// MemStore 线程安全的内存 Store，用于和无 DB 启动模式。

type MemStore struct {
	mu          sync.RWMutex
	hops        map[string]HalfHop // key=ID
	hopsByHopID map[string]string  // hopID→ID
	plans        map[string]CloseoutPlan
	plansByHopID map[string]string // hopID→planID
	legs         map[string]CloseoutLeg
	legsByIdem map[string]string // idemKey→legID
	slas map[string]XferSLA
}

// NewMemStore 构造内存 Store，预填充默认 SLA 规则。

   func NewMemStore() *MemStore {
	   s := &MemStore{
		  hops:        map[string]HalfHop{},
		   hopsByHopID: map[string]string{},
		   plans:       map[string]CloseoutPlan{},
		   plansByHopID: map[string]string{},
		   legs:       map[string]CloseoutLeg{},
		  legsByIdem: map[string]string{},
		   slas:       map[string]XferSLA{},
	  }
	  for _, r := range DefaultSLARules() {
		  s.slas[r.ID] = r
	   }
	  return s
  }

// ── HalfHop ──────────────────────────────────────────────────────────────────

   func (s *MemStore) PutHop(_ context.Context, h HalfHop) error {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  if _, ok := s.hopsByHopID[h.HopID]; ok {
		   return ErrConflict
	  }
	   s.hops[h.ID] = h
	  s.hopsByHopID[h.HopID] = h.ID
	  return nil
   }

   func (s *MemStore) UpdateHop(_ context.Context, h HalfHop) error {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  if _, ok := s.hops[h.ID]; !ok {
		   return ErrNotFound
	   }
	  h.UpdatedAt = time.Now().UTC()
	  s.hops[h.ID] = h
	  return nil
  }

  func (s *MemStore) GetHop(_ context.Context, id string) (HalfHop, error) {
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	  h, ok := s.hops[id]
	   if !ok {
		  return HalfHop{}, ErrNotFound
	  }
	  return h, nil
  }

  func (s *MemStore) GetByHopID(_ context.Context, hopID string) (HalfHop, error) {
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	   id, ok := s.hopsByHopID[hopID]
	  if !ok {
		  return HalfHop{}, ErrNotFound
	  }
	  h, ok2 := s.hops[id]
	   if !ok2 {
		   return HalfHop{}, ErrNotFound
	   }
	  return h, nil
  }

   func (s *MemStore) ListActive(_ context.Context, offset, limit int) ([]HalfHop, error) {
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	   if limit <= 0 || limit > 500 {
		  limit = 100
	  }
	   var out []HalfHop
	   for _, h := range s.hops {
		  if !h.Terminal() {
			  out = append(out, h)
		  }
	   }
	  sortHops(out)
	   if offset >= len(out) {
		   return nil, nil
	   }
	   out = out[offset:]
	  if len(out) > limit {
		  out = out[:limit]
	  }
	   return out, nil
  }

   func (s *MemStore) ListByState(_ context.Context, state HopState, offset, limit int) ([]HalfHop, error) {
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	   if limit <= 0 || limit > 500 {
		  limit = 100
	   }
	   var out []HalfHop
	  for _, h := range s.hops {
		  if h.State == state {
			   out = append(out, h)
		  }
	  }
	  sortHops(out)
	   if offset >= len(out) {
		   return nil, nil
	   }
	  out = out[offset:]
	   if len(out) > limit {
		  out = out[:limit]
	   }
	  return out, nil
   }

// ── CloseoutPlan ─────────────────────────────────────────────────────────────

  func (s *MemStore) PutPlan(_ context.Context, p CloseoutPlan) error {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   if existID, ok := s.plansByHopID[p.HopID]; ok {
		  existing := s.plans[existID]
		  if existing.State == PlanOpen || existing.State == PlanRunning {
			   return ErrPlanRunning
		  }
	   }
	  s.plans[p.ID] = p
	  s.plansByHopID[p.HopID] = p.ID
	   return nil
   }

   func (s *MemStore) UpdatePlan(_ context.Context, p CloseoutPlan) error {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   if _, ok := s.plans[p.ID]; !ok {
		  return ErrNotFound
	  }
	  p.UpdatedAt = time.Now().UTC()
	   s.plans[p.ID] = p
	   return nil
   }

  func (s *MemStore) GetPlan(_ context.Context, id string) (CloseoutPlan, error) {
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	   p, ok := s.plans[id]
	  if !ok {
		  return CloseoutPlan{}, ErrNotFound
	   }
	  return p, nil
  }

  func (s *MemStore) GetPlanByHopID(_ context.Context, hopID string) (CloseoutPlan, bool, error) {
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	   id, ok := s.plansByHopID[hopID]
	   if !ok {
		   return CloseoutPlan{}, false, nil
	   }
	   p, ok2 := s.plans[id]
	   if !ok2 {
		  return CloseoutPlan{}, false, nil
	  }
	  return p, true, nil
   }

  func (s *MemStore) ListPlans(_ context.Context, state PlanState, offset, limit int) ([]CloseoutPlan, error) {
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	  if limit <= 0 || limit > 500 {
		   limit = 100
	   }
	  var out []CloseoutPlan
	  for _, p := range s.plans {
		  if p.State == state {
			  out = append(out, p)
		   }
	  }
	  sort.Slice(out, func(i, j int) bool {
		  return out[i].CreatedAt.Before(out[j].CreatedAt)
	  })
	   if offset >= len(out) {
		   return nil, nil
	   }
	   out = out[offset:]
	   if len(out) > limit {
		   out = out[:limit]
	  }
	   return out, nil
  }

// ── CloseoutLeg ──────────────────────────────────────────────────────────────

   func (s *MemStore) PutLeg(_ context.Context, l CloseoutLeg) error {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  if _, ok := s.legsByIdem[l.IdemKey]; ok {
		  return ErrIdempotent
	   }
	  s.legs[l.ID] = l
	  s.legsByIdem[l.IdemKey] = l.ID
	  return nil
   }

   func (s *MemStore) UpdateLeg(_ context.Context, l CloseoutLeg) error {
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   if _, ok := s.legs[l.ID]; !ok {
		   return ErrNotFound
	   }
	  s.legs[l.ID] = l
	  return nil
   }

   func (s *MemStore) GetLeg(_ context.Context, id string) (CloseoutLeg, error) {
	  s.mu.RLock()
	  defer s.mu.RUnlock()
	   l, ok := s.legs[id]
	   if !ok {
		   return CloseoutLeg{}, ErrNotFound
	   }
	  return l, nil
  }

  func (s *MemStore) GetLegByIdem(_ context.Context, idemKey string) (CloseoutLeg, error) {
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	   id, ok := s.legsByIdem[idemKey]
	   if !ok {
		   return CloseoutLeg{}, ErrNotFound
	  }
	   l, ok2 := s.legs[id]
	  if !ok2 {
		   return CloseoutLeg{},
			  ErrNotFound
	  }
	  return l, nil
  }

  func (s *MemStore) ListLegs(_ context.Context, planID string) ([]CloseoutLeg, error) {
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	  var out []CloseoutLeg
	  for _, l := range s.legs {
		   if l.PlanID == planID {
			   out = append(out, l)
		   }
	  }
	  sort.Slice(out, func(i, j int) bool {
		   return out[i].SeqNo < out[j].SeqNo
	  })
	  return out, nil
  }

// ── XferSLA ──────────────────────────────────────────────────────────────────

   func (s *MemStore) PutSLA(_ context.Context, r XferSLA) error {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  if _, ok := s.slas[r.ID]; ok {
		   return ErrConflict
	   }
	  s.slas[r.ID] = r
	   return nil
   }

   func (s *MemStore) UpdateSLA(_ context.Context, r XferSLA) error {
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   if _, ok := s.slas[r.ID]; !ok {
		  return ErrNotFound
	   }
	  r.UpdatedAt = time.Now().UTC()
	   s.slas[r.ID] = r
	  return nil
   }

   func (s *MemStore) GetSLA(_ context.Context, id string) (XferSLA, error) {
	   s.mu.RLock()
	  defer s.mu.RUnlock()
	   r, ok := s.slas[id]
	  if !ok {
		  return XferSLA{}, ErrNotFound
	  }
	  return r, nil
   }

  func (s *MemStore) ListSLAs(_ context.Context) ([]XferSLA, error) {
	  s.mu.RLock()
	   defer s.mu.RUnlock()
	   out := make([]XferSLA, 0, len(s.slas))
	  for _, r := range s.slas {
		  out = append(out, r)
	  }
	  sortRules(out)
	  return out, nil
   }

   func (s *MemStore) DeleteSLA(_ context.Context, id string) error {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   if _, ok := s.slas[id]; !ok {
		  return ErrNotFound
	   }
	   delete(s.slas, id)
	  return nil
  }

// ── helpers ───────────────────────────────────────────────────────────────────

   func sortHops(hops []HalfHop) {
	   sort.Slice(hops, func(i, j int) bool {
		   return hops[i].DetectedAt.Before(hops[j].DetectedAt)
	   })
   }

// IsNotFound reports whether err is ErrNotFound from this package or from persist.
  func IsNotFound(err error) bool {
	   if err == nil {
		   return false
	   }
	   return errors.Is(err, ErrNotFound) || strings.Contains(err.Error(), "not found")
   }
