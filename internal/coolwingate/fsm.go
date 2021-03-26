package coolwingate

import "fmt"

// ──────────────────────────────────────────
// Lease FSM: open → bound → expired
// ──────────────────────────────────────────

type leaseFSM struct {
	edges map[LeaseState]map[string]LeaseState
}

  func newLeaseFSM() *leaseFSM {
	  return &leaseFSM{
		   edges: map[LeaseState]map[string]LeaseState{
			   LeaseOpen: {
				   LeaseEvBind: LeaseBound,
				   LeaseEvExpire: LeaseExpired,
			   },
			  LeaseBound: {
				   LeaseEvExpire: LeaseExpired,
			   },
		  },
	  }
   }

  func (f *leaseFSM) Transition(from LeaseState, event string) (LeaseState, error) {
	   evts, ok := f.edges[from]
	  if !ok {
		  return from, fmt.Errorf("%w: lease from=%s event=%s", ErrFSMTransition, from, event)
	   }
	   to, ok := evts[event]
	  if !ok {
		   return from, fmt.Errorf("%w: lease from=%s event=%s", ErrFSMTransition, from, event)
	   }
	  return to, nil
  }

   func (f *leaseFSM) CanTransition(from LeaseState, event string) bool {
	  evts, ok := f.edges[from]
	   if !ok {
		  return false
	   }
	  _, ok = evts[event]
	   return ok
  }

// ──────────────────────────────────────────
// Bind FSM: open → revoked | consumed
// ──────────────────────────────────────────

type bindFSM struct {
	edges map[BindState]map[string]BindState
}

  func newBindFSM() *bindFSM {
	  return &bindFSM{
		   edges: map[BindState]map[string]BindState{
			  BindOpen: {
				   BindEvRevoke:  BindRevoked,
				   BindEvConsume: BindConsumed,
			   },
		  },
	  }
   }

  func (f *bindFSM) Transition(from BindState, event string) (BindState, error) {
	  evts, ok := f.edges[from]
	   if !ok {
		  return from, fmt.Errorf("%w: bind from=%s event=%s", ErrFSMTransition, from, event)
	  }
	  to, ok := evts[event]
	   if !ok {
		  return from, fmt.Errorf("%w: bind from=%s event=%s", ErrFSMTransition, from, event)
	   }
	   return to, nil
  }

   func (f *bindFSM) CanTransition(from BindState, event string) bool {
	  evts, ok := f.edges[from]
	   if !ok {
		   return false
	  }
	   _, ok = evts[event]
	  return ok
  }

// ──────────────────────────────────────────
// Case FSM: open → proven → closed
// ──────────────────────────────────────────

type caseFSM struct {
	edges map[CaseState]map[string]CaseState
}

   func newCaseFSM() *caseFSM {
	   return &caseFSM{
		  edges: map[CaseState]map[string]CaseState{
			   CaseOpen: {
				   CaseEvProve: CaseProven,
			   },
			  CaseProven: {
				   CaseEvClose: CaseClosed,
			   },
		   },
	  }
  }

  func (f *caseFSM) Transition(from CaseState, event string) (CaseState, error) {
	   evts, ok := f.edges[from]
	  if !ok {
		   return from, fmt.Errorf("%w: case from=%s event=%s", ErrFSMTransition, from, event)
	  }
	  to, ok := evts[event]
	   if !ok {
		  return from, fmt.Errorf("%w: case from=%s event=%s", ErrFSMTransition, from, event)
	   }
	  return to, nil
   }

  func (f *caseFSM) CanTransition(from CaseState, event string) bool {
	   evts, ok := f.edges[from]
	  if !ok {
		   return false
	  }
	  _, ok = evts[event]
	  return ok
   }
