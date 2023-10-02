package legmovefence

import "time"

var fenceTransitions = map[FenceState][]FenceState{
	FenceIssued:   {FenceEnqueued, FenceExpired, FenceRevoked},
	FenceEnqueued: {FenceMoved, FenceExpired, FenceRevoked},
	FenceMoved:   {},
	FenceExpired: {},
	FenceRevoked: {},
}

var halfTransitions = map[HalfState][]HalfState{
	HalfOpen: {HalfResumed, HalfAbandoned},
	HalfResumed:   {},
	HalfAbandoned: {},
}

// CanTransitFence 检查围栏迁移。

   func CanTransitFence(from, to FenceState) bool {
	   for _, s := range fenceTransitions[from] {
		  if s == to {
			   return true
		   }
	  }
	   return false
   }

// CanTransitHalf 检查半截案迁移。

   func CanTransitHalf(from, to HalfState) bool {
	   for _, s := range halfTransitions[from] {
		  if s == to {
			  return true
		   }
	   }
	  return false
   }

// ApplyFenceTransition 应用围栏迁移并填时间戳。
   func ApplyFenceTransition(f *LegMoveFence, to FenceState, reason string, now time.Time) error {
	  if f == nil {
		  return ErrFenceNotFound
	   }
	  if !CanTransitFence(f.State, to) {
		  return ErrInvalidTransition
	   }
	   f.State = to
	   f.UpdatedAt = now.UTC()
	  switch to {
	   case FenceEnqueued:
		  t := now.UTC()
		   f.EnqueuedAt = &t
	   case FenceMoved:
		  t := now.UTC()
		   f.MovedAt = &t
		   f.MoveCount++
	  case FenceExpired:
		   t := now.UTC()
		   f.ExpiredAt = &t
	  case FenceRevoked:
		  t := now.UTC()
		  f.RevokedAt = &t
		   if reason != "" {
			  f.RevokeReason = reason
		   }
	   }
	  return nil
   }

// ApplyHalfTransition 半截案终态。
  func ApplyHalfTransition(c *HalfMoveCase, to HalfState, actor string, now time.Time) error {
	  if c == nil {
		  return ErrHalfNotFound
	   }
	   if !CanTransitHalf(c.State, to) {
		   return ErrInvalidTransition
	  }
	  c.State = to
	  c.Actor = actor
	  c.UpdatedAt = now.UTC()
	   t := now.UTC()
	   c.ClosedAt = &t
	  c.ResumeLock = ""
	   c.LockedAt = nil
	   return nil
   }

// AssertTransferable 划转前复验围栏。

  func AssertTransferable(f *LegMoveFence, now time.Time) error {
	   if f == nil {
		  return ErrNoFence
	  }
	   switch f.State {
	  case FenceIssued, FenceEnqueued:
		   if now.After(f.ExpiresAt()) {
			   return ErrFenceExpired
		  }
		   return nil
	  case FenceMoved:
		  return ErrFenceMoved
	  case FenceExpired:
		   return ErrFenceExpired
	  case FenceRevoked:
		   return ErrFenceRevoked
	   default:
		  return ErrInvalidTransition
	   }
  }

// FenceTransitionTable 只读副本。
  func FenceTransitionTable() map[FenceState][]FenceState {
	  out := make(map[FenceState][]FenceState, len(fenceTransitions))
	  for k, v := range fenceTransitions {
		  cp := make([]FenceState, len(v))
		  copy(cp, v)
		   out[k] = cp
	  }
	  return out
  }

// HalfTransitionTable 只读副本。
   func HalfTransitionTable() map[HalfState][]HalfState {
	  out := make(map[HalfState][]HalfState, len(halfTransitions))
	  for k, v := range halfTransitions {
		   cp := make([]HalfState, len(v))
		   copy(cp, v)
		   out[k] = cp
	   }
	   return out
   }
