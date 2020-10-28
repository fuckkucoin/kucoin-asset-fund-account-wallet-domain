package withdraw

import (
	"context"
	"fmt"
	"account-assets.kucoin.net/funding/internal/holdcat"
)

// Cancel 解冻未广播的出金。已落链 hash 走 ledger.ErrSubmittedLive，不得当成功。
  func (s *Service) Cancel(ctx context.Context, id, reason string) error {
	  if s == nil || s.led == nil {
		   return fmt.Errorf("withdraw ledger unavailable")
	   }
	   if reason == "" {
		   reason = "cancel"
	   }
	  var uid string
	   if tr, err := s.led.Get(ctx, id); err == nil && tr != nil {
		  uid = tr.UserID
	   }
	  run, ok := s.runFor(id)
	  if ok {
		  st := run.Freeze
		   if st == FreezeNone {
			  // 兼容旧 Run：按 Step 推断
			   st = freezeFromStep(run.Step)
		  }
		   if !CanCancel(st) {
			   return fmt.Errorf("%w: state=%s", ErrFreezeUnlockDenied, st)
		  }
		  if err := AssertUnlock(st); err != nil {
			   return err
		  }
	   }
	   if row, ok := s.Broadcasts().Get(id); ok {
		  if err := RefuseAutoUnfreeze(row); err != nil {
			   s.noteLiveDeny(id, row.TxHash, "cancel")
			   return err
		  }
	  }
	  if err := s.led.Cancel(ctx, id, reason); err != nil {
		  return err
	  }
	   s.dropHotIntent(id)
	   s.markIdemUnfrozen(id)
	   if s.holds != nil && uid != "" {
		  _, _ = s.holds.ReleaseByRef(ctx, uid, holdcat.ReasonPendingWithdraw, id)
	  }
	   s.releaseLimit(ctx, id, reason)
	   if ok {
		  run.UserID = uid
		   run.TransferID = id
		  _ = s.AdvanceFreeze(ctx, &run, FreezeCancelled, "withdraw.Cancel", reason)
		  run.Step = StepFailed
		   run.Err = reason
		   s.book.Put(run)
	   }
	  return nil
   }

   func freezeFromStep(step Step) FreezeState {
	  switch step {
	   case StepRequested, StepFrozen:
		  return FreezeHeld
	   case StepAllowlisted, StepDecrypted, StepKYT, StepReview:
		   return FreezeScreening
	   case StepSigned:
		   return FreezeSigning
	  case StepSubmitted:
		  return FreezeBroadcast
	   case StepConfirmed:
		   return FreezeSucceeded
	   case StepFailed:
		   return FreezeFailed
	  default:
		   return FreezeHeld
	   }
   }

  func (s *Service) RunStep(id string) (Step, bool) {
	   run, ok := s.runFor(id)
	   if !ok {
		  return "", false
	  }
	   return run.Step, true
  }

// RunFreeze 读取出金冻结态。
   func (s *Service) RunFreeze(id string) (FreezeState, bool) {
	   run, ok := s.runFor(id)
	  if !ok {
		   return "", false
	   }
	  if run.Freeze != FreezeNone {
		   return run.Freeze, true
	   }
	   return freezeFromStep(run.Step), true
  }
