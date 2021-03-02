package batchresiduefence

import "time"

   func NewDupProof(id, batch, item, winner, loser, actor, fp string, at time.Time) BatchDupFreezeProof {
	   return BatchDupFreezeProof{
		  ID: id, BatchID: batch, ItemIdem: item,
		  WinnerFenceID: winner, LoserAttemptID: loser, LoserActor: actor,
		  Reason: ReasonDupFreeze, Fingerprint: fp, CreatedAt: at,
	  }
   }

  func ProofValid(p BatchDupFreezeProof) error {
	  if p.ID == "" || p.BatchID == "" || p.ItemIdem == "" {
		   return ErrProof
	   }
	   if p.WinnerFenceID == "" || p.LoserAttemptID == "" {
		  return ErrProof
	   }
	  if p.Reason == "" {
		  return ErrProof
	   }
	   return nil
   }

  func ProofsForItem(rows []BatchDupFreezeProof, itemIdem string) []BatchDupFreezeProof {
	  out := make([]BatchDupFreezeProof,
		  0)
	   for _, p := range rows {
		  if p.ItemIdem == itemIdem {
			  out = append(out, p)
		   }
	  }
	  return out
   }

   func UnexportedCount(rows []BatchDupFreezeProof) int {
	   n := 0
	  for _, p := range rows {
		  if !p.Exported {
			   n++
		   }
	  }
	   return n
  }
