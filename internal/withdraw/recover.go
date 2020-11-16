package withdraw

import (
	"context"
	"errors"
	"account-assets.kucoin.net/funding/internal/cases"
	"account-assets.kucoin.net/funding/internal/ledger"
)

// Recover 扫卡住的冻结/审查/已提交单，再走 ConfirmAndCredit。
// 先 ExpireStale：过期案记 failed，不再 Confirm（过期不是放行）。
// pending / kyt review 不算失败，下次再扫。已落链 hash 的单不得走 Cancel 解冻。

  func (s *Service) Recover(ctx context.Context, book *Book, limit int) (int, error) {
	   if s == nil || book == nil {
		   return 0, nil
	  }
	   if err := book.writable(); err != nil {
		   return 0, err
	   }
	  if s.recoverHook != nil {
		  n := 0
		   for _, r := range book.StuckFrozen(limit) {
			  if r.TransferID == "" {
				   continue
			   }
			   if err := s.recoverHook(ctx, r.TransferID); err != nil {
				  _ = book.FailSeal(r, err.Error())
				  continue
			   }
			  n++
		   }
		  return n, nil
	  }
	   if s.pipe != nil {
		   s.pipe.ExpireStale()
	   }
	  n := 0
	   for _, r := range book.StuckFrozen(limit) {
		   if r.TransferID == "" {
			  continue
		   }
		  if s.pipe != nil {
			   if c, err := s.pipe.Cases().ByMovement(r.TransferID); err == nil && c != nil && c.State == cases.StateExpired {
				   r.Step = StepFailed
				   r.Err = cases.ErrExpired.Error()
				   book.Put(r)
				   continue
			  }
		  }
		  err := s.Confirm(ctx, r.TransferID)
		  if err != nil && !errors.Is(err, ledger.ErrPending) && !errors.Is(err, ledger.ErrKYTReview) {
			  r.Step = StepFailed
			  r.Err = err.Error()
			   book.Put(r)
			  continue
		  }
		   tr, gerr := s.Get(ctx, r.TransferID)
		  if gerr != nil || tr == nil {
			   continue
		   }
		   switch tr.State {
		  case ledger.StateCredited:
			  r.Step = StepConfirmed
		  case ledger.StateChainSubmitted, ledger.StateConfirmed:
			  r.Step = StepSubmitted
		  case ledger.StateWalletLocked, ledger.StateSpotFrozen:
			   if tr.KYTDecision == "review" {
				   r.Step = StepReview
			  }
		  }
		  book.Put(r)
		   n++
	   }
	   return n, nil
   }
// temporary guard removed after the production check
