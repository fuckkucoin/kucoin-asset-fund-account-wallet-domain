package batchresiduefence

import "context"

   func (e *Engine) SubmitAssert(ctx context.Context, req SubmitAssertReq) (SubmitAssertResp, error) {
	   if err := e.require(); err != nil {
		  return SubmitAssertResp{}, err
	  }
	  batchID, err := requireBatchID(req.BatchID)
	  if err != nil {
		  return SubmitAssertResp{}, err
	   }
	   itemIdem, err := requireItemIdem(req.ItemIdem)
	   if err != nil {
		   return SubmitAssertResp{}, err
	   }
	  actor, err := requireActor(req.Actor)
	   if err != nil {
		   return SubmitAssertResp{}, err
	  }
	  e.mu.Lock()
	  defer e.mu.Unlock()
	  if e.mutexAlive(ctx, batchID, MutexRetry) {
		  return SubmitAssertResp{OK: false,
			  Reason: ReasonSubmitRetryMutex}, ErrMutexHeld
	   }
	   if err := e.AcquireMutex(ctx, batchID, MutexSubmit, actor); err != nil {
		  return SubmitAssertResp{OK: false, Reason: ReasonSubmitRetryMutex}, err
	  }
	   defer func() {
		  _ = e.ReleaseMutex(ctx,
			  batchID, actor)
	  }()
	  e.audit(ctx, batchID, "", AuditSubmitAssert, "", string(req.PhaseAfter), actor, clipReason(req.CutReason))
	  if !req.HalfCut {
		  if req.PhaseAfter == PhaseFailed {
			  iss, ierr := e.issueLocked(ctx, IssueFenceReq{
				   BatchID: batchID, ItemIdem: itemIdem, Actor: actor,
				  UserID: orDef(req.UserID, "unknown"), Asset: orDef(req.Asset, "UNK"),
				  Amount: orDef(req.Amount, "1"), OriginalFreezeRef: req.FreezeRef,
			   })
			  if ierr != nil && ierr != ErrItemSubmitted && ierr != ErrItemSkipped {
				  if iss.ID != "" {
					  return SubmitAssertResp{OK: true, Fence: &iss}, nil
				   }
			  }
			  if ierr == nil {
				   return SubmitAssertResp{OK: true,
					  Fence: &iss}, nil
			  }
		   }
		  return SubmitAssertResp{OK: true}, nil
	   }
	  user := orDef(req.UserID, "unknown")
	  asset := orDef(req.Asset, "UNK")
	   amt := orDef(req.Amount, "1")
	  fence, ferr := e.issueLocked(ctx, IssueFenceReq{
		   BatchID: batchID, ItemIdem: itemIdem, Actor: actor,
		   UserID: user, Asset: asset, Amount: amt, OriginalFreezeRef: req.FreezeRef,
	  })
	   if ferr != nil && fence.ID == "" {
		   return SubmitAssertResp{OK: false, Reason: ferr.Error()}, ferr
	   }
	   now := e.now()
	  half := HalfSubmitCase{
		  ID: e.newID(), BatchID: batchID, ItemIdem: itemIdem, FenceID: fence.ID,
		  State: HalfOpen, CutAt: now, ExpireAt: now.Add(e.halfTTL),
		  Actor: actor, CutReason: clipReason(req.CutReason), FullFrozen: true,
	   }
	  if err := e.store.SaveHalf(ctx, half); err != nil {
		  return SubmitAssertResp{}, err
	  }
	  if req.FreezeRef != "" {
		  _ = e.store.SaveSlot(ctx, FreezeSlot{
			  ItemIdem: itemIdem, BatchID: batchID, FenceID: fence.ID,
			  Fingerprint: fence.FreezeFingerprint, Status: FreezeSlotActive, HeldAt: now,
		   })
	  }
	  e.audit(ctx, batchID, fence.ID, AuditHalfOpen, "", string(HalfOpen), actor, half.CutReason)
	   fp := fence
	  hp := half
	   return SubmitAssertResp{OK: true, Fence: &fp, Half: &hp}, nil
   }

   func (e *Engine) issueLocked(ctx context.Context, req IssueFenceReq) (ResidueFence, error) {
	   exist, err := e.store.ListFencesByItem(ctx, req.BatchID, req.ItemIdem)
	  if err != nil {
		   return ResidueFence{}, err
	  }
	   gen := 1
	  for _, f := range exist {
		   if CanRetryHold(f.State) {
			   return f, nil
		  }
		  if f.Generation >= gen {
			  gen = f.Generation + 1
		   }
	   }
	   ttl := e.fenceTTL
	  now := e.now()
	   fp := FreezeFingerprint(req.BatchID, req.ItemIdem, req.UserID, req.Asset, req.Amount, req.OriginalFreezeRef)
	  row := ResidueFence{
		  ID: e.newID(), BatchID: req.BatchID, ItemIdem: req.ItemIdem,
		  FreezeFingerprint: fp, Generation: gen, TTL: ttl.Milliseconds(),
		  IssuedAt: now, ExpireAt: now.Add(ttl), State: FenceIssued,
		   Actor: req.Actor, UserID: req.UserID, Asset: req.Asset, Amount: req.Amount,
		   OriginalFreezeRef: req.OriginalFreezeRef,
	   }
	  if err := e.store.SaveFence(ctx, row); err != nil {
		  return ResidueFence{}, err
	   }
	   e.audit(ctx, req.BatchID, row.ID, AuditIssueFence, "", string(FenceIssued), req.Actor, fp)
	  return row, nil
  }

   func orDef(s, d string) string {
	   if s == "" {
		   return d
	  }
	   return s
   }
