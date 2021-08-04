package holdwritegate

import "fmt"

// ──────────────────────────────────────────
// BlockLease FSM: open → blocking → cleared / expired
// ──────────────────────────────────────────

type blockLeaseFSM struct {
	edges map[BlockLeaseState]map[string]BlockLeaseState
}

   func newBlockLeaseFSM() *blockLeaseFSM {
	   return &blockLeaseFSM{
		   edges: map[BlockLeaseState]map[string]BlockLeaseState{
			   BlockLeaseOpen: {
				   BlockEvActivate: BlockLeaseBlocking,
				  BlockEvExpire: BlockLeaseExpired,
			   },
			   BlockLeaseBlocking: {
				  BlockEvClear:  BlockLeaseCleared,
				   BlockEvExpire: BlockLeaseExpired,
			  },
		  },
	   }
  }

  func (f *blockLeaseFSM) Transition(from BlockLeaseState, event string) (BlockLeaseState, error) {
	  evts, ok := f.edges[from]
	  if !ok {
		  return from, fmt.Errorf("%w: block_lease from=%s event=%s", ErrFSMTransition, from, event)
	   }
	  to, ok := evts[event]
	  if !ok {
		  return from, fmt.Errorf("%w: block_lease from=%s event=%s", ErrFSMTransition, from, event)
	  }
	   return to, nil
   }

  func (f *blockLeaseFSM) CanTransition(from BlockLeaseState, event string) bool {
	   evts, ok := f.edges[from]
	  if !ok {
		  return false
	  }
	  _, ok = evts[event]
	  return ok
  }

// ──────────────────────────────────────────
// Escalation FSM: open → proven → closed
// ──────────────────────────────────────────

type escalationFSM struct {
	edges map[EscalationState]map[string]EscalationState
}

   func newEscalationFSM() *escalationFSM {
	  return &escalationFSM{
		  edges: map[EscalationState]map[string]EscalationState{
			  EscalationOpen: {
				  EscalationEvProve: EscalationProven,
			   },
			  EscalationProven: {
				   EscalationEvClose: EscalationClosed,
			   },
		  },
	  }
   }

  func (f *escalationFSM) Transition(from EscalationState, event string) (EscalationState, error) {
	   evts, ok := f.edges[from]
	  if !ok {
		   return from, fmt.Errorf("%w: escalation from=%s event=%s", ErrFSMTransition, from, event)
	  }
	  to, ok := evts[event]
	   if !ok {
		   return from, fmt.Errorf("%w: escalation from=%s event=%s", ErrFSMTransition, from, event)
	  }
	  return to, nil
   }

  func (f *escalationFSM) CanTransition(from EscalationState, event string) bool {
	  evts, ok := f.edges[from]
	   if !ok {
		  return false
	  }
	  _, ok = evts[event]
	   return ok
  }
