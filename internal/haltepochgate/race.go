package haltepochgate

import (
	"context"
	"sync"
)

// RacePair 并发 Clear / Engage 夹具结果。
type RacePair struct {
	EngageEpoch int64
	ClearEpoch  int64
	EngageErr   error
	ClearErr    error
	FinalState  EpochState
	FinalGen    int64
	LaneBlocked bool
	LaneGen int64
	Proofs int
}

// RunEngageClearRace 同作用域并发 Engage 与 Clear，断言世代单调且仅一态生效。

   func (s *Service) RunEngageClearRace(ctx context.Context, scope ScopeKey, engageActor, clearActor, reason string) RacePair {
	   var out RacePair
	   var wg sync.WaitGroup
	   wg.Add(2)
	   go func() {
		  defer wg.Done()
		   ep, err := s.EngageWithEpoch(ctx, EngageRequest{
			   Scope: scope.Scope, Chain: scope.Chain, Asset: scope.Asset,
			  Actor: engageActor, Reason: reason, IdemKey: "race-engage",
		  })
		  out.EngageErr = err
		   if ep != nil {
			  out.EngageEpoch = ep.Generation
		  }
	  }()
	   go func() {
		  defer wg.Done()
		   ep, _, err := s.ClearWithEpoch(ctx, ClearRequest{
			  Scope: scope.Scope,
			  Chain: scope.Chain, Asset: scope.Asset,
			  Actor: clearActor, Reason: reason, IdemKey: "race-clear", Force: true,
		   })
		   out.ClearErr = err
		   if ep != nil {
			  out.ClearEpoch = ep.Generation
		  }
	   }()
	  wg.Wait()
	   cur, err := s.store.GetEpoch(ctx, scope)
	  if err == nil && cur != nil {
		  out.FinalState = cur.State
		  out.FinalGen = cur.Generation
		  c := cur.LaneConclusion()
		   out.LaneBlocked = c.Blocked
		   out.LaneGen = c.Generation
	   }
	   proofs,
		  _ := s.store.ListProofsByScope(ctx, scope, 50)
	  out.Proofs = len(proofs)
	  return out
   }

// LaneMatchesEpoch Lane 结论世代必须等于当前 HaltEpoch。

   func LaneMatchesEpoch(c LaneConclusion, ep HaltEpoch) bool {
	  if c.Generation != ep.Generation {
		  return false
	  }
	   if c.State != ep.State {
		   return false
	   }
	   expectBlock := ep.State.Halted() || ep.State.WindowSealed() || !ep.LaneOpen
	   return c.Blocked == expectBlock
   }

// Monotonic 两次世代快照单调。
   func Monotonic(prev, next int64) bool {
	   return next >= prev
  }

// HoldMutex 辅助：占用 Engage/Clear 互斥。
   func (s *Service) HoldMutex(ctx context.Context, scope ScopeKey, actor, kind string) error {
	   cur,
		   err := s.loadOrInit(ctx, scope)
	  if err != nil {
		   return err
	   }
	  cur.MutexHolder = actor
	  cur.MutexKind = kind
	  cur.UpdatedAt = s.now()
	  return s.store.PutEpoch(ctx, *cur)
  }
