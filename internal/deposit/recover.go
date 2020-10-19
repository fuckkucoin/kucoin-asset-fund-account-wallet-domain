// 充值入账域之故障恢复。

// Author: Blaise
// Date: 2020.10.18

package deposit

import (
	"context"
	"errors"
	"account-assets.kucoin.net/funding/internal/ledger"
)

  func (s *Service) recoverConfirm(ctx context.Context, id string) error {
	   if s != nil && s.recoverHook != nil {
		   return s.recoverHook(ctx, id)
	   }
	   return s.Confirm(ctx, id)
  }

// Recover 扫未入账的观察单，再走 ConfirmAndCredit。
// pending / 观察不一致不算本轮扫描失败，下次再扫。链 RPC 仍在 Confirm 的事务外。
  func (s *Service) Recover(ctx context.Context, book *Book, limit int) (int, error) {
	  if s == nil || book == nil {
		   return 0, nil
	  }
	  if err := book.writable(); err != nil {
		  return 0, err
	   }
	   n := 0
	  for _, r := range book.Pending(limit) {
		   if r.TransferID == "" {
			   continue
		  }
		  err := s.recoverConfirm(ctx, r.TransferID)
		   if err != nil && !errors.Is(err, ledger.ErrPending) && !errors.Is(err, ledger.ErrObservationMismatch) {
			   r.Step = StepFailed
			   r.Err = err.Error()
			  book.Put(r)
			  continue
		   }
		  if s.recoverHook != nil && err == nil {
			  n++
			   continue
		   }
		   tr,
			   gerr := s.Get(ctx, r.TransferID)
		   if gerr != nil || tr == nil {
			   continue
		   }
		  switch tr.State {
		   case ledger.StateCredited:
			   r.Step = StepCredited
		   case ledger.StateHold:
			  r.Step = StepHeld
		  case ledger.StateReorged:
			   r.Step = StepUnwound
		   case ledger.StateConfirmed:
			   r.Step = StepConfirmed
		   }
		  r.Err = ""
		   book.Put(r)
		   n++
	   }
	  return n, nil
  }
