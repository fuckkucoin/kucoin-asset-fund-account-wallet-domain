package batchresiduefence

import "account-assets.kucoin.net/funding/internal/fsm"

const (
	FenceEvFreeze = "freeze"
	FenceEvRelease = "release"
	FenceEvExpire = "expire"
	HalfEvResume = "resume"
	HalfEvAbandon = "abandon"
)

  func FenceStateMachine() *fsm.Machine[string, string] {
	  return fsm.New([]fsm.Edge[string, string]{
		   {From: string(FenceIssued), Event: FenceEvFreeze, To: string(FenceFrozen)},
		  {From: string(FenceIssued), Event: FenceEvExpire, To: string(FenceExpired)},
		   {From: string(FenceIssued),
			  Event: FenceEvRelease, To: string(FenceReleased)},
		  {From: string(FenceFrozen), Event: FenceEvRelease, To: string(FenceReleased)},
		  {From: string(FenceFrozen), Event: FenceEvExpire, To: string(FenceExpired)},
	   }, string(FenceReleased), string(FenceExpired))
   }

   func HalfStateMachine() *fsm.Machine[string, string] {
	   return fsm.New([]fsm.Edge[string, string]{
		  {From: string(HalfOpen), Event: HalfEvResume, To: string(HalfResumed)},
		   {From: string(HalfOpen), Event: HalfEvAbandon, To: string(HalfAbandoned)},
	  }, string(HalfResumed), string(HalfAbandoned))
   }

  func MustApplyFence(cur FenceState, ev string) (FenceState, error) {
	   next,
		  err := FenceStateMachine().Apply(string(cur), ev)
	  if err != nil {
		   return cur, ErrIllegalTransition
	   }
	   return FenceState(next), nil
  }

   func MustApplyHalf(cur HalfState, ev string) (HalfState, error) {
	   next, err := HalfStateMachine().Apply(string(cur), ev)
	   if err != nil {
		  return cur, ErrHalfIllegal
	  }
	   return HalfState(next), nil
  }

  func CanRetryHold(s FenceState) bool {
	  return s == FenceIssued || s == FenceFrozen
   }

   func CanIssueNextGen(s FenceState) bool {
	   return s == FenceReleased || s == FenceExpired
  }

   func TerminalFence(s FenceState) bool {
	   return FenceStateMachine().Terminal(string(s))
  }

   func TerminalHalf(s HalfState) bool {
	   return HalfStateMachine().Terminal(string(s))
  }

   func CanFreeze(s FenceState) bool { return s == FenceIssued }
  func CanRelease(s FenceState) bool {
	   return s == FenceIssued || s == FenceFrozen
   }
  func CanExpire(s FenceState) bool {
	   return s == FenceIssued || s == FenceFrozen
   }
  func CanResumeHalf(s HalfState) bool  { return s == HalfOpen }
   func CanAbandonHalf(s HalfState) bool { return s == HalfOpen }
