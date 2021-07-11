package haltepochgate

import (
	"context"
	"encoding/json"
	"time"
)

// ProofExport 证明导出摘要。

type ProofExport struct {
	GeneratedAt time.Time `json:"generated_at"`
	Count       int       `json:"count"`
	Open int `json:"open"`
	Proven int `json:"proven"`
	Closed      int         `json:"closed"`
	Proofs      []SlipProof `json:"proofs"`
	DigestChain []string    `json:"digest_chain"`
}

  func (s *Service) ExportProofs(ctx context.Context, limit int) (*ProofExport, error) {
	  if limit <= 0 {
		   limit = 200
	   }
	   all, err := s.store.ListProofs(ctx, "", limit)
	  if err != nil {
		  return nil, err
	  }
	  ex := &ProofExport{GeneratedAt: s.now(), Count: len(all), Proofs: all, DigestChain: make([]string, 0, len(all))}
	   for _, p := range all {
		   switch p.State {
		  case SlipOpen:
			  ex.Open++
		  case SlipProven:
			  ex.Proven++
		   case SlipClosed:
			  ex.Closed++
		  }
		  ex.DigestChain = append(ex.DigestChain, p.Digest)
	   }
	   return ex, nil
  }

  func (s *Service) ExportProofsJSON(ctx context.Context, limit int) ([]byte, error) {
	   ex, err := s.ExportProofs(ctx, limit)
	  if err != nil {
		   return nil, err
	   }
	   return json.MarshalIndent(ex, "", "  ")
   }

// EpochExport 世代只读快照。

type EpochExport struct {
	GeneratedAt time.Time `json:"generated_at"`
	Epochs []HaltEpoch `json:"epochs"`
}

   func (s *Service) ExportEpochs(ctx context.Context, limit int) (*EpochExport, error) {
	  list, err := s.store.ListEpochs(ctx, limit)
	   if err != nil {
		  return nil, err
	   }
	   return &EpochExport{GeneratedAt: s.now(), Epochs: list}, nil
  }

   func (s *Service) MarkExported(ctx context.Context, proofID string) error {
	   p, err := s.store.GetProof(ctx, proofID)
	  if err != nil {
		  return err
	   }
	   p.ExportedAt = s.now()
	  return s.store.PutProof(ctx, *p)
  }
