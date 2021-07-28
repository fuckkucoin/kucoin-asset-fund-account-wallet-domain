package holdopsgate

import "context"

  func (s *Service) AckWithLease(ctx context.Context, leaseID string) (*HoldOpsLease, *HoldOpsRaceProof, error) {
	  if s == nil {
		   return nil, nil, ErrUnavailable
	   }
	   lease, err := s.store.GetLease(ctx, leaseID)
	  if err != nil {
		  return nil, nil, err
	   }
	  return s.Exec(ctx, ExecReq{
		   LeaseID: lease.ID,
		   Kind: OpsGapAck,
		   UserID: lease.UserID,
		   Asset:  lease.Asset,
		   Amount: lease.Amount,
		  TokenID: lease.TokenID,
		  GapID:   lease.GapID,
		   Actor:   lease.Actor,
		   Consume: false,
	  })
  }

   func (s *Service) CloseWithLease(ctx context.Context, leaseID, ticket string) (*HoldOpsLease, *HoldOpsRaceProof, error) {
	  if s == nil {
		  return nil, nil, ErrUnavailable
	   }
	  lease,
		  err := s.store.GetLease(ctx, leaseID)
	  if err != nil {
		   return nil, nil, err
	   }
	  return s.Exec(ctx, ExecReq{
		   LeaseID: lease.ID,
		   Kind:    OpsGapClose,
		   UserID:  lease.UserID,
		   Asset: lease.Asset,
		   Amount:  lease.Amount,
		   TokenID: lease.TokenID,
		   GapID:   lease.GapID,
		   Actor:   lease.Actor,
		  Ticket:  ticket,
		  Consume: true,
	  })
  }

  func gapCloseDenied(openCount int) bool { return openCount > 0 }
