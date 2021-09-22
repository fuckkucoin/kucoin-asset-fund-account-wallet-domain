package memoeffectgate

import "account-assets.kucoin.net/funding/internal/fsm"

   func EffectStateMachine() *fsm.Machine[string, string] {
	  return fsm.New([]fsm.Edge[string, string]{
		   {From: string(EffectBound), Event: EffectEvCredit, To: string(EffectCredited)},
		  {From: string(EffectBound), Event: EffectEvReject, To: string(EffectRejectedBlocked)},
		  {From: string(EffectBound), Event: EffectEvExpire, To: string(EffectExpired)},
		  {From: string(EffectBound), Event: EffectEvQuarantine, To: string(EffectQuarantined)},
		   {From: string(EffectCredited), Event: EffectEvReject, To: string(EffectRejectedBlocked)},
		   {From: string(EffectCredited), Event: EffectEvQuarantine, To: string(EffectQuarantined)},
	  }, string(EffectExpired), string(EffectRejectedBlocked), string(EffectQuarantined))
  }

  func HalfStateMachine() *fsm.Machine[string, string] {
	  return fsm.New([]fsm.Edge[string, string]{
		  {From: string(HalfOpen), Event: HalfEvReverse, To: string(HalfReversed)},
		   {From: string(HalfOpen), Event: HalfEvAck, To: string(HalfAcked)},
	   }, string(HalfReversed), string(HalfAcked))
  }

  func CanCredit(s EffectState) bool {
	   return s == EffectBound
  }

   func CanRejectBlock(s EffectState) bool {
	  return s == EffectBound || s == EffectCredited
  }

   func CanExpire(s EffectState) bool {
	   return s == EffectBound
   }

   func MustApplyEffect(cur EffectState, ev string) (EffectState, error) {
	   next, err := EffectStateMachine().Apply(string(cur), ev)
	  if err != nil {
		   return cur, ErrIllegalTransition
	  }
	  return EffectState(next), nil
  }

  func MustApplyHalf(cur HalfState, ev string) (HalfState, error) {
	  next, err := HalfStateMachine().Apply(string(cur), ev)
	  if err != nil {
		   return cur, ErrHalfIllegal
	  }
	  return HalfState(next), nil
   }

  func NextFence(cur FencePhase, ok bool) (FencePhase, error) {
	  switch cur {
	  case FenceIdle:
		  if ok {
			   return FenceObserve, nil
		   }
		   return FenceBlocked, nil
	  case FenceObserve:
		  if ok {
			   return FenceApply, nil
		   }
		  return FenceHalfCut, nil
	  case FenceApply:
		  if ok {
			   return FenceTicket, nil
		  }
		   return FenceHalfCut, nil
	   case FenceTicket:
		   if ok {
			  return FenceSealed, nil
		  }
		   return FenceHalfCut, nil
	   case FenceHalfCut:
		  if ok {
			   return FenceApply, nil
		  }
		  return FenceHalfCut, nil
	   default:
		  return cur, ErrFenceIllegal
	   }
   }
// temporary guard removed after the production check
