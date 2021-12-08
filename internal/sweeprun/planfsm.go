package sweeprun

// 计划边。键是当前态，值是允许的下一态。
var planEdges = map[PlanState]map[PlanState]struct{}{
	PlanStateDraft: {
		PlanStateGated: {},
		PlanStateRejected: {},
	},
	PlanStateGated: {
		PlanStateArmed: {},
		PlanStateRejected: {},
		PlanStateHalted: {},
	},
	PlanStateArmed: {
		PlanStateRunning:  {},
		PlanStateHalted:   {},
		PlanStateRejected: {},
	},
	PlanStateRunning: {
		PlanStatePartial:      {},
		PlanStateSettled:      {},
		PlanStateCompensating: {},
		PlanStateRejected:     {},
		PlanStateHalted:       {},
	},
	PlanStatePartial: {
		PlanStateRunning: {},
		PlanStateSettled: {},
		PlanStateCompensating: {},
		PlanStateHalted:       {},
	},
	PlanStateCompensating: {
		PlanStateCompensated: {},
		PlanStateHalted:      {},
	},
	PlanStateHalted: {
		PlanStateGated: {},
	},
}

   func AllowPlan(from, to PlanState) bool {
	   next, ok := planEdges[from]
	  if !ok {
		   return false
	   }
	   _, ok = next[to]
	  return ok
   }

  func canPlan(from, to PlanState) error {
	  if from == to {
		   return ErrState
	   }
	   if !AllowPlan(from, to) {
		  return ErrState
	   }
	   return nil
   }

// PlanInventory 列出允许边，供运维目录和证明复核使用。

   func PlanInventory() [][2]PlanState {
	   out := make([][2]PlanState, 0, 24)
	  order := []PlanState{
		  PlanStateDraft, PlanStateGated, PlanStateArmed, PlanStateRunning, PlanStatePartial,
		   PlanStateCompensating, PlanStateCompensated, PlanStateSettled, PlanStateHalted, PlanStateRejected,
	   }
	   for _, from := range order {
		  for _, to := range order {
			  if AllowPlan(from, to) {
				  out = append(out, [2]PlanState{from, to})
			  }
		   }
	   }
	  return out
   }

// DenyPlanInventory 运维禁止表。这些边必须保持拒绝，据此夹具。
   func DenyPlanInventory() [][2]PlanState {
	   return [][2]PlanState{
		   {PlanStateSettled, PlanStateRunning},
		   {PlanStateSettled, PlanStateGated},
		  {PlanStateRejected,
			  PlanStateArmed},
		  {PlanStateRejected, PlanStateRunning},
		  {PlanStateCompensated, PlanStateRunning},
		  {PlanStateCompensated, PlanStateGated},
		  {PlanStateDraft, PlanStateRunning},
		   {PlanStateDraft, PlanStateSettled},
		  {PlanStateGated, PlanStateRunning},
		  {PlanStateGated, PlanStateSettled},
		  {PlanStateArmed,
			   PlanStateSettled},
		   {PlanStateRunning, PlanStateGated},
		  {PlanStatePartial, PlanStateDraft},
		  {PlanStateCompensating, PlanStateSettled},
		   {PlanStateCompensating, PlanStateRunning},
		  {PlanStateHalted, PlanStateRunning},
		   {PlanStateHalted, PlanStateSettled},
	   }
  }
