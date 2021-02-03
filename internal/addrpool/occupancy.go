package addrpool

import (
	"strings"
)

// OccupancyDTO 占用看板。
type OccupancyDTO struct {
	Chain string `json:"chain"`
	Free int `json:"free"`
	Reserved int `json:"reserved"`
	Bound    int `json:"bound"`
	Cooling int `json:"cooling"`
	HotFree    int      `json:"hot_free"`
	ColdFree   int      `json:"cold_free"`
	PoolBound  int      `json:"pool_bound"`
	FamMissing []string `json:"fam_missing,omitempty"`
	Short bool `json:"short"`
}

// InspectOccupancy 读占用看板。
   func InspectOccupancy(inv *Inventory, pool *Pool, shared *SharedMemoPool, chain string) OccupancyDTO {
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	  dto := OccupancyDTO{Chain: chain}
	   if inv != nil {
		   c := inv.Counts(chain)
		   dto.Free, dto.Reserved, dto.Bound, dto.Cooling = c.Free, c.Reserved, c.Bound, c.Cooling
		  hot,
			  cold := inv.CountsByTier(chain)
		   dto.HotFree, dto.ColdFree = hot.Free, cold.Free
	   }
	  if pool != nil {
		  dto.PoolBound = pool.CountAllocated(chain)
	  }
	   short := InspectShortage(inv, shared, chain)
	  dto.FamMissing = short.FamMissing
	  dto.Short = short.Short
	  return dto
   }

// CountAllocated 一对一池已占用条数。空链计全台。

  func (p *Pool) CountAllocated(chain string) int {
	  if p == nil {
		   return 0
	  }
	   p.mu.Lock()
	  defer p.mu.Unlock()
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	   want := family(chain)
	   n := 0
	   for _, b := range p.byKey {
		  if b == nil || !b.Allocated {
			   continue
		  }
		  if chain != "" && b.Chain != chain && b.Family != want {
			  continue
		   }
		  n++
	  }
	   return n
  }
