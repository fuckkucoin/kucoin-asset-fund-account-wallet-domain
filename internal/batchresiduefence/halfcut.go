package batchresiduefence

import "context"

   func (e *Engine) GetOpenHalf(ctx context.Context, batchID, itemIdem string) (HalfSubmitCase, error) {
	   if err := e.require(); err != nil {
		  return HalfSubmitCase{}, err
	   }
	  id, err := requireBatchID(batchID)
	  if err != nil {
		  return HalfSubmitCase{}, err
	   }
	  idem, err := requireItemIdem(itemIdem)
	   if err != nil {
		   return HalfSubmitCase{}, err
	  }
	   h, ok, err := e.store.GetOpenHalf(ctx, id, idem)
	  if err != nil {
		  return HalfSubmitCase{}, err
	  }
	   if !ok {
		   return HalfSubmitCase{}, ErrHalf
	  }
	   return h, nil
   }

  func (e *Engine) AbandonHalf(ctx context.Context, batchID, itemIdem, actor string) (HalfSubmitCase, error) {
	   if err := e.require(); err != nil {
		  return HalfSubmitCase{}, err
	  }
	  id, err := requireBatchID(batchID)
	   if err != nil {
		  return HalfSubmitCase{}, err
	   }
	  idem, err := requireItemIdem(itemIdem)
	   if err != nil {
		   return HalfSubmitCase{}, err
	   }
	  act, err := requireActor(actor)
	   if err != nil {
		  return HalfSubmitCase{}, err
	  }
	  e.mu.Lock()
	  defer e.mu.Unlock()
	   h, ok, err := e.store.GetOpenHalf(ctx, id, idem)
	  if err != nil {
		  return HalfSubmitCase{}, err
	   }
	  if !ok {
		  return HalfSubmitCase{},
			   ErrHalf
	   }
	  next, err := MustApplyHalf(h.State, HalfEvAbandon)
	  if err != nil {
		   return HalfSubmitCase{}, err
	  }
	  h.State = next
	  if err := e.store.SaveHalf(ctx, h); err != nil {
		  return HalfSubmitCase{}, err
	   }
	  e.audit(ctx, id, h.FenceID, AuditHalfAbandon, string(HalfOpen), string(next), act, "manual")
	   return h, nil
  }
