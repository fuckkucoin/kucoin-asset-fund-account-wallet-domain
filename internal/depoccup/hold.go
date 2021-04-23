package depoccup

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
)

  func (s *Service) Observe(ctx context.Context, in ObserveIn) (Lot, error) {
	  if s == nil {
		  return Lot{}, ErrUnavailable
	   }
	  user, err := normUser(in.UserID)
	   if err != nil {
		  return Lot{}, err
	   }
	   asset, err := NormAsset(in.Asset)
	   if err != nil {
		  return Lot{}, err
	  }
	   chain, err := NormChain(in.Chain)
	   if err != nil {
		   return Lot{}, err
	   }
	   amt, err := money.Parse(in.Amount)
	   if err != nil {
		   return Lot{}, ErrAmount
	  }
	  xfer := strings.TrimSpace(in.TransferID)
	   if xfer == "" {
		  return Lot{}, ErrConflict
	  }
	  tx := strings.TrimSpace(in.TxHash)
	  if tx == "" {
		  return Lot{}, ErrConflict
	  }
	   idem := strings.TrimSpace(in.Idem)
	   src, err := s.src.Classify(in.SourceHint, in.FromAddress)
	  if err != nil {
		   return Lot{}, err
	   }
	  kyc, err := s.readKYC(ctx, user, in.KYC)
	  if err != nil {
		   return Lot{}, err
	  }
	  actor := strings.TrimSpace(in.Actor)
	   if actor == "" {
		  actor = "deposit-observe"
	   }
	   now := s.now()
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  if id, ok := s.byXfer[xfer]; ok {
		  return s.replayLocked(id, user, asset, chain, amt.String())
	   }
	   if idem != "" {
		  if id, ok := s.byIdem[idem]; ok {
			   return s.replayLocked(id, user, asset, chain, amt.String())
		   }
	  }
	   undo := s.markUserLocked(user)
	  s.noteKYCLocked(user, kyc, xfer, now)
	  s.noteSourceLocked(user, src, xfer, now)
	   reps := s.countKindLocked(user, SigDustFloor, 24*time.Hour)
	  if in.DustRepeats > reps {
		  reps = in.DustRepeats
	   }
	   if kinds, err := s.dustSignals(user, asset, amt.String(), reps+1); err != nil {
		  undo()
		   return Lot{}, err
	  } else {
		   for _, k := range kinds {
			  if _, err := s.applySignalLocked(Signal{UserID: user, Kind: k, Ref: xfer, Asset: asset, Chain: chain}, now); err != nil {
				  undo()
				   return Lot{}, err
			   }
		   }
	   }
	  st := s.ensureTierLocked(user,
		  kyc, now)
	  pol, ok := s.bestLocked(Query{KYC: kyc, Asset: asset, Source: src, Chain: chain})
	   if !ok {
		  undo()
		  return Lot{}, ErrPolicyMiss
	   }
	  eff := raiseFloor(st.Tier,
		   pol.TierFloor)
	   bps := ApplyTierBPS(pol.RatioBPS, eff)
	   if src == SourceMixer {
		  bps = MaxRatioBPS
	   }
	  occ, free, err := Split(amt.String(), bps)
	   if err != nil {
		   undo()
		   return Lot{}, err
	  }
	  deadline, err := WindowDeadline(now, pol.MaxWindow)
	   if err != nil {
		  undo()
		   return Lot{}, err
	   }
	   lot := &Lot{
		   ID: s.nextID("lot"), TransferID: xfer, Idem: idem, UserID: user,
		   Chain: chain, Asset: asset, Source: src, KYC: kyc, TxHash: tx,
		   FromAddress: strings.TrimSpace(in.FromAddress),
		   Observed: amt.String(), Occupied: occ, Free: free, State: StateTransit,
		   Tier:     eff,
		   PolicyID: pol.ID, RatioBPS: bps,
		  MaxSec: pol.MaxSec, MinSec: pol.MinSec, EarlyCredit: in.EarlyCredit,
		  Confirms: in.Confirms, FullNeed: FullNeed(chain),
		  ObservedAt: now, Deadline: deadline, UpdatedAt: now,
	  }
	  if err := lotConserves(lot.Observed, lot.Occupied, lot.Free); err != nil {
		  undo()
		  return Lot{}, err
	  }
	  if err := s.addStateLocked(user, asset, StateTransit, lot.Occupied); err != nil {
		  undo()
		  return Lot{}, err
	   }
	  s.lots[lot.ID] = lot
	   s.byXfer[xfer] = lot.ID
	  if idem != "" {
		   s.byIdem[idem] = lot.ID
	   }
	   st.LastSource = src
	  st.KYC = kyc
	  if err := s.bucketsMatchLocked(user, asset); err != nil {
		  s.dropLotLocked(lot)
		   _ = s.subStateLocked(user, asset, StateTransit, lot.Occupied)
		  undo()
		  return Lot{}, err
	  }
	  if err := s.persistLotLocked(lot); err != nil {
		  s.dropLotLocked(lot)
		   _ = s.subStateLocked(user, asset, StateTransit, lot.Occupied)
		  undo()
		   s.saveErr = err.Error()
		  return Lot{}, err
	   }
	   s.auditLocked(Audit{
		   LotID: lot.ID, UserID: user, Event: "observe_hold", ToState: StateTransit,
		   Amount: lot.Occupied, Actor: actor,
	   })
	  return lot.copy(), nil
  }

   func (s *Service) replayLocked(id, user, asset, chain, amount string) (Lot, error) {
	   lot := s.lots[id]
	  if lot == nil {
		   return Lot{}, ErrLotMiss
	  }
	  if lot.UserID != user || lot.Asset != asset || lot.Chain != chain || !sameAmt(lot.Observed, amount) {
		   return Lot{}, ErrIdem
	   }
	  return lot.copy(), nil
   }

   func sameAmt(a, b string) bool {
	  aa, err1 := money.ParseBook(a)
	   bb, err2 := money.ParseBook(b)
	  if err1 != nil || err2 != nil {
		  return a == b
	  }
	   return aa.Equal(bb)
  }

   func (s *Service) readKYC(ctx context.Context, user, explicit string) (string, error) {
	  if strings.TrimSpace(explicit) != "" {
		   kyc, err := NormKYC(explicit)
		   if err != nil || kyc == "*" {
			   return "", ErrKYC
		  }
		  return kyc, nil
	  }
	   if s.kyc != nil {
		  lv, err := s.kyc.Level(ctx, user)
		   if err != nil || strings.TrimSpace(lv) == "" {
			  return KYCNone, nil
		  }
		  kyc, err := NormKYC(lv)
		  if err != nil || kyc == "*" {
			   return KYCNone, nil
		  }
		  return kyc, nil
	   }
	   return KYCNone, nil
  }

