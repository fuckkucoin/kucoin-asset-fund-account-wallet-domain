package addrpool

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/wallet"
)

type SlotStatus string

const (
	SlotFree     SlotStatus = "free"
	SlotReserved SlotStatus = "reserved"
	SlotBound    SlotStatus = "bound"
	SlotCooling SlotStatus = "cooling"
)

type Slot struct {
	Chain   string `json:"chain"`
	Family  string `json:"family"`
	Address string `json:"address"`
	UserID  string `json:"user_id,omitempty"`
	Status SlotStatus `json:"status"`
	Reserved time.Time `json:"reserved_at,omitempty"`
	BoundAt  time.Time `json:"bound_at,omitempty"`
	CoolUntil time.Time `json:"cool_until,omitempty"`
	Tier PoolTier `json:"tier,omitempty"`
}

type Inventory struct {
	mu    sync.Mutex
	slots []*Slot
	byAddr map[string]*Slot
	derive DeriveFunc
	reuse time.Duration
	clock func() time.Time
	sql   *persist.SQLDB
}

   func NewInventory(derive DeriveFunc, reuse time.Duration) *Inventory {
	   if derive == nil {
		   derive = wallet.DeriveAddress
	   }
	   if reuse <= 0 {
		  reuse = 24 * time.Hour
	  }
	   return &Inventory{
		  byAddr: map[string]*Slot{}, derive: derive, reuse: reuse,
		  clock: func() time.Time { return time.Now().UTC() },
	   }
  }

// Prefill 预派生 n 个空闲地址。派生失败或撞车跳过，不得把空串放进库存。

  func (inv *Inventory) Prefill(chain string, n int, seedPrefix string) (int, error) {
	   if inv == nil {
		   return 0, fmt.Errorf("address inventory unavailable")
	  }
	   chain = strings.ToUpper(strings.TrimSpace(chain))
	   if n <= 0 {
		   return 0, fmt.Errorf("prefill n")
	  }
	   added := 0
	  inv.mu.Lock()
	   defer inv.mu.Unlock()
	  for i := 0; i < n; i++ {
		  seed := fmt.Sprintf("%s-%s-%d", seedPrefix, chain, i)
		  addr := inv.derive(seed, chain)
		   if addr == "" || wallet.ValidChainAddress(chain, addr) != nil {
			  continue
		   }
		   key := strings.ToLower(addr)
		   if _, ok := inv.byAddr[key]; ok {
			  continue
		  }
		   s := &Slot{Chain: chain,
			  Family: family(chain), Address: addr, Status: SlotFree, Tier: TierHot}
		  if err := inv.persistSlot(s); err != nil {
			  continue
		   }
		   inv.slots = append(inv.slots, s)
		   inv.byAddr[key] = s
		   added++
	  }
	  return added, nil
  }

// Reserve 从空闲池取一把。没有空闲返回错误，调用方不得现场派生串户。

   func (inv *Inventory) Reserve(chain, userID string) (Slot, error) {
	   if inv == nil {
		   return Slot{}, fmt.Errorf("address inventory unavailable")
	   }
	   chain = strings.ToUpper(strings.TrimSpace(chain))
	   userID = strings.TrimSpace(userID)
	  if userID == "" {
		   return Slot{}, fmt.Errorf("user_id required")
	   }
	  now := inv.clock()
	  inv.mu.Lock()
	  defer inv.mu.Unlock()
	   wantFam := family(chain)
	   for _, s := range inv.slots {
		   if s.Status == SlotCooling && !now.Before(s.CoolUntil) {
			   s.Status = SlotFree
			  s.UserID = ""
			  s.CoolUntil = time.Time{}
		   }
		   if s.Status != SlotFree {
			   continue
		   }
		  if s.Family != wantFam && s.Chain != chain {
			   continue
		  }
		   s.Status = SlotReserved
		   s.UserID = userID
		   s.Reserved = now
		   s.Chain = chain
		   if err := inv.persistSlot(s); err != nil {
			   s.Status = SlotFree
			   s.UserID = ""
			   s.Reserved = time.Time{}
			   return Slot{}, err
		   }
		  return *s, nil
	   }
	  return Slot{}, fmt.Errorf("no free address for %s", chain)
  }

