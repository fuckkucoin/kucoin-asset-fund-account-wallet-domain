package wdpartial

// GetHold 查询 ShardHold。
  func (s *Service) GetHold(id string) (ShardHold, error) {
	   if s == nil {
		  return ShardHold{}, ErrUnavailable
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  h, ok := s.shardHolds[id]
	  if !ok {
		   return ShardHold{}, ErrNotFound
	  }
	   return h.Copy(), nil
  }

// GetHoldByLeg 按腿 ID 查询 ShardHold。

  func (s *Service) GetHoldByLeg(legID string) (ShardHold, error) {
	   if s == nil {
		  return ShardHold{}, ErrUnavailable
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   hid, ok := s.byLeg[legID]
	   if !ok {
		  return ShardHold{}, ErrNotFound
	  }
	  h := s.shardHolds[hid]
	   if h == nil {
		   return ShardHold{}, ErrNotFound
	   }
	   return h.Copy(), nil
  }

// GetFill 查询 PartialFill。
  func (s *Service) GetFill(id string) (PartialFill, error) {
	  if s == nil {
		  return PartialFill{}, ErrUnavailable
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  f, ok := s.shardFills[id]
	   if !ok {
		  return PartialFill{}, ErrFillNotFound
	  }
	   return f.Copy(), nil
  }

// ListHoldsByPlan 查询计划下所有 ShardHold。
   func (s *Service) ListHoldsByPlan(planID string) []ShardHold {
	   if s == nil {
		  return nil
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   ids := s.byPlan[planID]
	   out := make([]ShardHold, 0, len(ids))
	  for _, id := range ids {
		  if h, ok := s.shardHolds[id]; ok {
			   out = append(out, h.Copy())
		   }
	   }
	   return out
  }

// ListFillsByPlan 查询计划下所有 PartialFill。
   func (s *Service) ListFillsByPlan(planID string) []PartialFill {
	  if s == nil {
		  return nil
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   ids := s.fillsByPlan[planID]
	   out := make([]PartialFill, 0, len(ids))
	   for _, id := range ids {
		  if f, ok := s.shardFills[id]; ok {
			   out = append(out, f.Copy())
		   }
	   }
	  return out
   }
