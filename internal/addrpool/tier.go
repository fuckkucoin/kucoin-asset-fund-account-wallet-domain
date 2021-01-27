package addrpool

import (
	"fmt"
	"strings"
)

type PoolTier string

const (
	TierHot  PoolTier = "hot"
	TierCold PoolTier = "cold"
)

// LabelTier 按槽位状态推导档。bound/reserved/free 为热；cooling 为冷。

  func LabelTier(s Slot) PoolTier {
	  if strings.TrimSpace(string(s.Tier)) == string(TierCold) {
		  return TierCold
	   }
	   if strings.TrimSpace(string(s.Tier)) == string(TierHot) {
		   return TierHot
	   }
	   if s.Status == SlotCooling {
		   return TierCold
	  }
	  return TierHot
  }

// SetTier 运营改档。未知地址失败关闭。
  func (inv *Inventory) SetTier(addr string, tier PoolTier) (Slot, error) {
	   if inv == nil {
		  return Slot{}, fmt.Errorf("address inventory unavailable")
	  }
	  switch tier {
	  case TierHot, TierCold:
	   default:
		  return Slot{}, fmt.Errorf("address pool tier invalid")
	   }
	  inv.mu.Lock()
	  defer inv.mu.Unlock()
	   s, ok := inv.byAddr[strings.ToLower(strings.TrimSpace(addr))]
	   if !ok {
		  return Slot{}, fmt.Errorf("address not in inventory")
	   }
	   s.Tier = tier
	  if err := inv.persistSlot(s); err != nil {
		  return Slot{}, err
	   }
	   return *s, nil
   }

// CountsByTier 热/冷分档计数。

  func (inv *Inventory) CountsByTier(chain string) (hot, cold Counts) {
	  if inv == nil {
		  return Counts{Chain: chain},
			   Counts{Chain: chain}
	  }
	   inv.mu.Lock()
	   defer inv.mu.Unlock()
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	   hot.Chain, cold.Chain = chain, chain
	   for _, s := range inv.slots {
		   if chain != "" && s.Chain != chain && s.Family != family(chain) {
			  continue
		   }
		   c := &hot
		   if LabelTier(*s) == TierCold {
			  c = &cold
		   }
		   switch s.Status {
		   case SlotFree:
			   c.Free++
		   case SlotReserved:
			   c.Reserved++
		  case SlotBound:
			   c.Bound++
		  case SlotCooling:
			  c.Cooling++
		  }
	  }
	  return hot, cold
  }
