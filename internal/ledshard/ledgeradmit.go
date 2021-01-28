package ledshard

import (
	"fmt"
	"account-assets.kucoin.net/funding/internal/ledger"
)

// AdmitLedgerPhase 只读调用 ledger.Apply，确认协议相位没有跳过账本允许的转移。
// 不写账本行，不碰围栏令牌，也不进入补账内核。
  func AdmitLedgerPhase(cur ledger.State, ev ledger.Event) (ledger.State, error) {
	  next,
		   err := ledger.Apply(cur, ev)
	  if err != nil {
		  return cur, fmt.Errorf("%w: %s", ErrTransition, err.Error())
	  }
	  return next, nil
   }

   func admitStep(kind StepKind, froze, credited bool) (ledger.State, ledger.State, error) {
	  var cur ledger.State
	  var ev ledger.Event
	  switch kind {
	   case StepPrepare, StepFreeze:
		   cur, ev = ledger.StateInitiated, ledger.EventFreezeSpot
	   case StepCredit:
		  cur, ev = ledger.StateConfirmed, ledger.EventCredit
	  case StepAbort:
		  cur, ev = ledger.StateInitiated, ledger.EventFail
	   case StepCompensate, StepUnfreeze, StepVoid:
		  if !froze {
			  cur, ev = ledger.StateInitiated, ledger.EventFail
		  } else {
			  cur,
				   ev = ledger.StateSpotFrozen, ledger.EventRollback
		   }
	  case StepReverse:
		   if !credited {
			   return "", "", fmt.Errorf("%w: reverse without credit", ErrDoubleCredit)
		   }
		  cur, ev = ledger.StateCredited, ledger.EventReorgUnwind
	  case StepCommit, StepMint, StepDesk:
		   return "", "", nil
	  default:
		  return "", "", fmt.Errorf("%w: %s", ErrTransition, kind)
	  }
	   next, err := AdmitLedgerPhase(cur, ev)
	  if err != nil {
		   return cur, "", err
	   }
	   return cur, next, nil
   }
