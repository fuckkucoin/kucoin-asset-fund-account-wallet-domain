package signpipegate

import "context"

  func (s *Service) ListArmed(ctx context.Context, limit int) ([]*InflightLease, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	  }
	  return s.store.ListLeasesByStatus(ctx, LeaseArmed, limit)
  }

  func (s *Service) ListInflight(ctx context.Context, limit int) ([]*InflightLease, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	   return s.store.ListLeasesByStatus(ctx, LeaseInflight, limit)
  }

  func (s *Service) ListSealed(ctx context.Context, limit int) ([]*InflightLease, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	  }
	   return s.store.ListLeasesByStatus(ctx, LeaseSealed, limit)
  }

   func (s *Service) ListMovement(ctx context.Context, movementID string) ([]*InflightLease, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	  if err := validateMovement(movementID); err != nil {
		   return nil, err
	  }
	  return s.store.ListLeasesByMovement(ctx, movementID)
   }

  func (s *Service) ListOpenFaults(ctx context.Context, limit int) ([]*FaultCase, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	  return s.store.ListOpenFaults(ctx, limit)
   }

  func (s *Service) ListOpenCloseouts(ctx context.Context,
	  limit int) ([]*CloseoutProof, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	  return s.store.ListOpenCloseouts(ctx,
		   limit)
   }
