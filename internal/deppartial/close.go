package deppartial

import "strings"

// ──────────────────────────────────────────────────
// 批次关闭
// ──────────────────────────────────────────────────
// CloseBatch 关闭批次。所有残余必须已解决（credited/abandoned），且所有一致证明通过。
   func (svc *Service) CloseBatch(req CloseReq) (BatchResidueView, error) {
	  if svc == nil {
		   return BatchResidueView{}, ErrUnavailable
	  }
	  if err := requireActor(req.Actor); err != nil {
		  return BatchResidueView{}, err
	   }
	   batchID := strings.TrimSpace(req.BatchID)
	  if batchID == "" {
		   return BatchResidueView{}, ErrBatchNotFound
	   }
	  state,
		   ok := svc.store.GetBatchState(batchID)
	  if !ok {
		  return BatchResidueView{}, ErrBatchNotFound
	  }
	  if state == BRClosed {
		  return BatchResidueView{}, ErrAlreadyClosed
	   }
	   svc.mu.Lock()
	   defer svc.mu.Unlock()
	   openCount := svc.store.CountOpenResidues(batchID)
	   if openCount > 0 {
		   return BatchResidueView{}, ErrResidueNotDone
	  }
	  if !svc.store.AllProofsPassed(batchID) {
		  return BatchResidueView{}, ErrProofMissing
	   }
	  var ev string
	  if state == BRReopening {
		  ev = brEvResolveDone
	   } else if state == BRResidueOpen {
		  ev = brEvForceClose
	   } else {
		   return BatchResidueView{}, ErrState
	  }
	   next, err := svc.batchFSM.Apply(state, ev)
	  if err != nil {
		   return BatchResidueView{}, ErrState
	  }
	  svc.store.SetBatchState(batchID, next)
	  fences := svc.store.ListFencesByBatch(batchID)
	  for _, f := range fences {
		  svc.store.CloseFence(f.Chain, f.TxHash)
	  }
	   return svc.store.BuildView(batchID), nil
  }

// ForceClose 强制关闭批次（跳过一致证明检查）。需双人控制。

   func (svc *Service) ForceClose(batchID, actor, approver string) (BatchResidueView, error) {
	   if svc == nil {
		  return BatchResidueView{}, ErrUnavailable
	   }
	  if err := requireActor(actor); err != nil {
		   return BatchResidueView{}, err
	   }
	  if err := requireActor(approver); err != nil {
		  return BatchResidueView{}, ErrDualControl
	  }
	  if strings.TrimSpace(actor) == strings.TrimSpace(approver) {
		  return BatchResidueView{}, ErrDualControl
	  }
	   batchID = strings.TrimSpace(batchID)
	  if batchID == "" {
		  return BatchResidueView{}, ErrBatchNotFound
	  }
	   state, ok := svc.store.GetBatchState(batchID)
	  if !ok {
		  return BatchResidueView{}, ErrBatchNotFound
	  }
	   if state == BRClosed {
		   return BatchResidueView{}, ErrAlreadyClosed
	   }
	   svc.mu.Lock()
	  defer svc.mu.Unlock()
	   if state == BRResidueOpen || state == BRReopening {
		   svc.store.SetBatchState(batchID, BRClosed)
	  } else {
		   return BatchResidueView{}, ErrState
	  }
	  fences := svc.store.ListFencesByBatch(batchID)
	  for _, f := range fences {
		   svc.store.CloseFence(f.Chain, f.TxHash)
	  }
	   return svc.store.BuildView(batchID), nil
  }

// CanClose 检查批次是否满足关闭条件。

   func (svc *Service) CanClose(batchID string) (bool, string, error) {
	   if svc == nil {
		  return false, "", ErrUnavailable
	  }
	  batchID = strings.TrimSpace(batchID)
	  if batchID == "" {
		  return false, "", ErrBatchNotFound
	  }
	  state, ok := svc.store.GetBatchState(batchID)
	  if !ok {
		   return false, "", ErrBatchNotFound
	  }
	   if state == BRClosed {
		   return false, "already closed", nil
	   }
	   openCount := svc.store.CountOpenResidues(batchID)
	  if openCount > 0 {
		  return false, "open residues remain", nil
	  }
	   if !svc.store.AllProofsPassed(batchID) {
		   return false, "proof not all passed", nil
	   }
	   return true, "", nil
  }
