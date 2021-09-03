package memoeffectgate

import "context"

   func OccupiedByOther(ctx context.Context, mux OutputMutex, key, holder string) (bool, Occupancy, error) {
	   if mux == nil {
		  return true, Occupancy{}, ErrMutexUnavailable
	   }
	  occ, ok, err := mux.Lookup(ctx, key)
	   if err != nil {
		   return true, occ, err
	  }
	   if !ok || !occ.Taken {
		  return false, occ, nil
	  }
	   if occ.Holder == holder {
		   return false, occ, nil
	  }
	   return true, occ, nil
   }

  func AutoClaimed(occ Occupancy) bool {
	   return occ.Taken && (occ.AutoClaimed || occ.Holder == HolderAutoClaim)
   }

  func SameLeg(occ Occupancy, holder, ticketID string) bool {
	   if !occ.Taken {
		  return false
	  }
	   if occ.Holder != holder {
		  return false
	  }
	  if ticketID != "" && occ.TicketID != "" && occ.TicketID != ticketID {
		  return false
	   }
	  return true
  }

   func OccupyFail(occ Occupancy) error {
	  if !occ.Taken {
		  return nil
	  }
	   if AutoClaimed(occ) {
		   return ErrAutoClaimOccupied
	   }
	  return ErrOutputOccupied
   }

   func MustFreeOutput(ctx context.Context, mux OutputMutex, chain, hash string, idx int) error {
	   key, err := CanonicalOutputKey(chain, hash, idx)
	  if err != nil {
		  return err
	   }
	  busy, occ, err := OccupiedByOther(ctx, mux, key, HolderMemoApprove)
	   if err != nil {
		   return err
	   }
	   if busy {
		  return OccupyFail(occ)
	   }
	  return nil
   }
