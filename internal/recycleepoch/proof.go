package recycleepoch

import (
	"context"
	"fmt"
	"time"
	"github.com/google/uuid"
)

  func (s *Service) openProof(ctx context.Context, e *RecycleEpoch, slot SlotKey, gen, carried int64, kind ProofKind, prev, attempt, current, tx, asset, amount, actor string, now time.Time) *MiscreditProof {
	   epochID := ""
	  if e != nil {
		  epochID = e.EpochID
		  if gen == 0 {
			  gen = e.Generation
		   }
		  if prev == "" {
			   prev = e.PrevUser
		  }
		   if current == "" {
			   current = e.NextUser
		   }
	  }
	   summary := fmt.Sprintf("%s slot=%s gen=%d carried=%d prev=%s attempt=%s current=%s fail_closed=true",
		  kind,
		   slot.String(), gen, carried, prev, attempt, current)
	  p := MiscreditProof{
		  ProofID: uuid.NewString(),
		  Slot: slot,
		   EpochID:    epochID,
		   Generation: gen,
		   CarriedGen: carried,
		   Kind:  kind,
		  State: ProofOpen,
		   PrevUser:    prev,
		   AttemptUser: attempt,
		  CurrentUser: current,
		   TxHash: tx,
		  Asset:  asset,
		  Amount: amount,
		  Disposition: s.disp,
		   FailClosed:  true,
		   Summary:     summary,
		  OpenedAt: now,
	   }
	  if err := s.store.InsertProof(ctx, p); err != nil {
		   return nil
	  }
	  s.audit(ctx, slot, "miscredit_open", actor, summary, now)
	   return &p
   }

   func (s *Service) AckProof(ctx context.Context, req AckProofRequest) (*MiscreditProof, error) {
	  if req.ProofID == "" || req.ActorID == "" {
		  return nil, ErrInvalidRequest
	   }
	   now := s.now()
	  if err := s.store.AckProof(ctx, req.ProofID, now); err != nil {
		  return nil, err
	   }
	  return s.store.GetProof(ctx,
		  req.ProofID)
   }

   func (s *Service) DrainOpenProofs(ctx context.Context, limit int) ([]MiscreditProof, error) {
	   if limit <= 0 {
		  limit = 200
	   }
	   return s.store.ListOpenProofs(ctx, limit)
  }

  func (s *Service) ExportProofs(ctx context.Context, limit int) ([]MiscreditProof, error) {
	   if limit <= 0 {
		  limit = 200
	  }
	   ps,
		   err := s.store.ListOpenProofs(ctx, limit)
	  if err != nil {
		   return nil, err
	   }
	  // 同时导出已开证明（含按槽查询的全集上限）。
	   now := s.now()
	   for i := range ps {
		   _ = s.store.MarkProofExported(ctx, ps[i].ProofID, now)
	   }
	   return ps, nil
   }

   func (s *Service) GetProof(ctx context.Context, proofID string) (*MiscreditProof, error) {
	  if proofID == "" {
		   return nil, ErrProofNotFound
	  }
	  return s.store.GetProof(ctx, proofID)
  }

  func (s *Service) ListProofsBySlot(ctx context.Context, chain, address string) ([]MiscreditProof, error) {
	  slot := NormalizeSlot(chain, address)
	   if slot.Empty() {
		  return nil, ErrAddressEmpty
	  }
	   return s.store.ListProofsBySlot(ctx, slot)
   }
