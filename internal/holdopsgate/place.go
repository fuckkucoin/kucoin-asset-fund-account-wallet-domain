package holdopsgate

import "context"

   func (s *Service) PlaceWithLease(ctx context.Context, leaseID, actor string) (*HoldOpsLease, *HoldOpsRaceProof, error) {
	   if s == nil {
		   return nil, nil, ErrUnavailable
	   }
	   lease, err := s.store.GetLease(ctx, leaseID)
	   if err != nil {
		  return nil, nil, err
	  }
	  return s.Exec(ctx, ExecReq{
		  LeaseID: lease.ID,
		  Kind:    OpsPlace,
		   UserID:  lease.UserID,
		  Asset:   lease.Asset,
		  Amount:  lease.Amount,
		  TokenID: lease.TokenID,
		   GapID:   lease.GapID,
		  Actor:   firstNonEmpty(actor, lease.Actor),
		   Consume: true,
	  })
  }

  func seedTokenFromLease(l HoldOpsLease) ShadowToken {
	  return ShadowToken{
		   ID: l.TokenID,
		   UserID: l.UserID,
		  Asset:  l.Asset,
		  Amount: l.Amount,
		  Reason: firstNonEmpty(l.Reason, "ops_place"),
		   Ref: l.Ref,
		  GapID: l.GapID,
		   Open:  true,
	  }
  }
