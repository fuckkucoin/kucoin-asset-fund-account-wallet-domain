package holdopsgate

import "context"

   func (s *Service) ReleaseWithLease(ctx context.Context, leaseID, ticket string) (*HoldOpsLease, *HoldOpsRaceProof, error) {
	   if s == nil {
		   return nil, nil, ErrUnavailable
	   }
	   lease, err := s.store.GetLease(ctx, leaseID)
	  if err != nil {
		  return nil, nil, err
	   }
	  return s.Exec(ctx, ExecReq{
		   LeaseID: lease.ID,
		  Kind: OpsRelease,
		  UserID: lease.UserID,
		  Asset:   lease.Asset,
		  Amount:  lease.Amount,
		   TokenID: lease.TokenID,
		  GapID:   lease.GapID,
		  Actor: lease.Actor,
		   Ticket: ticket,
		  Consume: true,
	  })
  }

   func releaseMutexTicket(lease HoldOpsLease, ticket string) string {
	  if ticket != "" {
		  return ticket
	   }
	  return lease.ID
  }
