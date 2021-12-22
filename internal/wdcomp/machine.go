package wdcomp

import "time"

// Transit 校验结构迁移。热态与双人确认由调用方在进入 executing 之前另判。
  func Transit(from, to PlanState) error {
	  if from == to {
		  return nil
	   }
	   if !legal(from, to) {
		  return ErrState
	  }
	   return nil
  }

   func legal(from, to PlanState) bool {
	   switch from {
	   case PlanOpen:
		  return to == PlanPlanning || to == PlanAborted
	   case PlanPlanning:
		  return to == PlanExecuting || to == PlanNeedsHuman || to == PlanAborted
	   case PlanExecuting:
		  return to == PlanCommitted || to == PlanNeedsHuman || to == PlanAborted
	  case PlanNeedsHuman:
		   return to == PlanExecuting || to == PlanAborted
	   default:
		   return false
	  }
  }

  func (o *Orchestrator) transit(p *CompensatePlan, to PlanState, now time.Time) error {
	  if p == nil {
		   return ErrState
	  }
	   if err := Transit(p.State, to); err != nil {
		   return err
	  }
	  p.State = to
	   p.UpdatedAt = now
	  return nil
  }

  func legsReady(legs []*CompensateLeg) bool {
	  if len(legs) == 0 {
		  return false
	   }
	  for _, leg := range legs {
		  if leg.Status != LegDone && leg.Status != LegSkipped {
			   return false
		  }
	  }
	   return true
   }

  func isMoneyKind(k LegKind) bool {
	   switch k {
	  case LegUnfreeze, LegFee, LegOccupancy, LegShard:
		   return true
	   default:
		  return false
	   }
   }
