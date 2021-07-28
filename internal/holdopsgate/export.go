package holdopsgate

import "context"

type ProofExport struct {
	Proof  HoldOpsRaceProof `json:"proof"`
	Digest string           `json:"digest"`
	HoldOpsGen int64  `json:"hold_ops_gen"`
	Isolation  string `json:"isolation_mark"`
}

   func ExportProof(svc *Service, ctx context.Context, id string) (ProofExport, error) {
	   if svc == nil {
		  return ProofExport{}, ErrUnavailable
	  }
	   p, err := svc.GetProof(ctx, id)
	   if err != nil {
		  return ProofExport{}, err
	  }
	   gen, _ := svc.GetGeneration(ctx)
	   return ProofExport{
		  Proof:  p,
		  Digest: proofDigest(p),
		  HoldOpsGen: gen.OpsGen,
		   Isolation:  IsolationMark,
	  }, nil
   }
