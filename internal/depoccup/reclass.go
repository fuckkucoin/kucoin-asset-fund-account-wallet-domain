package depoccup

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/pkg/money"
)

  func (s *Service) Reclass(ctx context.Context, transferID, source, actor string) (Lot, error) {
	  if s == nil {
		  return Lot{}, ErrUnavailable
	   }
	   src, err := ParseSource(source)
	   if err != nil || src == "*" {
		  return Lot{}, ErrSource
	   }
	  actor = orActor(actor, "")
	  if actor == "occupancy-fail" || actor == "" {
		   return Lot{}, ErrActor
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   lot, err := s.lotByXferLocked(strings.TrimSpace(transferID))
	   if err != nil {
		  return Lot{}, err
	   }
	   if lot.State != StateTransit {
		   return Lot{}, ErrClosed
	   }
	   if lot.Source == src {
		   return lot.copy(), nil
	  }
	   return s.ratchetLocked(lot, src, lot.KYC, actor)
  }

  func (s *Service) RefreshUser(ctx context.Context, userID, actor string) ([]Lot, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	   user, err := normUser(userID)
	  if err != nil {
		   return nil, err
	   }
	  actor = strings.TrimSpace(actor)
	  if actor == "" {
		   return nil, ErrActor
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   var out []Lot
	  for _, lot := range s.lots {
		  if lot == nil || lot.UserID != user || lot.State != StateTransit {
			  continue
		  }
		   got, err := s.ratchetLocked(lot, lot.Source, lot.KYC, actor)
		   if err != nil {
			  return out, err
		  }
		   out = append(out, got)
	   }
	  _ = ctx
	  return out, nil
  }

   func (s *Service) ratchetLocked(lot *Lot, src Source, kyc, actor string) (Lot, error) {
	   prevSrc := lot.Source
	   prevOcc := lot.Occupied
	   prevFree := lot.Free
	   prevBPS := lot.RatioBPS
	  prevPol := lot.PolicyID
	  prevTier := lot.Tier
	  undo := s.markUserLocked(lot.UserID)
	   if src != prevSrc {
		   s.noteSourceLocked(lot.UserID, src, lot.TransferID, s.now())
	   }
	   st := s.ensureTierLocked(lot.UserID, kyc, s.now())
	  pol, ok := s.bestLocked(Query{KYC: kyc, Asset: lot.Asset, Source: src, Chain: lot.Chain})
	   if !ok {
		  undo()
		  return Lot{}, ErrPolicyMiss
	  }
	  eff := raiseFloor(st.Tier, pol.TierFloor)
	  bps := ApplyTierBPS(pol.RatioBPS,
		  eff)
	   if src == SourceMixer {
		  bps = MaxRatioBPS
	   }
	  occ, free, err := Split(lot.Observed, bps)
	   if err != nil {
		   undo()
		  return Lot{}, err
	   }
	  old, err := money.ParseBook(lot.Occupied)
	  if err != nil {
		   undo()
		  return Lot{}, ErrInvariant
	  }
	  next, err := money.ParseBook(occ)
	  if err != nil {
		   undo()
		   return Lot{}, ErrInvariant
	  }
	  if next.Cmp(old) < 0 {
		   occ = lot.Occupied
		   free = lot.Free
		  bps = lot.RatioBPS
	   } else if next.Cmp(old) > 0 {
		   delta, err := next.Sub(old)
		  if err != nil {
			   return Lot{}, ErrInvariant
		  }
		   if err := s.addStateLocked(lot.UserID, lot.Asset, StateTransit, delta.String()); err != nil {
			  undo()
			  return Lot{}, err
		  }
	   }
	   lot.Source = src
	   lot.Occupied = occ
	  lot.Free = free
	  lot.RatioBPS = bps
	  lot.PolicyID = pol.ID
	   lot.Tier = eff
	   lot.UpdatedAt = s.now()
	  if err := lotConserves(lot.Observed, lot.Occupied, lot.Free); err != nil {
		  s.rewindRatchet(lot, prevSrc, prevOcc, prevFree, prevBPS, prevPol, prevTier, old, next)
		  undo()
		  return Lot{}, err
	  }
	  if err := s.bucketsMatchLocked(lot.UserID, lot.Asset); err != nil {
		  s.rewindRatchet(lot, prevSrc, prevOcc, prevFree, prevBPS, prevPol, prevTier, old, next)
		  undo()
		   return Lot{}, err
	  }
	  if err := s.persistLotLocked(lot); err != nil {
		  s.rewindRatchet(lot, prevSrc, prevOcc, prevFree, prevBPS, prevPol, prevTier, old, next)
		  undo()
		   s.saveErr = err.Error()
		  return Lot{}, err
	  }
	  s.auditLocked(Audit{
		  LotID: lot.ID, UserID: lot.UserID, Event: "ratchet",
		  FromState: StateTransit, ToState: StateTransit, Amount: lot.Occupied, Actor: actor,
	  })
	   st.LastSource = src
	   return lot.copy(), nil
  }

  func (s *Service) rewindRatchet(lot *Lot, src Source, occ, free string, bps int, pol string, tier Tier, old, next money.Amount) {
	  if next.Cmp(old) > 0 {
		  delta, err := next.Sub(old)
		   if err == nil {
			  _ = s.subStateLocked(lot.UserID, lot.Asset, StateTransit, delta.String())
		   }
	   }
	   lot.Source = src
	   lot.Occupied = occ
	  lot.Free = free
	  lot.RatioBPS = bps
	  lot.PolicyID = pol
	   lot.Tier = tier
   }
