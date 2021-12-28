package wdpartial

import (
	"sync"
	"time"
)

// HoldService 部分成交会计服务（ShardHold / PartialFill 子系统）。
type HoldService struct {
	mu sync.Mutex
	holds map[string]*ShardHold
	fills map[string]*PartialFill
	byLeg       map[string]string
	byPlan      map[string][]string
	fillsByPlan map[string][]string
	clock func() time.Time
}

// NewHoldService 构造部分成交会计服务。
   func NewHoldService() *HoldService {
	  return &HoldService{
		   holds: make(map[string]*ShardHold),
		  fills:       make(map[string]*PartialFill),
		  byLeg:       make(map[string]string),
		   byPlan:      make(map[string][]string),
		  fillsByPlan: make(map[string][]string),
		   clock:       func() time.Time { return time.Now().UTC() },
	   }
  }

   func (s *HoldService) now() time.Time {
	  if s == nil || s.clock == nil {
		   return time.Now().UTC()
	  }
	  return s.clock()
   }

// GetHold 查询 ShardHold。

  func (s *HoldService) GetHold(id string) (ShardHold, error) {
	   if s == nil {
		  return ShardHold{}, ErrUnavailable
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   h, ok := s.holds[id]
	  if !ok {
		   return ShardHold{}, ErrHoldNotFound
	   }
	  return h.Copy(), nil
   }

// GetHoldByLeg 按腿 ID 查询 ShardHold。
   func (s *HoldService) GetHoldByLeg(legID string) (ShardHold, error) {
	  if s == nil {
		  return ShardHold{}, ErrUnavailable
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   hid, ok := s.byLeg[legID]
	   if !ok {
		   return ShardHold{}, ErrHoldNotFound
	  }
	  h := s.holds[hid]
	  if h == nil {
		   return ShardHold{},
			   ErrHoldNotFound
	   }
	  return h.Copy(), nil
   }

// GetFill 查询 PartialFill。
  func (s *HoldService) GetFill(id string) (PartialFill, error) {
	  if s == nil {
		   return PartialFill{}, ErrUnavailable
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  f, ok := s.fills[id]
	   if !ok {
		  return PartialFill{},
			   ErrFillNotFound
	   }
	  return f.Copy(), nil
  }

// ListHoldsByPlan 查询计划下所有 ShardHold。

  func (s *HoldService) ListHoldsByPlan(planID string) []ShardHold {
	  if s == nil {
		   return nil
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   ids := s.byPlan[planID]
	  out := make([]ShardHold, 0, len(ids))
	  for _, id := range ids {
		   if h, ok := s.holds[id]; ok {
			  out = append(out, h.Copy())
		   }
	  }
	  return out
  }

// ListFillsByPlan 查询计划下所有 PartialFill。
   func (s *HoldService) ListFillsByPlan(planID string) []PartialFill {
	   if s == nil {
		  return nil
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   ids := s.fillsByPlan[planID]
	   out := make([]PartialFill, 0, len(ids))
	  for _, id := range ids {
		   if f, ok := s.fills[id]; ok {
			  out = append(out, f.Copy())
		  }
	  }
	   return out
   }
