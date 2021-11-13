package signpipegate

import (
	"context"
	"time"
)

type ExportBundle struct {
	Lease *InflightLease `json:"lease"`
	Artifact *SignedArtifact `json:"artifact,omitempty"`
	Faults []FaultCase `json:"faults,omitempty"`
	Closeouts     []CloseoutProof `json:"closeouts,omitempty"`
	IsolationMark string          `json:"isolation_mark"`
	ExportedAt    time.Time       `json:"exported_at"`
}

  func (s *Service) Export(ctx context.Context, leaseID string) (*ExportBundle, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	   }
	   lease,
		   err := s.store.GetLease(ctx, leaseID)
	  if err != nil {
		  return nil, err
	   }
	   b := &ExportBundle{
		   Lease:         lease,
		   IsolationMark: IsolationMark,
		   ExportedAt:    s.now(),
	   }
	   if art, err := s.store.GetArtifactByMovement(ctx, lease.MovementID); err != nil {
		   return nil, err
	  } else if art != nil {
		   b.Artifact = art
	   }
	   faults,
		   err := s.store.ListBlockingFaults(ctx, lease.MovementID)
	  if err != nil {
		  return nil, err
	  }
	   for _, f := range faults {
		  b.Faults = append(b.Faults, f.Copy())
	  }
	  closes, err := s.store.ListBlockingCloseouts(ctx, lease.MovementID)
	  if err != nil {
		  return nil, err
	   }
	  for _, c := range closes {
		   b.Closeouts = append(b.Closeouts, c.Copy())
	  }
	  return b, nil
  }
