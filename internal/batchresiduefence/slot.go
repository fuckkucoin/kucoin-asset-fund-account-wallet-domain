package batchresiduefence

import (
	"context"
	"strings"
)

   func (e *Engine) GetSlot(ctx context.Context, itemIdem string) (FreezeSlot, error) {
	  if err := e.require(); err != nil {
		   return FreezeSlot{}, err
	   }
	   idem, err := requireItemIdem(itemIdem)
	   if err != nil {
		   return FreezeSlot{}, err
	   }
	  s, ok, err := e.store.GetSlot(ctx, idem)
	   if err != nil {
		  return FreezeSlot{}, err
	  }
	   if !ok {
		  return FreezeSlot{}, ErrSlot
	  }
	  return s, nil
   }

   func (e *Engine) SlotActive(ctx context.Context, itemIdem string) bool {
	   s, err := e.GetSlot(ctx, itemIdem)
	   if err != nil {
		  return false
	   }
	  return s.Status == FreezeSlotActive && strings.TrimSpace(s.FenceID) != ""
  }

   func SameSlotFence(s FreezeSlot, fenceID string) bool {
	  return s.Status == FreezeSlotActive && s.FenceID == strings.TrimSpace(fenceID)
   }

  func SlotConflicts(s FreezeSlot, fenceID string) bool {
	  return s.Status == FreezeSlotActive && s.FenceID != "" && s.FenceID != strings.TrimSpace(fenceID)
  }
