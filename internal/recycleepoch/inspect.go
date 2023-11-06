package recycleepoch

import (
	"context"
	"time"
)

// InspectFinding 巡检发现。
type InspectFinding struct {
	Slot SlotKey `json:"slot"`
	Kind string `json:"kind"`
	Generation int64  `json:"generation"`
	Detail     string `json:"detail"`
}

   func (s *Service) InspectBindings(ctx context.Context, limit int) ([]InspectFinding, error) {
	   if limit <= 0 {
		   limit = 200
	  }
	  now := s.now()
	   var out []InspectFinding
	   for _, st := range []EpochState{EpochCooling, EpochReady, EpochAssigned} {
		  rows, err := s.store.ListEpochsByState(ctx, st, limit)
		   if err != nil {
			   return nil, err
		  }
		  for _, e := range rows {
			  f := inspectOne(e, now)
			  b, berr := s.store.GetBind(ctx, e.Slot)
			  if berr == nil && b != nil {
				   if e.State == EpochAssigned && b.State == BindActive && b.UserID != e.NextUser {
					   f = append(f, InspectFinding{Slot: e.Slot, Kind: "bind_user_mismatch", Generation: e.Generation, Detail: b.UserID})
				  }
				   if e.State != EpochAssigned && b.State == BindActive {
					  f = append(f, InspectFinding{Slot: e.Slot, Kind: "active_bind_outside_assigned", Generation: e.Generation, Detail: string(e.State)})
				   }
			   }
			   out = append(out, f...)
		  }
	  }
	   return out, nil
   }

  func inspectOne(e RecycleEpoch, now time.Time) []InspectFinding {
	  var out []InspectFinding
	  if e.State == EpochCooling && e.CoolUntil.IsZero() {
		   out = append(out, InspectFinding{Slot: e.Slot, Kind: "cool_until_missing", Generation: e.Generation})
	   }
	  if e.State == EpochReady && !e.CoolElapsed(now) && e.ReadyAt != nil {
		   out = append(out, InspectFinding{Slot: e.Slot, Kind: "ready_before_cool", Generation: e.Generation})
	  }
	   if e.Generation < 1 {
		   out = append(out, InspectFinding{Slot: e.Slot, Kind: "generation_zero", Generation: e.Generation})
	   }
	  return out
  }

// PumpReadyElapsed 将已满冷却的 cooling 推进 ready（worker 与 ops 共用）。

  func (s *Service) PumpReadyElapsed(ctx context.Context, actor string, limit int) (int, error) {
	   if actor == "" {
		   actor = "recycle-epoch-pump"
	   }
	   if limit <= 0 {
		   limit = 100
	  }
	   rows, err := s.store.ListCoolingElapsed(ctx, s.now(), limit)
	   if err != nil {
		  return 0, err
	  }
	  n := 0
	  for _, e := range rows {
		  if _, err := s.ReadyAssert(ctx, ReadyRequest{Chain: e.Slot.Chain, Address: e.Slot.Address, ActorID: actor}); err == nil {
			  n++
		  }
	   }
	  return n, nil
   }
