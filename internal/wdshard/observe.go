package wdshard

import "strings"

// Observe 只读批次残差挂钩。不改写 wdbatch E 包残差围栏。
type BatchHookView struct {
	BatchID string `json:"batch_id,omitempty"`
	ResidueRef string `json:"residue_ref,omitempty"`
	Readonly bool `json:"readonly"`
}

   func ReadOnlyBatchHook(batchID, residueRef string) BatchHookView {
	  return BatchHookView{
		   BatchID:    strings.TrimSpace(batchID),
		   ResidueRef: strings.TrimSpace(residueRef),
		   Readonly:   true,
	  }
  }

// LiveHashFence 在播腿世代挡写摘要视图。
type LiveHashFence struct {
	PlanID string
	LegID  string
	HoldID string
	LiveHash string
	Status LegStatus
}

  func (b *Book) LiveHashFence(planID, legID string) (LiveHashFence, error) {
	  snap, err := b.Snapshot(planID, legID)
	   if err != nil {
		  return LiveHashFence{}, err
	  }
	  holdID := snap.Leg.HoldID
	  return LiveHashFence{
		  PlanID: snap.Plan.PlanID, LegID: snap.Leg.LegID, HoldID: holdID,
		   LiveHash: snap.Leg.LiveHash,
		   Status:   snap.Leg.Status,
	  }, nil
   }

// Inventory 统计。

type Inventory struct {
	Plans int
	Legs int
	ActiveHolds int
	Broadcasting int
	Failed int
	Released int
	Filled   int
}

   func (b *Book) Inventory() Inventory {
	  if b == nil {
		  return Inventory{}
	   }
	  b.mu.RLock()
	   defer b.mu.RUnlock()
	  inv := Inventory{Plans: len(b.plans), Legs: len(b.legs)}
	  for _, l := range b.legs {
		   switch l.Status {
		  case HoldLegBroadcasting:
			   inv.Broadcasting++
		   case HoldLegFailed:
			  inv.Failed++
		  case HoldLegReleased:
			  inv.Released++
		   case HoldLegFilled:
			  inv.Filled++
		   }
	  }
	   for _, h := range b.holds {
		  if h.Active && h.Residual() > 0 {
			  inv.ActiveHolds++
		  }
	  }
	  return inv
   }