type userUndo func()

   func (s *Service) markUserLocked(user string) userUndo {
	  var tierSnap *TierState
	  if cur := s.tiers[user]; cur != nil {
		   cp := cur.copy()
		  tierSnap = &cp
	   }
	  sigBefore := len(s.sigByUser[user])
	  moveBefore := len(s.moves[user])
	   return func() {
		  for _, id := range s.sigByUser[user][sigBefore:] {
			  delete(s.signals, id)
		   }
		   if sigBefore == 0 {
			   delete(s.sigByUser, user)
		  } else {
			  s.sigByUser[user] = s.sigByUser[user][:sigBefore]
		   }
		   if moveBefore == 0 {
			  delete(s.moves, user)
		   } else {
			   s.moves[user] = s.moves[user][:moveBefore]
		   }
		   if tierSnap == nil {
			   delete(s.tiers, user)
		  } else {
			   s.tiers[user] = tierSnap
		   }
	   }
  }

  func (s *Service) noteKYCLocked(user, kyc, ref string, now interface{ IsZero() bool }) {
	  _ = now
	   st := s.tiers[user]
	   if st == nil || st.KYC == "" || st.KYC == kyc {
		   return
	   }
	  kind := SigKYCUp
	  if KYCRank(kyc) < KYCRank(st.KYC) {
		  kind = SigKYCDown
	  }
	  _, _ = s.applySignalLocked(Signal{UserID: user, Kind: kind, Ref: ref, Detail: st.KYC + "->" + kyc}, s.now())
   }

   func (s *Service) noteSourceLocked(user string, src Source, ref string, now interface{ IsZero() bool }) {
	  _ = now
	   st := s.tiers[user]
	  if st != nil && st.LastSource != "" && st.LastSource != src && SourceRank(src) > SourceRank(st.LastSource) {
		  _, _ = s.applySignalLocked(Signal{UserID: user, Kind: SigNewSource, Ref: ref, Detail: string(st.LastSource) + "->" + string(src)}, s.now())
	  }
	   switch src {
	   case SourceMixer:
		   _, _ = s.applySignalLocked(Signal{UserID: user, Kind: SigMixer, Ref: ref}, s.now())
	   case SourceUnknown:
		   _, _ = s.applySignalLocked(Signal{UserID: user, Kind: SigUnknown, Ref: ref}, s.now())
	   case SourceContract:
		   _, _ = s.applySignalLocked(Signal{UserID: user, Kind: SigContract, Ref: ref}, s.now())
	   }
  }

  func (s *Service) dropLotLocked(lot *Lot) {
	  if lot == nil {
		   return
	   }
	  delete(s.lots, lot.ID)
	   delete(s.byXfer, lot.TransferID)
	   if lot.Idem != "" {
		  delete(s.byIdem, lot.Idem)
	  }
  }
