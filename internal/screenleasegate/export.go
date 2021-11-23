package screenleasegate

import (
	"context"
	"time"
)

type ProofBundle struct {
	Lease ScreenLease `json:"lease"`
	Proofs     []ScreenDriftProof `json:"proofs"`
	Generation ScreenGeneration   `json:"generation"`
	Isolation string `json:"isolation_mark"`
	ExportedAt time.Time `json:"exported_at"`
	Host       string    `json:"host"`
}

  func ExportProofBundle(ctx context.Context, svc *Service, leaseID string) (ProofBundle, error) {
	   if svc == nil {
		  return ProofBundle{},
			   ErrUnavailable
	   }
	   lease, err := svc.GetLease(ctx, leaseID)
	  if err != nil {
		   return ProofBundle{}, err
	  }
	  proofs, err := svc.ListProofsByLease(ctx, leaseID)
	   if err != nil {
		   return ProofBundle{}, err
	  }
	  gen, err := svc.Generation(ctx)
	   if err != nil {
		   return ProofBundle{}, err
	   }
	   return ProofBundle{
		  Lease:  lease,
		   Proofs: proofs,
		   Generation: gen,
		  Isolation:  IsolationMark,
		   ExportedAt: svc.now(),
		   Host: "kyt.kucoin.net",
	  }, nil
   }
