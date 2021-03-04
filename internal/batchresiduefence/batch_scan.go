package batchresiduefence

import "context"

type ItemScanRow struct {
	ItemIdem string `json:"item_idem"`
	Phase        ItemPhase `json:"phase"`
	HasLiveFence bool      `json:"has_live_fence"`
	HasOpenHalf  bool      `json:"has_open_half"`
	SlotActive bool `json:"slot_active"`
	CanRetry    bool   `json:"can_retry"`
	BlockReason string `json:"block_reason,omitempty"`
}

  func (e *Engine) ScanBatch(ctx context.Context, batchID string, items []BatchItemView) ([]ItemScanRow, error) {
	  if err := e.require(); err != nil {
		  return nil, err
	   }
	  id, err := requireBatchID(batchID)
	  if err != nil {
		  return nil, err
	  }
	   out := make([]ItemScanRow, 0, len(items))
	  for _, it := range items {
		  row := ItemScanRow{ItemIdem: it.ItemIdem, Phase: it.Phase}
		   if err := forbidRetryPhase(it.Phase); err != nil {
			   row.BlockReason = PolicyReason(PolicyRejectPhase)
			   if it.Phase == PhaseSubmitted {
				   row.BlockReason = ReasonItemSubmitted
			   }
			   if it.Phase == PhaseSkipped {
				   row.BlockReason = ReasonItemSkipped
			   }
			   out = append(out, row)
			  continue
		  }
		   fences, err := e.store.ListFencesByItem(ctx, id, it.ItemIdem)
		  if err != nil {
			   return nil, err
		   }
		   for _, f := range fences {
			  if CanRetryHold(f.State) && !e.now().After(f.ExpireAt) {
				  row.HasLiveFence = true
				  break
			  }
		   }
		   if h, ok, _ := e.store.GetOpenHalf(ctx, id, it.ItemIdem); ok {
			  row.HasOpenHalf = true
			  _ = h
		  }
		  if slot, ok, _ := e.store.GetSlot(ctx, it.ItemIdem); ok && slot.Status == FreezeSlotActive {
			   row.SlotActive = true
		  }
		   if !row.HasLiveFence {
			  row.BlockReason = ReasonNoFence
		  } else {
			   row.CanRetry = true
		  }
		  out = append(out, row)
	   }
	  return out, nil
  }

   func CountRetryable(rows []ItemScanRow) int {
	   n := 0
	   for _, r := range rows {
		  if r.CanRetry {
			  n++
		   }
	   }
	  return n
   }
