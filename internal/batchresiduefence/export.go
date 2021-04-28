package batchresiduefence

import "context"

type BatchResidueSnapshot struct {
	BatchID string `json:"batch_id"`
	Fences []ResidueFence `json:"fences"`
	Proofs []BatchDupFreezeProof `json:"proofs"`
	Halves []HalfSubmitCase      `json:"halves"`
	Audits []AuditEvent `json:"audits"`
}

  func (e *Engine) Snapshot(ctx context.Context, batchID string) (BatchResidueSnapshot, error) {
	  if err := e.require(); err != nil {
		   return BatchResidueSnapshot{}, err
	  }
	   id, err := requireBatchID(batchID)
	   if err != nil {
		   return BatchResidueSnapshot{}, err
	  }
	  fences, err := e.store.ListFencesByBatch(ctx, id)
	  if err != nil {
		  return BatchResidueSnapshot{}, err
	   }
	  proofs, err := e.store.ListProofs(ctx, id)
	  if err != nil {
		   return BatchResidueSnapshot{}, err
	   }
	  halves, err := e.store.ListHalvesByBatch(ctx, id)
	   if err != nil {
		  return BatchResidueSnapshot{}, err
	   }
	   audits, err := e.store.ListAudits(ctx, id, 200)
	  if err != nil {
		  return BatchResidueSnapshot{}, err
	  }
	  return BatchResidueSnapshot{
		  BatchID: id, Fences: fences, Proofs: proofs, Halves: halves, Audits: audits,
	  }, nil
   }

   func CountActiveSlots(proofs []BatchDupFreezeProof) int {
	   n := 0
	   for _, p := range proofs {
		   if p.Reason == ReasonDupFreeze {
			   n++
		  }
	  }
	  return n
   }
