package recycleepoch

import (
	"context"
	"errors"
	"fmt"
	"time"
	"github.com/google/uuid"
)

// ObserveCreditAssertEpoch 观察贷记复验。成功才允许调用方增加 available。

  func (s *Service) ObserveCreditAssertEpoch(ctx context.Context, req ObserveCreditRequest) (*ObserveRecord, error) {
	   norm, slot, err := normalizeObserve(req)
	  if err != nil {
		   return nil, err
	   }
	  if rec, ierr := s.store.GetObserveByIdem(ctx, norm.IdemKey); ierr == nil {
		  if rec.UserID != norm.UserID || rec.TxHash != norm.TxHash {
			  return nil, ErrObserveIdemConflict
		   }
		  if !rec.CreditPosted() {
			   return rec, replayDeniedObserve(rec)
		  }
		  return rec, nil
	   }
	   now := s.now()
	   cur, err := s.store.GetEpoch(ctx, slot)
	  if err != nil {
		  if errors.Is(err, ErrEpochNotFound) {
			  p := s.openProof(ctx, nil, slot, 0, norm.CarriedGen, ProofMissingEpoch, "", norm.UserID, "", norm.TxHash, norm.Asset, norm.Amount, norm.ActorID, now)
			   rec := s.persistObserve(ctx, slot, "", 0, norm, OutcomeRejected, proofID(p), now)
			   return rec, ErrEpochMissing
		  }
		   return nil, err
	  }
	   if cur.State == EpochRetired {
		   p := s.openProof(ctx, cur, slot, cur.Generation, norm.CarriedGen, ProofRevokedBind, cur.PrevUser, norm.UserID, cur.NextUser, norm.TxHash, norm.Asset, norm.Amount, norm.ActorID, now)
		  rec := s.persistObserve(ctx,
			   slot, cur.EpochID, cur.Generation, norm, OutcomeRejected, proofID(p), now)
		   return rec, ErrEpochRetired
	   }
	   if norm.CarriedGen != cur.Generation {
		   p := s.openProof(ctx, cur, slot, cur.Generation, norm.CarriedGen, ProofStaleEpoch, cur.PrevUser, norm.UserID, cur.NextUser, norm.TxHash, norm.Asset, norm.Amount, norm.ActorID, now)
		   rec := s.persistObserve(ctx, slot, cur.EpochID, cur.Generation, norm, OutcomeRejected, proofID(p), now)
		  return rec, ErrEpochStale
	   }
	   if cur.State != EpochAssigned || stringsEqual(cur.NextUser, "") {
		  // 冷却中或仅 Ready 未分配：不得按历史用户贷记。
		  if stringsEqual(norm.UserID, cur.PrevUser) || cur.State != EpochAssigned {
			  kind := ProofLateObserveOldUser
			   if cur.State == EpochCooling && !cur.CoolElapsed(now) {
				   kind = ProofForcedReuse
			  }
			   p := s.openProof(ctx, cur, slot, cur.Generation, norm.CarriedGen, kind, cur.PrevUser, norm.UserID, cur.NextUser, norm.TxHash, norm.Asset, norm.Amount, norm.ActorID, now)
			   out := OutcomeRejected
			   var ret error = ErrCreditBlocked
			  if s.disp == DispQuarantine {
				  out = OutcomeQuarantined
				   ret = ErrQuarantined
			   }
			  if kind == ProofLateObserveOldUser {
				  ret = ErrWrongUserCredit
			   }
			   if kind == ProofForcedReuse {
				  ret = ErrForcedReuse
			  }
			  rec := s.persistObserve(ctx,
				   slot, cur.EpochID, cur.Generation, norm, out, proofID(p), now)
			  return rec, ret
		  }
	  }
	  if !stringsEqual(norm.UserID, cur.NextUser) {
		   p := s.openProof(ctx, cur, slot, cur.Generation, norm.CarriedGen, ProofLateObserveOldUser, cur.PrevUser, norm.UserID, cur.NextUser, norm.TxHash, norm.Asset, norm.Amount, norm.ActorID, now)
		  out := OutcomeRejected
		   ret := ErrWrongUserCredit
		  if s.disp == DispQuarantine {
			   out = OutcomeQuarantined
			  ret = ErrQuarantined
		   }
		  rec := s.persistObserve(ctx, slot, cur.EpochID, cur.Generation, norm, out, proofID(p), now)
		   return rec, ret
	  }
	   bind,
		   berr := s.store.GetBind(ctx, slot)
	  if berr != nil || bind == nil || !bind.AllowsCredit(norm.UserID, cur.Generation) {
		   p := s.openProof(ctx, cur, slot, cur.Generation, norm.CarriedGen, ProofRevokedBind, cur.PrevUser, norm.UserID, cur.NextUser, norm.TxHash, norm.Asset, norm.Amount, norm.ActorID, now)
		  rec := s.persistObserve(ctx,
			  slot, cur.EpochID, cur.Generation, norm, OutcomeRejected, proofID(p), now)
		  return rec, ErrBindRevoked
	   }
	   rec := ObserveRecord{
		   ObserveID: uuid.NewString(),
		  Slot:       slot,
		   EpochID:    cur.EpochID,
		   Generation: cur.Generation,
		  CarriedGen: norm.CarriedGen,
		  UserID:     norm.UserID,
		  TxHash:     norm.TxHash,
		  Asset:      norm.Asset,
		  Amount:    norm.Amount,
		   Outcome:   OutcomeCredited,
		   IdemKey:   norm.IdemKey,
		   CreatedAt: now,
	  }
	  if err := s.store.InsertObserve(ctx, rec); err != nil {
		  return nil, err
	   }
	  s.audit(ctx, slot, "observe_credit", norm.ActorID, fmt.Sprintf("gen=%d user=%s credited", cur.Generation, norm.UserID), now)
	  return &rec, nil
   }

  func (s *Service) persistObserve(ctx context.Context, slot SlotKey, epochID string, gen int64, req ObserveCreditRequest, out ObserveOutcome, proofID string, now time.Time) *ObserveRecord {
	  rec := ObserveRecord{
		   ObserveID: uuid.NewString(),
		   Slot:      slot,
		   EpochID:    epochID,
		   Generation: gen,
		  CarriedGen: req.CarriedGen,
		  UserID: req.UserID,
		   TxHash: req.TxHash,
		   Asset:  req.Asset,
		  Amount:    req.Amount,
		  Outcome:   out,
		   ProofID:   proofID,
		   IdemKey:   req.IdemKey,
		  CreatedAt: now,
	   }
	   _ = s.store.InsertObserve(ctx, rec)
	   return &rec
   }

  func replayDeniedObserve(rec *ObserveRecord) error {
	  switch rec.Outcome {
	  case OutcomeQuarantined:
		  return ErrQuarantined
	  default:
		  return ErrCreditBlocked
	  }
   }

   func stringsEqual(a, b string) bool { return a == b }

  func proofID(p *MiscreditProof) string {
	   if p == nil {
		   return ""
	  }
	  return p.ProofID
  }
