package recycleepoch

import (
	"fmt"
	"time"
)

  func AllowedTransition(from, to EpochState) bool {
	  if to == EpochRetired {
		  return from != EpochRetired
	  }
	  switch from {
	  case "":
		   return to == EpochCooling
	   case EpochCooling:
		   return to == EpochReady
	  case EpochReady:
		  return to == EpochAssigned || to == EpochCooling
	   case EpochAssigned:
		   return to == EpochCooling
	   default:
		  return false
	   }
   }

   func (e RecycleEpoch) CanMarkAgain() bool {
	  return e.State == EpochAssigned || e.State == EpochReady
   }

  func (e RecycleEpoch) CanReady() bool {
	  return e.State == EpochCooling
  }

  func (e RecycleEpoch) CanAssign() bool {
	   return e.State == EpochReady
   }

   func NextOnMark(cur *RecycleEpoch) (EpochState, int64, error) {
	   if cur == nil {
		   return EpochCooling, 1, nil
	  }
	  if cur.State == EpochRetired {
		   return "", 0, ErrEpochRetired
	  }
	   if cur.State == EpochCooling {
		  return EpochCooling,
			  cur.Generation, nil
	   }
	  if !cur.CanMarkAgain() {
		  return "", 0, ErrStateInvalid
	  }
	   return EpochCooling, cur.Generation + 1, nil
   }

  func CheckReady(cur *RecycleEpoch, now time.Time, force bool) error {
	  if cur == nil {
		  return ErrEpochMissing
	  }
	   if cur.State == EpochReady {
		   return nil
	  }
	   if !cur.CanReady() {
		  return ErrStateInvalid
	  }
	  if !cur.CoolElapsed(now) {
		  if force {
			  return ErrForcedReuse
		  }
		   return ErrCoolNotElapsed
	  }
	   return nil
  }

  func DescribeTransition(from, to EpochState, gen int64) string {
	  return fmt.Sprintf("recycle_epoch %s→%s generation=%d", from, to, gen)
  }
