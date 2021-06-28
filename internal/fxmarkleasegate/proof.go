package fxmarkleasegate

import (
	"context"
	"strings"
)

   func (e *Engine) GetProof(ctx context.Context, id string) (FxMarkDriftProof, error) {
	  if err := e.require(); err != nil {
		   return FxMarkDriftProof{}, err
	   }
	  id = strings.TrimSpace(id)
	  if id == "" {
		  return FxMarkDriftProof{}, ErrProofNotFound
	   }
	   return e.store.GetProof(ctx, id)
  }

  func (e *Engine) ProofsForLease(ctx context.Context, leaseID string) ([]FxMarkDriftProof, error) {
	  if err := e.require(); err != nil {
		   return nil, err
	   }
	   return e.store.ListProofsByLease(ctx, strings.TrimSpace(leaseID), 50)
   }

   func ProofBlocksPost(p *FxMarkDriftProof) bool {
	  return p != nil && p.Blocked && p.Sealed
   }
