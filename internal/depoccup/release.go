package depoccup

import (
	"context"
	"strings"
	"time"
)

   func (s *Service) Confirm(ctx context.Context, in ConfirmIn) (Lot, error) {
	   if s == nil {
		  return Lot{}, ErrUnavailable
	  }
	   xfer := strings.TrimSpace(in.TransferID)
	   if xfer == "" {
		   return Lot{}, ErrConflict
	   }
	  actor := strings.TrimSpace(in.Actor)
	  if actor == "" {
		   actor = "deposit-confirm"
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  lot, err := s.lotByXferLocked(xfer)
	  if err != nil {
		  return Lot{}, err
	   }
	  if in.Confirms > lot.Confirms {
		   lot.Confirms = in.Confirms
	   }
	   if in.EarlyCredit {
	   if lot.State != StateTransit {
		  return Lot{}, ErrClosed
	  }
	   if err := s.shiftLocked(lot.UserID, lot.Asset, StateTransit, StateAvail, lot.Occupied); err != nil {
		   return Lot{}, err
	  }
	   prev := lot.State
	   now := s.now()
	  lot.State = StateAvail
	  lot.ReleaseAt = now
	   lot.UpdatedAt = now
	  lot.Cause = ""
	  if err := s.bucketsMatchLocked(lot.UserID, lot.Asset); err != nil {
		  _ = s.shiftLocked(lot.UserID, lot.Asset, StateAvail, StateTransit, lot.Occupied)
		   lot.State = prev
		  lot.ReleaseAt = time.Time{}
		   return Lot{}, err
	   }
	   undo := s.markUserLocked(lot.UserID)
	   if _, err := s.applySignalLocked(Signal{UserID: lot.UserID, Kind: SigFullConfirm, Ref: lot.TransferID, Asset: lot.Asset, Chain: lot.Chain}, now); err != nil {
		  undo()
		  _ = s.shiftLocked(lot.UserID, lot.Asset, StateAvail, StateTransit, lot.Occupied)
		   lot.State = prev
		  lot.ReleaseAt = time.Time{}
		   return Lot{}, err
	   }
	  if err := s.persistLotLocked(lot); err != nil {
		  undo()
		  _ = s.shiftLocked(lot.UserID,
			  lot.Asset, StateAvail, StateTransit, lot.Occupied)
		   lot.State = prev
		   lot.ReleaseAt = time.Time{}
		   s.saveErr = err.Error()
		  return Lot{}, err
	   }
	  s.auditLocked(Audit{
		  LotID: lot.ID, UserID: lot.UserID, Event: "confirm_release",
		   FromState: prev,
		  ToState: StateAvail, Amount: lot.Occupied, Actor: actor,
	   })
	   _ = ctx
	   return lot.copy(), nil
   }

  func (s *Service) lotByXferLocked(xfer string) (*Lot, error) {
	  id, ok := s.byXfer[xfer]
	   if !ok {
		   return nil, ErrLotMiss
	   }
	   lot := s.lots[id]
	  if lot == nil {
		   return nil, ErrLotMiss
	  }
	  return lot, nil
   }
