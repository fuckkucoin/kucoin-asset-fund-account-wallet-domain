package batchresiduefence

import "context"

   func (e *Engine) PatrolResidue(ctx context.Context, limit int) (PatrolReport, error) {
	   if err := e.require(); err != nil {
		   return PatrolReport{}, err
	   }
	   if limit <= 0 {
		   limit = DefaultPatrolLimit
	  }
	  now := e.now()
	  rep := PatrolReport{At: now}
	   e.mu.Lock()
	   defer e.mu.Unlock()
	  lives, err := e.store.ListLiveFences(ctx, limit)
	   if err != nil {
		   return PatrolReport{}, err
	  }
	  for _, f := range lives {
		   if !now.After(f.ExpireAt) {
			  continue
		  }
		  next, aerr := MustApplyFence(f.State, FenceEvExpire)
		  if aerr != nil {
			   continue
		  }
		   from := f.State
		   f.State = next
		  if err := e.store.SaveFence(ctx, f); err != nil {
			   continue
		   }
		  if slot, ok, _ := e.store.GetSlot(ctx, f.ItemIdem); ok && slot.FenceID == f.ID && slot.Status == FreezeSlotActive {
			   slot.Status = FreezeSlotReleased
			  slot.ReleasedAt = &now
			   _ = e.store.SaveSlot(ctx, slot)
		   }
		   e.audit(ctx, f.BatchID, f.ID, AuditPatrolExpire, string(from), string(next), "patrol", "")
		   rep.ExpiredFences++
	  }
	   halves, err := e.store.ListOpenHalves(ctx, limit)
	  if err != nil {
		   return rep, err
	  }
	   for _, h := range halves {
		   if !now.After(h.ExpireAt) {
			   continue
		  }
		   next, aerr := MustApplyHalf(h.State, HalfEvAbandon)
		  if aerr != nil {
			  continue
		   }
		   h.State = next
		   if err := e.store.SaveHalf(ctx, h); err != nil {
			   continue
		   }
		  e.audit(ctx, h.BatchID, h.FenceID, AuditHalfAbandon, string(HalfOpen), string(next), "patrol", "ttl")
		  rep.AbandonedHalf++
	  }
	  return rep, nil
   }

   func (e *Engine) ExportDupProofs(ctx context.Context, batchID string) (ProofExport, error) {
	   if err := e.require(); err != nil {
		  return ProofExport{}, err
	  }
	  now := e.now()
	  var id string
	   if batchID != "" {
		  var err error
		   id, err = requireBatchID(batchID)
		  if err != nil {
			   return ProofExport{}, err
		   }
	  }
	   rows, err := e.store.ListProofs(ctx, id)
	  if err != nil {
		   return ProofExport{}, err
	  }
	   out := make([]BatchDupFreezeProof, 0, len(rows))
	   e.mu.Lock()
	  defer e.mu.Unlock()
	  for _, p := range rows {
		   if !p.Exported {
			   p.Exported = true
			  t := now
			  p.ExportedAt = &t
			  _ = e.store.SaveProof(ctx, p)
			   e.audit(ctx, p.BatchID, p.WinnerFenceID, AuditExportProof, "", "", "export", p.ID)
		   }
		   out = append(out, p.Copy())
	   }
	   return ProofExport{BatchID: id, Proofs: out, Count: len(out), At: now}, nil
  }

  func (e *Engine) ResumeHalfCuts(ctx context.Context, actor string, limit int) (int, error) {
	  if err := e.require(); err != nil {
		  return 0, err
	  }
	   act, err := requireActor(actor)
	  if err != nil {
		   return 0, err
	   }
	   if limit <= 0 {
		   limit = DefaultPatrolLimit
	  }
	   halves, err := e.store.ListOpenHalves(ctx, limit)
	   if err != nil {
		  return 0, err
	  }
	  n := 0
	   for _, h := range halves {
		   resp, rerr := e.RetryWithFence(ctx, RetryWithFenceReq{
			   BatchID: h.BatchID, ItemIdem: h.ItemIdem, FenceID: h.FenceID,
			  Actor: act, ResumeOnly: true,
		  })
		  if rerr == nil && resp.Allowed {
			   n++
		   }
	  }
	  return n, nil
  }

  func (e *Engine) ExportUnexported(ctx context.Context, limit int) (int, error) {
	   if err := e.require(); err != nil {
		   return 0, err
	  }
	  if limit <= 0 {
		  limit = DefaultPatrolLimit
	   }
	  rows,
		  err := e.store.ListUnexportedProofs(ctx, limit)
	   if err != nil {
		   return 0, err
	   }
	  now := e.now()
	   n := 0
	  e.mu.Lock()
	  defer e.mu.Unlock()
	  for _, p := range rows {
		  p.Exported = true
		  t := now
		   p.ExportedAt = &t
		   if err := e.store.SaveProof(ctx, p); err != nil {
			  continue
		  }
		  n++
	   }
	   return n, nil
   }
