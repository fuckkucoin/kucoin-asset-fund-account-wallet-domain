package usageleasegate

import (
	"context"
	"github.com/google/uuid"
)

   func (e *Engine) sealProof(ctx context.Context, l UsageLease, live WindowView, kind DriftKind, attempt, reserved string, exceed bool, detail string) UsageDriftProof {
	  p := UsageDriftProof{
		   ID: uuid.NewString(),
		   LeaseID: l.ID,
		  UserID:  l.UserID,
		   Kind:    kind,
		   OccKind: string(l.Kind),
		  Asset:   l.Asset,
		   Dest: l.Dest,
		  LeaseToday: l.TodaySum,
		   LiveToday:  live.Today,
		  LeasePrint: l.WindowPrint,
		  LivePrint: ComputeViewPrint(live),
		  LeaseCap: l.DailyCap,
		   LiveCap:        live.DailyCap,
		   AttemptAmount:  attempt,
		  ReservedAmount: reserved,
		  WouldExceed: exceed,
		   Blocked:     true,
		  Sealed:      true,
		   Detail:      detail,
		   CreatedAt:   e.now(),
	   }
	  if live.DailyCap == "" {
		  p.LiveCap = l.DailyCap
	  }
	   saved, err := e.store.SaveProof(ctx, p)
	   if err != nil {
		   return p
	  }
	  l.ProofID = saved.ID
	   l.UpdatedAt = e.now()
	   _, _ = e.store.SaveLease(ctx, l)
	   e.audit(ctx, l.ID, "proof", l.State, l.State, "engine", string(kind))
	   return saved
   }

   func (e *Engine) ExportProofs(ctx context.Context, req ProofExportReq) (ProofExportResp, error) {
	   if err := e.require(); err != nil {
		  return ProofExportResp{}, err
	   }
	   limit := req.Limit
	   var list []UsageDriftProof
	  var err error
	   switch {
	   case req.LeaseID != "":
		  list, err = e.store.ListProofsByLease(ctx, req.LeaseID, limit)
	  case req.UserID != "":
		  list, err = e.store.ListProofsByUser(ctx, req.UserID, limit)
	   default:
		   list, err = e.store.ListProofs(ctx, DriftKind(req.Kind), limit)
	  }
	   if err != nil {
		   return ProofExportResp{}, err
	  }
	   if list == nil {
		   list = []UsageDriftProof{}
	  }
	  return ProofExportResp{Proofs: list, Count: len(list)}, nil
  }
