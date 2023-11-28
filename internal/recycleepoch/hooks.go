package recycleepoch

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/addrpool"
)

// RecycleBookReader 只读 RecycleRec。
type RecycleBookReader interface {
	Peek(ctx context.Context, addr string) (addrpool.RecycleRec, error)
}

// InventoryReader 只读库存槽。
type InventoryReader interface {
	Hint(chain, address string) (InventoryHint, bool)
}

// DepositObserveHint 入金观察调用点只读摘要（不改 deposit 公式）。
type DepositObserveHint struct {
	UserID string
	Chain   string
	Address string
	TxHash  string
	Asset   string
	Amount string
}

type DepositObserveReader interface {
	LastObserve(chain, address string) (DepositObserveHint, bool)
}

type NopRecycleBook struct{}

   func (NopRecycleBook) Peek(context.Context, string) (addrpool.RecycleRec, error) {
	   return addrpool.RecycleRec{}, ErrHookUnavailable
   }

type NopInventory struct{}

   func (NopInventory) Hint(string,
	   string) (InventoryHint, bool) {
	  return InventoryHint{}, false
  }

type NopDepositObserve struct{}

  func (NopDepositObserve) LastObserve(string, string) (DepositObserveHint, bool) {
	   return DepositObserveHint{}, false
   }

// RecycleBookAdapter 包装 RecycleBook.Ready 为只读 Peek。
// Ready 在冷却未满会返回记录+error；此处仍投影 CoolUntil。
type RecycleBookAdapter struct {
	Book *addrpool.RecycleBook
}

   func (a RecycleBookAdapter) Peek(ctx context.Context, addr string) (addrpool.RecycleRec, error) {
	  if a.Book == nil {
		  return addrpool.RecycleRec{},
			   ErrHookUnavailable
	   }
	  return a.Book.PeekRecycle(ctx, addr)
  }

// InventoryAdapter 只读投影 Inventory 槽。

type InventoryAdapter struct {
	Inv *addrpool.Inventory
}

   func (a InventoryAdapter) Hint(chain, address string) (InventoryHint, bool) {
	  if a.Inv == nil {
		  return InventoryHint{}, false
	  }
	  s, ok := a.Inv.Lookup(address)
	  if !ok {
		   return InventoryHint{}, false
	  }
	  if chain != "" && strings.ToUpper(strings.TrimSpace(s.Chain)) != strings.ToUpper(strings.TrimSpace(chain)) {
		  return InventoryHint{}, false
	   }
	  cool := ""
	  if !s.CoolUntil.IsZero() {
		   cool = s.CoolUntil.UTC().Format(time.RFC3339)
	  }
	  return InventoryHint{
		  Address: s.Address,
		  Chain:  s.Chain,
		  UserID: s.UserID,
		  Status: string(s.Status),
		  CoolUntil: cool,
	  }, true
  }

// StaticDepositObserve 用观察提示。

type StaticDepositObserve struct {
	Items map[string]DepositObserveHint
}

  func (s StaticDepositObserve) LastObserve(chain, address string) (DepositObserveHint, bool) {
	  if s.Items == nil {
		   return DepositObserveHint{}, false
	   }
	   k := strings.ToUpper(strings.TrimSpace(chain)) + "|" + strings.ToLower(strings.TrimSpace(address))
	  h, ok := s.Items[k]
	   return h, ok
   }
