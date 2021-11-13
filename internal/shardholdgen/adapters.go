package shardholdgen

import (
	"account-assets.kucoin.net/funding/internal/wdshard"
)

// BookAdapter 只读 Plan/Leg/Hold/Align/批次挂钩。

type BookAdapter struct {
	Book *wdshard.Book
}

   func (a BookAdapter) Snapshot(planID, legID string) (wdshard.Snapshot, error) {
	   if a.Book == nil {
		  return wdshard.Snapshot{},
			  wdshard.ErrUnavailable
	  }
	  return a.Book.Snapshot(planID, legID)
  }

   func (a BookAdapter) Inventory() wdshard.Inventory {
	  if a.Book == nil {
		  return wdshard.Inventory{}
	  }
	  return a.Book.Inventory()
   }

   func (a BookAdapter) LiveHash(planID, legID string) (wdshard.LiveHashFence, error) {
	  if a.Book == nil {
		  return wdshard.LiveHashFence{}, wdshard.ErrUnavailable
	  }
	  return a.Book.LiveHashFence(planID, legID)
  }

   func (a BookAdapter) BatchHook(batchID, residue string) wdshard.BatchHookView {
	  return wdshard.ReadOnlyBatchHook(batchID,
		  residue)
  }

   func (a BookAdapter) HoldFunc() wdshard.HoldAPI {
	   return wdshard.BookHoldFunc{Book: a.Book}
  }

  func (a BookAdapter) AlignHook() wdshard.AlignHook {
	  return wdshard.BookAlign{Book: a.Book}
   }

// ReadonlyBatchHook 明确不改写 wdbatch residue_fence。

   func ReadonlyBatchHook(batchID string) wdshard.BatchHookView {
	  return wdshard.ReadOnlyBatchHook(batchID, "")
  }
