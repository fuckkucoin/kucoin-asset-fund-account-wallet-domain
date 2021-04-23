package depoccup

import (
	"context"
	"account-assets.kucoin.net/funding/internal/deposit"
)

  func (s *Service) NoteObserved(ctx context.Context, in deposit.OccObserve) error {
	   if s == nil {
		   return nil
	  }
	  _, err := s.Observe(ctx, ObserveIn{
		   TransferID: in.TransferID, Idem: in.Idem, UserID: in.UserID,
		   Chain: in.Chain, Asset: in.Asset, Amount: in.Amount, TxHash: in.TxHash,
		   SourceHint:  in.SourceHint,
		   FromAddress: in.FromAddress, Confirms: in.Confirms,
		  EarlyCredit: in.EarlyCredit, KYC: in.KYC, Actor: "deposit-observe",
		   DustRepeats: in.DustRepeats,
	   })
	  if err != nil {
		  s.bumpNoteErr()
	  }
	   return err
   }

   func (s *Service) NoteConfirmed(ctx context.Context, in deposit.OccConfirm) error {
	  if s == nil {
		   return nil
	   }
	  _, err := s.Confirm(ctx, ConfirmIn{
		   TransferID: in.TransferID, Confirms: in.Confirms, EarlyCredit: in.EarlyCredit, Actor: "deposit-confirm",
	   })
	   if err != nil {
		  s.bumpNoteErr()
	   }
	  return err
   }

   func (s *Service) NoteUnwound(ctx context.Context, transferID string) error {
	   if s == nil {
		   return nil
	  }
	  err := s.NoteVoid(transferID, "reorg")
	   if err != nil {
		  s.bumpNoteErr()
	  }
	   _ = ctx
	   return err
  }

   func (s *Service) NoteFailed(ctx context.Context, transferID, reason string) error {
	  if s == nil {
		  return nil
	  }
	  err := s.NoteVoid(transferID, "fail")
	  if err != nil {
		   s.bumpNoteErr()
	   }
	   _ = reason
	   _ = ctx
	   return err
   }
