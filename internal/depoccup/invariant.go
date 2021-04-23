package depoccup

import "account-assets.kucoin.net/funding/pkg/money"

   func lotConserves(observed, occupied, free string) error {
	   ob, err := money.ParseBook(observed)
	   if err != nil {
		  return ErrInvariant
	   }
	  oc, err := money.ParseBook(occupied)
	   if err != nil {
		   return ErrInvariant
	  }
	  fr, err := money.ParseBook(free)
	   if err != nil {
		   return ErrInvariant
	   }
	  sum, err := oc.Add(fr)
	  if err != nil {
		   return ErrInvariant
	  }
	  if err := money.Conserve(ob, sum); err != nil {
		   return ErrInvariant
	  }
	   return nil
  }

  func (s *Service) bucketsMatchLocked(user, asset string) error {
	   var inT, av, ro money.Amount
	  for _, lot := range s.lots {
		  if lot == nil || lot.UserID != user || lot.Asset != asset {
			  continue
		   }
		  part, err := money.ParseBook(lot.Occupied)
		  if err != nil {
			   return ErrInvariant
		   }
		   switch lot.State {
		  case StateTransit:
			   inT, err = inT.Add(part)
		   case StateAvail:
			  av, err = av.Add(part)
		   case StateFailed, StateRolled:
			  ro, err = ro.Add(part)
		   default:
			  return ErrInvariant
		   }
		  if err != nil {
			  return ErrInvariant
		  }
		  if err := lotConserves(lot.Observed, lot.Occupied, lot.Free); err != nil {
			   return err
		   }
	   }
	   b := s.bucketLocked(user, asset)
	  gotIn, err := money.ParseBook(b.InTransit)
	  if err != nil {
		   return ErrInvariant
	   }
	  gotAv, err := money.ParseBook(b.Available)
	  if err != nil {
		   return ErrInvariant
	  }
	   gotRo,
		  err := money.ParseBook(b.Rolled)
	  if err != nil {
		   return ErrInvariant
	   }
	  if err := money.Conserve(gotIn, inT); err != nil {
		  return ErrInvariant
	  }
	  if err := money.Conserve(gotAv, av); err != nil {
		   return ErrInvariant
	   }
	  if err := money.Conserve(gotRo, ro); err != nil {
		   return ErrInvariant
	  }
	   return nil
   }

  func bumpAmt(cur, delta string, add bool) (string, error) {
	   c,
		  err := money.ParseBook(zeroAmt(cur))
	   if err != nil {
		   return "", ErrAmount
	  }
	  d, err := money.ParseBook(zeroAmt(delta))
	  if err != nil {
		  return "", ErrAmount
	   }
	   var next money.Amount
	   if add {
		  next, err = c.Add(d)
	  } else {
		  next, err = c.Sub(d)
	   }
	   if err != nil {
		   return "", ErrInvariant
	   }
	   return next.String(), nil
   }

   func zeroAmt(s string) string {
	   if s == "" {
		  return "0"
	   }
	  return s
   }

  func (s *Service) addStateLocked(user, asset string, state LotState, amt string) error {
	   b := s.bucketLocked(user, asset)
	  var err error
	   switch state {
	  case StateTransit:
		   b.InTransit, err = bumpAmt(b.InTransit, amt, true)
	  case StateAvail:
		  b.Available, err = bumpAmt(b.Available, amt, true)
	   case StateFailed, StateRolled:
		   b.Rolled, err = bumpAmt(b.Rolled, amt, true)
	  default:
		   return ErrInvariant
	  }
	  return err
   }

   func (s *Service) subStateLocked(user, asset string, state LotState, amt string) error {
	  b := s.bucketLocked(user, asset)
	  var err error
	  switch state {
	  case StateTransit:
		  b.InTransit, err = bumpAmt(b.InTransit, amt, false)
	   case StateAvail:
		   b.Available, err = bumpAmt(b.Available, amt, false)
	   case StateFailed, StateRolled:
		  b.Rolled, err = bumpAmt(b.Rolled, amt, false)
	  default:
		   return ErrInvariant
	   }
	  return err
  }

   func (s *Service) shiftLocked(user, asset string, from, to LotState, amt string) error {
	   if from == to {
		   return nil
	  }
	  if err := s.subStateLocked(user, asset, from, amt); err != nil {
		  return err
	   }
	   if err := s.addStateLocked(user, asset, to, amt); err != nil {
		   _ = s.addStateLocked(user, asset, from, amt)
		  return err
	   }
	  return nil
  }

   func allowedShift(from, to LotState) bool {
	  switch from {
	  case StateTransit:
		   return to == StateAvail || to == StateFailed || to == StateRolled
	  case StateAvail:
		   return to == StateRolled
	   default:
		  return false
	   }
  }
