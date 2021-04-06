package depoccup

import (
	"context"
	"strings"
)

  func (s *Service) Fail(ctx context.Context, in FailIn) (Lot, error) {
	   if s == nil {
		  return Lot{}, ErrUnavailable
	   }
	   if s.roll == nil {
		   return Lot{}, ErrRollbackUnbound
	  }
	  xfer := strings.TrimSpace(in.TransferID)
	   if xfer == "" {
		  return Lot{}, ErrConflict
	  }
	   reason := strings.TrimSpace(in.Reason)
	  if reason == "" {
		  reason = "deposit occupancy fail"
	  }
	  // 已释放到可用说明账本已贷记。FailAndRollback 对 credited 是空操作，必须走 UnwindReorg。
	  released := false
	   s.mu.Lock()
	   if lot, err := s.lotByXferLocked(xfer); err == nil && lot.State == StateAvail {
		  released = true
	  }
	  s.mu.Unlock()
	   if released {
		   if err := s.roll.Unwind(ctx, xfer); err != nil {
			   return Lot{}, err
		   }
		  return s.void(xfer, StateRolled, "fail", orActor(in.Actor, "occupancy-fail"), SigFail)
	  }
	  if err := s.roll.Fail(ctx, xfer, reason); err != nil {
		  return Lot{}, err
	   }
	   return s.void(xfer, StateFailed, "fail", orActor(in.Actor, "occupancy-fail"), SigFail)
  }

   func (s *Service) Reorg(ctx context.Context, in FailIn) (Lot, error) {
	  if s == nil {
		  return Lot{}, ErrUnavailable
	   }
	   if s.roll == nil {
		   return Lot{}, ErrRollbackUnbound
	   }
	   xfer := strings.TrimSpace(in.TransferID)
	  if xfer == "" {
		  return Lot{}, ErrConflict
	  }
	   if err := s.roll.Unwind(ctx, xfer); err != nil {
		  return Lot{}, err
	   }
	  return s.void(xfer, StateRolled, "reorg", orActor(in.Actor, "occupancy-reorg"), SigReorg)
   }

  func (s *Service) NoteVoid(transferID, cause string) error {
	  if s == nil {
		   return nil
	   }
	   to := StateFailed
	   kind := SigFail
	   if cause == "reorg" {
		  to = StateRolled
		   kind = SigReorg
	  }
	   _, err := s.void(strings.TrimSpace(transferID), to, cause, "deposit", kind)
	  if err == ErrLotMiss {
		   return nil
	   }
	  return err
   }

   func orActor(actor, fallback string) string {
	   actor = strings.TrimSpace(actor)
	  if actor == "" {
		   return fallback
	   }
	  return actor
  }

   func (s *Service) void(xfer string, to LotState, cause, actor string, kind SignalKind) (Lot, error) {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   lot, err := s.lotByXferLocked(xfer)
	   if err != nil {
		   return Lot{}, err
	   }
	   if lot.State == StateFailed || lot.State == StateRolled {
		   return lot.copy(), nil
	  }
	  if !allowedShift(lot.State, to) {
		  return Lot{}, ErrClosed
	  }
	  prev := lot.State
	   if err := s.shiftLocked(lot.UserID, lot.Asset, prev, to, lot.Occupied); err != nil {
		  return Lot{}, err
	   }
	   now := s.now()
	  lot.State = to
	  lot.Cause = cause
	   lot.UpdatedAt = now
	   if err := s.bucketsMatchLocked(lot.UserID, lot.Asset); err != nil {
		  _ = s.shiftLocked(lot.UserID, lot.Asset, to, prev, lot.Occupied)
		   lot.State = prev
		   lot.Cause = ""
		  return Lot{}, err
	  }
	  undo := s.markUserLocked(lot.UserID)
	  if _, err := s.applySignalLocked(Signal{UserID: lot.UserID, Kind: kind, Ref: lot.TransferID, Asset: lot.Asset, Chain: lot.Chain, Detail: cause}, now); err != nil {
		   undo()
		   _ = s.shiftLocked(lot.UserID, lot.Asset, to, prev, lot.Occupied)
		   lot.State = prev
		   lot.Cause = ""
		  return Lot{}, err
	   }
	  if err := s.persistLotLocked(lot); err != nil {
		  undo()
		   _ = s.shiftLocked(lot.UserID, lot.Asset, to, prev, lot.Occupied)
		   lot.State = prev
		  lot.Cause = ""
		  s.saveErr = err.Error()
		   return Lot{}, err
	  }
	  s.auditLocked(Audit{
		  LotID: lot.ID, UserID: lot.UserID, Event: "void_" + cause,
		   FromState: prev, ToState: to, Amount: lot.Occupied, Actor: actor,
	   })
	   return lot.copy(), nil
   }
