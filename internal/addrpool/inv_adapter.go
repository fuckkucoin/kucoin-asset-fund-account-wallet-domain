package addrpool

import "fmt"

// InventoryAdapter 把 Inventory 的 Reserve/Bind 包成 (addr, err) 形式。
type InventoryAdapter struct {
	inv *Inventory
}

// NewInventoryAdapter 构造适配器。

  func NewInventoryAdapter(inv *Inventory) *InventoryAdapter {
	   return &InventoryAdapter{inv: inv}
   }

// Reserve 预留一个空闲槽，返回地址字符串。
  func (a *InventoryAdapter) Reserve(chain, userID string) (string, error) {
	   if a.inv == nil {
		  return "", fmt.Errorf("inventory unavailable")
	   }
	  slot, err := a.inv.Reserve(chain, userID)
	   if err != nil {
		  return "", err
	   }
	   return slot.Address, nil
   }

// Bind 把已预留的地址绑定到用户。
  func (a *InventoryAdapter) Bind(addr, userID string) error {
	  if a.inv == nil {
		   return fmt.Errorf("inventory unavailable")
	  }
	  _, err := a.inv.Bind(addr, userID)
	   return err
  }

// PoolOwnerAdapter 把 Pool 包成 dustguard.PoolForceReleaser 接口。

type PoolOwnerAdapter struct {
	pool *Pool
}

// NewPoolOwnerAdapter 构造适配器。

   func NewPoolOwnerAdapter(pool *Pool) *PoolOwnerAdapter {
	   return &PoolOwnerAdapter{pool: pool}
  }

// ForceRelease 强制释放（绕冷却）。
  func (a *PoolOwnerAdapter) ForceRelease(userID, chain string) error {
	  if a.pool == nil {
		  return fmt.Errorf("pool unavailable")
	   }
	   return a.pool.ForceRelease(userID, chain)
  }

// ByAddress 查询地址 owner。

   func (a *PoolOwnerAdapter) ByAddress(addr string) (userID, chain string, ok bool) {
	  if a.pool == nil {
		  return "", "", false
	   }
	  return a.pool.ByAddressOwner(addr)
   }