// Bind 把 reserved 落成用户充值地址。别人占用拒。

  func (inv *Inventory) Bind(addr, userID string) (Slot, error) {
	  inv.mu.Lock()
	  defer inv.mu.Unlock()
	  s,
		  ok := inv.byAddr[strings.ToLower(strings.TrimSpace(addr))]
	   if !ok {
		  return Slot{}, fmt.Errorf("address not in inventory")
	   }
	  if s.Status != SlotReserved || s.UserID != userID {
		  return Slot{}, fmt.Errorf("address not reserved for user")
	  }
	  s.Status = SlotBound
	  s.BoundAt = inv.clock()
	   if err := inv.persistSlot(s); err != nil {
		  s.Status = SlotReserved
		   s.BoundAt = time.Time{}
		   return Slot{}, err
	   }
	   return *s, nil
   }

// Recycle 解绑进冷却。冷却未满不得回到 free。

   func (inv *Inventory) Recycle(addr string) error {
	  inv.mu.Lock()
	   defer inv.mu.Unlock()
	   s, ok := inv.byAddr[strings.ToLower(strings.TrimSpace(addr))]
	  if !ok {
		   return fmt.Errorf("address not in inventory")
	   }
	   if s.Status != SlotBound && s.Status != SlotReserved {
		   return nil
	  }
	  prev := *s
	  s.Status = SlotCooling
	  s.CoolUntil = inv.clock().Add(inv.reuse)
	   s.UserID = ""
	   if err := inv.persistSlot(s); err != nil {
		   *s = prev
		   return err
	  }
	   return nil
  }

type Counts struct {
	Chain string `json:"chain"`
	Free  int    `json:"free"`
	Reserved int `json:"reserved"`
	Bound   int `json:"bound"`
	Cooling int `json:"cooling"`
}

  func (inv *Inventory) Counts(chain string) Counts {
	  inv.mu.Lock()
	  defer inv.mu.Unlock()
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	   out := Counts{Chain: chain}
	   for _, s := range inv.slots {
		  if chain != "" && s.Chain != chain && s.Family != family(chain) {
			  continue
		  }
		  switch s.Status {
		   case SlotFree:
			   out.Free++
		  case SlotReserved:
			   out.Reserved++
		   case SlotBound:
			  out.Bound++
		   case SlotCooling:
			   out.Cooling++
		   }
	   }
	   return out
  }

// ReclaimSlot 冷却满后把槽位放回 free。未满失败关闭。

  func (inv *Inventory) ReclaimSlot(addr string) (Slot, error) {
	  if inv == nil {
		   return Slot{}, fmt.Errorf("address inventory unavailable")
	  }
	   now := inv.clock()
	  inv.mu.Lock()
	   defer inv.mu.Unlock()
	  s, ok := inv.byAddr[strings.ToLower(strings.TrimSpace(addr))]
	   if !ok {
		  return Slot{}, fmt.Errorf("address not in inventory")
	  }
	  if s.Status == SlotFree {
		   return *s, nil
	  }
	   if s.Status == SlotCooling && now.Before(s.CoolUntil) {
		   return *s, fmt.Errorf("address reuse cooldown")
	   }
	   prev := *s
	   s.Status = SlotFree
	  s.UserID = ""
	   s.CoolUntil = time.Time{}
	  if err := inv.persistSlot(s); err != nil {
		   *s = prev
		  return Slot{}, err
	   }
	   return *s, nil
  }

   func (inv *Inventory) Lookup(addr string) (Slot, bool) {
	   inv.mu.Lock()
	  defer inv.mu.Unlock()
	   s, ok := inv.byAddr[strings.ToLower(strings.TrimSpace(addr))]
	  if !ok {
		   return Slot{}, false
	   }
	   return *s, true
  }
