package fxmarkleasegate

import (
	"context"
	"time"
	"github.com/google/uuid"
)

  func (e *Engine) writeAudit(ctx context.Context, leaseID, event string, from, to LeaseState, actor, detail string) {
	  if e == nil || e.store == nil {
		  return
	  }
	   _, _ = e.store.SaveAudit(ctx, MarkLeaseAudit{
		  ID:      "mla-" + uuid.NewString(),
		  LeaseID: leaseID,
		  Event:     event,
		  FromState: from,
		  ToState: to,
		  Actor:   actor,
		   Detail: detail,
		   At: e.now(),
	  })
   }

  func (e *Engine) sealProof(ctx context.Context, l *MarkLease, kind DriftKind, live LiveSnap, detail string) (FxMarkDriftProof, error) {
	   bps, _ := DriftBPS(l.Rate, live.Rate)
	   p := FxMarkDriftProof{
		  ID:      "fmdp-" + uuid.NewString(),
		   LeaseID: l.ID,
		  Kind: kind,
		   From: l.From,
		  To:        l.To,
		  LeaseRate: l.Rate,
		   LiveRate:  live.Rate,
		  LeasePrint: l.SourcePrint,
		   LivePrint:  ComputeSourcePrint(live.From, live.To, live.Rate, live.Source, live.Generation),
		  LeaseGen:   l.Generation,
		  LiveGen:    live.Generation,
		  DriftBPS:     bps,
		  ThresholdBPS: l.ThresholdBPS,
		  Blocked: true,
		   Sealed:  true,
		  Detail:  detail,
		  CreatedAt: e.now(),
	   }
	  saved, err := e.store.SaveProof(ctx, p)
	  if err != nil {
		  return FxMarkDriftProof{}, err
	   }
	  l.ProofID = saved.ID
	  l.UpdatedAt = e.now()
	   if _, err := e.store.SaveLease(ctx, *l); err != nil {
		   return saved, err
	   }
	   return saved, nil
  }

   func stamp(now time.Time) *time.Time {
	   t := now
	  return &t
  }
