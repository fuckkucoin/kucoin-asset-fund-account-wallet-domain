package batchresiduefence

import "context"

type ResidueSummary struct {
	BatchID        string `json:"batch_id"`
	LiveFences     int    `json:"live_fences"`
	ExpiredFences  int    `json:"expired_fences"`
	ReleasedFences int    `json:"released_fences"`
	OpenHalves     int    `json:"open_halves"`
	DupProofs int `json:"dup_proofs"`
	FailClosedHits int `json:"fail_closed_hits"`
}

   func (e *Engine) Summary(ctx context.Context, batchID string) (ResidueSummary, error) {
	   if err := e.require(); err != nil {
		   return ResidueSummary{}, err
	   }
	   id, err := requireBatchID(batchID)
	   if err != nil {
		   return ResidueSummary{}, err
	   }
	   snap, err := e.Snapshot(ctx, id)
	  if err != nil {
		   return ResidueSummary{}, err
	   }
	   s := ResidueSummary{BatchID: id, DupProofs: len(snap.Proofs)}
	  for _, f := range snap.Fences {
		  switch f.State {
		  case FenceIssued, FenceFrozen:
			  s.LiveFences++
		   case FenceExpired:
			  s.ExpiredFences++
		   case FenceReleased:
			  s.ReleasedFences++
		   }
	  }
	   for _, h := range snap.Halves {
		   if h.State == HalfOpen {
			   s.OpenHalves++
		   }
	  }
	   for _, a := range snap.Audits {
		  if a.Event == AuditRetryReject {
			  s.FailClosedHits++
		  }
	   }
	  return s, nil
   }
