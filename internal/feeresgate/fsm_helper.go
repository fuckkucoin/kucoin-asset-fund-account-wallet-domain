package feeresgate

import "account-assets.kucoin.net/funding/internal/feereserve"

// leaseFSMHelper 封装 Lease FSM，避免每次调用重建。
type leaseFSMHelper struct {
	m *leaseMachine
}

type leaseMachine struct {
	edges map[feereserve.LeaseState]map[string]feereserve.LeaseState
}

  func newLeaseFSMHelper() *leaseFSMHelper {
	  m := &leaseMachine{
		   edges: map[feereserve.LeaseState]map[string]feereserve.LeaseState{
			   feereserve.LeaseReserved: {
				   feereserve.LeaseEvSettle:  feereserve.LeaseSettled,
				  feereserve.LeaseEvRelease: feereserve.LeaseReleased,
				  feereserve.LeaseEvBlock: feereserve.LeaseBlocked,
			   },
			   feereserve.LeaseSettled: {
				  feereserve.LeaseEvBlock: feereserve.LeaseBlocked,
			  },
			   feereserve.LeaseReleased: {
				   feereserve.LeaseEvBlock: feereserve.LeaseBlocked,
			   },
		   },
	  }
	   return &leaseFSMHelper{m: m}
   }

   func (h *leaseFSMHelper) apply(cur feereserve.LeaseState, ev string) (feereserve.LeaseState, error) {
	  evMap, ok := h.m.edges[cur]
	   if !ok {
		  return cur, feereserve.ErrState
	   }
	   next, ok := evMap[ev]
	  if !ok {
		  return cur, feereserve.ErrState
	  }
	   return next, nil
  }

// caseFSMHelper 封装 Case FSM。
type caseFSMHelper struct {
	edges map[feereserve.CaseState]map[string]feereserve.CaseState
}

  func newCaseFSMHelper() *caseFSMHelper {
	   return &caseFSMHelper{
		   edges: map[feereserve.CaseState]map[string]feereserve.CaseState{
			  feereserve.CaseOpen: {
				   feereserve.CaseEvProve: feereserve.CaseProven,
				  feereserve.CaseEvClose: feereserve.CaseClosed,
			   },
			  feereserve.CaseProven: {
				  feereserve.CaseEvClose: feereserve.CaseClosed,
			   },
		   },
	  }
   }

   func (h *caseFSMHelper) apply(cur feereserve.CaseState, ev string) (feereserve.CaseState, error) {
	   evMap, ok := h.edges[cur]
	  if !ok {
		   return cur, feereserve.ErrState
	   }
	  next, ok := evMap[ev]
	   if !ok {
		  return cur, feereserve.ErrState
	  }
	  return next, nil
  }
